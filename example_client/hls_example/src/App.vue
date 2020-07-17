<template>
  <div id="app" >
    <p>HLS player</p>
    <div>
      <HLSPlayer schema="http" server="localhost" :port="8090" suuid="c2680f08-07cc-4dee-9231-685893e2661f" :verbose="false" />
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
