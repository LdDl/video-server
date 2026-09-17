[![GoDoc](https://godoc.org/github.com/LdDl/video-server?status.svg)](https://godoc.org/github.com/LdDl/video-server)
[![Sourcegraph](https://sourcegraph.com/github.com/LdDl/video-server/-/badge.svg)](https://sourcegraph.com/github.com/LdDl/video-server?badge)
[![Go Report Card](https://goreportcard.com/badge/github.com/LdDl/video-server)](https://goreportcard.com/report/github.com/LdDl/video-server)
[![GitHub tag](https://img.shields.io/github/tag/LdDl/video-server.svg)](https://github.com/LdDl/video-server/releases)

# Golang-based video-server for re-streaming RTSP to HLS/MSE

## Table of Contents

- [Golang-based video-server for re-streaming RTSP to HLS/MSE](#golang-based-video-server-for-re-streaming-rtsp-to-hlsmse)
  - [Table of Contents](#table-of-contents)
  - [About](#about)
  - [Instalation](#instalation)
    - [Binaries](#binaries)
    - [From source](#from-source)
  - [Usage](#usage)
    - [Start server](#start-server)
    - [Test Client-Server](#test-client-server)
  - [Dependencies](#dependencies)
  - [License](#license)
  - [Developers](#developers)


## About
Simple WS/HTTP server for re-streaming video (RTSP) to client in MSE/HLS format.

It is highly inspired by https://github.com/deepch and his projects. So why am I trying to reinvent the wheel? Well, I'm just trying to fit my needs.

## Instalation
### Binaries
Linux - [link](https://github.com/LdDl/video-server/releases/download/v0.8.1/linux-amd64-video_server.tar.gz)

### From source
```bash
go get github.com/LdDl/video-server
# or just clone it
# git clone https://github.com/LdDl/video-server.git
```
Go to root folder of downloaded repository, move to cmd/video_server folder:
```bash
cd $CLONED_PATH/cmd/video_server
go build -o video_server main.go
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
