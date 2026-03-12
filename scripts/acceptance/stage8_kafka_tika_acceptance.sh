#!/usr/bin/env bash
set -euo pipefail

BASE_URL="http://localhost:8081"
TOKEN="${TOKEN:-}"
LOGIN_USERNAME="${LOGIN_USERNAME:-}"
LOGIN_PASSWORD="${LOGIN_PASSWORD:-}"
AUTO_REGISTER="${AUTO_REGISTER:-1}"
AUTO_USER_PREFIX="${AUTO_USER_PREFIX:-stage8_accept}"
AUTO_PASSWORD="${AUTO_PASSWORD:-Passw0rd!Stage8}"
ORG_TAG="${ORG_TAG:-}"
FILE=""
LOG_FILE="${LOG_FILE:-./logs/app.log}"
WAIT_SECONDS="${WAIT_SECONDS:-25}"
POLL_MS="${POLL_MS:-500}"
CHUNK_SIZE="${CHUNK_SIZE:-5242880}"
IS_PUBLIC="${IS_PUBLIC:-false}"
CHECK_DOCKER=1
CHECK_REDIS=1
KEEP_TMP=0
VERBOSE=0

DOCKER_CONTAINERS="${DOCKER_CONTAINERS:-mysql paismart-v2-redis paismart-v2-minio paismart-v2-zookeeper paismart-v2-kafka paismart-v2-tika}"

REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6381}"
REDIS_PASSWORD="${REDIS_PASSWORD:-PaiSmart2025}"
REDIS_DB="${REDIS_DB:-1}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
WORK_DIR="${WORK_DIR:-/tmp/paismart-stage8-acceptance-$$}"

REQUEST_STATUS=""
REQUEST_BODY=""

usage() {
  cat <<'EOF'
Usage:
  stage8_kafka_tika_acceptance.sh [options]

Options:
  -b <base_url>   服务地址，默认: http://localhost:8081
  -t <token>      直接指定 Bearer token（优先）
  -u <username>   登录用户名（当未提供 -t 时使用；不传则自动注册测试用户）
  -p <password>   登录密码（与 -u 配套）
  -o <org_tag>    可选 orgTag（simple upload 表单字段）
  -f <file>       上传文件（可选，不传则自动生成 txt）
  -l <log_file>   应用日志文件路径，默认: ./logs/app.log
  -w <seconds>    等待异步链路日志的超时秒数，默认: 25
  -P              chunk 上传时 isPublic=true（默认 false）
  -k              保留临时目录（默认自动清理）
  -v              输出详细响应
  -D              跳过 Docker 容器运行状态检查
  -R              跳过 Redis retry-key 检查
  -h              显示帮助

Env (optional):
  DOCKER_CONTAINERS="mysql ... paismart-v2-tika"
  REDIS_ADDR=127.0.0.1:6381
  REDIS_PASSWORD=PaiSmart2025
  REDIS_DB=1
  WAIT_SECONDS=25
  POLL_MS=500
  CHUNK_SIZE=5242880
  IS_PUBLIC=false
  AUTO_REGISTER=1
  AUTO_USER_PREFIX=stage8_accept
  AUTO_PASSWORD=Passw0rd!Stage8
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

wait_for_log_pattern() {
  local pattern="$1"
  local desc="$2"
  local deadline=$(( $(date +%s) + WAIT_SECONDS ))

  while (( $(date +%s) <= deadline )); do
    if rg -n --fixed-strings "${pattern}" "${LOG_FILE}" >/dev/null 2>&1; then
      pass "${desc}"
      return 0
    fi
    sleep "$(awk "BEGIN { printf \"%.3f\", ${POLL_MS}/1000 }")"
  done

  fail "${desc} 超时: 未在 ${WAIT_SECONDS}s 内匹配日志 pattern=${pattern}"
}

redis_exists_key() {
  local key="$1"
  local host="${REDIS_ADDR%:*}"
  local port="${REDIS_ADDR##*:}"
  local -a cmd=(redis-cli -h "${host}" -p "${port}" -n "${REDIS_DB}")
  if [[ -n "${REDIS_PASSWORD}" ]]; then
    cmd+=(-a "${REDIS_PASSWORD}")
  fi
  cmd+=(EXISTS "${key}")
  "${cmd[@]}"
}

file_size() {
  local file="$1"
  if stat -c%s "${file}" >/dev/null 2>&1; then
    stat -c%s "${file}"
  else
    stat -f%z "${file}"
  fi
}

md5_file() {
  local file="$1"
  if command -v md5sum >/dev/null 2>&1; then
    md5sum "${file}" | awk '{print $1}'
  elif command -v md5 >/dev/null 2>&1; then
    md5 -q "${file}"
  else
    fail "系统缺少 md5sum/md5 命令"
  fi
}

md5_text() {
  local text="$1"
  if command -v md5sum >/dev/null 2>&1; then
    printf '%s' "${text}" | md5sum | awk '{print $1}'
  elif command -v md5 >/dev/null 2>&1; then
    printf '%s' "${text}" | md5 | awk '{print $NF}'
  else
    fail "系统缺少 md5sum/md5 命令"
  fi
}

run_chunk_merge_path() {
  local src_file="$1"
  local src_name total_size base_md5 run_id chunk_md5
  local chunk_dir check_payload merge_payload
  local -a chunks=()
  local total_chunks idx

  src_name="$(basename "${src_file}")"
  total_size="$(file_size "${src_file}")"
  base_md5="$(md5_file "${src_file}")"
  run_id="$(date +%s%N)"
  chunk_md5="$(md5_text "${base_md5}_chunk_${run_id}")"
  chunk_dir="${WORK_DIR}/chunk_flow_${run_id}"

  (( total_size > 0 )) || fail "chunk 流程文件为空，无法验收"

  mkdir -p "${chunk_dir}"
  split -b "${CHUNK_SIZE}" -d -a 6 "${src_file}" "${chunk_dir}/chunk_"
  mapfile -t chunks < <(find "${chunk_dir}" -maxdepth 1 -type f -name 'chunk_*' | sort)
  total_chunks="${#chunks[@]}"
  (( total_chunks > 0 )) || fail "chunk 流程切片失败"
  log "执行 chunk/merge 流程: file=${src_name} md5=${chunk_md5} chunks=${total_chunks}"

  check_payload="$(jq -nc --arg md5 "${chunk_md5}" '{md5:$md5}')"
  request \
    -X POST "${BASE_URL}/api/v1/upload/check" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "${check_payload}"
  assert_status 200 "chunk 流程首次 check 失败"
  jq -e '.code == 200 and .data.completed == false and (.data.uploadedChunks | type == "array")' \
    <<<"${REQUEST_BODY}" >/dev/null || fail "chunk 流程首次 check 断言失败"
  pass "chunk 流程首次 check 通过"

  for idx in "${!chunks[@]}"; do
    request \
      -X POST "${BASE_URL}/api/v1/upload/chunk" \
      -H "Authorization: Bearer ${TOKEN}" \
      -F "fileMd5=${chunk_md5}" \
      -F "fileName=${src_name}" \
      -F "totalSize=${total_size}" \
      -F "chunkIndex=${idx}" \
      -F "orgTag=${ORG_TAG}" \
      -F "isPublic=${IS_PUBLIC}" \
      -F "file=@${chunks[$idx]}"
    assert_status 200 "chunk 流程上传分片 ${idx} 失败"
    jq -e --argjson i "${idx}" '.code == 200 and (.data.uploadedChunks | index($i) != null)' \
      <<<"${REQUEST_BODY}" >/dev/null || fail "chunk 流程分片 ${idx} 上传断言失败"
  done
  pass "chunk 流程分片上传通过"

  request \
    -X POST "${BASE_URL}/api/v1/upload/check" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "${check_payload}"
  assert_status 200 "chunk 流程上传后 check 失败"
  jq -e --argjson total "${total_chunks}" \
    '.code == 200 and .data.completed == false and (.data.uploadedChunks | length == $total)' \
    <<<"${REQUEST_BODY}" >/dev/null || fail "chunk 流程上传后 check 断言失败"
  pass "chunk 流程上传后 check 通过"

  merge_payload="$(jq -nc --arg md5 "${chunk_md5}" --arg fn "${src_name}" '{fileMd5:$md5,fileName:$fn}')"
  request \
    -X POST "${BASE_URL}/api/v1/upload/merge" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "${merge_payload}"
  assert_status 200 "chunk 流程 merge 失败"
  jq -e --arg md5 "${chunk_md5}" --arg fn "${src_name}" \
    '.code == 200 and .data.fileMd5 == $md5 and .data.fileName == $fn and (.data.objectUrl | length > 0)' \
    <<<"${REQUEST_BODY}" >/dev/null || fail "chunk 流程 merge 返回断言失败"
  pass "chunk 流程 merge 通过"

  request \
    -X POST "${BASE_URL}/api/v1/upload/check" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "${check_payload}"
  assert_status 200 "chunk 流程 merge 后 check 失败"
  jq -e '.code == 200 and .data.completed == true' <<<"${REQUEST_BODY}" >/dev/null || \
    fail "chunk 流程 merge 后 check 断言失败"
  pass "chunk 流程 merge 后 check 通过"

  wait_for_log_pattern "文件处理任务已投递: md5=${chunk_md5}" "chunk 流程 Producer 投递日志命中"
  wait_for_log_pattern "[Consumer] 文件任务处理成功: MD5=${chunk_md5}" "chunk 流程 Consumer 成功日志命中"
  wait_for_log_pattern "[Processor] Tika 提取文本成功: md5=${chunk_md5}" "chunk 流程 Tika 提取成功日志命中"

  if [[ "${CHECK_REDIS}" -eq 1 ]]; then
    local retry_key exists_val
    retry_key="kafka:retry:${chunk_md5}"
    exists_val="$(redis_exists_key "${retry_key}")"
    [[ "${exists_val}" == "0" ]] || fail "chunk 流程重试键未清理: ${retry_key} exists=${exists_val}"
    pass "chunk 流程 Redis retry key 校验通过（不存在）"
  fi

  log "chunk 流程完成: md5=${chunk_md5}"
}

auto_register_and_login() {
  local attempt
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

while getopts ":b:t:u:p:o:f:l:w:PkDvRh" opt; do
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
    R) CHECK_REDIS=0 ;;
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
require_cmd split

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

if [[ -z "${FILE}" ]]; then
  FILE="${WORK_DIR}/stage8_auto_$(date +%s).txt"
  {
    echo "stage8 acceptance"
    echo "timestamp=$(date +%s)"
    echo "rand=$RANDOM"
  } > "${FILE}"
fi
[[ -f "${FILE}" ]] || fail "文件不存在: ${FILE}"

FILE_NAME="$(basename "${FILE}")"
log "执行 simple upload: ${FILE_NAME}"
request \
  -X POST "${BASE_URL}/api/v1/upload/simple" \
  -H "Authorization: Bearer ${TOKEN}" \
  -F "orgTag=${ORG_TAG}" \
  -F "file=@${FILE}"
assert_status 200 "simple upload 失败"
FILE_MD5="$(jq -er '.data.fileMd5' <<<"${REQUEST_BODY}")" || fail "无法解析上传响应中的 fileMd5"
pass "simple upload 通过，md5=${FILE_MD5}"

[[ -f "${LOG_FILE}" ]] || fail "日志文件不存在: ${LOG_FILE}（请确认服务已启动并写日志）"

wait_for_log_pattern "文件处理任务已投递: md5=${FILE_MD5}" "Producer 投递日志命中"
wait_for_log_pattern "[Consumer] 文件任务处理成功: MD5=${FILE_MD5}" "Consumer 成功日志命中"
wait_for_log_pattern "[Processor] Tika 提取文本成功: md5=${FILE_MD5}" "Tika 提取成功日志命中"

if [[ "${CHECK_REDIS}" -eq 1 ]]; then
  require_cmd redis-cli
  retry_key="kafka:retry:${FILE_MD5}"
  exists_val="$(redis_exists_key "${retry_key}")"
  [[ "${exists_val}" == "0" ]] || fail "重试键未清理: ${retry_key} exists=${exists_val}"
  pass "Redis retry key 校验通过（不存在）"
fi

run_chunk_merge_path "${FILE}"

printf '\n'
pass "阶段八验收完成: simple + chunk/merge 均已通过 Kafka + Tika 链路"
log "simpleFile=${FILE_NAME} simpleMD5=${FILE_MD5}"
