<template>
        <video ref="livestream" class="videosize" controls></video>
</template>

<script>
    import Hls from 'hls.js';

    export default {
        name: 'hls-video-player',
        props: {
            schema: {
                type: String,
                default: "http"
            },
            server: {
                type: String,
                default: "localhost"
            },
            port: {
                type: Number,
                default: 36137
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
                hls: null,
                hlsLink: "",
                isPlaying: false,
                streamingStarted: false,
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
                this.hls = new Hls();
                this.hlsLink = `${this.schema}://${this.server}:${this.port}/hls/${this.suuid}.m3u8`;
                this.$refs["livestream"].onpause = () => {
                    console.log("The HLS video has been paused");
                    this.stop();
                };
                this.$refs["livestream"].onplay = () => {
                    // console.log("The HLS video has been started");
                    this.$nuxt.$bus.$emit("setCameraLoader", false)
                    if (this.isPlaying === false) {
                        this.start();
                    }
                };
                var self_hls = this.hls;
                var self_link = this.hlsLink;
                var self_video = this.$refs["livestream"];
                if (Hls.isSupported()) {
                    self_hls.attachMedia(this.$refs["livestream"]);
                    self_hls.on(Hls.Events.MEDIA_ATTACHED, function () {
                        console.log("video and hls.js are now bound together !");
                        self_hls.loadSource(self_link);
                        self_hls.on(Hls.Events.MANIFEST_PARSED, function (event, data) {
                            console.log("manifest loaded, found " + data.levels.length + " quality level");
                            self_video.play();
                        });
                    });
                } else {
                    self_video.src = this.hlsLink;
                    if (/iPhone|iPod/.test(navigator.userAgent)) {
                        self_video.autoplay = true;
                        self_video.muted = true;
                    }
                    self_video.addEventListener('loadedmetadata', function() {
                        self_video.play();
                    }, false);
                }
            },
            start() {
                this.isPlaying = true;
            },
            stop() {
                this.isPlaying = false;
                this.hls.destroy();
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