#!/usr/bin/env bash
set -euo pipefail

BASE_URL="http://localhost:8081"
TOKEN="${TOKEN:-}"
LOGIN_USERNAME="${LOGIN_USERNAME:-}"
LOGIN_PASSWORD="${LOGIN_PASSWORD:-}"
AUTO_REGISTER="${AUTO_REGISTER:-1}"
AUTO_USER_PREFIX="${AUTO_USER_PREFIX:-stage9_accept}"
AUTO_PASSWORD="${AUTO_PASSWORD:-Passw0rd!Stage9}"
ORG_TAG="${ORG_TAG:-}"
FILE=""
LOG_FILE="${LOG_FILE:-./logs/app.log}"
WAIT_SECONDS="${WAIT_SECONDS:-25}"
POLL_MS="${POLL_MS:-500}"
IS_PUBLIC="${IS_PUBLIC:-false}"
CHECK_DOCKER=1
CHECK_IDEMPOTENCY="${CHECK_IDEMPOTENCY:-1}"
KEEP_TMP=0
VERBOSE=0

DOCKER_CONTAINERS="${DOCKER_CONTAINERS:-mysql paismart-v2-redis paismart-v2-minio paismart-v2-zookeeper paismart-v2-kafka paismart-v2-tika}"
MYSQL_DSN="${MYSQL_DSN:-root:PaiSmart2025@tcp(127.0.0.1:3307)/paismart_v2?parseTime=True}"
KAFKA_BROKERS="${KAFKA_BROKERS:-127.0.0.1:9093}"
KAFKA_TOPIC="${KAFKA_TOPIC:-file-processing}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
WORK_DIR="${WORK_DIR:-/tmp/paismart-stage9-acceptance-$$}"
GO_BIN="${GO_BIN:-/home/yyy/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.11.linux-amd64/bin/go}"
GO_CACHE_DIR="${GO_CACHE_DIR:-${REPO_ROOT}/.tmp/gocache}"

REQUEST_STATUS=""
REQUEST_BODY=""

usage() {
  cat <<'EOF'
Usage:
  stage9_chunk_vector_acceptance.sh [options]

Options:
  -b <base_url>   服务地址，默认: http://localhost:8081
  -t <token>      直接指定 Bearer token（优先）
  -u <username>   登录用户名（当未提供 -t 时使用；不传则自动注册测试用户）
  -p <password>   登录密码（与 -u 配套）
  -o <org_tag>    可选 orgTag（simple upload 表单字段）
  -f <file>       上传文件（可选，不传则自动生成较长 txt）
  -l <log_file>   应用日志文件路径，默认: ./logs/app.log
  -w <seconds>    等待异步链路日志/分块入库的超时秒数，默认: 25
  -P              isPublic=true（默认 false）
  -k              保留临时目录（默认自动清理）
  -v              输出详细响应
  -D              跳过 Docker 容器运行状态检查
  -I              跳过重复消息幂等性验证
  -h              显示帮助

Env (optional):
  MYSQL_DSN=root:PaiSmart2025@tcp(127.0.0.1:3307)/paismart_v2?parseTime=True
  KAFKA_BROKERS=127.0.0.1:9093
  KAFKA_TOPIC=file-processing
  GO_BIN=/path/to/go
  GO_CACHE_DIR=./.tmp/gocache
  CHECK_IDEMPOTENCY=1
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

while getopts ":b:t:u:p:o:f:l:w:PkDvIh" opt; do
  case "${opt}" in
    b) BASE_URL="${OPTARG}" ;;
    t) TOKEN="${OPTARG}" ;;
    u) LOGIN_USERNAME="${OPTARG}" ;;
    p) LOGIN_PASSWORD="${OPTARG}" ;;
    o) ORG_TAG="${OPTARG}" ;;
    f) FILE="${OPTARG}" ;;
    l) LOG_FILE="${OPTARG}" ;;
    w) WAIT_SECONDS="${OPTARG}" ;;
    P) IS_PUBLIC="true" ;;
    k) KEEP_TMP=1 ;;
    D) CHECK_DOCKER=0 ;;
    I) CHECK_IDEMPOTENCY=0 ;;
    v) VERBOSE=1 ;;
    h)
      usage
      exit 0
      ;;
    :)
      fail "参数 -${OPTARG} 缺少值"
      ;;
    \?)
      fail "未知参数: -${OPTARG}"
      ;;
  esac
done

require_cmd curl
require_cmd jq
require_cmd awk
require_cmd rg
[[ -x "${GO_BIN}" ]] || fail "GO_BIN 不存在或不可执行: ${GO_BIN}"

mkdir -p "${WORK_DIR}"
if [[ "${KEEP_TMP}" -eq 0 ]]; then
  trap 'rm -rf "${WORK_DIR}"' EXIT
fi

cd "${REPO_ROOT}"

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

GENERATED_FILE=0
if [[ -z "${FILE}" ]]; then
  FILE="${WORK_DIR}/stage9_auto_$(date +%s).txt"
  GENERATED_FILE=1
  UNIQUE_RUN_ID="$(date +%s%N)_$RANDOM"
  : > "${FILE}"
  printf 'stage9 acceptance run id: %s\n' "${UNIQUE_RUN_ID}" >> "${FILE}"
  for i in $(seq 1 180); do
    printf 'stage9 acceptance chunk line %03d run=%s PaiSmart RAG text splitting 验证文本分块与重叠窗口。\n' "${i}" "${UNIQUE_RUN_ID}" >> "${FILE}"
  done
fi
[[ -f "${FILE}" ]] || fail "文件不存在: ${FILE}"

FILE_NAME="$(basename "${FILE}")"
log "执行 stage9 simple upload: ${FILE_NAME}"
request \
  -X POST "${BASE_URL}/api/v1/upload/simple" \
  -H "Authorization: Bearer ${TOKEN}" \
  -F "orgTag=${ORG_TAG}" \
  -F "file=@${FILE}"
assert_status 200 "simple upload 失败"
FILE_MD5="$(jq -er '.data.fileMd5' <<<"${REQUEST_BODY}")" || fail "无法解析上传响应中的 fileMd5"
IS_QUICK="$(jq -r '.data.isQuick' <<<"${REQUEST_BODY}")" || fail "无法解析上传响应中的 isQuick"
[[ "${IS_QUICK}" == "false" ]] || fail "stage9 验收要求新文件触发完整异步链路，但当前命中了秒传，请重新运行"
pass "simple upload 通过，md5=${FILE_MD5}"

OBJECT_KEY="uploads/${USER_ID}/${FILE_MD5}/${FILE_NAME}"

[[ -f "${LOG_FILE}" ]] || fail "日志文件不存在: ${LOG_FILE}"
wait_for_log_pattern "文件处理任务已投递: md5=${FILE_MD5}" "Producer 投递日志命中"
wait_for_log_pattern "[Consumer] 文件任务处理成功: MD5=${FILE_MD5}" "Consumer 成功日志命中"
wait_for_log_pattern "[Processor] Tika 提取文本成功: md5=${FILE_MD5}" "Tika 提取成功日志命中"
wait_for_log_pattern "[Processor] 文本分块完成: md5=${FILE_MD5}" "文本分块完成日志命中"
wait_for_log_pattern "[Processor] 批量写入 document_vectors 成功: md5=${FILE_MD5}" "document_vectors 写入日志命中"

EXPECTED_MIN_CHUNKS=1
if [[ "${GENERATED_FILE}" -eq 1 ]]; then
  EXPECTED_MIN_CHUNKS=2
fi

PROBE_STDOUT="${WORK_DIR}/vector_probe.json"
PROBE_STDERR="${WORK_DIR}/vector_probe.err"
if ! run_vector_probe "${FILE_MD5}" "${EXPECTED_MIN_CHUNKS}" "${PROBE_STDOUT}" "${PROBE_STDERR}"; then
  [[ ! -s "${PROBE_STDERR}" ]] || printf '[DEBUG] vector probe stderr:\n%s\n' "$(cat "${PROBE_STDERR}")" >&2
  [[ ! -s "${PROBE_STDOUT}" ]] || printf '[DEBUG] vector probe stdout:\n%s\n' "$(cat "${PROBE_STDOUT}")" >&2
  fail "document_vectors 探针执行失败"
fi
jq -e . "${PROBE_STDOUT}" >/dev/null 2>&1 || fail "document_vectors 探针输出不是有效 JSON"

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
pass "document_vectors 校验通过: chunkCount=${INITIAL_CHUNK_COUNT}"

if [[ "${CHECK_IDEMPOTENCY}" == "1" ]]; then
  consumer_before="$(pattern_count "[Consumer] 文件任务处理成功: MD5=${FILE_MD5}")"
  write_before="$(pattern_count "[Processor] 批量写入 document_vectors 成功: md5=${FILE_MD5}")"

  log "重放同一条 Kafka 文件任务，验证阶段九幂等性"
  replay_file_task "${FILE_MD5}" "${FILE_NAME}" "${USER_ID}" "${INITIAL_ORG_TAG}" "${INITIAL_IS_PUBLIC}" "${OBJECT_KEY}" || fail "重放 Kafka 文件任务失败"

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
pass "阶段九验收完成: 文本分块、document_vectors 落库、重复消息幂等性均通过"
log "file=${FILE_NAME} md5=${FILE_MD5} chunkCount=${INITIAL_CHUNK_COUNT}"
