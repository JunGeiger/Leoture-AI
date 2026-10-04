package service

import (
	"leoture/internal/config"
	"leoture/internal/types"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type Set struct {
	GeneralChatService GeneralChatService
}

func NewSet(cfg *config.Config,
	chatGraphMap map[string]compose.Runnable[*types.UseCaseParams, *schema.Message],
	dialogueMemory compose.Runnable[*types.UseCaseAssistantResponse, []string],
	callbackHandler callbacks.Handler) *Set {
	return &Set{
		GeneralChatService: NewGeneralChatService(chatGraphMap, dialogueMemory, callbackHandler),
	}
}
