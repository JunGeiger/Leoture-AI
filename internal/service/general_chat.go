package service

import (
	"context"
	"fmt"
	"leoture/internal/components/indexer"
	"leoture/internal/components/memory"
	"leoture/internal/compose/general_chat"
	"leoture/internal/config"
	"leoture/internal/types"
	"log/slog"

	"github.com/cloudwego/eino-ext/callbacks/langfuse"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/qdrant/go-client/qdrant"

	components_qdrant "github.com/cloudwego/eino-ext/components/retriever/qdrant"
)

type GeneralChatService interface {
	GeneralChatStream(ctx context.Context, req *types.ChatRequest) (*schema.StreamReader[*schema.Message], error)
	GeneralChatInvoke(ctx context.Context, req *types.ChatRequest) (*schema.Message, error)
	GeneralChatSaveDialogueMemory(ctx context.Context, req *types.ChatRequest, msg *schema.Message)
	assemblyData(req *types.ChatRequest) (*types.UseCaseParams, *config.ChatModelPromptParams, error)
}

type generalChatService struct {
	chatGraphMap    map[string]compose.Runnable[*types.UseCaseParams, *schema.Message]
	dialogueMemory  compose.Runnable[*types.UseCaseAssistantResponse, []string]
	callbackHandler callbacks.Handler
}

func NewGeneralChatService(chatGraphMap map[string]compose.Runnable[*types.UseCaseParams, *schema.Message],
	dialogueMemory compose.Runnable[*types.UseCaseAssistantResponse, []string],
	callbackHandler callbacks.Handler) GeneralChatService {
	return &generalChatService{
		chatGraphMap:    chatGraphMap,
		dialogueMemory:  dialogueMemory,
		callbackHandler: callbackHandler,
	}
}

func (s *generalChatService) GeneralChatInvoke(ctx context.Context, req *types.ChatRequest) (*schema.Message, error) {
	in, presetValue, err := s.assemblyData(req)
	if err != nil {
		return nil, err
	}
	var resultMessage *schema.Message
	runnable := s.chatGraphMap[presetValue.ModelParams.Provider]

	if s.callbackHandler != nil {
		ctx = newGeneralChatObserveTags(ctx, in, req, "completion")
	}

	var topK int = 3
	// 已提issure和pr，暂时不起作用，需要在components/retriever中统一设置
	var scoreThreshold float64 = 0.7
	filter := newDialogueRecordFilter(req)
	resultMessage, err = runnable.Invoke(ctx, in,
		compose.WithChatModelOption(
			model.WithModel(presetValue.ModelParams.Model),
			model.WithTemperature(*presetValue.ModelParams.Temperature),
			model.WithTopP(*presetValue.ModelParams.TopP),
			model.WithMaxTokens(*presetValue.ModelParams.MaxCompletionTokens),
			openai.WithExtraFields(*presetValue.ModelParams.ExtraParams)),
		compose.WithRetrieverOption(
			retriever.WithScoreThreshold(scoreThreshold),
			retriever.WithTopK(topK),
			components_qdrant.WithFilter(filter)),
	)

	if err != nil {
		return nil, err
	}
	return resultMessage, nil
}

func (s *generalChatService) GeneralChatStream(ctx context.Context, req *types.ChatRequest) (*schema.StreamReader[*schema.Message], error) {
	in, presetValue, err := s.assemblyData(req)
	if err != nil {
		return nil, err
	}

	var resultMessage *schema.StreamReader[*schema.Message]
	runnable := s.chatGraphMap[presetValue.ModelParams.Provider]

	if s.callbackHandler != nil {
		ctx = newGeneralChatObserveTags(ctx, in, req, "streaming")
	}

	var topK int = 3
	// 已提issure和pr，暂时不起作用，需要在components/retriever中统一设置
	var scoreThreshold float64 = 0.7
	filter := newDialogueRecordFilter(req)
	resultMessage, err = runnable.Stream(ctx, in, //compose.WithCallbacks(s.callbackHandler),
		compose.WithChatModelOption(
			model.WithModel(presetValue.ModelParams.Model),
			model.WithTemperature(*presetValue.ModelParams.Temperature),
			model.WithTopP(*presetValue.ModelParams.TopP),
			model.WithMaxTokens(*presetValue.ModelParams.MaxCompletionTokens),
			openai.WithExtraFields(*presetValue.ModelParams.ExtraParams)),
		compose.WithRetrieverOption(
			retriever.WithScoreThreshold(scoreThreshold),
			retriever.WithTopK(topK),
			components_qdrant.WithFilter(filter)))

	if err != nil {
		return nil, err
	}
	return resultMessage, nil
}

func (s *generalChatService) assemblyData(req *types.ChatRequest) (*types.UseCaseParams, *config.ChatModelPromptParams, error) {
	chatModelPrompts := config.GetPropmptMap()
	chatModelPrompt, ok := chatModelPrompts[req.UseCase]

	if !ok || !chatModelPrompt.Enabled {
		return nil, nil, fmt.Errorf("GeneralChat: usecase ChatModelPrompt %s not found or disabled", req.UseCase)
	}

	// 预设系统提示词
	values, ok := chatModelPrompt.PresetValues[chatModelPrompt.PromptVersion]
	if !ok {
		return nil, nil, fmt.Errorf("GeneralChat: prompt_preset version %s not found", chatModelPrompt.PromptVersion)
	}

	if values == nil {
		return nil, nil, fmt.Errorf("GeneralChat: ChatModelPrompt not match to version, UseCase: %s, PromptVersion: %s", req.UseCase, chatModelPrompt.PromptVersion)
	}

	in := &types.UseCaseParams{
		ChatRequest:     req,
		ChatModelPrompt: chatModelPrompt,
	}
	return in, values, nil
}

func (s *generalChatService) GeneralChatSaveDialogueMemory(ctx context.Context, req *types.ChatRequest, msg *schema.Message) {
	if s.callbackHandler != nil {
		tags := []string{
			general_chat.CollectionDialogueMemory,
			req.DialogueID,
		}
		ctx = langfuse.SetTrace(ctx,
			langfuse.WithName(req.UseCase),
			langfuse.WithUserID(req.UserID),
			langfuse.WithSessionID(req.SessionID),
			langfuse.WithTags(tags...),
		)
	}

	_, err := s.dialogueMemory.Invoke(ctx, &types.UseCaseAssistantResponse{
		UseCase:    req.UseCase,
		UserID:     req.UserID,
		SessionID:  req.SessionID,
		DialogueID: req.DialogueID,
		Task:       req.Task,
		Message:    msg,
	})
	if err != nil {
		slog.Error("GeneralChatSaveDialogueMemory failed", slog.String("error", err.Error()))
	}
	return
}

func newDialogueRecordFilter(req *types.ChatRequest) *qdrant.Filter {
	return &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatch(indexer.QdrantMetaData+"."+memory.DialogueUseCaseKey, req.UseCase),
			qdrant.NewMatch(indexer.QdrantMetaData+"."+memory.DialogueUserIDKey, req.UserID),
			qdrant.NewMatch(indexer.QdrantMetaData+"."+memory.DialogueSessionIDKey, req.SessionID),
		},
	}
}

func newGeneralChatObserveTags(ctx context.Context, in *types.UseCaseParams, req *types.ChatRequest, t string) context.Context {
	tags := in.ChatModelPrompt.Tags
	tags = append(tags, req.DialogueID)
	tags = append(tags, t)
	return langfuse.SetTrace(ctx,
		langfuse.WithName(req.UseCase),
		langfuse.WithUserID(req.UserID),
		langfuse.WithInput(req.Task),
		langfuse.WithSessionID(req.SessionID),
		langfuse.WithRelease(in.ChatModelPrompt.PromptVersion),
		langfuse.WithTags(tags...))
}
