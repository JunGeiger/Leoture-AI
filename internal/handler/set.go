package handler

import "leoture/internal/service"

type Set struct {
	GeneralChatHandler *GeneralChatHandler
}

func NewSet(set *service.Set) *Set {
	return &Set{
		GeneralChatHandler: newGeneralChatHandler(set.GeneralChatService),
	}
}
