package types

import "github.com/cloudwego/eino/schema"

const (
	ResponseSuccess = "success"
	ResponseFailed  = "failed"
)

type UseCaseAssistantResponse struct {
	UseCase    string
	UserID     string
	SessionID  string
	DialogueID string
	Task       string
	Message    *schema.Message
}
