<template>
  <div id="app" >
    <p>HLS player</p>
    <div>
      <HLSPlayer schema="http" server="localhost" :port="8090" suuid="a1b8e7cb-09e6-4f3a-b857-30c7b272a744" :verbose="false" />
    </div>
  </div>
</template>


<script>
import HLSPlayer from './components/HLSPlayer.vue'

export default {
  name: 'App',
  components: {
    HLSPlayer
  }
}
</script>

<style>
  #app {
    font-family: Avenir, Helvetica, Arial, sans-serif;
    -webkit-font-smoothing: antialiased;
    -moz-osx-font-smoothing: grayscale;
    text-align: center;
    color: #2c3e50;
    margin-top: 60px;
  }
</style>
