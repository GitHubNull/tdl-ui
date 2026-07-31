#!/usr/bin/env bash
# 生成分组化的 Release Notes（Markdown 输出到 stdout）。
#
# 分组约定与 ref/tdl/.goreleaser.yaml 的 changelog groups 保持一致：
#   feat -> 新增功能 / fix -> Bug 修复 / docs -> 文档更新 /
#   refactor -> 重构 / perf -> 性能优化 / 其余 -> 其他变更
#
# 用法:
#   gen-release-notes.sh <tag>
# 环境变量:
#   REPO_URL  仓库主页（形如 https://github.com/owner/repo），用于生成对比链接；
#             留空时自动从 remote.origin.url 推断。
#
# 无上一个 tag 时回退为「全量历史」，保证首个 release 也能生成说明。
set -euo pipefail

TAG="${1:-$(git describe --tags --abbrev=0 2>/dev/null || echo HEAD)}"

# 定位上一个 tag：从当前 tag 的父提交向前查找，避免匹配到自身。
PREV_TAG="$(git describe --tags --abbrev=0 "${TAG}^" 2>/dev/null || true)"

if [ -n "${PREV_TAG}" ]; then
  RANGE="${PREV_TAG}..${TAG}"
else
  RANGE="${TAG}"
fi

# 推断仓库主页，用于 Full Changelog 对比链接。
REPO_URL="${REPO_URL:-}"
if [ -z "${REPO_URL}" ]; then
  origin="$(git config --get remote.origin.url 2>/dev/null || true)"
  case "${origin}" in
    git@github.com:*) REPO_URL="https://github.com/${origin#git@github.com:}" ;;
    https://*)        REPO_URL="${origin}" ;;
  esac
  REPO_URL="${REPO_URL%.git}"
fi

# 收集提交：短 hash + tab + 主题；排除 merge 提交。
LOG="$(git log --no-merges --pretty=format:'%h%x09%s' "${RANGE}" 2>/dev/null || true)"

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT
: >"${TMP}/feat"; : >"${TMP}/fix"; : >"${TMP}/docs"
: >"${TMP}/refactor"; : >"${TMP}/perf"; : >"${TMP}/others"

if [ -n "${LOG}" ]; then
  printf '%s\n' "${LOG}" | while IFS="$(printf '\t')" read -r hash subject; do
    [ -z "${hash}" ] && continue
    line="- ${subject} (${hash})"
    lc="$(printf '%s' "${subject}" | tr '[:upper:]' '[:lower:]')"
    case "${lc}" in
      feat*)     printf '%s\n' "${line}" >>"${TMP}/feat" ;;
      fix*)      printf '%s\n' "${line}" >>"${TMP}/fix" ;;
      docs*)     printf '%s\n' "${line}" >>"${TMP}/docs" ;;
      refactor*) printf '%s\n' "${line}" >>"${TMP}/refactor" ;;
      perf*)     printf '%s\n' "${line}" >>"${TMP}/perf" ;;
      *)         printf '%s\n' "${line}" >>"${TMP}/others" ;;
    esac
  done
fi

emit_group() {
  local title="$1" file="$2"
  if [ -s "${file}" ]; then
    printf '### %s\n\n' "${title}"
    cat "${file}"
    printf '\n'
  fi
}

# 头部
printf '## tdl UI %s\n\n' "${TAG}"

emit_group "新增功能" "${TMP}/feat"
emit_group "Bug 修复" "${TMP}/fix"
emit_group "文档更新" "${TMP}/docs"
emit_group "重构" "${TMP}/refactor"
emit_group "性能优化" "${TMP}/perf"
emit_group "其他变更" "${TMP}/others"

# 若本次区间没有任何提交，给出占位说明，避免正文为空。
if ! { [ -s "${TMP}/feat" ] || [ -s "${TMP}/fix" ] || [ -s "${TMP}/docs" ] || \
       [ -s "${TMP}/refactor" ] || [ -s "${TMP}/perf" ] || [ -s "${TMP}/others" ]; }; then
  printf '_本次发布没有可归类的提交记录。_\n\n'
fi

# 尾部对比链接
if [ -n "${PREV_TAG}" ] && [ -n "${REPO_URL}" ]; then
  printf '**Full Changelog**: %s/compare/%s...%s\n' "${REPO_URL}" "${PREV_TAG}" "${TAG}"
fi
