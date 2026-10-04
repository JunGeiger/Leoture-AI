package types

const (
	ApiRequestIDKey = "X-Request-ID"
)

type ChatRequest struct {
	UseCase    string `json:"useCase,required"`
	UserID     string `json:"userID,required"`
	SessionID  string `json:"sessionID,required"`
	DialogueID string `json:"dialogueID"`

	EnabledStream bool `json:"enabledStream,required"`

	Task string `json:"task,required"`
	// 问答示例
	Example string `json:"example"`
	// 输入数据
	References []string `json:"references"`
	// 思考步骤、任务流程
	Instructions []string `json:"instructions"`
}
