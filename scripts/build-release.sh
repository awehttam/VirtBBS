#!/bin/zsh
# Build VirtBBS release packages (server, VirtAnd, source).
set -euo pipefail

VERSION="${1:-$(grep 'const Version' internal/version/version.go | sed 's/.*"\(.*\)".*/\1/')}"
REPO="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${RELEASE_DIR:-/tmp/virtbbs-release-${VERSION}}"

cd "$REPO"
rm -rf "$OUT"
mkdir -p "$OUT"

echo "==> Building VirtBBS release ${VERSION} -> ${OUT}"

pack_server() {
  local goos=$1 goarch=$2 label=$3
  local name="virtbbs-server-${VERSION}-${label}"
  local dir="${OUT}/${name}"
  local bin="virtbbs"
  [[ "$goos" == windows ]] && bin="virtbbs.exe"

  mkdir -p "${dir}/display" "${dir}/ppe"
  echo "  server ${label}"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build -trimpath -ldflags="-s -w" -o "${dir}/${bin}" ./cmd/virtbbs
  cp VirtBBS.DAT.example "${dir}/"
  cp display/LOGON.ANS display/LOGOFF.ASC display/NEWUSER.ASC "${dir}/display/"
  cp ppe/*.pps "${dir}/ppe/"
  (cd "$OUT" && zip -rq "${name}.zip" "$name")
}

pack_server darwin amd64 darwin-amd64
pack_server darwin arm64 darwin-arm64
pack_server linux amd64 linux-amd64
pack_server windows amd64 windows-amd64

echo "  VirtAnd APK"
export JAVA_HOME="${JAVA_HOME:-/usr/local/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home}"
(cd android/VirtAnd && ./android-build.sh :app:assembleDebug)
APK=$(find android/VirtAnd/app/build/outputs/apk/debug -name '*.apk' -print | head -1)
cp "$APK" "${OUT}/VirtAnd-${VERSION}-debug.apk"
(cd "$OUT" && zip -q "VirtAnd-${VERSION}-debug.zip" "VirtAnd-${VERSION}-debug.apk")

echo "  source zip"
git archive --format=zip --prefix="VirtBBS-${VERSION}/" "v${VERSION}" \
  -o "${OUT}/VirtBBS-${VERSION}-source.zip"

echo "==> Done. Packages in ${OUT}:"
ls -lh "$OUT"/*.{zip,apk} 2>/dev/null
