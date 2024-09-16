package videoserver

const (
	SCOPE_STREAM     = "stream"
	SCOPE_WS_HANDLER = "ws_handler"
	SCOPE_API_SERVER = "api_server"
	SCOPE_WS_SERVER  = "ws_server"

	EVENT_API_PREPARE     = "api_server_prepare"
	EVENT_API_START       = "api_server_start"
	EVENT_API_CORS_ENABLE = "api_server_cors_enable"
	EVENT_API_REQUEST     = "api_request"

	EVENT_WS_PREPARE     = "ws_server_prepare"
	EVENT_WS_START       = "ws_server_start"
	EVENT_WS_CORS_ENABLE = "ws_server_cors_enable"
	EVENT_WS_REQUEST     = "ws_request"
	EVENT_WS_UPGRADER    = "ws_upgrader"
)
