package types

import (
	"leoture/internal/config"

	"github.com/cloudwego/eino/components/prompt"
)

type DialogueRecord struct {
	ID     string
	Task   string
	Answer string
}

// ToMap 转换为 map，供 text/template 渲染使用
func ToUserMessageMap(cp *ChatRequest) map[string]any {
	return map[string]any{
		"Task":         cp.Task,
		"Example":      cp.Example,
		"References":   cp.References,
		"Instructions": cp.Instructions,
	}
}

// ToMap 转换为 map，供 text/template 渲染使用
func ToSystemMessageMap(cp config.PromptParams) map[string]any {
	return map[string]any{
		"Role":         cp.Role,
		"Context":      cp.Context,
		"Constraints":  cp.Constraints,
		"OutputFormat": cp.OutputFormat,
	}
}

// ToMap 转换为 map，供 text/template 渲染使用
func ToAssistantMessageMap(cp []*DialogueRecord) map[string]any {
	return map[string]any{"DialogueRecords": cp}
}

// UseCaseParams 场景化参数 + 通用请求参数
type UseCaseParams struct {
	ChatRequest              *ChatRequest
	ChatModelPrompt          *config.ChatModelPrompt
	DialogueMemory           []*DialogueRecord
	UserMessageTemplate      prompt.ChatTemplate
	SystemMessageTemplate    prompt.ChatTemplate
	AssistantMessageTemplate prompt.ChatTemplate
}
