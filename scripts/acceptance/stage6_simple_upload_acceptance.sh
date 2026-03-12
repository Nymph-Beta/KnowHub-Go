#!/usr/bin/env bash
set -euo pipefail

BASE_URL="http://localhost:8081"
TOKEN="${TOKEN:-}"
LOGIN_USERNAME="${LOGIN_USERNAME:-}"
LOGIN_PASSWORD="${LOGIN_PASSWORD:-}"
AUTO_REGISTER="${AUTO_REGISTER:-1}"
AUTO_USER_PREFIX="${AUTO_USER_PREFIX:-stage6_accept}"
AUTO_PASSWORD="${AUTO_PASSWORD:-Passw0rd!Stage6}"
ORG_TAG="${ORG_TAG:-}"
FILE=""
KEEP_TMP=0
VERBOSE=0
CHECK_DOCKER=1

DOCKER_CONTAINERS="${DOCKER_CONTAINERS:-mysql paismart-v2-redis paismart-v2-minio paismart-v2-zookeeper paismart-v2-kafka}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
WORK_DIR="${WORK_DIR:-/tmp/paismart-stage6-acceptance-$$}"

REQUEST_STATUS=""
REQUEST_BODY=""

usage() {
  cat <<'EOF'
Usage:
  stage6_simple_upload_acceptance.sh [options]

Options:
  -b <base_url>   服务地址，默认: http://localhost:8081
  -t <token>      直接指定 Bearer token（优先）
  -u <username>   登录用户名（当未提供 -t 时使用；不传则自动注册测试用户）
  -p <password>   登录密码（与 -u 配套）
  -o <org_tag>    可选 orgTag（simple upload 表单字段）
  -f <file>       上传文件（可选，不传则自动生成 txt）
  -k              保留临时目录（默认自动清理）
  -v              输出详细响应
  -D              跳过 Docker 容器运行状态检查
  -h              显示帮助

Env (optional):
  DOCKER_CONTAINERS="mysql paismart-v2-redis paismart-v2-minio paismart-v2-zookeeper paismart-v2-kafka"
  AUTO_REGISTER=1
  AUTO_USER_PREFIX=stage6_accept
  AUTO_PASSWORD=Passw0rd!Stage6
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

while getopts ":b:t:u:p:o:f:kDvh" opt; do
  case "${opt}" in
    b) BASE_URL="${OPTARG}" ;;
    t) TOKEN="${OPTARG}" ;;
    u) LOGIN_USERNAME="${OPTARG}" ;;
    p) LOGIN_PASSWORD="${OPTARG}" ;;
    o) ORG_TAG="${OPTARG}" ;;
    f) FILE="${OPTARG}" ;;
    k) KEEP_TMP=1 ;;
    D) CHECK_DOCKER=0 ;;
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
pass "当前用户 ID: $(jq -er '.data.id' <<<"${REQUEST_BODY}")"

if [[ -z "${FILE}" ]]; then
  FILE="${WORK_DIR}/stage6_auto_$(date +%s).txt"
  UNIQUE_RUN_ID="$(date +%s%N)_$RANDOM"
  {
    echo "stage6 acceptance"
    echo "run_id=${UNIQUE_RUN_ID}"
    echo "验证简单上传、秒传和下载闭环"
  } > "${FILE}"
fi
[[ -f "${FILE}" ]] || fail "文件不存在: ${FILE}"

FILE_NAME="$(basename "${FILE}")"
LOCAL_MD5="$(md5_file "${FILE}")"
LOCAL_SIZE="$(file_size "${FILE}")"

request \
  -X POST "${BASE_URL}/api/v1/upload/simple" \
  -H "Authorization: Bearer ${TOKEN}" \
  -F "orgTag=${ORG_TAG}" \
  -F "file=@${FILE}"
assert_status 200 "首次 simple upload 失败"
FILE_MD5="$(jq -er '.data.fileMd5' <<<"${REQUEST_BODY}")" || fail "无法解析首次上传 fileMd5"
FIRST_IS_QUICK="$(jq -r '.data.isQuick' <<<"${REQUEST_BODY}")" || fail "无法解析首次上传 isQuick"
[[ "${FIRST_IS_QUICK}" == "false" ]] || fail "首次 simple upload 不应命中秒传"
[[ "$(jq -er '.data.fileName' <<<"${REQUEST_BODY}")" == "${FILE_NAME}" ]] || fail "首次 simple upload fileName 不匹配"
[[ "$(jq -er '.data.totalSize' <<<"${REQUEST_BODY}")" == "${LOCAL_SIZE}" ]] || fail "首次 simple upload totalSize 不匹配"
pass "首次 simple upload 通过"

request \
  -X POST "${BASE_URL}/api/v1/upload/simple" \
  -H "Authorization: Bearer ${TOKEN}" \
  -F "orgTag=${ORG_TAG}" \
  -F "file=@${FILE}"
assert_status 200 "重复 simple upload 失败"
SECOND_MD5="$(jq -er '.data.fileMd5' <<<"${REQUEST_BODY}")" || fail "无法解析重复上传 fileMd5"
SECOND_IS_QUICK="$(jq -r '.data.isQuick' <<<"${REQUEST_BODY}")" || fail "无法解析重复上传 isQuick"
[[ "${SECOND_MD5}" == "${FILE_MD5}" ]] || fail "重复 simple upload 返回了不同的 fileMd5"
[[ "${SECOND_IS_QUICK}" == "true" ]] || fail "重复 simple upload 未命中秒传"
pass "秒传命中校验通过"

DOWNLOAD_FILE="${WORK_DIR}/downloaded_${FILE_NAME}"
DOWNLOAD_STATUS="$(curl -sS -o "${DOWNLOAD_FILE}" -w '%{http_code}' \
  -H "Authorization: Bearer ${TOKEN}" \
  "${BASE_URL}/api/v1/documents/download?fileMd5=${FILE_MD5}")" || fail "下载请求失败"
[[ "${DOWNLOAD_STATUS}" == "200" ]] || fail "下载失败: 期望 HTTP 200，实际 ${DOWNLOAD_STATUS}"
[[ -f "${DOWNLOAD_FILE}" ]] || fail "下载文件不存在"
DOWNLOADED_MD5="$(md5_file "${DOWNLOAD_FILE}")"
[[ "${DOWNLOADED_MD5}" == "${LOCAL_MD5}" ]] || fail "下载文件内容与原文件不一致"
pass "下载闭环校验通过"

printf '\n'
pass "阶段六验收完成: 简单上传、秒传、下载均通过"
log "file=${FILE_NAME} fileMd5=${FILE_MD5}"
