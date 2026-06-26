#!/bin/zsh
#
# android-build.sh - Build VirtAnd from CLI (Android project on JohnDovey drive)
#
# Toolchain matches ClonesApp — see ../../CLAUDE.md.
#
# Usage examples:
#   ./android-build.sh                      # :app:assembleDebug (default)
#   ./android-build.sh :core:test           # JVM module only (no Android SDK)
#   ./android-build.sh :app:installDebug
#   ./android-build.sh clean
#   ./android-build.sh --no-configuration-cache :app:assembleDebug
#
# Make sure the JohnDovey drive is mounted.

set -e

if [[ -f ~/source-john-dovey.sh ]]; then
    source ~/source-john-dovey.sh --quiet
fi

export JAVA_HOME="${JAVA_HOME:-/usr/local/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home}"

cd "$(dirname "$0")"
chmod +x gradlew 2>/dev/null || true

if [ $# -eq 0 ]; then
    set -- ":app:assembleDebug"
fi

echo "Building VirtAnd..."
echo "   Args: $@"
echo "   JAVA_HOME=$JAVA_HOME"
echo "   ANDROID_HOME=${ANDROID_HOME:-/Volumes/JohnDovey/Android/Sdk}"
echo ""

./gradlew "$@"

echo ""
echo "Build finished"