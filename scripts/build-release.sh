#!/bin/bash
# Build cckey-proxy for multiple platforms
set -e

VERSION="${1:-dev}"
OUTDIR="dist"
MODULE="github.com/huaguihai/cckey/cckey-proxy"

cd "$(dirname "$0")/../cckey-proxy"

rm -rf "../$OUTDIR"
mkdir -p "../$OUTDIR"

PLATFORMS=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
)

for platform in "${PLATFORMS[@]}"; do
    os="${platform%/*}"
    arch="${platform#*/}"
    output="../${OUTDIR}/cckey-proxy-${os}-${arch}"
    [ "$os" = "windows" ] && output="${output}.exe"

    echo "Building ${os}/${arch}..."
    GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build \
        -ldflags "-s -w -X main.version=${VERSION}" \
        -o "$output" .
done

echo ""
echo "Binaries in ${OUTDIR}/:"
ls -lh "../${OUTDIR}/"
