[![GoDoc](https://godoc.org/github.com/LdDl/video-server?status.svg)](https://godoc.org/github.com/LdDl/video-server)
[![Sourcegraph](https://sourcegraph.com/github.com/LdDl/video-server/-/badge.svg)](https://sourcegraph.com/github.com/LdDl/video-server?badge)
[![Go Report Card](https://goreportcard.com/badge/github.com/LdDl/video-server)](https://goreportcard.com/report/github.com/LdDl/video-server)
[![GitHub tag](https://img.shields.io/github/tag/LdDl/video-server.svg)](https://github.com/LdDl/video-server/releases)

# Golang-based video-server for re-streaming RTSP to HLS/MSE

## Table of Contents

- [Golang-based video-server for re-streaming RTSP to HLS/MSE](#golang-based-video-server-for-re-streaming-rtsp-to-hlsmse)
  - [Table of Contents](#table-of-contents)
  - [About](#about)
  - [Installation](#installation)
    - [Pre-built binaries](#pre-built-binaries)
    - [From source](#from-source)
  - [Usage](#usage)
    - [Start server](#start-server)
    - [Test Client-Server](#test-client-server)
  - [Docker](#docker)
    - [Docker Compose](#docker-compose)
  - [On-demand streams](#on-demand-streams)
    - [Health check](#health-check)
    - [Status fields](#status-fields)
  - [Archive](#archive)
    - [Recording vs Serving](#recording-vs-serving)
    - [Global Configuration](#global-configuration)
    - [Per-Stream Configuration](#per-stream-configuration)
    - [Configuration Fallback](#configuration-fallback)
    - [MinIO Configuration](#minio-configuration)
    - [Archive REST API](#archive-rest-api)
  - [Dependencies](#dependencies)
  - [License](#license)
  - [Developers](#developers)


## About
Simple WS/HTTP server for re-streaming video (RTSP) to client in MSE/HLS format.

It is highly inspired by https://github.com/deepch and his projects. So why am I trying to reinvent the wheel? Well, I'm just trying to fit my needs.

## Installation
### Pre-built binaries

Download the archive for your platform from the [latest release](https://github.com/LdDl/video-server/releases/latest):

| Platform | Archive |
| --- | --- |
| Linux amd64 | [linux-amd64-video_server.tar.gz](https://github.com/LdDl/video-server/releases/latest/download/linux-amd64-video_server.tar.gz) |
| Linux arm64 | [linux-arm64-video_server.tar.gz](https://github.com/LdDl/video-server/releases/latest/download/linux-arm64-video_server.tar.gz) |
| macOS amd64 | [darwin-amd64-video_server.tar.gz](https://github.com/LdDl/video-server/releases/latest/download/darwin-amd64-video_server.tar.gz) |
| macOS arm64 | [darwin-arm64-video_server.tar.gz](https://github.com/LdDl/video-server/releases/latest/download/darwin-arm64-video_server.tar.gz) |
| Windows amd64 | [windows-amd64-video_server.zip](https://github.com/LdDl/video-server/releases/latest/download/windows-amd64-video_server.zip) |

Each archive contains `video_server` or `video_server.exe`. Extract it into a directory in your `PATH`.

Quick installation on Linux amd64:

```bash
curl -fsSL https://github.com/LdDl/video-server/releases/latest/download/linux-amd64-video_server.tar.gz \
  | sudo tar -xz -C /usr/local/bin video_server
```

For Linux arm64, replace `linux-amd64` with `linux-arm64`. On macOS, use `darwin-amd64` for Intel or `darwin-arm64` for Apple Silicon.

If you would rather run the server in a container, see [Docker](#docker).

### From source

```bash
go install github.com/LdDl/video-server/cmd/video_server@latest
```

Or clone the repository and build it yourself:

```bash
git clone https://github.com/LdDl/video-server.git
cd video-server
go build -o video_server ./cmd/video_server
```

## Usage
```shell
video_server -h
```
```shell
-conf string
    Path to configuration either TOML-file or JSON-file (default "conf.toml")
-cpuprofile file
    write cpu profile to file
-memprofile file
    write memory profile to file
```

### Start server
Prepare configuration file (example [here](cmd/video_server/conf.json)). Then run binary:
```shell
video_server --conf=conf.toml
```
### Test Client-Server
For HLS-based player go to [hls-subdirectory](example_client/hls_example).

For MSE-based (websockets) player go to [mse-subdirectory](mse_example/hls_example).

Then follow this set of commands:
```shell
npm install
export NODE_OPTIONS=--openssl-legacy-provider
npm run dev
```

You will se something like this after succesfull fron-end start:
```shell
DONE  Compiled successfully in 1783ms                                                                                                                                                                         12:09:30 PM
App running at:
- Local:   http://localhost:8080/ 
```
Paste link to the browser and check if video loaded successfully.

## Docker

The image is published as [dimahkiin/video_server](https://hub.docker.com/r/dimahkiin/video_server).

The server does not read environment variables, so a configuration file has to be mounted into the container. Both servers must listen on `0.0.0.0` inside it, otherwise the published ports stay unreachable:

```toml
[api]
host = "0.0.0.0"
port = 8091

[video]
host = "0.0.0.0"
port = 8090
```

Start the latest image with that configuration file:

```bash
docker run --rm --name video_server --stop-timeout 30 \
  -p 127.0.0.1:8090:8090 -p 127.0.0.1:8091:8091 \
  --mount type=bind,source="$(pwd)/conf.toml",target=/app/conf.toml,readonly \
  -v video_server_hls:/app/hls \
  -v video_server_mp4:/app/mp4 \
  docker.io/dimahkiin/video_server:latest -conf /app/conf.toml
```

Port 8090 serves MSE websockets and HLS files, port 8091 serves the REST API. To accept connections from other hosts, drop the `127.0.0.1:` prefix from the port mappings. The named volumes keep HLS segments and archive files across container recreation; skip them when neither HLS nor recording is enabled.

### Docker Compose

From the repository root, use the published image with the example configuration:

```bash
VIDEO_SERVER_IMAGE=dimahkiin/video_server:latest \
docker compose up --no-build --pull always
```

`VIDEO_SERVER_CONFIG` selects the configuration file (use an absolute path for a file outside the repository), `VIDEO_SERVER_PORT` and `VIDEO_SERVER_API_PORT` change the published host ports, and `VIDEO_SERVER_BIND_HOST` changes the host bind address. The `video_server_hls` and `video_server_mp4` volumes survive container recreation.

```bash
VIDEO_SERVER_IMAGE=dimahkiin/video_server:latest \
VIDEO_SERVER_CONFIG=/etc/video_server/conf.toml \
VIDEO_SERVER_BIND_HOST=0.0.0.0 \
docker compose up -d --no-build --pull always
```

The [docker-compose.yaml](docker-compose.yaml) file is unrelated to the server itself: it starts a MinIO instance for the archive, see [MinIO Configuration](#minio-configuration).

## On-demand streams

By default every configured stream is pulled from its source all the time, whether anybody is watching or not. For a fleet of cameras that nobody looks at most of the day this is a lot of wasted inbound traffic and CPU.

With on-demand mode the upstream connection (RTSP session or local file reader) is opened when the first viewer arrives and closed a little while after the last one leaves:

- **MSE**: the WebSocket connection is the viewer. The first connection dials the source, the last disconnect arms the idle timer.
- **HLS**: there is no persistent connection, so every playlist/segment request counts as activity. The stream stays alive while requests keep coming. The very first playlist request of a cold stream waits for the first segment to be cut (about two `ms_per_segment`), so expect a start-up delay of that order.
- **Recording**: a stream with `archive.recording = true` is never released, otherwise the archive would have gaps.

```toml
[on_demand]
# default for all streams
enabled = true
# keep the upstream alive this long after the last viewer
idle_ms = 30000
# probe idle sources so that `online` stays meaningful
health_check = true
health_interval_ms = 30000
health_timeout_ms = 3000

[[rtsp_streams]]
guid = "..."
url = "rtsp://..."
output_types = ["mse"]
# per-stream override: this one is always on
on_demand = false
```

The first viewer of a cold stream waits for the source to answer (typically 1-3 seconds for an RTSP camera) before video starts.

### Health check

An idle on-demand stream is not connected, so the server would not know whether the camera is alive. When `health_check` is enabled, idle sources are probed every `health_interval_ms` with a plain RTSP `DESCRIBE` (Basic and Digest auth are supported). No `PLAY` is sent, so the camera never starts streaming media for a probe. Local-file sources are checked for file existence. Streams with a live upstream are not probed: the session itself is the health signal.

### Status fields

`GET /status` reports, per stream:

- `on_demand` - whether the stream is lazily connected
- `online` - last known reachability of the source (from the live session or from the health probe)
- `streaming` - whether the upstream loop is running right now
- `viewers` - number of attached MSE clients
- `last_health_check`, `last_health_error` - when the source was last checked and what went wrong, if anything
- `status` - kept for compatibility: `true` while a live session has codec data

`online = true, streaming = false, viewers = 0` is the normal state of a healthy camera that nobody is watching.

## Archive

You can configure application to write MP4 chunks of custom duration (but not less than first keyframe duration) to the filesystem or [S3 MinIO](https://min.io/). The archive system supports two independent modes: **recording** (writing new segments) and **serving** (playback of existing archive files).

### Recording vs Serving

The archive configuration has two separate flags:

- **`recording`** - Enables writing new archive segments. When `true`, the server will continuously record video stream chunks to storage.
- **`serving`** - Enables playback of existing archive files via WebSocket. When `true`, clients can request archive playback even if recording is disabled.

This allows flexible deployment scenarios:
- `recording = true, serving = true` - Full archive functionality (record and playback)
- `recording = true, serving = false` - Record only, no playback API
- `recording = false, serving = true` - Playback only from existing archive files (useful for read-only archive access)
- `recording = false, serving = false` - Archive completely disabled

### Global Configuration

Global archive settings provide defaults for all streams:

```toml
[archive]
recording = true
serving = true
directory = "./mp4"
ms_per_file = 30000
```

### Per-Stream Configuration

Each stream can override global settings. Per-stream settings take precedence over global configuration.

For filesystem storage:
```toml
[[rtsp_streams]]
# ...
# Some other single stream props
# ...
archive = { recording = true, ms_per_file = 20000, type = "filesystem", directory = "custom_folder" }
```

For S3 MinIO storage:
```toml
[[rtsp_streams]]
# ...
# Some other single stream props
# ...
archive = { recording = true, ms_per_file = 20000, type = "minio", directory = "custom_folder", minio_bucket = "vod-bucket", minio_path = "/var/archive_data_custom" }
```

### Configuration Fallback

When a client requests archive playback:
1. The server first checks per-stream archive configuration
2. If per-stream storage is not available, it falls back to global archive settings
3. Global `serving` flag must be `true` for fallback to work

This allows you to:
- Record with per-stream settings but serve using global directory
- Serve pre-existing archive files without per-stream configuration

### MinIO Configuration

For storing archive to S3 MinIO, configure both filesystem (for temporary files) and MinIO settings:

```toml
[archive]
recording = true
serving = true
directory = "./mp4"
ms_per_file = 30000
minio_settings = { host = "localhost", port = 29199, user = "minio_secret_login", password = "minio_secret_password", default_bucket = "archive-bucket", default_path = "/var/archive_data" }
```

To install MinIO you can use [./docker-compose.yaml](docker-compose.yaml) or [./scripts/minio-ansible.yml](Ansible script) for example of deployment workflows.

### Archive REST API

The server provides a REST API endpoint to query available archive time ranges:

```
GET /archive/:stream_id/ranges
```

Response example:
```json
{
  "stream_id": "0742091c-19cd-4658-9b4f-5320da160f45",
  "ranges": [
    {
      "start": "2025-12-20T10:00:00Z",
      "end": "2025-12-20T12:30:00Z"
    },
    {
      "start": "2025-12-20T14:00:00Z",
      "end": "2025-12-20T18:00:00Z"
    }
  ]
}
```

## Dependencies
GIN web-framework - [https://github.com/gin-gonic/gin](https://github.com/gin-gonic/gin). License is [MIT](https://github.com/gin-gonic/gin/blob/master/LICENSE)

Media library - [http://github.com/deepch/vdk](https://github.com/deepch/vdk). License is [MIT](https://github.com/deepch/vdk/blob/master/LICENSE).

UUID generation and parsing - [https://github.com/google/uuid](https://github.com/google/uuid). License is [BSD 3-Clause](https://github.com/google/uuid/blob/master/LICENSE)

Websockets - [https://github.com/gorilla/websocket](https://github.com/gorilla/websocket). License is [BSD 2-Clause](https://github.com/gorilla/websocket/blob/master/LICENSE)

m3u8 library - [https://github.com/grafov/m3u8](https://github.com/grafov/m3u8). License is [BSD 3-Clause](https://github.com/grafov/m3u8/blob/master/LICENSE)

Working with mp4 files - [https://github.com/Eyevinn/mp4ff](https://github.com/Eyevinn/mp4ff). License is [MIT](https://github.com/Eyevinn/mp4ff?tab=MIT-1-ov-file#readme)

errors wrapping - [https://github.com/pkg/errors](https://github.com/pkg/errors) . License is [BSD 2-Clause](https://github.com/pkg/errors/blob/master/LICENSE)

## License
You can check it [here](LICENSE.md)

## Developers
Roman - https://github.com/webver

Pavel - https://github.com/Pavel7824

Dimitrii Lopanov - https://github.com/LdDl

Morozka - https://github.com/morozka
