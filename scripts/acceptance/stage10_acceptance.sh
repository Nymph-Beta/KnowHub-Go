#!/usr/bin/env bash
set -euo pipefail

BASE_URL="http://localhost:8081"
TOKEN="${TOKEN:-}"
LOGIN_USERNAME="${LOGIN_USERNAME:-}"
LOGIN_PASSWORD="${LOGIN_PASSWORD:-}"
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

DOCKER_CONTAINERS="${DOCKER_CONTAINERS:-mysql paismart-v2-redis paismart-v2-minio paismart-v2-zookeeper paismart-v2-kafka paismart-v2-tika elasticsearch}"
MYSQL_DSN="${MYSQL_DSN:-root:PaiSmart2025@tcp(127.0.0.1:3307)/paismart_v2?parseTime=True}"
KAFKA_BROKERS="${KAFKA_BROKERS:-127.0.0.1:9093}"
KAFKA_TOPIC="${KAFKA_TOPIC:-file-processing}"

ES_BASE_URL="${ES_BASE_URL:-http://127.0.0.1:9200}"
ES_INDEX="${ES_INDEX:-knowledge_base}"
ES_USERNAME="${ES_USERNAME:-}"
ES_PASSWORD="${ES_PASSWORD:-}"
ES_VECTOR_DIMS="${ES_VECTOR_DIMS:-2048}"
EMBEDDING_MODEL="${EMBEDDING_MODEL:-text-embedding-v4}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
STAGE6_TO_9_SCRIPT="${SCRIPT_DIR}/stage6_to_9_acceptance.sh"
DEFAULT_PDF_FILE="${REPO_ROOT}/scripts/test_data/Hashimoto.pdf"
WORK_DIR="${WORK_DIR:-/tmp/paismart-stage10-acceptance-$$}"
GO_BIN="${GO_BIN:-/home/yyy/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.11.linux-amd64/bin/go}"
GO_CACHE_DIR="${GO_CACHE_DIR:-${REPO_ROOT}/.tmp/gocache}"

REQUEST_STATUS=""
REQUEST_BODY=""
ES_STATUS=""
ES_BODY=""

usage() {
  cat <<'EOF'
Usage:
  stage10_acceptance.sh [options]

Options:
  --pdf-file       阶段十验收使用的 PDF 文件，默认: scripts/test_data/Hashimoto.pdf
  --stage7-file    --pdf-file 的兼容别名
  -b <base_url>    服务地址，默认: http://localhost:8081
  -t <token>       直接指定 Bearer token
  -u <username>    登录用户名
  -p <password>    登录密码
  -o <org_tag>     可选 orgTag
  -l <log_file>    应用日志路径，默认: ./logs/app.log
  -w <seconds>     等待异步链路/ES 入索引超时秒数，默认: 35
  -P               isPublic=true（默认 false）
  -k               保留临时目录
  -v               输出详细响应
  -D               跳过 Docker 容器检查
  -I               跳过重复消息幂等性验证
  -h               显示帮助

Elasticsearch envs:
  ES_BASE_URL=http://127.0.0.1:9200
  ES_INDEX=knowledge_base
  ES_USERNAME=
  ES_PASSWORD=
  ES_VECTOR_DIMS=2048
  EMBEDDING_MODEL=text-embedding-v4
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

run_stage6_to_9() {
  local result_json="$1"
  local -a cmd

  cmd=(
    "${STAGE6_TO_9_SCRIPT}"
    --pdf-file "${PDF_FILE}"
    -b "${BASE_URL}"
    -l "${LOG_FILE}"
    -w "${WAIT_SECONDS}"
    -I
  )

  if [[ -n "${TOKEN}" ]]; then
    cmd+=(-t "${TOKEN}")
  fi
  if [[ -n "${LOGIN_USERNAME}" ]]; then
    cmd+=(-u "${LOGIN_USERNAME}")
  fi
  if [[ -n "${LOGIN_PASSWORD}" ]]; then
    cmd+=(-p "${LOGIN_PASSWORD}")
  fi
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

  RESULT_JSON_FILE="${result_json}" "${cmd[@]}"
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

sha256_text() {
  local text="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s' "${text}" | sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    printf '%s' "${text}" | shasum -a 256 | awk '{print $1}'
  else
    fail "系统缺少 sha256sum/shasum 命令"
  fi
}

wait_for_es_documents() {
  local file_md5="$1"
  local expected_min="$2"
  local output_file="$3"
  local deadline=$(( $(date +%s) + WAIT_SECONDS ))
  local search_body

  search_body="$(jq -nc \
    --arg md5 "${file_md5}" \
    '{
      size: 1000,
      sort: [{chunk_id: {order: "asc"}}],
      _source: ["vector_id","file_md5","chunk_id","text_content","model_version","user_id","org_tag","is_public"],
      query: {term: {file_md5: $md5}}
    }')"

  while (( $(date +%s) <= deadline )); do
    es_request POST "/${ES_INDEX}/_search" "${search_body}"
    if [[ "${ES_STATUS}" == "200" ]]; then
      local count
      count="$(jq -r '.hits.hits | length' <<<"${ES_BODY}")" || count=0
      if [[ "${count}" =~ ^[0-9]+$ ]] && (( count >= expected_min )); then
        printf '%s' "${ES_BODY}" > "${output_file}"
        pass "Elasticsearch 文档已可查询: fileMd5=${file_md5} count=${count}"
        return 0
      fi
    fi
    sleep "$(awk "BEGIN { printf \"%.3f\", ${POLL_MS}/1000 }")"
  done

  fail "等待 Elasticsearch 文档超时: fileMd5=${file_md5} expectedMin=${expected_min}"
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

[[ -x "${STAGE6_TO_9_SCRIPT}" ]] || fail "缺少脚本: ${STAGE6_TO_9_SCRIPT}"
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

STAGE69_RESULT_JSON="${WORK_DIR}/stage69_result.json"
run_stage6_to_9 "${STAGE69_RESULT_JSON}"

[[ -s "${STAGE69_RESULT_JSON}" ]] || fail "阶段六到九结果文件为空"
FILE_MD5="$(jq -er '.fileMd5' "${STAGE69_RESULT_JSON}")" || fail "无法解析 fileMd5"
FILE_NAME="$(jq -er '.fileName' "${STAGE69_RESULT_JSON}")" || fail "无法解析 fileName"
USER_ID="$(jq -er '.userId' "${STAGE69_RESULT_JSON}")" || fail "无法解析 userId"
RESULT_ORG_TAG="$(jq -er '.orgTag' "${STAGE69_RESULT_JSON}")" || fail "无法解析 orgTag"
RESULT_IS_PUBLIC="$(jq -r '.isPublic' "${STAGE69_RESULT_JSON}")" || fail "无法解析 isPublic"
OBJECT_KEY="$(jq -er '.objectKey' "${STAGE69_RESULT_JSON}")" || fail "无法解析 objectKey"
CHUNK_COUNT="$(jq -er '.chunkCount' "${STAGE69_RESULT_JSON}")" || fail "无法解析 chunkCount"

wait_for_log_pattern "[Processor] Embedding 生成成功: md5=${FILE_MD5}" "阶段十 Embedding 成功日志命中"
wait_for_log_pattern "[Processor] Elasticsearch 索引成功: md5=${FILE_MD5}" "阶段十 Elasticsearch 索引成功日志命中"
wait_for_log_pattern "[Processor] 文件处理成功完成: md5=${FILE_MD5}" "阶段十文件处理完成日志命中"

es_request GET "/${ES_INDEX}/_mapping"
[[ "${ES_STATUS}" == "200" ]] || fail "获取 Elasticsearch mapping 失败"
jq -e --arg idx "${ES_INDEX}" --argjson dims "${ES_VECTOR_DIMS}" '
  .[$idx].mappings.properties.vector.type == "dense_vector" and
  .[$idx].mappings.properties.vector.dims == $dims and
  .[$idx].mappings.properties.text_content.type == "text"
' <<<"${ES_BODY}" >/dev/null || fail "Elasticsearch mapping 断言失败"
pass "Elasticsearch mapping 校验通过"

ES_SEARCH_JSON="${WORK_DIR}/es_search.json"
wait_for_es_documents "${FILE_MD5}" "${CHUNK_COUNT}" "${ES_SEARCH_JSON}"

ES_DOC_COUNT="$(jq -r '.hits.hits | length' "${ES_SEARCH_JSON}")" || fail "无法解析 ES 文档数"
ES_CONTIGUOUS="$(jq -r '[.hits.hits[]._source.chunk_id] as $ids | reduce range(0; ($ids | length)) as $i (true; . and ($ids[$i] == $i))' "${ES_SEARCH_JSON}")" || fail "无法解析 ES contiguous"
ES_DUPLICATE="$(jq -r '([.hits.hits[]._source.vector_id] | length) != ([.hits.hits[]._source.vector_id] | unique | length)' "${ES_SEARCH_JSON}")" || fail "无法解析 ES duplicate"
ES_MODEL_ALL_MATCH="$(jq -r --arg model "${EMBEDDING_MODEL}" 'all(.hits.hits[]._source.model_version; . == $model)' "${ES_SEARCH_JSON}")" || fail "无法解析 ES model_version"
ES_USER_ID="$(jq -r '.hits.hits[0]._source.user_id' "${ES_SEARCH_JSON}")" || fail "无法解析 ES user_id"
ES_ORG_TAG="$(jq -r '.hits.hits[0]._source.org_tag // ""' "${ES_SEARCH_JSON}")" || fail "无法解析 ES org_tag"
ES_IS_PUBLIC="$(jq -r '.hits.hits[0]._source.is_public' "${ES_SEARCH_JSON}")" || fail "无法解析 ES is_public"
ES_VECTOR_IDS_MATCH="$(jq -r --arg md5 "${FILE_MD5}" 'all(.hits.hits[]._source.vector_id; test("^" + $md5 + "_[0-9]+$"))' "${ES_SEARCH_JSON}")" || fail "无法解析 ES vector_id"
ES_FIRST_TEXT_LEN="$(jq -r '(.hits.hits[0]._source.text_content // "") | length' "${ES_SEARCH_JSON}")" || fail "无法解析 ES text_content 长度"
ES_SIGNATURE_RAW="$(jq -r '.hits.hits[]._source | "\(.chunk_id):\(.text_content)"' "${ES_SEARCH_JSON}")"
ES_SIGNATURE="$(sha256_text "${ES_SIGNATURE_RAW}")"

[[ "${ES_DOC_COUNT}" == "${CHUNK_COUNT}" ]] || fail "ES 文档数与 chunkCount 不一致: es=${ES_DOC_COUNT} chunk=${CHUNK_COUNT}"
[[ "${ES_CONTIGUOUS}" == "true" ]] || fail "ES chunk_id 不是连续递增"
[[ "${ES_DUPLICATE}" == "false" ]] || fail "ES vector_id 出现重复"
[[ "${ES_MODEL_ALL_MATCH}" == "true" ]] || fail "ES model_version 与预期模型不一致"
[[ "${ES_USER_ID}" == "${USER_ID}" ]] || fail "ES user_id 与阶段九结果不一致"
[[ "${ES_ORG_TAG}" == "${RESULT_ORG_TAG}" ]] || fail "ES org_tag 与阶段九结果不一致"
[[ "${ES_IS_PUBLIC}" == "${RESULT_IS_PUBLIC}" ]] || fail "ES is_public 与阶段九结果不一致"
[[ "${ES_VECTOR_IDS_MATCH}" == "true" ]] || fail "ES vector_id 命名不符合预期"
[[ "${ES_FIRST_TEXT_LEN}" =~ ^[0-9]+$ ]] || fail "ES 首个文本长度非法: ${ES_FIRST_TEXT_LEN}"
(( ES_FIRST_TEXT_LEN > 0 )) || fail "ES text_content 为空"
pass "阶段十 Elasticsearch 文档校验通过: docs=${ES_DOC_COUNT}"

if [[ "${CHECK_IDEMPOTENCY}" == "1" ]]; then
  consumer_before="$(pattern_count "[Consumer] 文件任务处理成功: MD5=${FILE_MD5}")"
  es_before="$(pattern_count "[Processor] Elasticsearch 索引成功: md5=${FILE_MD5}")"

  log "重放同一条 Kafka 文件任务，验证阶段十 ES 终态幂等性"
  replay_file_task "${FILE_MD5}" "${FILE_NAME}" "${USER_ID}" "${RESULT_ORG_TAG}" "${RESULT_IS_PUBLIC}" "${OBJECT_KEY}" || fail "重放 Kafka 文件任务失败"

  wait_for_log_count_increment "[Consumer] 文件任务处理成功: MD5=${FILE_MD5}" "${consumer_before}" "重复消息 Consumer 成功日志命中"
  wait_for_log_count_increment "[Processor] Elasticsearch 索引成功: md5=${FILE_MD5}" "${es_before}" "重复消息 Elasticsearch 索引成功日志命中"

  ES_REPLAY_JSON="${WORK_DIR}/es_search_replay.json"
  wait_for_es_documents "${FILE_MD5}" "${CHUNK_COUNT}" "${ES_REPLAY_JSON}"

  REPLAY_DOC_COUNT="$(jq -r '.hits.hits | length' "${ES_REPLAY_JSON}")" || fail "无法解析 replay ES 文档数"
  REPLAY_SIGNATURE_RAW="$(jq -r '.hits.hits[]._source | "\(.chunk_id):\(.text_content)"' "${ES_REPLAY_JSON}")"
  REPLAY_SIGNATURE="$(sha256_text "${REPLAY_SIGNATURE_RAW}")"
  REPLAY_DUPLICATE="$(jq -r '([.hits.hits[]._source.vector_id] | length) != ([.hits.hits[]._source.vector_id] | unique | length)' "${ES_REPLAY_JSON}")" || fail "无法解析 replay ES duplicate"

  [[ "${REPLAY_DOC_COUNT}" == "${ES_DOC_COUNT}" ]] || fail "重复消息后 ES 文档数变化: before=${ES_DOC_COUNT} after=${REPLAY_DOC_COUNT}"
  [[ "${REPLAY_DUPLICATE}" == "false" ]] || fail "重复消息后 ES vector_id 出现重复"
  [[ "${REPLAY_SIGNATURE}" == "${ES_SIGNATURE}" ]] || fail "重复消息后 ES 文本签名变化"
  pass "阶段十 Elasticsearch 幂等性校验通过"
fi

printf '\n'
pass "阶段十验收完成"
log "pdfFile=$(basename "${PDF_FILE}") md5=${FILE_MD5} chunkCount=${CHUNK_COUNT} esIndex=${ES_INDEX}"
