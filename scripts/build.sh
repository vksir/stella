#!/bin/sh
# 构建各平台产物到 dist/。
set -e
cd "$(dirname "$0")/.."

targets="windows/amd64 linux/amd64"

for target in $targets; do
    os=${target%/*}
    arch=${target#*/}
    out="dist/stella-${os}-${arch}"
    if [ "$os" = "windows" ]; then
        out="${out}.exe"
    fi
    echo "building $out"
    CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -o "$out" ./cmd/stella
done
