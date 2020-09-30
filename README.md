[![GoDoc](https://godoc.org/github.com/LdDl/video-server?status.svg)](https://godoc.org/github.com/LdDl/video-server)
[![Sourcegraph](https://sourcegraph.com/github.com/LdDl/video-server/-/badge.svg)](https://sourcegraph.com/github.com/LdDl/video-server?badge)
[![Go Report Card](https://goreportcard.com/badge/github.com/LdDl/video-server)](https://goreportcard.com/report/github.com/LdDl/video-server)
[![GitHub tag](https://img.shields.io/github/tag/LdDl/video-server.svg)](https://github.com/LdDl/video-server/releases)

# Golang-based video-server for re-streaming RTSP to HLS/MSE

## Table of Contents

- [About](#about)
- [Installation](#installation)
    - [Binaries](#binaries)
    - [From source](#from-source)
- [Usage](#usage)
    - [Server](#start-server)
    - [Client](#test-client-server)
- [Dependencies](#dependencies)
- [License](#license)
- [Developers](#developers)


## About
Simple WS/HTTP server for re-streaming video (RTSP) to client in MSE/HLS format.

## Instalation
### Binaries
Linux - [link](https://github.com/LdDl/video-server/releases/download/v0.1.0/linux-video_server.tar.xz)

Windows - [link](https://github.com/LdDl/video-server/releases/download/v0.1.0/windows-video_server.zip)

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
npm run dev
```

You will se something like this after succesfull fron-end start:
```shell
DONE  Compiled successfully in 1783ms                                                                                                                                                                         12:09:30 PM
App running at:
- Local:   http://localhost:8080/ 
```
Paste link to the browser and check if video loaded successfully.

## Dependencies
GIN web-framework - [https://github.com/gin-gonic/gin](https://github.com/gin-gonic/gin). License is [MIT](https://github.com/gin-gonic/gin/blob/master/LICENSE)

Media library - [github.com/morozka/vdk](https://github.com/morozka/vdk). License is [MIT](https://github.com/morozka/vdk/blob/master/LICENSE)

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