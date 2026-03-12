#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STAGE7_SCRIPT="${SCRIPT_DIR}/stage7_chunk_upload_acceptance.sh"
STAGE8_SCRIPT="${SCRIPT_DIR}/stage8_kafka_tika_acceptance.sh"
STAGE9_SCRIPT="${SCRIPT_DIR}/stage9_chunk_vector_acceptance.sh"

STAGE=""
STAGE7_FILE=""
STAGE8_FILE=""
STAGE9_FILE=""

BASE_URL=""
TOKEN=""
LOGIN_USERNAME=""
LOGIN_PASSWORD=""
ORG_TAG=""
KEEP_TMP=0
VERBOSE=0

usage() {
  cat <<'EOF'
Usage:
  run_acceptance.sh -s <7|8|9|all> [options]

Options:
  -s <stage>       指定阶段：7 | 8 | 9 | all（必填）
  --stage7-file    阶段七验收文件（stage=7/all 时必填）
  --stage8-file    阶段八验收文件（可选，不传则自动生成）
  --stage9-file    阶段九验收文件（可选，不传则自动生成）
  -b <base_url>    服务地址（透传给子脚本）
  -t <token>       Bearer token（透传）
  -u <username>    登录用户名（透传）
  -p <password>    登录密码（透传）
  -o <org_tag>     orgTag（透传）
  -k               保留子脚本临时目录
  -v               详细输出
  -h               显示帮助

Examples:
  ./scripts/acceptance/run_acceptance.sh -s 8 -u test -p 123456
  ./scripts/acceptance/run_acceptance.sh -s 9 -u test -p 123456
  ./scripts/acceptance/run_acceptance.sh -s 7 --stage7-file ./testdata/large.pdf -t "$TOKEN"
  ./scripts/acceptance/run_acceptance.sh -s all --stage7-file ./testdata/large.pdf -u test -p 123456
EOF
}

fail() {
  printf '[FAIL] %s\n' "$*" >&2
  exit 1
}

build_common_args() {
  COMMON_ARGS=()
  if [[ -n "${BASE_URL}" ]]; then
    COMMON_ARGS+=(-b "${BASE_URL}")
  fi
  if [[ -n "${TOKEN}" ]]; then
    COMMON_ARGS+=(-t "${TOKEN}")
  fi
  if [[ -n "${LOGIN_USERNAME}" ]]; then
    COMMON_ARGS+=(-u "${LOGIN_USERNAME}")
  fi
  if [[ -n "${LOGIN_PASSWORD}" ]]; then
    COMMON_ARGS+=(-p "${LOGIN_PASSWORD}")
  fi
  if [[ -n "${ORG_TAG}" ]]; then
    COMMON_ARGS+=(-o "${ORG_TAG}")
  fi
  if [[ "${KEEP_TMP}" -eq 1 ]]; then
    COMMON_ARGS+=(-k)
  fi
  if [[ "${VERBOSE}" -eq 1 ]]; then
    COMMON_ARGS+=(-v)
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -s)
      STAGE="${2:-}"
      shift 2
      ;;
    --stage7-file)
      STAGE7_FILE="${2:-}"
      shift 2
      ;;
    --stage8-file)
      STAGE8_FILE="${2:-}"
      shift 2
      ;;
    --stage9-file)
      STAGE9_FILE="${2:-}"
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
    -k)
      KEEP_TMP=1
      shift
      ;;
    -v)
      VERBOSE=1
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

[[ -n "${STAGE}" ]] || fail "必须指定 -s <7|8|9|all>"
[[ -x "${STAGE7_SCRIPT}" ]] || fail "缺少脚本: ${STAGE7_SCRIPT}"
[[ -x "${STAGE8_SCRIPT}" ]] || fail "缺少脚本: ${STAGE8_SCRIPT}"
[[ -x "${STAGE9_SCRIPT}" ]] || fail "缺少脚本: ${STAGE9_SCRIPT}"

build_common_args

run_stage7() {
  [[ -n "${STAGE7_FILE}" ]] || fail "stage7 需要 --stage7-file <file>"
  "${STAGE7_SCRIPT}" -f "${STAGE7_FILE}" "${COMMON_ARGS[@]}"
}

run_stage8() {
  local args=("${COMMON_ARGS[@]}")
  if [[ -n "${STAGE8_FILE}" ]]; then
    args+=(-f "${STAGE8_FILE}")
  fi
  "${STAGE8_SCRIPT}" "${args[@]}"
}

run_stage9() {
  local args=("${COMMON_ARGS[@]}")
  if [[ -n "${STAGE9_FILE}" ]]; then
    args+=(-f "${STAGE9_FILE}")
  fi
  "${STAGE9_SCRIPT}" "${args[@]}"
}

case "${STAGE}" in
  7)
    run_stage7
    ;;
  8)
    run_stage8
    ;;
  9)
    run_stage9
    ;;
  all)
    run_stage7
    run_stage8
    run_stage9
    ;;
  *)
    fail "无效 stage: ${STAGE}（仅支持 7 | 8 | 9 | all）"
    ;;
esac
