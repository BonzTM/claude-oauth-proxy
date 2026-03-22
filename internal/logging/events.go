package logging

const (
	OperationAuthPrepareLogin      = "auth.prepare_login"
	OperationAuthCompleteLogin     = "auth.complete_login"
	OperationAuthStatus            = "auth.status"
	OperationAuthLogout            = "auth.logout"
	OperationAuthAccessToken       = "auth.access_token"
	OperationProviderModels        = "provider.models"
	OperationProviderChat          = "provider.chat"
	OperationProviderChatStreaming = "provider.chat_streaming"
)

const (
	EventServiceOperationStart  = "service.operation.start"
	EventServiceOperationFinish = "service.operation.finish"

	EventCLICommandStart  = "cli.command.start"
	EventCLICommandFinish = "cli.command.finish"

	EventHTTPRequestStart  = "http.request.start"
	EventHTTPRequestFinish = "http.request.finish"
)
