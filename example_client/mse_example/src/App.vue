<template>
  <div id="app" >
    <p>MSE player</p>
    <div>
      <MSEPlayer schema="ws" server="localhost" :port="8090" suuid="13bf3e5c-d537-4f10-98e8-9d0afca8b070" :verbose="false" />
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
