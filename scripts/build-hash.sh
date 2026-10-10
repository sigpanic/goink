#!/usr/bin/env bash
set -euo pipefail

# 在前端生成、资源改写等构建操作之前调用，保留源码的原始 dirty 状态。
cd "$(dirname "${BASH_SOURCE[0]}")/.."
revision=$(git rev-parse --verify HEAD)
changes=$(git --no-optional-locks status --porcelain --untracked-files=normal)
build_hash=${revision:0:8}
if [[ -n "$changes" ]]; then
  build_hash+="-dirty"
fi
printf '%s\n' "$build_hash"
