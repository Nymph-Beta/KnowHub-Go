#!/usr/bin/env bash
set -euo pipefail

BASE_URL="http://localhost:8081"
TOKEN="${TOKEN:-}"
LOGIN_USERNAME="${LOGIN_USERNAME:-}"
LOGIN_PASSWORD="${LOGIN_PASSWORD:-}"
AUTO_REGISTER="${AUTO_REGISTER:-1}"
AUTO_USER_PREFIX="${AUTO_USER_PREFIX:-stage12_accept}"
AUTO_PASSWORD="${AUTO_PASSWORD:-Passw0rd!Stage12}"
ORG_TAG="${ORG_TAG:-}"
PDF_FILE=""
LOG_FILE="${LOG_FILE:-./logs/app.log}"
WAIT_SECONDS="${WAIT_SECONDS:-60}"
CHECK_DOCKER=1
CHECK_IDEMPOTENCY="${CHECK_IDEMPOTENCY:-1}"
KEEP_TMP=0
VERBOSE=0
CHAT_QUERY="${CHAT_QUERY:-}"
STOP_QUERY="${STOP_QUERY:-}"
REQUIRE_ANY_KEYWORDS="${REQUIRE_ANY_KEYWORDS:-}"
MIN_ANSWER_RUNES="${MIN_ANSWER_RUNES:-80}"
MAX_POST_STOP_CHUNKS="${MAX_POST_STOP_CHUNKS:-1}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
STAGE11_SCRIPT="${SCRIPT_DIR}/stage11_acceptance.sh"
DEFAULT_PDF_FILE="${REPO_ROOT}/scripts/test_data/Hashimoto.pdf"
WORK_DIR="${WORK_DIR:-/tmp/paismart-stage12-acceptance-$$}"
GO_BIN="${GO_BIN:-/home/yyy/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.11.linux-amd64/bin/go}"
GO_CACHE_DIR="${GO_CACHE_DIR:-${REPO_ROOT}/.tmp/gocache}"
WS_PROBE_PKG="./scripts/acceptance/ws_chat_probe"

REQUEST_STATUS=""
REQUEST_BODY=""

usage() {
  cat <<'EOF'
Usage:
  stage12_acceptance.sh [options]

Options:
  --pdf-file       阶段十二验收使用的 PDF 文件，默认: scripts/test_data/Hashimoto.pdf
  --stage7-file    --pdf-file 的兼容别名
  -b <base_url>    服务地址，默认: http://localhost:8081
  -t <token>       直接指定 owner Bearer token
  -u <username>    owner 登录用户名
  -p <password>    owner 登录密码
  -o <org_tag>     可选 orgTag
  -l <log_file>    应用日志路径，默认: ./logs/app.log
  -w <seconds>     等待超时秒数，默认: 60
  -q <query>       WebSocket 对话 query；默认会按 PDF 名选择
  --require-any-keywords <a|b>
                   要求回答至少包含一个关键词
  --min-answer-runes <n>
                   要求完整回答至少包含多少 rune，默认: 80
  --max-post-stop-chunks <n>
                   stop 发送后最多允许收到多少个尾部 chunk，默认: 1
  -k               保留临时目录
  -v               输出详细响应
  -D               跳过 Docker 容器检查
  -I               跳过阶段十一重复消息幂等性验证
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
    printf '[FAIL] last http status=%s body=%s\n' "${REQUEST_STATUS}" "${REQUEST_BODY}" >&2
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

login_user() {
  local username="$1"
  local password="$2"
  local login_payload

  login_payload="$(jq -nc --arg u "${username}" --arg p "${password}" '{username:$u,password:$p}')"
  request \
    -X POST "${BASE_URL}/api/v1/users/login" \
    -H "Content-Type: application/json" \
    -d "${login_payload}"
  assert_status 200 "登录失败"
  jq -er '.data.accessToken' <<<"${REQUEST_BODY}" || fail "登录成功但未解析到 accessToken"
}

register_and_login_user() {
  local username="$1"
  local password="$2"
  local register_payload

  register_payload="$(jq -nc --arg u "${username}" --arg p "${password}" '{username:$u,password:$p}')"
  request \
    -X POST "${BASE_URL}/api/v1/users/register" \
    -H "Content-Type: application/json" \
    -d "${register_payload}"
  [[ "${REQUEST_STATUS}" == "201" || "${REQUEST_STATUS}" == "409" ]] || fail "注册测试用户失败"
  login_user "${username}" "${password}"
}

ensure_owner_token() {
  if [[ -n "${TOKEN}" ]]; then
    return 0
  fi

  if [[ -n "${LOGIN_USERNAME}" && -n "${LOGIN_PASSWORD}" ]]; then
    TOKEN="$(login_user "${LOGIN_USERNAME}" "${LOGIN_PASSWORD}")"
    pass "owner 登录成功"
    return 0
  fi

  [[ "${AUTO_REGISTER}" == "1" ]] || fail "未提供 owner token/用户名密码，且禁用了自动注册"

  LOGIN_USERNAME="${AUTO_USER_PREFIX}_$(date +%s)_$RANDOM"
  LOGIN_PASSWORD="${AUTO_PASSWORD}"
  log "未提供 owner 凭据，自动注册测试用户: ${LOGIN_USERNAME}"
  TOKEN="$(register_and_login_user "${LOGIN_USERNAME}" "${LOGIN_PASSWORD}")"
  pass "owner 自动注册并登录成功"
}

run_stage11() {
  local -a cmd

  cmd=(
    "${STAGE11_SCRIPT}"
    --pdf-file "${PDF_FILE}"
    -b "${BASE_URL}"
    -t "${TOKEN}"
    -l "${LOG_FILE}"
    -w "${WAIT_SECONDS}"
  )

  if [[ -n "${ORG_TAG}" ]]; then
    cmd+=(-o "${ORG_TAG}")
  fi
  if [[ "${KEEP_TMP}" -eq 1 ]]; then
    cmd+=(-k)
  fi
  if [[ "${VERBOSE}" -eq 1 ]]; then
    cmd+=(-v)
  fi
  if [[ "${CHECK_DOCKER}" -eq 0 ]]; then
    cmd+=(-D)
  fi
  if [[ "${CHECK_IDEMPOTENCY}" == "0" ]]; then
    cmd+=(-I)
  fi

  "${cmd[@]}"
}

fetch_ws_token() {
  request \
    -X GET "${BASE_URL}/api/v1/chat/websocket-token" \
    -H "Authorization: Bearer ${TOKEN}"
  assert_status 200 "获取 WebSocket token 失败"
  jq -er '.data.cmdToken' <<<"${REQUEST_BODY}" || fail "WebSocket token 响应里缺少 cmdToken"
}

derive_default_query() {
  local pdf_basename
  pdf_basename="$(basename "${PDF_FILE}")"
  case "${pdf_basename,,}" in
    hashimoto.pdf)
      printf 'Tatsunori Hashimoto'
      ;;
    *)
      printf '请根据知识库总结这份文档的核心内容'
      ;;
  esac
}

derive_required_keywords() {
  local pdf_basename
  pdf_basename="$(basename "${PDF_FILE}")"
  case "${pdf_basename,,}" in
    hashimoto.pdf)
      printf 'hashimoto|natural language processing|pretrained language models'
      ;;
    *)
      printf ''
      ;;
  esac
}

build_ws_url() {
  local ws_token="$1"
  local origin="${BASE_URL}"
  origin="${origin/http:\/\//ws://}"
  origin="${origin/https:\/\//wss://}"
  printf '%s/chat/%s' "${origin}" "${ws_token}"
}

run_ws_probe() {
  local ws_url="$1"
  local message="$2"
  local expect_status="$3"
  local stop_mode="$4"
  local -a cmd

  mkdir -p "${GO_CACHE_DIR}"
  cmd=(
    "${GO_BIN}" run "${WS_PROBE_PKG}"
    -ws-url "${ws_url}"
    -message "${message}"
    -timeout-seconds "${WAIT_SECONDS}"
    -expect-status "${expect_status}"
  )
  if [[ "${expect_status}" == "finished" ]]; then
    cmd+=(-min-answer-runes "${MIN_ANSWER_RUNES}")
  fi
  if [[ "${expect_status}" == "finished" && -n "${REQUIRE_ANY_KEYWORDS}" ]]; then
    cmd+=(-require-any-keywords "${REQUIRE_ANY_KEYWORDS}")
  fi
  if [[ "${stop_mode}" == "1" ]]; then
    cmd+=(-stop-after-first-chunk -max-post-stop-chunks "${MAX_POST_STOP_CHUNKS}")
  fi
  if [[ "${VERBOSE}" -eq 1 ]]; then
    cmd+=(-verbose)
  fi

  (
    cd "${REPO_ROOT}" && env GOCACHE="${GO_CACHE_DIR}" "${cmd[@]}"
  )
}

assert_invalid_ws_token() {
  request -X GET "${BASE_URL}/chat/not-a-valid-websocket-token"
  assert_status 401 "无效 ws token 未返回 401"
  jq -e '.message == "Invalid websocket token"' <<<"${REQUEST_BODY}" >/dev/null 2>&1 || fail "无效 ws token 错误消息不符合预期"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --pdf-file|--stage7-file|-f)
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
    -q)
      CHAT_QUERY="${2:-}"
      shift 2
      ;;
    --require-any-keywords)
      REQUIRE_ANY_KEYWORDS="${2:-}"
      shift 2
      ;;
    --min-answer-runes)
      MIN_ANSWER_RUNES="${2:-}"
      shift 2
      ;;
    --max-post-stop-chunks)
      MAX_POST_STOP_CHUNKS="${2:-}"
      shift 2
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

[[ -x "${STAGE11_SCRIPT}" ]] || fail "缺少脚本: ${STAGE11_SCRIPT}"
require_cmd curl
require_cmd jq
if [[ ! -x "${GO_BIN}" ]]; then
  GO_BIN="$(command -v go || true)"
fi
[[ -n "${GO_BIN}" ]] || fail "缺少 go 命令"

mkdir -p "${WORK_DIR}"
if [[ "${KEEP_TMP}" -eq 0 ]]; then
  trap 'rm -rf "${WORK_DIR}"' EXIT
fi

cd "${REPO_ROOT}"

if [[ -z "${PDF_FILE}" ]]; then
  PDF_FILE="${DEFAULT_PDF_FILE}"
fi
[[ -f "${PDF_FILE}" ]] || fail "PDF 文件不存在: ${PDF_FILE}"
[[ "${MIN_ANSWER_RUNES}" =~ ^[0-9]+$ ]] || fail "min-answer-runes 必须是非负整数"
[[ "${MAX_POST_STOP_CHUNKS}" =~ ^[0-9]+$ ]] || fail "max-post-stop-chunks 必须是非负整数"

ensure_owner_token

log "先复用阶段十一验收，确保文档处理与检索链路已就绪"
run_stage11

if [[ -z "${CHAT_QUERY}" ]]; then
  CHAT_QUERY="$(derive_default_query)"
fi
if [[ -z "${STOP_QUERY}" ]]; then
  STOP_QUERY="${CHAT_QUERY}"
fi
if [[ -z "${REQUIRE_ANY_KEYWORDS}" ]]; then
  REQUIRE_ANY_KEYWORDS="$(derive_required_keywords)"
fi

assert_invalid_ws_token
pass "阶段十二无效 ws token 校验通过"

WS_TOKEN="$(fetch_ws_token)"
WS_URL="$(build_ws_url "${WS_TOKEN}")"
run_ws_probe "${WS_URL}" "${CHAT_QUERY}" "finished" "0"
pass "阶段十二正常流式对话校验通过"

WS_TOKEN="$(fetch_ws_token)"
WS_URL="$(build_ws_url "${WS_TOKEN}")"
run_ws_probe "${WS_URL}" "${STOP_QUERY}" "stopped" "1"
pass "阶段十二停止信号校验通过"

printf '\n'
pass "阶段十二验收完成"
log "pdfFile=$(basename "${PDF_FILE}") query=${CHAT_QUERY}"
