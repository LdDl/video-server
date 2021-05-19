<template>
        <video ref="livestream" class="videosize" controls autoplay muted></video>
</template>

<script>
    module.exports = {
        name: 'video-player',
        props: {
            schema: {
                type: String,
                default: "ws"
            },
            server: {
                type: String,
                default: "localhost"
            },
            port: {
                type: Number,
                default: 8081
            },
            suuid: {
                type: String,
                default: ""
            },
            verbose: {
                type: Boolean,
                default: false
            }
        },
        data: function () {
            return {
                isPlaying: false,
                streamingStarted: false,
                ms: null,
                queue: [],
                ws: null,
                sourceBuffer: null,
            };
        },
        mounted() {
            this.initialize()
        },
        beforeDestroy() {
            this.stop()
        },
        methods: {
            initialize() {
                if ('MediaSource' in window) {
                    this.ms = new MediaSource()
                    let start = this.start;
                    this.ms.addEventListener('sourceopen', start, false);
                    this.$refs["livestream"].src = window.URL.createObjectURL(this.ms);
                    this.$refs["livestream"].onpause = () => {
                        console.log("The video has been paused");
                        this.stop();
                    };
                    this.$refs["livestream"].onplay = () => {
                        console.log("The video has been started");
                        if (this.isPlaying === false) {
                            start();
                        }
                    };

                } else {
                    console.error("Unsupported MSE");
                }
            },
            start() {
                this.isPlaying = true;
                this.ws = new WebSocket(this.schema + "://" + this.server + ":" + this.port + "/ws/live?suuid=" + this.suuid);
                this.ws.binaryType = "arraybuffer";
                this.ws.onopen = (event) => {
                    console.log('Socket opened', event);
                }
                this.ws.onclose = (event) => {
                    console.log('Socket closed', event);
                    if (this.isPlaying === true) {
                        setTimeout(() => {
                            // this.start();
                        }, 1000);
                    }
                }
                this.ws.onerror = (err) => {
                    console.error('Socket encountered error: ', err.message, 'Closing socket');
                    this.ws.close();
                };
                this.ws.onmessage = (event) => {
                    const data = new Uint8Array(event.data);
                    if (data[0] === 9) {
                        let decoded_arr = data.slice(1);
                        let mimeCodec;
                        if (window.TextDecoder) {
                            mimeCodec = new TextDecoder("utf-8").decode(decoded_arr);
                        } else {
                            //mimeCodec =Utf8ArrayToStr(decoded_arr);
                            mimeCodec = String.fromCharCode(decoded_arr)
                        }
                        if (this.verbose) {
                            console.log('first packet with codec data: ' + mimeCodec);
                        }
                        if (!this.sourceBuffer && this.$refs['livestream']) {
                            this.sourceBuffer = this.ms.addSourceBuffer('video/mp4; codecs="' + mimeCodec + '"');
                            this.sourceBuffer.mode = "segments"
                            this.sourceBuffer.addEventListener("updateend", this.loadPacket);
                        }
                    } else {
                        this.pushPacket(event.data);
                    }
                }
            },
            stop() {
                this.isPlaying = false;
                if (this.ws) {
                    this.ws.close();
                    if (this.$refs["livestream"] && this.sourceBuffer) {
                        this.sourceBuffer.abort();
                        if (this.$refs["livestream"].currentTime > 0) {
                            this.sourceBuffer.remove(0, this.$refs["livestream"].currentTime);
                        }
                        this.$refs["livestream"].currentTime = 0
                    }
                }
            },
            pushPacket(arr) {
                let view = new Uint8Array(arr);
                if (this.verbose) {
                    console.log("got", arr.byteLength, "bytes.  Values=", view[0], view[1], view[2], view[3], view[4]);
                }
                let data = arr;
                if (!this.streamingStarted && this.$refs['livestream']) {
                    this.sourceBuffer.appendBuffer(data);
                    this.streamingStarted = true;
                    return;
                }
                this.queue.push(data);
                if (this.verbose) {
                    console.log("queue push:", this.queue.length);
                }
                if (!this.sourceBuffer.updating) {
                    this.loadPacket();
                }
            },
            loadPacket() {
                if (!this.sourceBuffer.updating && this.$refs['livestream']) {
                    if (this.queue.length > 0) {
                        let inp = this.queue.shift();
                        if (this.verbose) {
                            console.log("queue PULL:", this.queue.length);
                        }
                        let view = new Uint8Array(inp);
                        if (this.verbose) {
                            console.log("writing buffer with", view[0], view[1], view[2], view[3], view[4]);
                        }
                        this.sourceBuffer.appendBuffer(inp);
                    } else {
                        this.streamingStarted = false;
                    }
                }
            }
        }
    };
</script>

<style scoped>
    .videosize {
        /*position: absolute;*/
        z-index: -1;
        top: 0;
        left: 0;
        width: 100%;
        height: 100%;
        object-fit: cover;
    }
</style>