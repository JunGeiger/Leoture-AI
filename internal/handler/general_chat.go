package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"leoture/internal/service"
	"leoture/internal/types"
	"log/slog"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/protocol/sse"
)

// 预序列化的固定 SSE 事件 payload,避免每次请求重复 Marshal
var (
	doneEventPayload  = []byte(`{"status":"success","message":"complete"}`)
	errorEventPayload = []byte(`{"status":"failed","message":"stream reading failed"}`)
)

// Chat 处理聊天相关的 HTTP 请求
// 依赖 service.GeneralChatService 完成业务逻辑,自身只负责协议转换和响应写入
type GeneralChatHandler struct {
	service service.GeneralChatService
}

// newGeneralChatHandler 创建 Chat handler 实例
// 使用依赖注入,便于测试和替换实现
func newGeneralChatHandler(svc service.GeneralChatService) *GeneralChatHandler {
	return &GeneralChatHandler{
		service: svc,
	}
}

// GeneralChat 是 /api/v1/generalChat 端点的入口方法
// 根据请求中的 EnabledStream 字段决定走流式还是非流式分支
func (h *GeneralChatHandler) GeneralChat(ctx context.Context, c *app.RequestContext) {
	var req types.ChatRequest

	if err := c.BindJSON(&req); err != nil {
		slog.WarnContext(ctx, "GeneralChat: request params bind json failed", "error", err)
		c.JSON(consts.StatusBadRequest, map[string]string{
			"status": "failed", "message": "invalid request body",
		})
		return
	}
	if err := c.Validate(&req); err != nil {
		slog.WarnContext(ctx, "GeneralChat: validate failed", "error", err)
		c.JSON(consts.StatusBadRequest, map[string]string{
			"status": "failed", "message": "request validation failed",
		})
		return
	}

	// 路由到对应处理逻辑
	if req.EnabledStream {
		h.handleStream(ctx, c, &req)
	} else {
		h.handleInvoke(ctx, c, &req)
	}
}

// handleStream 处理流式 SSE 响应
// 从 service 层获取 StreamReader,循环读取消息并推送给客户端
func (h *GeneralChatHandler) handleStream(ctx context.Context, c *app.RequestContext, req *types.ChatRequest) {
	// 调用 service 层获取流式读取器
	// GeneralChatStream 内部应已配置 ctx 传递，支持服务端感知客户端断开
	sr, err := h.service.GeneralChatStream(ctx, req)
	if err != nil {
		slog.ErrorContext(ctx, "GeneralChat: stream init failed", "error", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{
			"status": "failed", "message": "stream initialization failed" + err.Error(),
		})
		return
	}

	// 防御性检查：sr 不应为 nil，但防止 service 层实现缺陷
	if sr == nil {
		slog.ErrorContext(ctx, "GeneralChat: stream reader is nil")
		c.JSON(consts.StatusInternalServerError, map[string]string{
			"status": "failed", "message": "stream reader is nil",
		})
		return
	}

	// 确保流资源在函数退出时被释放
	// 即使发生 panic 或提前 return，Close 都会被调用
	defer sr.Close()

	// 使用 hertz 内置 SSE Writer
	w := sse.NewWriter(c)
	defer w.Close()

	var assistantMessageContent strings.Builder
	var assistantMessage *schema.Message
	// 主循环：持续从流中读取消息并推送给客户端
	for {
		// 每次循环前检查上下文是否已取消
		// 这能及时感知客户端断开或请求超时，避免向已关闭的连接写入数据
		select {
		case <-ctx.Done():
			slog.WarnContext(ctx, "GeneralChat: stream cancelled by client disconnect or timeout")
			return
		default:
		}

		// 阻塞读取下一条消息
		// Recv 会在流结束、出错或上下文取消时返回
		msg, recvErr := sr.Recv()
		if recvErr != nil {
			// io.EOF 表示流正常结束
			if errors.Is(recvErr, io.EOF) {
				// 流正常结束,发送 done 事件
				if err := w.WriteEvent(req.DialogueID, "done", doneEventPayload); err != nil {
					slog.WarnContext(ctx, "SSE publish done event failed", "error", err)
				}
				if assistantMessage != nil {
					assistantMessage.Content = assistantMessageContent.String()
					h.service.GeneralChatSaveDialogueMemory(context.Background(), req, assistantMessage)
				}
				return
			}

			// 流中途出错
			slog.ErrorContext(ctx, "GeneralChat: stream recv failed", "error", recvErr)
			if err := w.WriteEvent(req.DialogueID, "error", errorEventPayload); err != nil {
				slog.WarnContext(ctx, "SSE publish error event failed", "error", err)
			}
			return
		}

		// 将消息序列化为 JSON
		data, marshalErr := json.Marshal(msg)
		if marshalErr != nil {
			// 单条消息序列化失败不应中断整个流
			slog.WarnContext(ctx, "GeneralChat: SSE message marshal failed", "error", marshalErr)
			continue
		}
		assistantMessage = msg
		assistantMessageContent.WriteString(msg.Content)
		// 发布消息事件到 SSE 流
		if publishErr := w.WriteEvent(req.DialogueID, "message", data); publishErr != nil {
			slog.WarnContext(ctx, "GeneralChat: stream publish failed, client may disconnected", "error", publishErr)
			return
		}
	}
}

// handleInvoke 处理非流式请求
// 等待完整响应后一次性返回给客户端
func (h *GeneralChatHandler) handleInvoke(ctx context.Context, c *app.RequestContext, req *types.ChatRequest) {
	// 调用 service 层执行完整推理
	msg, err := h.service.GeneralChatInvoke(ctx, req)
	if err != nil {
		slog.ErrorContext(ctx, "GeneralChat: invoke failed", "error", err)
		c.JSON(consts.StatusInternalServerError, map[string]string{
			"status": "failed", "message": "invoke failed: " + err.Error(),
		})
		return
	}

	// 防御性检查：防止 service 层返回 nil msg 且无 error 的情况
	if msg == nil {
		c.JSON(consts.StatusOK, map[string]any{
			"status":  "success",
			"content": "",
		})
	} else {
		// 返回统一格式的响应
		// 使用 map 构造响应，字段名与流式模式下 message 事件中的 msg JSON 结构保持一致
		c.JSON(consts.StatusOK, map[string]any{
			"status":           "success",
			"content":          msg.Content,
			"reasoningContent": msg.ReasoningContent,
			"responseMeta":     msg.ResponseMeta,
		})
		h.service.GeneralChatSaveDialogueMemory(context.Background(), req, msg)
	}
}
