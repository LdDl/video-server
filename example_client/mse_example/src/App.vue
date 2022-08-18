<template>
  <div id="app" >
    <p>MSE player</p>
    <div>
      <MSEPlayer schema="ws" server="localhost" :port="8090" suuid="a1b8e7cb-09e6-4f3a-b857-30c7b272a744" :verbose="false" />
    </div>
  </div>
</template>


<script>
import MSEPlayer from './components/MSEPlayer.vue'

export default {
  name: 'App',
  components: {
    MSEPlayer
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
