#!/bin/sh
set -e

APP=video_server
REGISTRY=${DOCKER_REGISTRY:-docker.io}
TAG=${DOCKER_TAG:-latest}

if [ -z "$DOCKER_REGISTRY_USERNAME" ]; then
    echo "DOCKER_REGISTRY_USERNAME is not set" >&2
    exit 1
fi

echo "$DOCKER_REGISTRY_PASSWORD" | docker login "$REGISTRY" -u "$DOCKER_REGISTRY_USERNAME" --password-stdin
DOCKER_BUILDKIT=1 docker build -t ${APP} -f Dockerfile .
docker tag ${APP} "$REGISTRY/$DOCKER_REGISTRY_USERNAME/${APP}:${TAG}"
docker push "$REGISTRY/$DOCKER_REGISTRY_USERNAME/${APP}:${TAG}"
