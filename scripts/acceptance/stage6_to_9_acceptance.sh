#!/usr/bin/env bash
set -euo pipefail

BASE_URL="http://localhost:8081"
TOKEN="${TOKEN:-}"
LOGIN_USERNAME="${LOGIN_USERNAME:-}"
LOGIN_PASSWORD="${LOGIN_PASSWORD:-}"
AUTO_REGISTER="${AUTO_REGISTER:-1}"
AUTO_USER_PREFIX="${AUTO_USER_PREFIX:-stage69_accept}"
AUTO_PASSWORD="${AUTO_PASSWORD:-Passw0rd!Stage69}"
ORG_TAG="${ORG_TAG:-}"
STAGE6_FILE=""
PDF_FILE=""
LOG_FILE="${LOG_FILE:-./logs/app.log}"
WAIT_SECONDS="${WAIT_SECONDS:-25}"
POLL_MS="${POLL_MS:-500}"
IS_PUBLIC="${IS_PUBLIC:-false}"
CHECK_DOCKER=1
CHECK_IDEMPOTENCY="${CHECK_IDEMPOTENCY:-1}"
KEEP_TMP=0
VERBOSE=0
RESULT_JSON_FILE="${RESULT_JSON_FILE:-}"

DOCKER_CONTAINERS="${DOCKER_CONTAINERS:-mysql paismart-v2-redis paismart-v2-minio paismart-v2-zookeeper paismart-v2-kafka paismart-v2-tika}"
MYSQL_DSN="${MYSQL_DSN:-root:PaiSmart2025@tcp(127.0.0.1:3307)/paismart_v2?parseTime=True}"
KAFKA_BROKERS="${KAFKA_BROKERS:-127.0.0.1:9093}"
KAFKA_TOPIC="${KAFKA_TOPIC:-file-processing}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
STAGE6_SCRIPT="${SCRIPT_DIR}/stage6_simple_upload_acceptance.sh"
STAGE7_SCRIPT="${SCRIPT_DIR}/stage7_chunk_upload_acceptance.sh"
DEFAULT_PDF_FILE="${REPO_ROOT}/scripts/test_data/Hashimoto.pdf"
WORK_DIR="${WORK_DIR:-/tmp/paismart-stage6-to-9-acceptance-$$}"
GO_BIN="${GO_BIN:-/home/yyy/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.11.linux-amd64/bin/go}"
GO_CACHE_DIR="${GO_CACHE_DIR:-${REPO_ROOT}/.tmp/gocache}"

REQUEST_STATUS=""
REQUEST_BODY=""

usage() {
  cat <<'EOF'
Usage:
  stage6_to_9_acceptance.sh [options]

Options:
  --stage6-file    阶段六 simple upload 文件（可选，不传则自动生成临时 txt）
  --pdf-file       阶段七到阶段九共用的 PDF 文件，默认: scripts/test_data/Hashimoto.pdf
  --stage7-file    --pdf-file 的兼容别名
  -b <base_url>    服务地址，默认: http://localhost:8081
  -t <token>       直接指定 Bearer token（优先）
  -u <username>    登录用户名（当未提供 -t 时使用；不传则自动注册测试用户）
  -p <password>    登录密码（与 -u 配套）
  -o <org_tag>     可选 orgTag
  -l <log_file>    应用日志文件路径，默认: ./logs/app.log
  -w <seconds>     等待异步链路日志/分块入库的超时秒数，默认: 25
  -P               isPublic=true（默认 false）
  -k               保留临时目录
  -v               输出详细响应
  -D               跳过 Docker 容器运行状态检查
  -I               跳过重复消息幂等性验证
  -h               显示帮助
EOF
}

log() {
  printf '[INFO] %s\n' "$*"
}

pass() {
  printf '[PASS] %s\n' "$*"
}

fail() {
  printf '[FAIL] %s\n' "$*" >&2
  if [[ -n "${REQUEST_BODY}" ]]; then
    printf '[FAIL] last response status=%s body=%s\n' "${REQUEST_STATUS}" "${REQUEST_BODY}" >&2
  fi
  exit 1
}

require_cmd() {
  local cmd="$1"
  command -v "$cmd" >/dev/null 2>&1 || fail "缺少依赖命令: $cmd"
}

request() {
  local body_file="${WORK_DIR}/response.json"
  REQUEST_STATUS="$(curl -sS -o "${body_file}" -w '%{http_code}' "$@")" || fail "curl 请求失败: $*"
  REQUEST_BODY="$(cat "${body_file}")"
  if [[ "${VERBOSE}" -eq 1 ]]; then
    printf '[DEBUG] HTTP %s\n' "${REQUEST_STATUS}"
    if jq . >/dev/null 2>&1 <<<"${REQUEST_BODY}"; then
      jq . <<<"${REQUEST_BODY}"
    else
      printf '%s\n' "${REQUEST_BODY}"
    fi
  fi
}

assert_status() {
  local expected="$1"
  local msg="$2"
  [[ "${REQUEST_STATUS}" == "${expected}" ]] || fail "${msg}: 期望 HTTP ${expected}，实际 ${REQUEST_STATUS}"
}

pattern_count() {
  local pattern="$1"
  if [[ ! -f "${LOG_FILE}" ]]; then
    echo 0
    return
  fi
  local count
  count="$(rg -F -c "${pattern}" "${LOG_FILE}" 2>/dev/null || true)"
  if [[ -z "${count}" ]]; then
    count=0
  fi
  echo "${count}"
}

wait_for_log_pattern() {
  local pattern="$1"
  local desc="$2"
  local deadline=$(( $(date +%s) + WAIT_SECONDS ))

  while (( $(date +%s) <= deadline )); do
    if [[ -f "${LOG_FILE}" ]] && rg -n --fixed-strings "${pattern}" "${LOG_FILE}" >/dev/null 2>&1; then
      pass "${desc}"
      return 0
    fi
    sleep "$(awk "BEGIN { printf \"%.3f\", ${POLL_MS}/1000 }")"
  done

  fail "${desc} 超时: 未在 ${WAIT_SECONDS}s 内匹配日志 pattern=${pattern}"
}

wait_for_log_count_increment() {
  local pattern="$1"
  local before="$2"
  local desc="$3"
  local deadline=$(( $(date +%s) + WAIT_SECONDS ))

  while (( $(date +%s) <= deadline )); do
    local current
    current="$(pattern_count "${pattern}")"
    if (( current > before )); then
      pass "${desc}"
      return 0
    fi
    sleep "$(awk "BEGIN { printf \"%.3f\", ${POLL_MS}/1000 }")"
  done

  fail "${desc} 超时: 日志计数未增长 pattern=${pattern} before=${before}"
}

auto_register_and_login() {
  local attempt register_payload login_payload
  for attempt in 1 2 3 4 5; do
    LOGIN_USERNAME="${AUTO_USER_PREFIX}_$(date +%s)_$RANDOM"
    LOGIN_PASSWORD="${AUTO_PASSWORD}"

    log "未提供凭据，自动注册测试用户: ${LOGIN_USERNAME}"
    register_payload="$(jq -nc --arg u "${LOGIN_USERNAME}" --arg p "${LOGIN_PASSWORD}" '{username:$u,password:$p}')"
    request \
      -X POST "${BASE_URL}/api/v1/users/register" \
      -H "Content-Type: application/json" \
      -d "${register_payload}"
    if [[ "${REQUEST_STATUS}" != "201" ]]; then
      continue
    fi

    login_payload="$(jq -nc --arg u "${LOGIN_USERNAME}" --arg p "${LOGIN_PASSWORD}" '{username:$u,password:$p}')"
    request \
      -X POST "${BASE_URL}/api/v1/users/login" \
      -H "Content-Type: application/json" \
      -d "${login_payload}"
    assert_status 200 "自动注册后登录失败"
    TOKEN="$(jq -er '.data.accessToken' <<<"${REQUEST_BODY}")" || fail "自动登录成功但未解析到 accessToken"
    pass "自动注册并登录成功"
    return 0
  done

  fail "自动注册测试用户失败，请改用 -u/-p 或 -t"
}

run_vector_probe() {
  local file_md5="$1"
  local expected_chunks="$2"
  local stdout_file="$3"
  local stderr_file="$4"
  local rc

  mkdir -p "${GO_CACHE_DIR}"
  set +e
  (
    cd "${REPO_ROOT}" && env GOCACHE="${GO_CACHE_DIR}" "${GO_BIN}" run ./scripts/acceptance/document_vector_probe \
      -mysql-dsn "${MYSQL_DSN}" \
      -file-md5 "${file_md5}" \
      -expected-min-chunks "${expected_chunks}" \
      -timeout-sec "${WAIT_SECONDS}" \
      -poll-ms "${POLL_MS}" \
      >"${stdout_file}" 2>"${stderr_file}"
  )
  rc=$?
  set -e
  return "${rc}"
}

replay_file_task() {
  local file_md5="$1"
  local file_name="$2"
  local user_id="$3"
  local org_tag="$4"
  local is_public="$5"
  local object_key="$6"

  mkdir -p "${GO_CACHE_DIR}"
  (
    cd "${REPO_ROOT}" && env GOCACHE="${GO_CACHE_DIR}" "${GO_BIN}" run ./scripts/acceptance/file_task_replay \
      -brokers "${KAFKA_BROKERS}" \
      -topic "${KAFKA_TOPIC}" \
      -file-md5 "${file_md5}" \
      -file-name "${file_name}" \
      -user-id "${user_id}" \
      -org-tag "${org_tag}" \
      -is-public="${is_public}" \
      -object-key "${object_key}" \
      >/dev/null
  )
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --stage6-file)
      STAGE6_FILE="${2:-}"
      shift 2
      ;;
    --pdf-file|--stage7-file)
      PDF_FILE="${2:-}"
      shift 2
      ;;
    -b)
      BASE_URL="${2:-}"
      shift 2
      ;;
    -t)
      TOKEN="${2:-}"
      shift 2
      ;;
    -u)
      LOGIN_USERNAME="${2:-}"
      shift 2
      ;;
    -p)
      LOGIN_PASSWORD="${2:-}"
      shift 2
      ;;
    -o)
      ORG_TAG="${2:-}"
      shift 2
      ;;
    -l)
      LOG_FILE="${2:-}"
      shift 2
      ;;
    -w)
      WAIT_SECONDS="${2:-}"
      shift 2
      ;;
    -P)
      IS_PUBLIC="true"
      shift
      ;;
    -k)
      KEEP_TMP=1
      shift
      ;;
    -v)
      VERBOSE=1
      shift
      ;;
    -D)
      CHECK_DOCKER=0
      shift
      ;;
    -I)
      CHECK_IDEMPOTENCY=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "未知参数: $1"
      ;;
  esac
done

[[ -x "${STAGE6_SCRIPT}" ]] || fail "缺少脚本: ${STAGE6_SCRIPT}"
[[ -x "${STAGE7_SCRIPT}" ]] || fail "缺少脚本: ${STAGE7_SCRIPT}"
[[ -x "${GO_BIN}" ]] || fail "GO_BIN 不存在或不可执行: ${GO_BIN}"

require_cmd curl
require_cmd jq
require_cmd awk
require_cmd rg

mkdir -p "${WORK_DIR}"
if [[ "${KEEP_TMP}" -eq 0 ]]; then
  trap 'rm -rf "${WORK_DIR}"' EXIT
fi

cd "${REPO_ROOT}"

if [[ -z "${PDF_FILE}" ]]; then
  PDF_FILE="${DEFAULT_PDF_FILE}"
fi
[[ -f "${PDF_FILE}" ]] || fail "PDF 文件不存在: ${PDF_FILE}"

if [[ -z "${STAGE6_FILE}" ]]; then
  STAGE6_FILE="${WORK_DIR}/stage6_phase_auto_$(date +%s).txt"
  UNIQUE_RUN_ID="$(date +%s%N)_$RANDOM"
  {
    echo "stage6 to stage9 acceptance"
    echo "run_id=${UNIQUE_RUN_ID}"
    echo "simple upload only uses a temporary txt file"
  } > "${STAGE6_FILE}"
fi
[[ -f "${STAGE6_FILE}" ]] || fail "阶段六文件不存在: ${STAGE6_FILE}"

if [[ "${CHECK_DOCKER}" -eq 1 ]]; then
  require_cmd docker
  for c in ${DOCKER_CONTAINERS}; do
    running="$(docker inspect -f '{{.State.Running}}' "${c}" 2>/dev/null || true)"
    [[ "${running}" == "true" ]] || fail "容器未运行: ${c}"
  done
  pass "Docker 容器检查通过"
fi

request -X GET "${BASE_URL}/health"
assert_status 200 "服务健康检查失败"
pass "服务健康检查通过"

if [[ -z "${TOKEN}" ]]; then
  if [[ -n "${LOGIN_USERNAME}" && -n "${LOGIN_PASSWORD}" ]]; then
    login_payload="$(jq -nc --arg u "${LOGIN_USERNAME}" --arg p "${LOGIN_PASSWORD}" '{username:$u,password:$p}')"
    request \
      -X POST "${BASE_URL}/api/v1/users/login" \
      -H "Content-Type: application/json" \
      -d "${login_payload}"
    assert_status 200 "登录接口失败"
    TOKEN="$(jq -er '.data.accessToken' <<<"${REQUEST_BODY}")" || fail "登录成功但未解析到 accessToken"
    pass "登录成功并获取 token"
  else
    [[ "${AUTO_REGISTER}" == "1" ]] || fail "未提供 TOKEN/账号密码，且 AUTO_REGISTER=0"
    auto_register_and_login
  fi
fi

request -X GET "${BASE_URL}/api/v1/users/me" -H "Authorization: Bearer ${TOKEN}"
assert_status 200 "获取当前用户信息失败"
USER_ID="$(jq -er '.data.id' <<<"${REQUEST_BODY}")" || fail "无法解析 userID"
pass "当前用户 ID: ${USER_ID}"

log "开始整阶段验收：阶段六(simple txt) -> 阶段七到九(shared pdf)"

stage6_cmd=(
  "${STAGE6_SCRIPT}"
  -b "${BASE_URL}"
  -t "${TOKEN}"
  -o "${ORG_TAG}"
  -f "${STAGE6_FILE}"
  -D
)
if [[ "${VERBOSE}" -eq 1 ]]; then
  stage6_cmd+=(-v)
fi
"${stage6_cmd[@]}"

STAGE7_RESULT_JSON="${WORK_DIR}/stage7_result.json"
stage7_cmd=(
  "${STAGE7_SCRIPT}"
  -b "${BASE_URL}"
  -t "${TOKEN}"
  -o "${ORG_TAG}"
  -f "${PDF_FILE}"
)
if [[ "${IS_PUBLIC}" == "true" ]]; then
  stage7_cmd+=(-P)
fi
if [[ "${KEEP_TMP}" -eq 1 ]]; then
  stage7_cmd+=(-k)
fi
if [[ "${VERBOSE}" -eq 1 ]]; then
  stage7_cmd+=(-v)
fi

log "执行阶段七主链路，输出将直接作为阶段九校验输入"
RESULT_JSON_FILE="${STAGE7_RESULT_JSON}" \
PIPELINE_LOG_FILE="${LOG_FILE}" \
PIPELINE_WAIT_SECONDS="${WAIT_SECONDS}" \
PIPELINE_POLL_MS="${POLL_MS}" \
"${stage7_cmd[@]}"

[[ -s "${STAGE7_RESULT_JSON}" ]] || fail "阶段七结果文件为空，无法继续阶段九校验"
jq -e . "${STAGE7_RESULT_JSON}" >/dev/null 2>&1 || fail "阶段七结果文件不是有效 JSON"

FILE_MD5="$(jq -er '.fileMd5' "${STAGE7_RESULT_JSON}")" || fail "无法解析阶段七 fileMd5"
FILE_NAME="$(jq -er '.fileName' "${STAGE7_RESULT_JSON}")" || fail "无法解析阶段七 fileName"
RESULT_USER_ID="$(jq -er '.userId' "${STAGE7_RESULT_JSON}")" || fail "无法解析阶段七 userId"
RESULT_ORG_TAG="$(jq -er '.orgTag' "${STAGE7_RESULT_JSON}")" || fail "无法解析阶段七 orgTag"
RESULT_IS_PUBLIC="$(jq -r '.isPublic' "${STAGE7_RESULT_JSON}")" || fail "无法解析阶段七 isPublic"
OBJECT_KEY="$(jq -er '.objectKey' "${STAGE7_RESULT_JSON}")" || fail "无法解析阶段七 objectKey"

[[ "${RESULT_USER_ID}" == "${USER_ID}" ]] || fail "阶段七 userId 与登录用户不一致"

wait_for_log_pattern "[Processor] 文本分块完成: md5=${FILE_MD5}" "阶段九文本分块完成日志命中"
wait_for_log_pattern "[Processor] 批量写入 document_vectors 成功: md5=${FILE_MD5}" "阶段九 document_vectors 写入日志命中"

PROBE_STDOUT="${WORK_DIR}/vector_probe.json"
PROBE_STDERR="${WORK_DIR}/vector_probe.err"
EXPECTED_MIN_CHUNKS=1
if ! run_vector_probe "${FILE_MD5}" "${EXPECTED_MIN_CHUNKS}" "${PROBE_STDOUT}" "${PROBE_STDERR}"; then
  [[ ! -s "${PROBE_STDERR}" ]] || printf '[DEBUG] vector probe stderr:\n%s\n' "$(cat "${PROBE_STDERR}")" >&2
  [[ ! -s "${PROBE_STDOUT}" ]] || printf '[DEBUG] vector probe stdout:\n%s\n' "$(cat "${PROBE_STDOUT}")" >&2
  fail "document_vectors 探针执行失败"
fi
INITIAL_CHUNK_COUNT="$(jq -er '.chunkCount' "${PROBE_STDOUT}")" || fail "无法解析 chunkCount"
INITIAL_CONTIGUOUS="$(jq -r '.contiguous' "${PROBE_STDOUT}")" || fail "无法解析 contiguous"
INITIAL_DUPLICATE="$(jq -r '.hasDuplicate' "${PROBE_STDOUT}")" || fail "无法解析 hasDuplicate"
INITIAL_SIGNATURE="$(jq -er '.signature' "${PROBE_STDOUT}")" || fail "无法解析 signature"
INITIAL_ORG_TAG="$(jq -er '.orgTag' "${PROBE_STDOUT}")" || fail "无法解析 orgTag"
INITIAL_IS_PUBLIC="$(jq -r '.isPublic' "${PROBE_STDOUT}")" || fail "无法解析 isPublic"
INITIAL_USER_ID="$(jq -er '.userId' "${PROBE_STDOUT}")" || fail "无法解析 userId"

[[ "${INITIAL_CHUNK_COUNT}" =~ ^[0-9]+$ ]] || fail "chunkCount 非法: ${INITIAL_CHUNK_COUNT}"
(( INITIAL_CHUNK_COUNT >= EXPECTED_MIN_CHUNKS )) || fail "chunkCount 不足: ${INITIAL_CHUNK_COUNT}"
[[ "${INITIAL_CONTIGUOUS}" == "true" ]] || fail "chunk_id 不是连续递增"
[[ "${INITIAL_DUPLICATE}" == "false" ]] || fail "chunk_id 出现重复"
[[ "${INITIAL_USER_ID}" == "${USER_ID}" ]] || fail "document_vectors.user_id 与当前用户不一致"
[[ "${INITIAL_ORG_TAG}" == "${RESULT_ORG_TAG}" ]] || fail "document_vectors.org_tag 与阶段七结果不一致"
[[ "${INITIAL_IS_PUBLIC}" == "${RESULT_IS_PUBLIC}" ]] || fail "document_vectors.is_public 与阶段七结果不一致"
pass "阶段九 document_vectors 校验通过: chunkCount=${INITIAL_CHUNK_COUNT}"

if [[ "${CHECK_IDEMPOTENCY}" == "1" ]]; then
  consumer_before="$(pattern_count "[Consumer] 文件任务处理成功: MD5=${FILE_MD5}")"
  write_before="$(pattern_count "[Processor] 批量写入 document_vectors 成功: md5=${FILE_MD5}")"

  log "重放同一条 Kafka 文件任务，验证整阶段终态幂等性"
  replay_file_task "${FILE_MD5}" "${FILE_NAME}" "${USER_ID}" "${RESULT_ORG_TAG}" "${RESULT_IS_PUBLIC}" "${OBJECT_KEY}" || fail "重放 Kafka 文件任务失败"

  wait_for_log_count_increment "[Consumer] 文件任务处理成功: MD5=${FILE_MD5}" "${consumer_before}" "重复消息 Consumer 成功日志命中"
  wait_for_log_count_increment "[Processor] 批量写入 document_vectors 成功: md5=${FILE_MD5}" "${write_before}" "重复消息 document_vectors 写入日志命中"

  REPLAY_PROBE_STDOUT="${WORK_DIR}/vector_probe_replay.json"
  REPLAY_PROBE_STDERR="${WORK_DIR}/vector_probe_replay.err"
  if ! run_vector_probe "${FILE_MD5}" "${EXPECTED_MIN_CHUNKS}" "${REPLAY_PROBE_STDOUT}" "${REPLAY_PROBE_STDERR}"; then
    [[ ! -s "${REPLAY_PROBE_STDERR}" ]] || printf '[DEBUG] replay vector probe stderr:\n%s\n' "$(cat "${REPLAY_PROBE_STDERR}")" >&2
    [[ ! -s "${REPLAY_PROBE_STDOUT}" ]] || printf '[DEBUG] replay vector probe stdout:\n%s\n' "$(cat "${REPLAY_PROBE_STDOUT}")" >&2
    fail "重复消息后的 document_vectors 探针执行失败"
  fi

  REPLAY_CHUNK_COUNT="$(jq -er '.chunkCount' "${REPLAY_PROBE_STDOUT}")" || fail "无法解析 replay chunkCount"
  REPLAY_CONTIGUOUS="$(jq -r '.contiguous' "${REPLAY_PROBE_STDOUT}")" || fail "无法解析 replay contiguous"
  REPLAY_DUPLICATE="$(jq -r '.hasDuplicate' "${REPLAY_PROBE_STDOUT}")" || fail "无法解析 replay hasDuplicate"
  REPLAY_SIGNATURE="$(jq -er '.signature' "${REPLAY_PROBE_STDOUT}")" || fail "无法解析 replay signature"
  REPLAY_ORG_TAG="$(jq -er '.orgTag' "${REPLAY_PROBE_STDOUT}")" || fail "无法解析 replay orgTag"
  REPLAY_IS_PUBLIC="$(jq -r '.isPublic' "${REPLAY_PROBE_STDOUT}")" || fail "无法解析 replay isPublic"
  REPLAY_USER_ID="$(jq -er '.userId' "${REPLAY_PROBE_STDOUT}")" || fail "无法解析 replay userId"

  [[ "${REPLAY_CONTIGUOUS}" == "true" ]] || fail "重复消息后 chunk_id 不连续"
  [[ "${REPLAY_DUPLICATE}" == "false" ]] || fail "重复消息后 chunk_id 重复"
  [[ "${REPLAY_CHUNK_COUNT}" == "${INITIAL_CHUNK_COUNT}" ]] || fail "重复消息后 chunkCount 变化: before=${INITIAL_CHUNK_COUNT} after=${REPLAY_CHUNK_COUNT}"
  [[ "${REPLAY_SIGNATURE}" == "${INITIAL_SIGNATURE}" ]] || fail "重复消息后分块内容签名变化"
  [[ "${REPLAY_ORG_TAG}" == "${INITIAL_ORG_TAG}" ]] || fail "重复消息后 orgTag 变化"
  [[ "${REPLAY_IS_PUBLIC}" == "${INITIAL_IS_PUBLIC}" ]] || fail "重复消息后 isPublic 变化"
  [[ "${REPLAY_USER_ID}" == "${INITIAL_USER_ID}" ]] || fail "重复消息后 userId 变化"
  pass "阶段九幂等性校验通过"
fi

printf '\n'
pass "阶段六到阶段九整阶段验收完成"
log "simpleFile=$(basename "${STAGE6_FILE}") pdfFile=$(basename "${PDF_FILE}") md5=${FILE_MD5} chunkCount=${INITIAL_CHUNK_COUNT}"

if [[ -n "${RESULT_JSON_FILE}" ]]; then
  jq -nc \
    --arg fileMd5 "${FILE_MD5}" \
    --arg fileName "${FILE_NAME}" \
    --argjson userId "${USER_ID}" \
    --arg orgTag "${INITIAL_ORG_TAG}" \
    --argjson isPublic "$([[ "${INITIAL_IS_PUBLIC}" == "true" ]] && echo true || echo false)" \
    --arg objectKey "${OBJECT_KEY}" \
    --argjson chunkCount "${INITIAL_CHUNK_COUNT}" \
    --arg signature "${INITIAL_SIGNATURE}" \
    '{
      fileMd5: $fileMd5,
      fileName: $fileName,
      userId: $userId,
      orgTag: $orgTag,
      isPublic: $isPublic,
      objectKey: $objectKey,
      chunkCount: $chunkCount,
      signature: $signature
    }' > "${RESULT_JSON_FILE}"
fi
