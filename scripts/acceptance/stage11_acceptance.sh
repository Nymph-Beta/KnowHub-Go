#!/usr/bin/env bash
set -euo pipefail

BASE_URL="http://localhost:8081"
TOKEN="${TOKEN:-}"
LOGIN_USERNAME="${LOGIN_USERNAME:-}"
LOGIN_PASSWORD="${LOGIN_PASSWORD:-}"
AUTO_REGISTER="${AUTO_REGISTER:-1}"
AUTO_USER_PREFIX="${AUTO_USER_PREFIX:-stage11_accept}"
AUTO_PASSWORD="${AUTO_PASSWORD:-Passw0rd!Stage11}"
STRANGER_USERNAME="${STRANGER_USERNAME:-}"
STRANGER_PASSWORD="${STRANGER_PASSWORD:-}"
STRANGER_AUTO_USER_PREFIX="${STRANGER_AUTO_USER_PREFIX:-stage11_stranger}"
STRANGER_AUTO_PASSWORD="${STRANGER_AUTO_PASSWORD:-Passw0rd!Stage11Other}"
ORG_TAG="${ORG_TAG:-}"
PDF_FILE=""
LOG_FILE="${LOG_FILE:-./logs/app.log}"
WAIT_SECONDS="${WAIT_SECONDS:-35}"
POLL_MS="${POLL_MS:-500}"
IS_PUBLIC="${IS_PUBLIC:-false}"
CHECK_DOCKER=1
CHECK_IDEMPOTENCY="${CHECK_IDEMPOTENCY:-1}"
KEEP_TMP=0
VERBOSE=0
TOP_K="${TOP_K:-5}"
SEARCH_QUERY="${SEARCH_QUERY:-}"
EXACT_SEARCH_QUERIES="${EXACT_SEARCH_QUERIES:-}"
TOPIC_SEARCH_QUERIES="${TOPIC_SEARCH_QUERIES:-}"

ES_BASE_URL="${ES_BASE_URL:-http://127.0.0.1:9200}"
ES_INDEX="${ES_INDEX:-knowledge_base}"
ES_USERNAME="${ES_USERNAME:-}"
ES_PASSWORD="${ES_PASSWORD:-}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
STAGE10_SCRIPT="${SCRIPT_DIR}/stage10_acceptance.sh"
DEFAULT_PDF_FILE="${REPO_ROOT}/scripts/test_data/Hashimoto.pdf"
WORK_DIR="${WORK_DIR:-/tmp/paismart-stage11-acceptance-$$}"

REQUEST_STATUS=""
REQUEST_BODY=""
ES_STATUS=""
ES_BODY=""

usage() {
  cat <<'EOF'
Usage:
  stage11_acceptance.sh [options]

Options:
  --pdf-file       阶段十一验收使用的 PDF 文件，默认: scripts/test_data/Hashimoto.pdf
  --stage7-file    --pdf-file 的兼容别名
  -b <base_url>    服务地址，默认: http://localhost:8081
  -t <token>       直接指定 owner Bearer token
  -u <username>    owner 登录用户名
  -p <password>    owner 登录密码
  -o <org_tag>     可选 orgTag
  -l <log_file>    应用日志路径，默认: ./logs/app.log
  -w <seconds>     等待超时秒数，默认: 35
  -K <topK>        搜索 topK，默认: 5
  -q <query>       指定搜索 query；不传则自动从已索引 chunk 截取
  -P               isPublic=true（默认 false）
  -k               保留临时目录
  -v               输出详细响应
  -D               跳过 Docker 容器检查
  -I               跳过阶段十重复消息幂等性验证
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
  if [[ -n "${ES_BODY}" ]]; then
    printf '[FAIL] last es status=%s body=%s\n' "${ES_STATUS}" "${ES_BODY}" >&2
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

es_request() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local body_file="${WORK_DIR}/es_response.json"
  local -a args

  args=(-sS -o "${body_file}" -w '%{http_code}' -X "${method}")
  if [[ -n "${ES_USERNAME}" || -n "${ES_PASSWORD}" ]]; then
    args+=(-u "${ES_USERNAME}:${ES_PASSWORD}")
  fi
  if [[ -n "${body}" ]]; then
    args+=(-H "Content-Type: application/json" -d "${body}")
  fi

  ES_STATUS="$(curl "${args[@]}" "${ES_BASE_URL}${path}")" || fail "ES 请求失败: ${method} ${path}"
  ES_BODY="$(cat "${body_file}")"
  if [[ "${VERBOSE}" -eq 1 ]]; then
    printf '[DEBUG] ES HTTP %s %s %s\n' "${ES_STATUS}" "${method}" "${path}"
    if jq . >/dev/null 2>&1 <<<"${ES_BODY}"; then
      jq . <<<"${ES_BODY}"
    else
      printf '%s\n' "${ES_BODY}"
    fi
  fi
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

ensure_stranger_token() {
  if [[ -n "${STRANGER_USERNAME}" && -n "${STRANGER_PASSWORD}" ]]; then
    register_and_login_user "${STRANGER_USERNAME}" "${STRANGER_PASSWORD}" >/dev/null
    jq -er '.data.accessToken' <<<"${REQUEST_BODY}" >/dev/null
    return 0
  fi

  STRANGER_USERNAME="${STRANGER_AUTO_USER_PREFIX}_$(date +%s)_$RANDOM"
  STRANGER_PASSWORD="${STRANGER_AUTO_PASSWORD}"
  log "自动注册 stranger 测试用户: ${STRANGER_USERNAME}"
  register_and_login_user "${STRANGER_USERNAME}" "${STRANGER_PASSWORD}" >/dev/null
}

run_stage10() {
  local result_json="$1"
  local -a cmd

  cmd=(
    "${STAGE10_SCRIPT}"
    --pdf-file "${PDF_FILE}"
    -b "${BASE_URL}"
    -t "${TOKEN}"
    -l "${LOG_FILE}"
    -w "${WAIT_SECONDS}"
  )

  if [[ -n "${ORG_TAG}" ]]; then
    cmd+=(-o "${ORG_TAG}")
  fi
  if [[ "${IS_PUBLIC}" == "true" ]]; then
    cmd+=(-P)
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

  RESULT_JSON_FILE="${result_json}" "${cmd[@]}"
}

wait_for_es_searchable() {
  local file_md5="$1"
  local output_file="$2"
  local deadline=$(( $(date +%s) + WAIT_SECONDS ))
  local search_body

  search_body="$(jq -nc \
    --arg md5 "${file_md5}" \
    '{
      size: 10,
      sort: [{chunk_id: {order: "asc"}}],
      _source: ["file_md5","chunk_id","text_content","user_id","org_tag","is_public"],
      query: {term: {file_md5: $md5}}
    }')"

  while (( $(date +%s) <= deadline )); do
    es_request POST "/${ES_INDEX}/_search" "${search_body}"
    if [[ "${ES_STATUS}" == "200" ]]; then
      local count
      count="$(jq -r '.hits.hits | length' <<<"${ES_BODY}")" || count=0
      if [[ "${count}" =~ ^[0-9]+$ ]] && (( count > 0 )); then
        printf '%s' "${ES_BODY}" > "${output_file}"
        pass "阶段十一查询样本文本已从 ES 准备完成: fileMd5=${file_md5} count=${count}"
        return 0
      fi
    fi
    sleep "$(awk "BEGIN { printf \"%.3f\", ${POLL_MS}/1000 }")"
  done

  fail "等待 ES 查询样本文本超时: fileMd5=${file_md5}"
}

derive_query_from_es() {
  local es_json="$1"
  jq -er '
    .hits.hits
    | map(._source.text_content // "")
    | map(gsub("\\s+"; " "))
    | map(gsub("^ +| +$"; ""))
    | map(select(length > 0))
    | .[0][0:18]
  ' "${es_json}" || fail "无法从 ES 文档提取搜索 query"
}

search_request() {
  local token="$1"
  local query="$2"
  local top_k="$3"
  local encoded_query
  encoded_query="$(jq -rn --arg v "${query}" '$v|@uri')"

  request \
    -X GET "${BASE_URL}/api/v1/search/hybrid?query=${encoded_query}&topK=${top_k}" \
    -H "Authorization: Bearer ${token}"
}

split_query_list() {
  local raw="$1"
  local -n out_ref="$2"
  local item

  out_ref=()
  while IFS= read -r item; do
    item="$(printf '%s' "${item}" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
    [[ -n "${item}" ]] || continue
    out_ref+=("${item}")
  done < <(printf '%s' "${raw}" | tr '|' '\n')
}

populate_default_queries() {
  local file_name="$1"
  local normalized_file

  EXACT_QUERY_CANDIDATES=()
  TOPIC_QUERY_CANDIDATES=()

  if [[ -n "${SEARCH_QUERY}" ]]; then
    EXACT_QUERY_CANDIDATES=("${SEARCH_QUERY}")
    TOPIC_QUERY_CANDIDATES=("${SEARCH_QUERY}")
    return 0
  fi

  if [[ -n "${EXACT_SEARCH_QUERIES}" ]]; then
    split_query_list "${EXACT_SEARCH_QUERIES}" EXACT_QUERY_CANDIDATES
  fi
  if [[ -n "${TOPIC_SEARCH_QUERIES}" ]]; then
    split_query_list "${TOPIC_SEARCH_QUERIES}" TOPIC_QUERY_CANDIDATES
  fi

  normalized_file="$(printf '%s' "${file_name}" | tr '[:upper:]' '[:lower:]')"
  if [[ "${#EXACT_QUERY_CANDIDATES[@]}" -eq 0 ]] && [[ "${normalized_file}" == "hashimoto.pdf" ]]; then
    EXACT_QUERY_CANDIDATES=(
      "Tatsunori Hashimoto"
      "Natural Language Processing"
    )
  fi
  if [[ "${#TOPIC_QUERY_CANDIDATES[@]}" -eq 0 ]] && [[ "${normalized_file}" == "hashimoto.pdf" ]]; then
    TOPIC_QUERY_CANDIDATES=(
      "pretrained language models"
      "预训练 语言模型"
    )
  fi
}

wait_for_any_query_contains_file() {
  local token="$1"
  local top_k="$2"
  local file_md5="$3"
  local output_file="$4"
  shift 4
  local queries=("$@")
  local deadline=$(( $(date +%s) + WAIT_SECONDS ))
  local query

  while (( $(date +%s) <= deadline )); do
    for query in "${queries[@]}"; do
      search_request "${token}" "${query}" "${top_k}"
      if [[ "${REQUEST_STATUS}" == "200" ]]; then
        if jq -e --arg md5 "${file_md5}" '.data | any(.fileMd5 == $md5)' <<<"${REQUEST_BODY}" >/dev/null 2>&1; then
          MATCHED_QUERY="${query}"
          printf '%s' "${REQUEST_BODY}" > "${output_file}"
          pass "搜索结果命中文档: fileMd5=${file_md5} query=${query}"
          return 0
        fi
      fi
    done
    sleep "$(awk "BEGIN { printf \"%.3f\", ${POLL_MS}/1000 }")"
  done

  fail "等待搜索命中文档超时: fileMd5=${file_md5} queries=${queries[*]}"
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
    -K)
      TOP_K="${2:-}"
      shift 2
      ;;
    -q)
      SEARCH_QUERY="${2:-}"
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

[[ -x "${STAGE10_SCRIPT}" ]] || fail "缺少脚本: ${STAGE10_SCRIPT}"
require_cmd curl
require_cmd jq
require_cmd awk

mkdir -p "${WORK_DIR}"
if [[ "${KEEP_TMP}" -eq 0 ]]; then
  trap 'rm -rf "${WORK_DIR}"' EXIT
fi

cd "${REPO_ROOT}"

if [[ -z "${PDF_FILE}" ]]; then
  PDF_FILE="${DEFAULT_PDF_FILE}"
fi
[[ -f "${PDF_FILE}" ]] || fail "PDF 文件不存在: ${PDF_FILE}"
[[ "${TOP_K}" =~ ^[0-9]+$ ]] || fail "topK 必须是正整数"
(( TOP_K > 0 )) || fail "topK 必须大于 0"

ensure_owner_token

STAGE10_RESULT_JSON="${WORK_DIR}/stage10_result.json"
run_stage10 "${STAGE10_RESULT_JSON}"

[[ -s "${STAGE10_RESULT_JSON}" ]] || fail "阶段十结果文件为空"
FILE_MD5="$(jq -er '.fileMd5' "${STAGE10_RESULT_JSON}")" || fail "无法解析 fileMd5"
FILE_NAME="$(jq -er '.fileName' "${STAGE10_RESULT_JSON}")" || fail "无法解析 fileName"
OWNER_USER_ID="$(jq -er '.userId' "${STAGE10_RESULT_JSON}")" || fail "无法解析 userId"
RESULT_IS_PUBLIC="$(jq -r '.isPublic' "${STAGE10_RESULT_JSON}")" || fail "无法解析 isPublic"

ES_HITS_JSON="${WORK_DIR}/es_hits.json"
wait_for_es_searchable "${FILE_MD5}" "${ES_HITS_JSON}"

populate_default_queries "${FILE_NAME}"
if [[ "${#EXACT_QUERY_CANDIDATES[@]}" -eq 0 ]]; then
  SEARCH_QUERY="$(derive_query_from_es "${ES_HITS_JSON}")"
  EXACT_QUERY_CANDIDATES=("${SEARCH_QUERY}")
fi
if [[ "${#TOPIC_QUERY_CANDIDATES[@]}" -eq 0 ]]; then
  TOPIC_QUERY_CANDIDATES=("${EXACT_QUERY_CANDIDATES[@]}")
fi
pass "阶段十一精确查询候选: ${EXACT_QUERY_CANDIDATES[*]}"
pass "阶段十一主题查询候选: ${TOPIC_QUERY_CANDIDATES[*]}"

OWNER_EXACT_SEARCH_JSON="${WORK_DIR}/owner_exact_search.json"
wait_for_any_query_contains_file "${TOKEN}" "${TOP_K}" "${FILE_MD5}" "${OWNER_EXACT_SEARCH_JSON}" "${EXACT_QUERY_CANDIDATES[@]}"
EXACT_MATCHED_QUERY="${MATCHED_QUERY}"

OWNER_TOPIC_SEARCH_JSON="${WORK_DIR}/owner_topic_search.json"
wait_for_any_query_contains_file "${TOKEN}" "${TOP_K}" "${FILE_MD5}" "${OWNER_TOPIC_SEARCH_JSON}" "${TOPIC_QUERY_CANDIDATES[@]}"
TOPIC_MATCHED_QUERY="${MATCHED_QUERY}"

OWNER_RESULT_COUNT="$(jq -r '.data | length' "${OWNER_EXACT_SEARCH_JSON}")" || fail "无法解析 owner 搜索结果数量"
OWNER_TOPK_OK="$(jq -r --argjson topk "${TOP_K}" '.data | length <= $topk' "${OWNER_EXACT_SEARCH_JSON}")" || fail "无法解析 owner topK"
OWNER_FILE_NAME_MATCH="$(jq -r --arg md5 "${FILE_MD5}" --arg name "${FILE_NAME}" '.data | map(select(.fileMd5 == $md5)) | any(.fileName == $name)' "${OWNER_EXACT_SEARCH_JSON}")" || fail "无法解析 owner fileName"
OWNER_TEXT_NON_EMPTY="$(jq -r --arg md5 "${FILE_MD5}" '.data | map(select(.fileMd5 == $md5)) | all((.textContent // "") | length > 0)' "${OWNER_EXACT_SEARCH_JSON}")" || fail "无法解析 owner textContent"
OWNER_SCORE_NUMERIC="$(jq -r --arg md5 "${FILE_MD5}" '.data | map(select(.fileMd5 == $md5)) | all((.score | type) == "number")' "${OWNER_EXACT_SEARCH_JSON}")" || fail "无法解析 owner score"
OWNER_USER_MATCH="$(jq -r --arg md5 "${FILE_MD5}" --argjson uid "${OWNER_USER_ID}" '.data | map(select(.fileMd5 == $md5)) | any(.userId == $uid)' "${OWNER_EXACT_SEARCH_JSON}")" || fail "无法解析 owner userId"
OWNER_TOPIC_TOPK_OK="$(jq -r --argjson topk "${TOP_K}" '.data | length <= $topk' "${OWNER_TOPIC_SEARCH_JSON}")" || fail "无法解析 owner topic topK"

[[ "${OWNER_RESULT_COUNT}" =~ ^[0-9]+$ ]] || fail "owner 搜索结果数量非法: ${OWNER_RESULT_COUNT}"
[[ "${OWNER_TOPK_OK}" == "true" ]] || fail "owner 搜索结果超出 topK"
[[ "${OWNER_FILE_NAME_MATCH}" == "true" ]] || fail "owner 搜索结果 fileName 不匹配"
[[ "${OWNER_TEXT_NON_EMPTY}" == "true" ]] || fail "owner 搜索结果 textContent 为空"
[[ "${OWNER_SCORE_NUMERIC}" == "true" ]] || fail "owner 搜索结果 score 不是数字"
[[ "${OWNER_USER_MATCH}" == "true" ]] || fail "owner 搜索结果 userId 不匹配"
[[ "${OWNER_TOPIC_TOPK_OK}" == "true" ]] || fail "owner 主题搜索结果超出 topK"
pass "阶段十一 owner 搜索结果校验通过: exact=${EXACT_MATCHED_QUERY} topic=${TOPIC_MATCHED_QUERY}"

ensure_stranger_token
STRANGER_TOKEN="$(jq -er '.data.accessToken' <<<"${REQUEST_BODY}")" || fail "无法解析 stranger accessToken"

search_request "${STRANGER_TOKEN}" "${EXACT_MATCHED_QUERY}" "${TOP_K}"
assert_status 200 "stranger 搜索请求失败"

if [[ "${RESULT_IS_PUBLIC}" == "true" ]]; then
  jq -e --arg md5 "${FILE_MD5}" '.data | any(.fileMd5 == $md5)' <<<"${REQUEST_BODY}" >/dev/null || fail "公开文档未被 stranger 搜到"
  pass "阶段十一公开文档 stranger 可见性校验通过"
else
  jq -e --arg md5 "${FILE_MD5}" '.data | any(.fileMd5 == $md5)' <<<"${REQUEST_BODY}" >/dev/null && fail "私有文档被 stranger 搜到了"
  pass "阶段十一私有文档 stranger 不可见校验通过"
fi

search_request "${TOKEN}" "${EXACT_MATCHED_QUERY}" "abc"
[[ "${REQUEST_STATUS}" == "400" ]] || fail "非法 topK 未返回 400"
pass "阶段十一非法 topK 参数校验通过"

printf '\n'
pass "阶段十一验收完成"
log "pdfFile=$(basename "${PDF_FILE}") md5=${FILE_MD5} exactQuery=${EXACT_MATCHED_QUERY} topicQuery=${TOPIC_MATCHED_QUERY} topK=${TOP_K} isPublic=${RESULT_IS_PUBLIC}"
