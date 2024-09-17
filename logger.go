package videoserver

const (
	SCOPE_APP        = "app"
	SCOPE_STREAM     = "stream"
	SCOPE_STREAMING  = "streaming"
	SCOPE_WS_HANDLER = "ws_handler"
	SCOPE_API_SERVER = "api_server"
	SCOPE_WS_SERVER  = "ws_server"
	SCOPE_ARCHIVE    = "archive"
	SCOPE_MP4        = "mp4"

	EVENT_APP_CORS_CONFIG = "app_cors_config"

	EVENT_STREAM_CODEC_ADD     = "stream_codec_add"
	EVENT_STREAM_STATUS_UPDATE = "stream_status_update"
	EVENT_STREAM_CLIENT_ADD    = "stream_client_add"
	EVENT_STREAM_CLIENT_DELETE = "stream_client_delete"
	EVENT_STREAM_CAST_PACKET   = "stream_cast"

	EVENT_STREAMING_RUN     = "streaming_run"
	EVENT_STREAMING_START   = "streaming_start"
	EVENT_STREAMING_DONE    = "streaming_done"
	EVENT_STREAMING_RESTART = "streaming_restart"

	EVENT_API_PREPARE     = "api_server_prepare"
	EVENT_API_START       = "api_server_start"
	EVENT_API_CORS_ENABLE = "api_server_cors_enable"
	EVENT_API_REQUEST     = "api_request"

	EVENT_WS_PREPARE     = "ws_server_prepare"
	EVENT_WS_START       = "ws_server_start"
	EVENT_WS_CORS_ENABLE = "ws_server_cors_enable"
	EVENT_WS_REQUEST     = "ws_request"
	EVENT_WS_UPGRADER    = "ws_upgrader"

	EVENT_ARCHIVE_CREATE_FILE = "archive_create_file"
	EVENT_ARCHIVE_CLOSE_FILE  = "archive_close_file"
	EVENT_MP4_WRITE           = "mp4_write"
	EVENT_MP4_WRITE_TRAIL     = "mp4_write_trail"
	EVENT_MP4_SAVE_MINIO      = "mp4_save_minio"
	EVENT_MP4_CLOSE           = "mp4_close"
)
