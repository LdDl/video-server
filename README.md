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
Linux - [link](https://github.com/LdDl/video-server/releases/download/v0.4.0/linux-video_server.tar.gz)

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
    Path to configuration JSON-file (default "conf.json")
-cpuprofile file
    write cpu profile to file
-memprofile file
    write memory profile to file
```

### Start server
Prepare configuration file (example [here](cmd/video_server/conf.json)). Then run binary:
```shell
video_server --conf=conf.json
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

You can configure application to write MP4 chunks of custom duration (but not less than first keyframe duration) to the filesystem or [S3 MinIO](https://min.io/)

- For storing archive to the filesystem. Point default directory for storing MP4 files and duration:
  ```json
  "archive": {
      "enabled": true,
      "directory": "./mp4",
      "ms_per_file": 30000
  }
  ```
  For each stream configuration you can override default directory and duration. Field "type" should have value "filesystem":
  ```json
  {
    ///
    // Some other single stream props...
    ///
    "archive": {
        "enabled": true,
        "ms_per_file": 20000,
        "type": "filesystem",
        "directory": "custom_folder"
    }
  }
  ```
- For storing archive to the S3 MinIO:
  Modify configuration file to have both filesystem and minio configuration (filesystem will be picked for storing temporary files before moving it to the MinIO), e.g.:
  ```json
  "archive": {
      "directory": "./mp4",
      "ms_per_file": 30000,
      "minio_settings": {
          "host": "localhost",
          "port": 29199,
          "user": "minio_secret_login",
          "password": "minio_secret_password",
          "default_bucket": "archive_bucket",
          "default_path": "/var/archive_data"
      }
  }
  ```
  For each stream configuration you can override default directory for temporary files, MinIO bucket and path in it and chunk duration. Field "type" should have value "minio":
  ```json
  {
    ///
    // Some other single stream props...
    ///
    "archive": {
        "enabled": true,
        "ms_per_file": 20000,
        "type": "filesystem",
        "directory": "custom_folder",
        "type": "minio",
        "minio_bucket": "vod-bucket",
        "minio_path": "/var/archive_data_custom"
    }
  }
  ```

- If you want disable archive for specified stream, just set value of the field `enabled` to `false` in streams array. For disabling archive at all you can do the same but in the main configuration (where default values are set)

- To install MinIO (in case if you want to store archive in S3) you can use [./docker-compose.yaml](docker-compose file) or [./scripts/minio-ansible.yml](Ansible script) for example of deployment workflows

## Dependencies
GIN web-framework - [https://github.com/gin-gonic/gin](https://github.com/gin-gonic/gin). License is [MIT](https://github.com/gin-gonic/gin/blob/master/LICENSE)

Media library - [http://github.com/deepch/vdk](https://github.com/deepch/vdk). License is [MIT](https://github.com/deepch/vdk/blob/master/LICENSE).

UUID generation and parsing - [https://github.com/google/uuid](https://github.com/google/uuid). License is [BSD 3-Clause](https://github.com/google/uuid/blob/master/LICENSE)

Websockets - [https://github.com/gorilla/websocket](https://github.com/gorilla/websocket). License is [BSD 2-Clause](https://github.com/gorilla/websocket/blob/master/LICENSE)

m3u8 library - [https://github.com/grafov/m3u8](https://github.com/grafov/m3u8). License is [BSD 3-Clause](https://github.com/grafov/m3u8/blob/master/LICENSE)

errors wrapping - [https://github.com/pkg/errors](https://github.com/pkg/errors) . License is [BSD 2-Clause](https://github.com/pkg/errors/blob/master/LICENSE)

## License
You can check it [here](LICENSE.md)

## Developers
Roman - https://github.com/webver

Pavel - https://github.com/Pavel7824

Dimitrii Lopanov - https://github.com/LdDl

Morozka - https://github.com/morozka
