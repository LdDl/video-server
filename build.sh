#!/bin/bash
set -e

APP=video_server
PKG=./cmd/${APP}

for tool in go tar zip; do
    if ! command -v ${tool} >/dev/null 2>&1; then
        echo "Required tool '${tool}' is not installed" >&2
        exit 1
    fi
done

echo "Building ${APP}..."

echo "linux/amd64"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
    go build -ldflags "-s -w" -buildvcs=false -trimpath -o ${APP} ${PKG}
tar -czvf linux-amd64-${APP}.tar.gz ${APP}

echo "linux/arm64"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
    go build -ldflags "-s -w" -buildvcs=false -trimpath -o ${APP} ${PKG}
tar -czvf linux-arm64-${APP}.tar.gz ${APP}

echo "windows/amd64"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
    go build -ldflags "-s -w" -buildvcs=false -trimpath -o ${APP}.exe ${PKG}
zip windows-amd64-${APP}.zip ${APP}.exe

echo "darwin/amd64"
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 \
    go build -ldflags "-s -w" -buildvcs=false -trimpath -o ${APP} ${PKG}
tar -czvf darwin-amd64-${APP}.tar.gz ${APP}

echo "darwin/arm64"
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 \
    go build -ldflags "-s -w" -buildvcs=false -trimpath -o ${APP} ${PKG}
tar -czvf darwin-arm64-${APP}.tar.gz ${APP}

# Remove intermediate binaries.
rm -f ${APP} ${APP}.exe

echo "Done!"
ls -lh *-${APP}.tar.gz *-${APP}.zip
