package general_chat

import (
	"context"
	"errors"
	"fmt"
	component_indexer "leoture/internal/components/indexer"
	component_memory "leoture/internal/components/memory"
	component_model "leoture/internal/components/model"
	component_prompt "leoture/internal/components/prompt"
	"leoture/internal/types"
	"leoture/internal/utils"
	"log/slog"

	qdrant_indexer "github.com/cloudwego/eino-ext/components/indexer/qdrant"
	qdrant_retriever "github.com/cloudwego/eino-ext/components/retriever/qdrant"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/qdrant/go-client/qdrant"
)

const (
	// qdrant collection name
	CollectionDialogueMemory = "dialogue_memory"

	// graph node name
	NodeRequestVerify        = "requestVerify"
	NodeRetrieverMemoryPre   = "retrieverMemoryPre"
	NodeRetrieverMemory      = "retrieverMemory"
	NodeRetrieverMemoryAfter = "retrieverMemoryAfter"
	NodeRedisMemory          = "redisMemory"
	NodeDialogueAssemble     = "dialogueAssemble"
	NodeMessageAssemble      = "messageAssemble"
	NodeChatModel            = "chatModel"

	// chain name
	ChainDialogueSave = "dialogueSave"
)

// NewGeneralChatGraph 通用聊天场景
//
//	支持对话记忆存储和检索
func NewGeneralChatGraph(ctx context.Context,
	cmp map[string]*openai.ChatModel,
	drm *component_memory.DialogueRecordMemory,
	retrieverMap map[string]*qdrant_retriever.Retriever,
) (map[string]compose.Runnable[*types.UseCaseParams, *schema.Message], error) {

	chainMap := make(map[string]compose.Runnable[*types.UseCaseParams, *schema.Message], len(cmp))
	rti, ok := retrieverMap[CollectionDialogueMemory]
	if !ok {
		return nil, fmt.Errorf("NewGeneralChatGraph: Qdrant Collection %s not not found", CollectionDialogueMemory)
	}

	// 不同 llm api 供应商对应不同实例
	for provider, m := range cmp {
		runnable, err := newProviderModelGraph(ctx, provider, m, drm, rti)
		if err != nil {
			return nil, err
		}
		chainMap[provider] = runnable
	}

	return chainMap, nil
}

func newProviderModelGraph(ctx context.Context,
	modelProvider string,
	instanceModel *openai.ChatModel,
	drm *component_memory.DialogueRecordMemory,
	rti *qdrant_retriever.Retriever,
) (compose.Runnable[*types.UseCaseParams, *schema.Message], error) {
	g := compose.NewGraph[*types.UseCaseParams, *schema.Message]()

	// 前置校验、获取提示词模板
	g.AddLambdaNode(NodeRequestVerify, compose.InvokableLambda(
		func(ctx context.Context, in *types.UseCaseParams) (*types.UseCaseParams, error) {
			if in.ChatRequest == nil {
				return nil, errors.New("GeneralChatGraph: ChatRequest can not be nil")
			}
			if in.ChatRequest.Task == "" {
				return nil, errors.New("GeneralChatGraph: task cannot be empty")
			}
			if in.ChatRequest.UseCase == "" {
				return nil, errors.New("GeneralChatGraph: use_case cannot be empty")
			}
			if in.ChatRequest.UserID == "" {
				return nil, errors.New("GeneralChatGraph: user_id cannot be empty")
			}
			if in.ChatRequest.SessionID == "" {
				return nil, errors.New("GeneralChatGraph: session_id cannot be empty")
			}

			var err error
			in.UserMessageTemplate, err = component_prompt.GetChatTemplate(
				schema.User, in.ChatModelPrompt.MessagesUserPath)
			if err != nil {
				return nil, fmt.Errorf("GeneralChatGraph: get user template: %w", err)
			}

			in.SystemMessageTemplate, err = component_prompt.GetChatTemplate(
				schema.System, in.ChatModelPrompt.MessagesSystemPath)
			if err != nil {
				return nil, fmt.Errorf("GeneralChatGraph: get system template: %w", err)
			}

			in.AssistantMessageTemplate, err = component_prompt.GetChatTemplate(
				schema.Assistant, in.ChatModelPrompt.MessagesAssistantPath)
			if err != nil {
				return nil, fmt.Errorf("GeneralChatGraph: get assistant template: %w", err)
			}

			return in, nil
		}), compose.WithOutputKey(NodeRequestVerify))

	// 准备调用QdrantRetriever，获取用户输入的问题、任务内容
	g.AddLambdaNode(NodeRetrieverMemoryPre, compose.InvokableLambda(
		func(ctx context.Context, in *types.UseCaseParams) (string, error) {
			return in.ChatRequest.Task, nil
		}))
	// 调用QdrantRetriever召回相关对话记录
	g.AddRetrieverNode(NodeRetrieverMemory, rti)
	g.AddLambdaNode(NodeRetrieverMemoryAfter, compose.InvokableLambda(
		func(ctx context.Context, in []*schema.Document) ([]*types.DialogueRecord, error) {
			slog.Info("GeneralChatGraph: qdrant retrieve memory", slog.Any("memory", in))
			records := make([]*types.DialogueRecord, 0, len(in))
			if in == nil || len(in) == 0 {
				return nil, nil
			}
			for _, v := range in {
				if v.MetaData == nil {
					continue
				}
				var md map[string]*qdrant.Value
				if vt, ok := v.MetaData[component_indexer.QdrantMetaData]; ok {
					if md, ok = vt.(map[string]*qdrant.Value); !ok {
						continue
					}
				}

				taskVal, ok := md[component_memory.DialogueTaskKey]
				if !ok {
					continue
				}

				t := taskVal.GetStringValue()
				a := v.Content
				if component_memory.MemoryRecordNeedTrim {
					// 超长裁切
					t = utils.HeadTailRunes(taskVal.GetStringValue(), utils.TaskTrimHeadLength, utils.TaskTrimTailLength)
					a = utils.HeadTailRunes(v.Content, utils.AnswerTrimHeadLength, utils.AnswerTrimTailLength)
				}

				records = append(records, &types.DialogueRecord{
					ID:     v.ID,
					Task:   t,
					Answer: a,
				})
			}
			if len(records) == 0 {
				slog.Warn("GeneralChatGraph: RetrieverMemory is empty")
				return nil, nil
			}

			return records, nil
		}), compose.WithOutputKey(NodeRetrieverMemory))

	// 获取Redis对话记录
	g.AddLambdaNode(NodeRedisMemory, compose.InvokableLambda(
		func(ctx context.Context, in *types.UseCaseParams) ([]*types.DialogueRecord, error) {
			if in.DialogueMemory != nil {
				slog.Warn("GeneralChatGraph: dialogue memory pre-injected, skip redis read",
					slog.Int("count", len(in.DialogueMemory)))
				return nil, nil
			}

			dr, err := drm.GetDialogueRecordMemory(ctx, in.ChatRequest.UseCase,
				in.ChatRequest.UserID, in.ChatRequest.SessionID)

			if err != nil {
				slog.Error("GeneralChatGraph: GetDialogueRecordMemory", slog.String("error", err.Error()))
				return nil, nil
			}
			if dr == nil || len(dr) == 0 {
				slog.Warn("GeneralChatGraph: GetDialogueRecordMemory is empty")
				return nil, nil
			}

			return dr, nil
		}), compose.WithOutputKey(NodeRedisMemory))

	// qdrant、redis对话记录拼接
	g.AddLambdaNode(NodeDialogueAssemble, compose.InvokableLambda(
		func(ctx context.Context, in map[string]any) (*types.UseCaseParams, error) {
			if in == nil || len(in) == 0 {
				return nil, nil
			}

			var qm, rm []*types.DialogueRecord

			if v, ok := in[NodeRetrieverMemory]; ok {
				if qm, ok = v.([]*types.DialogueRecord); !ok || qm == nil {
					qm = make([]*types.DialogueRecord, 0)
				}
			}
			if v, ok := in[NodeRedisMemory]; ok {
				if rm, ok = v.([]*types.DialogueRecord); !ok || rm == nil {
					rm = make([]*types.DialogueRecord, 0)
				}
			}

			total := len(rm) + len(qm)
			if total == 0 {
				return &types.UseCaseParams{
					DialogueMemory: make([]*types.DialogueRecord, 0),
				}, nil
			}

			// 聚合+去重，结果顺序: qdrant检索内容在前，redis查询内容在后
			combined := make([]*types.DialogueRecord, 0, total)
			if len(rm) != 0 {
				ct1 := rm
				if len(qm) != 0 {
					ct2 := make([]*types.DialogueRecord, 0, len(qm))
					for _, v1 := range qm {
						has := false
						for _, v2 := range ct1 {
							if v2.ID == v1.ID {
								has = true
								break
							}
						}
						if !has {
							ct2 = append(ct2, v1)
						}
					}
					combined = append(combined, ct2...)
				}
				combined = append(combined, ct1...)
			} else {
				combined = append(combined, qm...)
			}
			slog.Info("GeneralChatGraph: qdrant memories", slog.Any("memories", qm))
			slog.Info("GeneralChatGraph: redis memories", slog.Any("memories", rm))
			slog.Info("GeneralChatGraph: combined memories", slog.Any("memories", combined))
			return &types.UseCaseParams{
				DialogueMemory: combined,
			}, nil
		}), compose.WithOutputKey(NodeDialogueAssemble))

	// 组装Message
	g.AddLambdaNode(NodeMessageAssemble, compose.InvokableLambda(
		func(ctx context.Context, in map[string]any) ([]*schema.Message, error) {

			var or, ar *types.UseCaseParams
			if v, ok := in[NodeRequestVerify]; ok {
				if or, ok = v.(*types.UseCaseParams); !ok || or == nil {
					return nil, errors.New("GeneralChatGraph: original req is missing or nil")
				}
			}
			if v, ok := in[NodeDialogueAssemble]; ok {
				if ar, ok = v.(*types.UseCaseParams); !ok || ar == nil {
					slog.Warn("GeneralChatGraph: assemble dialogueRecord memory is missing or nil")
				}
			}

			var userMsg, systemMsg, assistantMsg []*schema.Message
			var err error

			// 用户提示词
			if userMsg, err = or.UserMessageTemplate.Format(ctx, types.ToUserMessageMap(or.ChatRequest)); err != nil {
				return nil, fmt.Errorf("GeneralChatGraph: format user message: %w", err)
			}

			// 预设系统提示词
			values, ok := or.ChatModelPrompt.PresetValues[or.ChatModelPrompt.PromptVersion]
			if !ok {
				return nil, fmt.Errorf("GeneralChatGraph: prompt preset version %s not found", or.ChatModelPrompt.PromptVersion)
			}
			systemMsg, err = or.SystemMessageTemplate.Format(ctx, types.ToSystemMessageMap(values.PromptParams))
			if err != nil {
				return nil, fmt.Errorf("GeneralChatGraph: format system message: %w", err)
			}

			// 对话记忆，允许外部注入
			memoryToUse := make([]*types.DialogueRecord, 0)
			if ar != nil && ar.DialogueMemory != nil && len(ar.DialogueMemory) != 0 {
				memoryToUse = ar.DialogueMemory
			}

			if len(memoryToUse) != 0 {
				assistantMsg, err = or.AssistantMessageTemplate.Format(ctx,
					types.ToAssistantMessageMap(memoryToUse))
				if err != nil {
					return nil, fmt.Errorf("GeneralChatGraph: format assistant message: %w", err)
				}
			}

			messages := make([]*schema.Message, 0, len(systemMsg)+len(memoryToUse)+len(assistantMsg))
			messages = append(messages, userMsg...)
			messages = append(messages, systemMsg...)
			messages = append(messages, assistantMsg...)
			slog.Info("GeneralChatGraph: llm api message", slog.Any("messages", messages))
			return messages, nil
		}))

	// 模型调用
	g.AddChatModelNode(NodeChatModel, instanceModel)

	// LLM主链路
	g.AddEdge(compose.START, NodeRequestVerify)
	g.AddEdge(NodeRequestVerify, NodeMessageAssemble)

	// Redis链路
	g.AddEdge(compose.START, NodeRedisMemory)
	g.AddEdge(NodeRedisMemory, NodeDialogueAssemble)

	// QdrantRetriever链路
	g.AddEdge(compose.START, NodeRetrieverMemoryPre)
	g.AddEdge(NodeRetrieverMemoryPre, NodeRetrieverMemory)
	g.AddEdge(NodeRetrieverMemory, NodeRetrieverMemoryAfter)
	g.AddEdge(NodeRetrieverMemoryAfter, NodeDialogueAssemble)

	g.AddEdge(NodeDialogueAssemble, NodeMessageAssemble)
	g.AddEdge(NodeMessageAssemble, NodeChatModel)
	g.AddEdge(NodeChatModel, compose.END)

	return g.Compile(ctx, compose.WithGraphName(modelProvider), compose.WithNodeTriggerMode(compose.AllPredecessor))
}

// NewDialogueMemorySaveGraph
//
//	对返回内容进行摘要并存储到Redis，向量化后自动存储到向量数据库
//	存入qdrant的playload参数动态option传入
func NewDialogueMemorySaveGraph(ctx context.Context,
	extractor *openai.ChatModel,
	indexerMap map[string]*qdrant_indexer.Indexer,
	drm *component_memory.DialogueRecordMemory) (compose.Runnable[*types.UseCaseAssistantResponse, []string], error) {

	ide, ok := indexerMap[CollectionDialogueMemory]
	if !ok {
		return nil, fmt.Errorf("NewDialogueMemorySaveGraph: Qdrant Collection %s not not found", CollectionDialogueMemory)
	}

	c := compose.NewChain[*types.UseCaseAssistantResponse, []string](
		compose.WithGenLocalState(
			func(ctx context.Context) (state *types.UseCaseAssistantResponse) {
				return &types.UseCaseAssistantResponse{}
			}))

	// 摘要 Message 拼装
	c.AppendLambda(compose.InvokableLambda(
		func(ctx context.Context, in *types.UseCaseAssistantResponse) ([]*schema.Message, error) {
			if in.Message == nil || in.Message.Content == "" {
				return nil, errors.New("DialogueMemorySaveGraph: AssistantResponse is nil or Content is empty")
			}
			var userMsg, systemMsg *schema.Message

			// 系统提示词
			systemMsg = &schema.Message{
				Role:    schema.System,
				Content: component_model.ExtractorPrompt,
			}

			// 用户提示词
			userMsg = &schema.Message{
				Role:    schema.User,
				Content: in.Message.Content,
			}

			messages := []*schema.Message{systemMsg, userMsg}
			err := compose.ProcessState(ctx, func(ctx context.Context, state *types.UseCaseAssistantResponse) error {
				state.UseCase = in.UseCase
				state.UserID = in.UserID
				state.SessionID = in.SessionID
				state.DialogueID = in.DialogueID
				state.Task = in.Task
				state.Message = &schema.Message{
					Content: in.Message.Content,
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("DialogueMemorySaveGraph: ProcessState failed: %w", err)
			}
			return messages, nil
		},
	))

	// SLM 模型进行摘要
	c.AppendChatModel(extractor)

	// 摘要后处理，MetaData会自动存到qdrant payload，key: metadata，原文key: content
	c.AppendLambda(compose.InvokableLambda(
		func(ctx context.Context, in *schema.Message) (*schema.Document, error) {
			if in == nil {
				return nil, errors.New("DialogueMemorySaveGraph: Extracted AssistantMessage is nil")
			}

			var assistantMessage types.UseCaseAssistantResponse
			err := compose.ProcessState(ctx, func(ctx context.Context, state *types.UseCaseAssistantResponse) error {
				assistantMessage = *state
				return nil
			})
			if err != nil {
				return nil, errors.New("DialogueMemorySaveGraph: graph state is nil")
			}

			var doc *schema.Document
			if in.Role == schema.Assistant && in.Content != "" {
				doc = &schema.Document{
					ID:      assistantMessage.DialogueID,
					Content: in.Content,
					MetaData: map[string]any{
						component_memory.DialogueUseCaseKey:             assistantMessage.UseCase,
						component_memory.DialogueUserIDKey:              assistantMessage.UserID,
						component_memory.DialogueSessionIDKey:           assistantMessage.SessionID,
						component_memory.DialogueTaskKey:                assistantMessage.Task,
						component_memory.DialogueAnswerKey:              assistantMessage.Message.Content,
						component_memory.DialogueAnswerTokensKey:        in.ResponseMeta.Usage.PromptTokens,
						component_memory.DialogueAnswerSummaryTokensKey: in.ResponseMeta.Usage.CompletionTokens,
					},
				}
			}
			if doc == nil {
				return nil, errors.New("DialogueMemorySaveGraph: Extracted AssistantMessage is nil or content is empty")
			}
			return doc, nil
		},
	))

	// 存入redis
	c.AppendLambda(compose.InvokableLambda(
		func(ctx context.Context, in *schema.Document) ([]*schema.Document, error) {
			if in == nil {
				return nil, errors.New("DialogueMemorySaveGraph: assistant message extracted doc is nil")
			}
			if err := drm.AppendDialogueRecordMemory(ctx, in); err != nil {
				slog.Error("DialogueMemorySaveGraph: append redis failed", slog.String("error", err.Error()))
			}
			docs := []*schema.Document{in}
			return docs, nil
		},
	))

	c.AppendIndexer(ide)

	return c.Compile(ctx, compose.WithGraphName(ChainDialogueSave))
}
