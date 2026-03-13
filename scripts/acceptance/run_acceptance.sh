#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STAGE6_SCRIPT="${SCRIPT_DIR}/stage6_simple_upload_acceptance.sh"
STAGE7_SCRIPT="${SCRIPT_DIR}/stage7_chunk_upload_acceptance.sh"
STAGE6_TO_9_SCRIPT="${SCRIPT_DIR}/stage6_to_9_acceptance.sh"
STAGE10_SCRIPT="${SCRIPT_DIR}/stage10_acceptance.sh"

STAGE=""
STAGE6_FILE=""
STAGE7_FILE=""

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
  run_acceptance.sh -s <6|7|6-9|10|all> [options]

Options:
  -s <stage>       指定阶段：6 | 7 | 6-9 | 10 | all（必填）
  --stage6-file    阶段六验收文件（可选，不传则自动生成）
  --stage7-file    阶段七验收文件；对 6-9/all 会作为共享 PDF 覆盖默认 test_data
  -b <base_url>    服务地址（透传给子脚本）
  -t <token>       Bearer token（透传）
  -u <username>    登录用户名（透传）
  -p <password>    登录密码（透传）
  -o <org_tag>     orgTag（透传）
  -k               保留子脚本临时目录
  -v               详细输出
  -h               显示帮助

Examples:
  ./scripts/acceptance/run_acceptance.sh -s 6 -u test -p 123456
  ./scripts/acceptance/run_acceptance.sh -s 7 --stage7-file ./scripts/test_data/Hashimoto.pdf -u test -p 123456
  ./scripts/acceptance/run_acceptance.sh -s 6-9 -u test -p 123456
  ./scripts/acceptance/run_acceptance.sh -s 6-9 --stage7-file ./scripts/test_data/Hashimoto.pdf -u test -p 123456
  ./scripts/acceptance/run_acceptance.sh -s 10 --stage7-file ./scripts/test_data/Hashimoto.pdf -u test -p 123456
  ./scripts/acceptance/run_acceptance.sh -s all --stage7-file ./scripts/test_data/Hashimoto.pdf -u test -p 123456
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
    --stage6-file)
      STAGE6_FILE="${2:-}"
      shift 2
      ;;
    --stage7-file)
      STAGE7_FILE="${2:-}"
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

[[ -n "${STAGE}" ]] || fail "必须指定 -s <6|7|6-9|all>"
[[ -x "${STAGE6_SCRIPT}" ]] || fail "缺少脚本: ${STAGE6_SCRIPT}"
[[ -x "${STAGE7_SCRIPT}" ]] || fail "缺少脚本: ${STAGE7_SCRIPT}"
[[ -x "${STAGE6_TO_9_SCRIPT}" ]] || fail "缺少脚本: ${STAGE6_TO_9_SCRIPT}"
[[ -x "${STAGE10_SCRIPT}" ]] || fail "缺少脚本: ${STAGE10_SCRIPT}"

build_common_args

run_stage6() {
  local args=("${COMMON_ARGS[@]}")
  if [[ -n "${STAGE6_FILE}" ]]; then
    args+=(-f "${STAGE6_FILE}")
  fi
  "${STAGE6_SCRIPT}" "${args[@]}"
}

run_stage7() {
  [[ -n "${STAGE7_FILE}" ]] || fail "stage7 需要 --stage7-file <file>"
  "${STAGE7_SCRIPT}" -f "${STAGE7_FILE}" "${COMMON_ARGS[@]}"
}

run_stage6_to_9() {
  local args=("${COMMON_ARGS[@]}")
  if [[ -n "${STAGE6_FILE}" ]]; then
    args+=(--stage6-file "${STAGE6_FILE}")
  fi
  if [[ -n "${STAGE7_FILE}" ]]; then
    args+=(--stage7-file "${STAGE7_FILE}")
  fi
  "${STAGE6_TO_9_SCRIPT}" "${args[@]}"
}

run_stage10() {
  local args=("${COMMON_ARGS[@]}")
  if [[ -n "${STAGE7_FILE}" ]]; then
    args+=(--stage7-file "${STAGE7_FILE}")
  fi
  "${STAGE10_SCRIPT}" "${args[@]}"
}

case "${STAGE}" in
  6)
    run_stage6
    ;;
  7)
    run_stage7
    ;;
  6-9)
    run_stage6_to_9
    ;;
  10)
    run_stage10
    ;;
  all)
    run_stage10
    ;;
  *)
    fail "无效 stage: ${STAGE}（仅支持 6 | 7 | 6-9 | 10 | all）"
    ;;
esac
