#!/usr/bin/env bash
set -euo pipefail

# 阶段七验收脚本：
# - 自动切片（5MB）
# - 循环上传分片
# - 关键响应断言（check/chunk/merge）
# - 幂等性断言（重复上传已完成文件分片）
# - 负例断言（分片不完整时 merge 返回 400）

BASE_URL="http://localhost:8081"
FILE=""
TOKEN="${TOKEN:-}"
LOGIN_USERNAME="${LOGIN_USERNAME:-}"
LOGIN_PASSWORD="${LOGIN_PASSWORD:-}"
AUTO_REGISTER="${AUTO_REGISTER:-1}"
AUTO_USER_PREFIX="${AUTO_USER_PREFIX:-stage7_accept}"
AUTO_PASSWORD="${AUTO_PASSWORD:-Passw0rd!Stage7}"
ORG_TAG="${ORG_TAG:-}"
IS_PUBLIC="${IS_PUBLIC:-false}"
KEEP_TMP=0
VERBOSE=0
USE_REAL_MD5=0
RESULT_JSON_FILE="${RESULT_JSON_FILE:-}"

CHUNK_SIZE=5242880 # 5 * 1024 * 1024
WORK_DIR="${WORK_DIR:-/tmp/paismart-stage7-acceptance-$$}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# merge 后异步清理校验参数（可通过环境变量覆盖）
CHECK_CLEANUP="${CHECK_CLEANUP:-1}"
CLEANUP_TIMEOUT_SEC="${CLEANUP_TIMEOUT_SEC:-20}"
CLEANUP_POLL_MS="${CLEANUP_POLL_MS:-500}"
REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}"
REDIS_PASSWORD="${REDIS_PASSWORD:-}"
REDIS_DB="${REDIS_DB:-1}"
MINIO_ENDPOINT="${MINIO_ENDPOINT:-127.0.0.1:9300}"
MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-minioadmin}"
MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-minioadmin}"
MINIO_USE_SSL="${MINIO_USE_SSL:-false}"
MINIO_BUCKET="${MINIO_BUCKET:-uploads}"
CHECK_STAGE8_PIPELINE_AFTER_MERGE="${CHECK_STAGE8_PIPELINE_AFTER_MERGE:-1}"
PIPELINE_LOG_FILE="${PIPELINE_LOG_FILE:-${REPO_ROOT}/logs/app.log}"
PIPELINE_WAIT_SECONDS="${PIPELINE_WAIT_SECONDS:-25}"
PIPELINE_POLL_MS="${PIPELINE_POLL_MS:-500}"

REQUEST_STATUS=""
REQUEST_BODY=""

usage() {
  cat <<'EOF'
Usage:
  stage7_chunk_upload_acceptance.sh -f <file> [options]

Options:
  -f <file>       要上传的文件（必填）
  -b <base_url>   服务地址，默认: http://localhost:8081
  -t <token>      直接指定 Bearer token（优先）
  -u <username>   登录用户名（当未提供 -t 时使用；不传则自动注册测试用户）
  -p <password>   登录密码（与 -u 配套）
  -o <org_tag>    可选 orgTag
  -P              isPublic=true（默认 false）
  -r              使用真实文件 MD5（默认会加随机盐，避免与历史记录冲突）
  -k              保留临时目录（默认自动清理）
  -v              输出详细响应
  -h              显示帮助

Examples:
  ./scripts/acceptance/stage7_chunk_upload_acceptance.sh -f ./testdata/large.pdf -t "$TOKEN"
  ./scripts/acceptance/stage7_chunk_upload_acceptance.sh -f ./testdata/large.pdf -u test -p 123456

Cleanup check envs (optional):
  CHECK_CLEANUP=1|0           是否检查 merge 后异步清理（默认 1）
  CLEANUP_TIMEOUT_SEC=20       清理等待超时秒数
  CLEANUP_POLL_MS=500          清理轮询间隔毫秒
  REDIS_ADDR=127.0.0.1:6379
  REDIS_PASSWORD=
  REDIS_DB=1
  MINIO_ENDPOINT=127.0.0.1:9300   # MinIO API 端口（不是 console 端口）
  MINIO_ACCESS_KEY=minioadmin
  MINIO_SECRET_KEY=minioadmin
  MINIO_USE_SSL=false
  MINIO_BUCKET=uploads
  CHECK_STAGE8_PIPELINE_AFTER_MERGE=1
  PIPELINE_LOG_FILE=./logs/app.log
  PIPELINE_WAIT_SECONDS=25
  PIPELINE_POLL_MS=500
  AUTO_REGISTER=1
  AUTO_USER_PREFIX=stage7_accept
  AUTO_PASSWORD=Passw0rd!Stage7
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

json() {
  jq -e "$1" >/dev/null <<<"$REQUEST_BODY"
}

request() {
  local body_file="${WORK_DIR}/response.json"
  REQUEST_STATUS="$(curl -sS -o "${body_file}" -w '%{http_code}' "$@")" || fail "curl 请求失败: $*"
  REQUEST_BODY="$(cat "${body_file}")"
  if [[ "$VERBOSE" -eq 1 ]]; then
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

wait_for_log_pattern() {
  local pattern="$1"
  local desc="$2"
  local deadline=$(( $(date +%s) + PIPELINE_WAIT_SECONDS ))

  while (( $(date +%s) <= deadline )); do
    if [[ -f "${PIPELINE_LOG_FILE}" ]] && rg -n --fixed-strings "${pattern}" "${PIPELINE_LOG_FILE}" >/dev/null 2>&1; then
      pass "${desc}"
      return 0
    fi
    sleep "$(awk "BEGIN { printf \"%.3f\", ${PIPELINE_POLL_MS}/1000 }")"
  done

  fail "${desc} 超时: 未在 ${PIPELINE_WAIT_SECONDS}s 内匹配日志 pattern=${pattern}"
}

while getopts ":f:b:t:u:p:o:Prkvh" opt; do
  case "${opt}" in
    f) FILE="${OPTARG}" ;;
    b) BASE_URL="${OPTARG}" ;;
    t) TOKEN="${OPTARG}" ;;
    u) LOGIN_USERNAME="${OPTARG}" ;;
    p) LOGIN_PASSWORD="${OPTARG}" ;;
    o) ORG_TAG="${OPTARG}" ;;
    P) IS_PUBLIC="true" ;;
    r) USE_REAL_MD5=1 ;;
    k) KEEP_TMP=1 ;;
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

[[ -n "${FILE}" ]] || {
  usage
  fail "必须指定 -f <file>"
}
[[ -f "${FILE}" ]] || fail "文件不存在: ${FILE}"

require_cmd curl
require_cmd jq
require_cmd split
require_cmd awk
if [[ "${CHECK_CLEANUP}" == "1" ]]; then
  require_cmd go
fi
if [[ "${CHECK_STAGE8_PIPELINE_AFTER_MERGE}" == "1" ]]; then
  require_cmd rg
fi

mkdir -p "${WORK_DIR}"
if [[ "${KEEP_TMP}" -eq 0 ]]; then
  trap 'rm -rf "${WORK_DIR}"' EXIT
fi

log "工作目录: ${WORK_DIR}"

if [[ -z "${TOKEN}" ]]; then
  if [[ -n "${LOGIN_USERNAME}" && -n "${LOGIN_PASSWORD}" ]]; then
    log "未提供 TOKEN，尝试登录获取 access token"
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

# 获取 userID，用于校验 Redis key: upload:{userID}:{fileMD5}
request \
  -X GET "${BASE_URL}/api/v1/users/me" \
  -H "Authorization: Bearer ${TOKEN}"
assert_status 200 "获取当前用户信息失败"
USER_ID="$(jq -er '.data.id' <<<"${REQUEST_BODY}")" || fail "无法从 /users/me 响应解析 userID"
pass "当前用户 ID: ${USER_ID}"

FILE_NAME="$(basename "${FILE}")"
TOTAL_SIZE="$(file_size "${FILE}")"
BASE_MD5="$(md5_file "${FILE}")"
RUN_ID="$(date +%s%N)"
if [[ "${USE_REAL_MD5}" -eq 1 ]]; then
  FILE_MD5="${BASE_MD5}"
else
  FILE_MD5="$(md5_text "${BASE_MD5}_${RUN_ID}")"
fi

(( TOTAL_SIZE > 0 )) || fail "文件为空，无法进行分片上传验收"

log "开始切片: file=${FILE_NAME} size=${TOTAL_SIZE} md5=${FILE_MD5}"
split -b "${CHUNK_SIZE}" -d -a 6 "${FILE}" "${WORK_DIR}/chunk_"
mapfile -t CHUNKS < <(find "${WORK_DIR}" -maxdepth 1 -type f -name 'chunk_*' | sort)
TOTAL_CHUNKS="${#CHUNKS[@]}"
(( TOTAL_CHUNKS > 0 )) || fail "切片失败，未生成任何分片"
pass "切片完成，总分片数: ${TOTAL_CHUNKS}"

# 1) check: 新文件应为 completed=false
check_payload="$(jq -nc --arg md5 "${FILE_MD5}" '{md5:$md5}')"
request \
  -X POST "${BASE_URL}/api/v1/upload/check" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${check_payload}"
assert_status 200 "首次 check 失败"
json '.code == 200 and .data.completed == false and (.data.uploadedChunks | type == "array")' || \
  fail "首次 check 断言失败"
pass "首次 check 通过（新文件未完成）"

# 2) 循环上传分片并断言
prev_progress=0
for idx in "${!CHUNKS[@]}"; do
  chunk_file="${CHUNKS[$idx]}"
  request \
    -X POST "${BASE_URL}/api/v1/upload/chunk" \
    -H "Authorization: Bearer ${TOKEN}" \
    -F "fileMd5=${FILE_MD5}" \
    -F "fileName=${FILE_NAME}" \
    -F "totalSize=${TOTAL_SIZE}" \
    -F "chunkIndex=${idx}" \
    -F "orgTag=${ORG_TAG}" \
    -F "isPublic=${IS_PUBLIC}" \
    -F "file=@${chunk_file}"
  assert_status 200 "上传分片 ${idx} 失败"
  jq -e --argjson i "${idx}" '.code == 200 and (.data.uploadedChunks | index($i) != null)' \
    <<<"${REQUEST_BODY}" >/dev/null || fail "分片 ${idx} 上传后未出现在 uploadedChunks"

  if ! jq -e --argjson p "${prev_progress}" '.data.progress >= $p' <<<"${REQUEST_BODY}" >/dev/null; then
    fail "分片 ${idx} 上传后 progress 未保持单调递增"
  fi
  prev_progress="$(jq -er '.data.progress' <<<"${REQUEST_BODY}")"
done
pass "分片循环上传通过"

# 3) 再次 check：应为 completed=false 且分片列表长度=total_chunks
request \
  -X POST "${BASE_URL}/api/v1/upload/check" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${check_payload}"
assert_status 200 "上传后 check 失败"
jq -e --argjson total "${TOTAL_CHUNKS}" \
  '.code == 200 and .data.completed == false and (.data.uploadedChunks | length == $total)' \
  <<<"${REQUEST_BODY}" >/dev/null || fail "上传后 check 断言失败"
pass "上传后 check 通过（分片齐全，尚未 merge）"

# 4) merge：应成功
merge_payload="$(jq -nc --arg md5 "${FILE_MD5}" --arg fn "${FILE_NAME}" '{fileMd5:$md5,fileName:$fn}')"
request \
  -X POST "${BASE_URL}/api/v1/upload/merge" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${merge_payload}"
assert_status 200 "merge 失败"
jq -e --arg md5 "${FILE_MD5}" --arg fn "${FILE_NAME}" \
  '.code == 200 and .data.fileMd5 == $md5 and .data.fileName == $fn and (.data.objectUrl | length > 0)' \
  <<<"${REQUEST_BODY}" >/dev/null || fail "merge 返回断言失败"
pass "merge 通过"

# 5) merge 后 check：应 completed=true
request \
  -X POST "${BASE_URL}/api/v1/upload/check" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${check_payload}"
assert_status 200 "merge 后 check 失败"
json '.code == 200 and .data.completed == true' || fail "merge 后 check 断言失败"
pass "merge 后 check 通过（completed=true）"

# 6) 幂等性：对已完成文件重复传同一分片，应 progress=100
request \
  -X POST "${BASE_URL}/api/v1/upload/chunk" \
  -H "Authorization: Bearer ${TOKEN}" \
  -F "fileMd5=${FILE_MD5}" \
  -F "fileName=${FILE_NAME}" \
  -F "totalSize=${TOTAL_SIZE}" \
  -F "chunkIndex=0" \
  -F "orgTag=${ORG_TAG}" \
  -F "isPublic=${IS_PUBLIC}" \
  -F "file=@${CHUNKS[0]}"
assert_status 200 "已完成文件重复上传分片失败"
jq -e --argjson total "${TOTAL_CHUNKS}" \
  '.code == 200 and .data.progress == 100 and (.data.uploadedChunks | length == $total)' \
  <<<"${REQUEST_BODY}" >/dev/null || fail "幂等性断言失败（未返回 100%）"
pass "幂等性断言通过"

# 7) 负例：分片不完整时 merge 应返回 400
INCOMPLETE_MD5="$(md5_text "incomplete_${RUN_ID}")"
INCOMPLETE_TOTAL_SIZE=$((CHUNK_SIZE * 2))
incomplete_merge_payload="$(jq -nc --arg md5 "${INCOMPLETE_MD5}" --arg fn "${FILE_NAME}" '{fileMd5:$md5,fileName:$fn}')"

request \
  -X POST "${BASE_URL}/api/v1/upload/chunk" \
  -H "Authorization: Bearer ${TOKEN}" \
  -F "fileMd5=${INCOMPLETE_MD5}" \
  -F "fileName=${FILE_NAME}" \
  -F "totalSize=${INCOMPLETE_TOTAL_SIZE}" \
  -F "chunkIndex=0" \
  -F "orgTag=${ORG_TAG}" \
  -F "isPublic=${IS_PUBLIC}" \
  -F "file=@${CHUNKS[0]}"
assert_status 200 "负例准备：上传第一个分片失败"

request \
  -X POST "${BASE_URL}/api/v1/upload/merge" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${incomplete_merge_payload}"
assert_status 400 "负例 merge 状态码不符合预期"
json '.code == 400 and (.message | ascii_downcase | contains("not all chunks"))' || \
  fail "负例 merge 错误信息断言失败"
pass "负例断言通过（分片不完整无法 merge）"

# 8) merge 后异步清理校验：Redis key 删除 + MinIO chunks 前缀清空
if [[ "${CHECK_CLEANUP}" == "1" ]]; then
  cleanup_redis_key="upload:${USER_ID}:${FILE_MD5}"
  cleanup_minio_prefix="chunks/${FILE_MD5}/"
  log "校验异步清理: redisKey=${cleanup_redis_key}, minioPrefix=${cleanup_minio_prefix}"

  cleanup_stdout="${WORK_DIR}/cleanup_probe.out"
  cleanup_stderr="${WORK_DIR}/cleanup_probe.err"
  set +e
  (
    cd "${REPO_ROOT}" && go run ./scripts/acceptance/storage_cleanup_probe.go \
      -redis-addr "${REDIS_ADDR}" \
      -redis-password "${REDIS_PASSWORD}" \
      -redis-db "${REDIS_DB}" \
      -redis-key "${cleanup_redis_key}" \
      -minio-endpoint "${MINIO_ENDPOINT}" \
      -minio-access-key "${MINIO_ACCESS_KEY}" \
      -minio-secret-key "${MINIO_SECRET_KEY}" \
      -minio-use-ssl="${MINIO_USE_SSL}" \
      -minio-bucket "${MINIO_BUCKET}" \
      -minio-prefix "${cleanup_minio_prefix}" \
      -timeout-sec "${CLEANUP_TIMEOUT_SEC}" \
      -poll-ms "${CLEANUP_POLL_MS}" \
      >"${cleanup_stdout}" 2>"${cleanup_stderr}"
  )
  cleanup_rc=$?
  set -e

  if [[ ${cleanup_rc} -ne 0 ]]; then
    if [[ -s "${cleanup_stderr}" ]]; then
      printf '[DEBUG] cleanup probe stderr:\n%s\n' "$(cat "${cleanup_stderr}")" >&2
    fi
    if [[ -s "${cleanup_stdout}" ]]; then
      printf '[DEBUG] cleanup probe stdout:\n%s\n' "$(cat "${cleanup_stdout}")" >&2
    fi
    fail "异步清理校验失败（超时或探针执行异常）"
  fi

  if [[ ! -s "${cleanup_stdout}" ]]; then
    fail "cleanup 探针无输出，无法解析结果"
  fi
  if ! jq -e . "${cleanup_stdout}" >/dev/null 2>&1; then
    if [[ -s "${cleanup_stderr}" ]]; then
      printf '[DEBUG] cleanup probe stderr:\n%s\n' "$(cat "${cleanup_stderr}")" >&2
    fi
    printf '[DEBUG] cleanup probe stdout:\n%s\n' "$(cat "${cleanup_stdout}")" >&2
    fail "cleanup 输出不是有效 JSON"
  fi

  redis_exists="$(jq -r '.redisExists' "${cleanup_stdout}")" || fail "cleanup 输出解析失败(redisExists)"
  minio_count="$(jq -r '.minioObjectCount' "${cleanup_stdout}")" || fail "cleanup 输出解析失败(minioObjectCount)"
  [[ "${redis_exists}" == "true" || "${redis_exists}" == "false" ]] || fail "cleanup 输出字段 redisExists 非法: ${redis_exists}"
  [[ "${minio_count}" =~ ^[0-9]+$ ]] || fail "cleanup 输出字段 minioObjectCount 非法: ${minio_count}"
  [[ "${redis_exists}" == "false" ]] || fail "Redis 清理失败：key 仍存在 (${cleanup_redis_key})"
  [[ "${minio_count}" == "0" ]] || fail "MinIO 清理失败：仍存在 ${minio_count} 个分片对象"
  pass "异步清理校验通过（Redis key 已删除，MinIO 分片已清空）"
fi

# 9) 端到端闭环：校验 merge 后阶段八异步链路日志（Producer -> Consumer -> Tika）
if [[ "${CHECK_STAGE8_PIPELINE_AFTER_MERGE}" == "1" ]]; then
  log "校验阶段八异步链路日志: fileMD5=${FILE_MD5}"
  wait_for_log_pattern "文件处理任务已投递: md5=${FILE_MD5}" "Producer 投递日志命中"
  wait_for_log_pattern "[Consumer] 文件任务处理成功: MD5=${FILE_MD5}" "Consumer 成功日志命中"
  wait_for_log_pattern "[Processor] Tika 提取文本成功: md5=${FILE_MD5}" "Tika 提取成功日志命中"
fi

printf '\n'
pass "阶段七验收完成: 全部断言通过"
log "file=${FILE_NAME} md5=${FILE_MD5} totalChunks=${TOTAL_CHUNKS}"

if [[ -n "${RESULT_JSON_FILE}" ]]; then
  OBJECT_KEY="uploads/${USER_ID}/${FILE_MD5}/${FILE_NAME}"
  jq -nc \
    --arg fileMd5 "${FILE_MD5}" \
    --arg fileName "${FILE_NAME}" \
    --argjson userId "${USER_ID}" \
    --arg orgTag "${ORG_TAG}" \
    --argjson isPublic "$([[ "${IS_PUBLIC}" == "true" ]] && echo true || echo false)" \
    --arg objectKey "${OBJECT_KEY}" \
    --argjson totalChunks "${TOTAL_CHUNKS}" \
    --argjson totalSize "${TOTAL_SIZE}" \
    '{
      fileMd5: $fileMd5,
      fileName: $fileName,
      userId: $userId,
      orgTag: $orgTag,
      isPublic: $isPublic,
      objectKey: $objectKey,
      totalChunks: $totalChunks,
      totalSize: $totalSize
    }' > "${RESULT_JSON_FILE}"
fi
