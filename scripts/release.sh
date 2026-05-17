#!/usr/bin/env bash
set -euo pipefail

# Build and package agentflow release artifacts for a single target platform.
# Usage: release.sh <version> <os> <arch> [outdir]
#
# Example:
#   ./scripts/release.sh v0.5.0 linux amd64 ./dist

VERSION="${1:-dev}"
OS="${2:-$(go env GOOS)}"
ARCH="${3:-$(go env GOARCH)}"
OUTDIR="${4:-./dist}"

COMMIT="${GITHUB_SHA:-$(git rev-parse HEAD 2>/dev/null || echo 'unknown')}"

utc_date_from_epoch() {
	epoch="$1"
	if date -u -r "$epoch" +%Y-%m-%dT%H:%M:%SZ >/dev/null 2>&1; then
		date -u -r "$epoch" +%Y-%m-%dT%H:%M:%SZ
	else
		date -u -d "@$epoch" +%Y-%m-%dT%H:%M:%SZ
	fi
}

touch_from_epoch() {
	epoch="$1"
	shift
	if date -u -r "$epoch" +%Y%m%d%H%M.%S >/dev/null 2>&1; then
		formatted="$(date -u -r "$epoch" +%Y%m%d%H%M.%S)"
	else
		formatted="$(date -u -d "@$epoch" +%Y%m%d%H%M.%S)"
	fi
	touch -t "$formatted" "$@"
}

if [ -n "${BUILD_DATE:-}" ]; then
	DATE="$BUILD_DATE"
	BUILD_EPOCH="${SOURCE_DATE_EPOCH:-$(git log -1 --format=%ct "$COMMIT" 2>/dev/null || date +%s)}"
else
	BUILD_EPOCH="${SOURCE_DATE_EPOCH:-$(git log -1 --format=%ct "$COMMIT" 2>/dev/null || date +%s)}"
	DATE="$(utc_date_from_epoch "$BUILD_EPOCH")"
fi

EXT=""
if [ "$OS" = "windows" ]; then
	EXT=".exe"
fi

mkdir -p "$OUTDIR"

LDFLAGS="-buildid= -X main.buildVersion=${VERSION} -X main.buildCommit=${COMMIT} -X main.buildDate=${DATE}"

echo >&2 "Building agentflow ${VERSION} (${COMMIT}) for ${OS}/${ARCH}..."

GOOS="$OS" GOARCH="$ARCH" go build -buildvcs=false -trimpath -ldflags "$LDFLAGS" -o "${OUTDIR}/agentflow${EXT}" ./cmd/agentflow
GOOS="$OS" GOARCH="$ARCH" go build -buildvcs=false -trimpath -ldflags "$LDFLAGS" -o "${OUTDIR}/agentflowd${EXT}" ./cmd/agentflowd

BUNDLE="agentflow-${VERSION}-${OS}-${ARCH}"
mkdir -p "${OUTDIR}/${BUNDLE}"
cp "${OUTDIR}/agentflow${EXT}" "${OUTDIR}/agentflowd${EXT}" "${OUTDIR}/${BUNDLE}/"
touch_from_epoch "$BUILD_EPOCH" "${OUTDIR}/agentflow${EXT}" "${OUTDIR}/agentflowd${EXT}" "${OUTDIR}/${BUNDLE}" "${OUTDIR}/${BUNDLE}/agentflow${EXT}" "${OUTDIR}/${BUNDLE}/agentflowd${EXT}"

if [ "$OS" = "windows" ]; then
	(cd "$OUTDIR" && zip -X -r -q "${BUNDLE}.zip" "$BUNDLE")
	echo "${BUNDLE}.zip"
else
	GZIP=-n tar -czf "${OUTDIR}/${BUNDLE}.tar.gz" -C "$OUTDIR" "$BUNDLE"
	echo "${BUNDLE}.tar.gz"
fi
