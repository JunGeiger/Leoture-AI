package memory

import (
	"context"
	"errors"
	"fmt"
	"leoture/internal/types"
	"leoture/internal/utils"
	"log/slog"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"
)

const (
	DialogueUseCaseKey             = "use_case"
	DialogueUserIDKey              = "user_id"
	DialogueSessionIDKey           = "session_id"
	DialogueIDKey                  = "dialogue_id"
	DialogueTaskKey                = "task"
	DialogueAnswerKey              = "answer"
	DialogueAnswerTokensKey        = "answer_tokens"
	DialogueAnswerSummaryKey       = "answer_summary"
	DialogueAnswerSummaryTokensKey = "answer_summary_tokens"
	MemoryRecordNeedTrim           = true

	memoryNumber         = 2
	lastMemoryIsOriginal = true

	cacheTTL = "24h"
)

type DialogueRecord struct {
	// 用户传入的Task字段，Tokens自己计算；超出截断逻辑：32 Token + ... + 32 Token = 64 Token
	Task       string `json:"task" redis:"task"`
	TaskTokens int    `json:"task_tokens" redis:"task_tokens"`

	// LLM返回的Content字段，Tokens由API返回；超出截断逻辑：256 Token + ... + 256 Token = 512 Token
	Answer       string `json:"answer" redis:"answer"`
	AnswerTokens int    `json:"answer_tokens" redis:"answer_tokens"`

	// 元数据（从 User Message.Extra 提取）
	UseCase    string `json:"use_case" redis:"use_case"`
	UserID     string `json:"user_id" redis:"user_id"`
	SessionID  string `json:"session_id" redis:"session_id"`
	DialogueID string `json:"dialogue_id" redis:"dialogue_id"`

	// 本地SLM(摘要)返回的Content字段，Tokens由API返回
	AnswerSummary       string `json:"answer_summary" redis:"answer_summary"`
	AnswerSummaryTokens int    `json:"answer_summary_tokens" redis:"answer_summary_tokens"`

	// Unix timestamp，作为 ZSET score
	CreatedAt int64 `json:"created_at" redis:"created_at"`
}

type DialogueRecordMemory struct {
	redisCli *redis.Client
}

// NewDialogueMemory Reids处理对话记录
//
//	如果LLM返回的Content超过256Token，则需要对Content进行摘要处理，存入到Message Extra["answer_summary"]
//	AssistantMessage作为对话记忆内容，返回2轮摘要，上一轮对话全文（超长需截断）。
func NewDialogueMemory(redisCli *redis.Client) *DialogueRecordMemory {
	return &DialogueRecordMemory{redisCli: redisCli}
}

func (d *DialogueRecordMemory) AppendDialogueRecordMemory(ctx context.Context, doc *schema.Document) error {
	// 防御性校验：至少需包含一条消息
	if doc == nil {
		return errors.New("AppendDialogueRecordMemory: assistant message extracted doc is nil")
	}

	// 将消息对转换为内部对话记录结构
	record := DialogueRecordFromDocument(doc)
	if record == nil {
		return errors.New("AppendDialogueRecordMemory: failed to construct DialogueRecord from assistant message extracted doc")
	}

	// 校验必填字段，防止构造出无效的 Redis Key 或无法反查的脏数据
	if record.DialogueID == "" || record.SessionID == "" || record.UserID == "" || record.UseCase == "" {
		return errors.New("AppendDialogueRecordMemory: record missing required fields (use_case, user_id, session_id, dialogue_id)")
	}
	// 校验时间戳合法性，ZSET score 依赖此值进行排序，零值或负值会导致排序失效
	if record.CreatedAt <= 0 {
		return errors.New("AppendDialogueRecordMemory: invalid CreatedAt")
	}

	// 构造各层级 Redis Key
	dialogueKey := d.dialogueKey(record.UseCase, record.UserID, record.SessionID, record.DialogueID)
	sessionIdxKey := d.sessionIndexKey(record.UseCase, record.UserID, record.SessionID)
	userIdxKey := d.userIndexKey(record.UseCase, record.UserID)
	useCaseIdxKey := d.useCaseIndexKey(record.UseCase)

	// 使用 TxPipeline 保证多 Key 写入的原子性：
	// 要么对话数据与所有索引同时写入成功，要么同时失败，避免产生脏索引。
	pipe := d.redisCli.TxPipeline()

	// 关键逻辑：DialogueID 已存在则不更新
	// HSetNX 返回值用于判断是否是首次写入
	setNXCmd := pipe.HSetNX(ctx, dialogueKey, DialogueIDKey, record.DialogueID)

	// session 级顺序索引：ZSET，score 为时间戳，支持按时间范围查询和最近 N 轮回溯
	pipe.ZAdd(ctx, sessionIdxKey, redis.Z{
		Score:  float64(record.CreatedAt),
		Member: record.DialogueID,
	})
	// user 级索引：SET，记录该 user 下的所有 session，支持会话列表查询
	pipe.SAdd(ctx, userIdxKey, record.SessionID)
	// useCase 级索引：SET，记录该 useCase 下的所有 user，支持多租户隔离与管理
	pipe.SAdd(ctx, useCaseIdxKey, record.UserID)
	// 对话完整记录以 Hash 类型存储，go-redis 自动将 struct 字段映射为 field-value
	pipe.HSet(ctx, dialogueKey, record)

	// 执行 pipeline，超时与重试策略由 redisClient 统一管理
	if _, err := pipe.Exec(ctx); err != nil {
		// 区分 context 取消/超时错误与 Redis 内部错误，便于上层判断是否需要重试
		if ctx.Err() != nil {
			return fmt.Errorf("AppendDialogueRecordMemory: context error: %w", ctx.Err())
		}
		return fmt.Errorf("AppendDialogueRecordMemory: exec failed %w", err)
	}

	// 判断是否为重复写入
	if setNXCmd.Val() == false {
		// DialogueID 已存在，直接幂等返回
		return nil
	}

	return nil
}

// GetDialogueRecordMemory 读取指定 session 下最近的 N 轮对话记录
//
//	层级关系：useCase -> userID -> sessionID -> dialogueID
//	返回结果按时间正序排列（从旧到新），加上是否存入qdrant判断，已存入的结果不返回，由qdrant处理
func (d *DialogueRecordMemory) GetDialogueRecordMemory(
	ctx context.Context,
	useCase string,
	userID string,
	sessionID string,
) ([]*types.DialogueRecord, error) {
	// 构造 session 级 ZSET 索引 Key
	sessionIdxKey := d.sessionIndexKey(useCase, userID, sessionID)

	// ZSET 按排名倒序获取最新的 memoryNumber 个 dialogueID
	// ByScore: false 表示按排名位置取，不按分数范围取
	// Rev: true 表示倒序（分数大的/最新的在前面）
	dialogueIDs, err := d.redisCli.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:     sessionIdxKey,
		Start:   0,
		Stop:    int64(memoryNumber - 1),
		ByScore: false,
		Rev:     true,
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("GetDialogueRecordMemory: get index failed: %w", err)
	}
	if len(dialogueIDs) == 0 {
		return nil, nil
	}

	// Pipeline 批量读取 Hash 数据
	pipe := d.redisCli.Pipeline()
	hGetAllCmds := make([]*redis.MapStringStringCmd, len(dialogueIDs))
	for i := range dialogueIDs {
		hGetAllCmds[i] = pipe.HGetAll(ctx, d.dialogueKey(useCase, userID, sessionID, dialogueIDs[len(dialogueIDs)-1-i]))
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("GetDialogueRecordMemory: pipeline exec failed : %w", err)
	}

	// 反序列化并组装结果
	var dialogueRecords []*types.DialogueRecord
	for i, cmd := range hGetAllCmds {
		var dialogueRecord DialogueRecord
		if err := cmd.Scan(&dialogueRecord); err != nil {
			return nil, fmt.Errorf("GetDialogueRecordMemory: scan failed [dialogue_id=%s]: %w",
				dialogueIDs[i], err,
			)
		}

		// 校验关键字段，过滤索引残留或脏数据
		if dialogueRecord.DialogueID == "" {
			slog.Warn("GetDialogueRecordMemory: stale index or corrupt record",
				DialogueUseCaseKey, useCase,
				DialogueUserIDKey, userID,
				DialogueSessionIDKey, sessionID,
				DialogueIDKey, dialogueIDs[i],
			)
			continue
		}

		isLatest := (i == len(hGetAllCmds)-1)
		dr := dialogueRecord.ToSimpleDialogueRecord(MemoryRecordNeedTrim, isLatest)
		if dr == nil {
			slog.Error("GetDialogueRecordMemory: ToSimpleDialogueRecord returned nil",
				"dialogue_id", dialogueIDs[i],
			)
			continue
		}
		dialogueRecords = append(dialogueRecords, dr)
	}

	return dialogueRecords, nil
}

// DeleteDialogueRecordMemory 删除单条对话记录
//
// 操作范围：
//   - 删除 dialogueKey（Hash）中的数据本体
//   - 从 sessionIdxKey（ZSET）中移除 dialogueID 索引
//   - 不清理 userIdxKey / useCaseIdxKey，因为 session/user 可能仍有其他数据
func (d *DialogueRecordMemory) DeleteDialogueRecordMemory(
	ctx context.Context,
	useCase string,
	userID string,
	sessionID string,
	dialogueID string,
) error {
	// 校验关键字段，防止构造出错误的 Key 导致误删
	if useCase == "" || userID == "" || sessionID == "" || dialogueID == "" {
		return errors.New("DeleteDialogueRecordMemory: missing required fields (use_case, user_id, session_id, dialogue_id)")
	}
	dialogueKey := d.dialogueKey(useCase, userID, sessionID, dialogueID)
	sessionIdxKey := d.sessionIndexKey(useCase, userID, sessionID)

	// 使用 TxPipeline 保证原子性：数据本体和索引要么同时删除，要么同时保留
	pipe := d.redisCli.TxPipeline()
	pipe.Del(ctx, dialogueKey)
	pipe.ZRem(ctx, sessionIdxKey, dialogueID)

	if _, err := pipe.Exec(ctx); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("DeleteDialogueRecordMemory: context error: %w", ctx.Err())
		}
		return fmt.Errorf("DeleteDialogueRecordMemory: exec failed: %w", err)
	}
	return nil
}

// DeleteDialogueSessionMemory 删除整个 session（级联删除所有 dialogue）
//
// 操作范围：
//   - 删除该 session 下所有 dialogueKey（Hash）数据本体
//   - 删除 sessionIdxKey（ZSET）索引
//   - 从 userIdxKey（SET）中移除 sessionID
//
// 设计决策：
//   - 不清理 useCaseIdxKey 中的 userID，因为 user 可能仍有其他 session
//   - 如果 session 下 dialogue 数量极大，建议未来改为分批删除
func (d *DialogueRecordMemory) DeleteDialogueSessionMemory(
	ctx context.Context,
	useCase string,
	userID string,
	sessionID string,
) error {
	// 校验关键字段，防止构造出错误的 Key 导致误删
	if useCase == "" || userID == "" || sessionID == "" {
		return errors.New("DeleteDialogueSessionMemory: missing required fields (use_case, user_id, session_id)")
	}
	sessionIdxKey := d.sessionIndexKey(useCase, userID, sessionID)
	userIdxKey := d.userIndexKey(useCase, userID)

	// 从 ZSET 获取该 session 下所有 dialogueID
	dialogueIDs, err := d.redisCli.ZRange(ctx, sessionIdxKey, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("DeleteDialogueSessionMemory: get session index failed : %w", err)
	}

	// 收集所有要删除的 dialogueKey
	keys := make([]string, 0, len(dialogueIDs))
	for _, dialogueID := range dialogueIDs {
		keys = append(keys, d.dialogueKey(useCase, userID, sessionID, dialogueID))
	}

	// 使用 TxPipeline 保证原子性：数据本体和索引要么同时删除，要么同时保留
	pipe := d.redisCli.TxPipeline()

	// 批量删除所有 dialogue 数据本体
	if len(keys) > 0 {
		pipe.Del(ctx, keys...)
	}
	// 删除 session 级索引（ZSET 本身）
	pipe.Del(ctx, sessionIdxKey)
	// 从 user 级索引中移除 sessionID
	pipe.SRem(ctx, userIdxKey, sessionID)

	if _, err := pipe.Exec(ctx); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("DeleteDialogueSessionMemory: context error : %w", ctx.Err())
		}
		return fmt.Errorf("DeleteDialogueSessionMemory: exec failed : %w", err)
	}

	return nil
}

// DeleteDialogueUserMemory 级联删除 useCase 下某个 user 的所有 session 和 dialogue
//
// 操作范围：
//   - 删除该 user 下所有 session 的 dialogueKey（Hash）数据本体
//   - 删除所有 sessionIdxKey（ZSET）索引
//   - 删除 userIdxKey（SET）索引本身
//   - 从 useCaseIdxKey（SET）中移除 userID
//
// 设计决策：
//   - 如果某个 session 的 ZRange 失败，跳过该 session 继续删除其他 session（降级策略）
//   - 删除操作使用 TxPipeline 保证原子性
func (d *DialogueRecordMemory) DeleteDialogueUserMemory(
	ctx context.Context,
	useCase string,
	userID string,
) error {
	// 校验关键字段
	if useCase == "" || userID == "" {
		return errors.New("DeleteDialogueUserMemory: missing required fields (use_case, user_id)")
	}

	userIdxKey := d.userIndexKey(useCase, userID)
	useCaseIdxKey := d.useCaseIndexKey(useCase)

	// 获取该 user 下所有 sessionID
	sessionIDs, err := d.redisCli.SMembers(ctx, userIdxKey).Result()
	if err != nil {
		return fmt.Errorf("DeleteDialogueUserMemory: get user index failed : %w", err)
	}

	// Pipeline 批量获取每个 session 下的 dialogueID
	pipe1 := d.redisCli.Pipeline()
	sessionIdxKeys := make([]string, 0, len(sessionIDs))
	sessionCmds := make([]*redis.StringSliceCmd, 0, len(sessionIDs))
	for _, sid := range sessionIDs {
		sk := d.sessionIndexKey(useCase, userID, sid)
		sessionIdxKeys = append(sessionIdxKeys, sk)
		sessionCmds = append(sessionCmds, pipe1.ZRange(ctx, sk, 0, -1))
	}

	// 如果 pipe1.Exec 返回错误，直接返回错误
	if _, err := pipe1.Exec(ctx); err != nil {
		return fmt.Errorf("DeleteDialogueUserMemory: batch get session indexes failed: %w", err)
	}

	// 收集所有待删除的 Key
	// 包含：所有 dialogueKey + 所有 sessionIdxKey + userIdxKey
	// user 索引本身
	allKeys := []string{userIdxKey}

	for i, cmd := range sessionCmds {
		dialogueIDs, err := cmd.Result()
		if err != nil {
			// 单个 session 获取失败，跳过该 session，避免阻塞其他 session 的删除
			slog.ErrorContext(ctx, "DeleteDialogueUserMemory: get dialogueIDs failed, skip session",
				"session_id", sessionIDs[i],
				"error", err,
			)
			continue
		}

		// 添加 session 索引 Key
		allKeys = append(allKeys, sessionIdxKeys[i])

		// 添加该 session 下所有 dialogueKey
		for _, dialogueID := range dialogueIDs {
			allKeys = append(allKeys, d.dialogueKey(useCase, userID, sessionIDs[i], dialogueID))
		}
	}

	if len(allKeys) == 0 {
		// 没有需要删除的 Key，直接返回
		return nil
	}

	// 使用 TxPipeline 保证原子性：批量删除所有 Key + 从 useCase 索引移除 userID
	pipe2 := d.redisCli.TxPipeline()
	pipe2.Del(ctx, allKeys...)
	pipe2.SRem(ctx, useCaseIdxKey, userID)

	if _, err := pipe2.Exec(ctx); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("DeleteDialogueUserMemory: context error : %w", ctx.Err())
		}
		return fmt.Errorf("DeleteDialogueUserMemory: batch delete failed: %w", err)
	}

	return nil
}

func (d *DialogueRecordMemory) dialogueKey(useCase, userID, sessionID, dialogueID string) string {
	return fmt.Sprintf("chat:%s:%s:%s:%s", useCase, userID, sessionID, dialogueID)
}

func (d *DialogueRecordMemory) sessionIndexKey(useCase, userID, sessionID string) string {
	return fmt.Sprintf("chat:index:%s:%s:%s", useCase, userID, sessionID)
}

func (d *DialogueRecordMemory) userIndexKey(useCase, userID string) string {
	return fmt.Sprintf("chat:index:%s:%s", useCase, userID)
}

func (d *DialogueRecordMemory) useCaseIndexKey(useCase string) string {
	return fmt.Sprintf("chat:index:%s", useCase)
}

// ToSimpleDialogueRecord 将内部 DialogueRecord 转换为对外精简记录
//
// 参数：
//   - needTrim: 是否对超长内容进行截断
//   - isLatest: 是否为最新一条记录（最后一条）
//
// 截断规则：
//   - Task 始终按 needTrim 决定是否截断
//   - Answer 的处理取决于 isLatest 和 lastMemoryIsOriginal：
//     如果是最新一条且配置为保留原文，则返回原文（超长截断）；
//     否则返回摘要（摘要为空则返回 nil）
func (d *DialogueRecord) ToSimpleDialogueRecord(needTrim bool, isLatest bool) *types.DialogueRecord {
	// DialogueID 为空则无法构成有效记录
	if d.DialogueID == "" {
		return nil
	}

	// Task 为空则无法构成有效记录
	if d.Task == "" {
		return nil
	}

	msg := &types.DialogueRecord{
		ID: d.DialogueID,
	}

	// Task 截断
	if needTrim && d.TaskTokens > (utils.TaskTrimHeadLength+utils.TaskTrimTailLength) {
		msg.Task = utils.HeadTailRunes(d.Task, utils.TaskTrimHeadLength, utils.TaskTrimTailLength)
	} else {
		msg.Task = d.Task
	}

	// Answer 处理
	switch {
	case isLatest && lastMemoryIsOriginal:
		// 最新一条且配置为保留原文：返回原文
		if d.Answer == "" {
			return nil
		}
		if needTrim && d.AnswerTokens > (utils.AnswerTrimHeadLength+utils.AnswerTrimTailLength) {
			msg.Answer = utils.HeadTailRunes(d.Answer, utils.AnswerTrimHeadLength, utils.AnswerTrimTailLength)
		} else {
			msg.Answer = d.Answer
		}
	default:
		// 非最新条或配置为使用摘要：返回摘要
		if d.AnswerSummary == "" {
			return nil
		}
		msg.Answer = d.AnswerSummary
	}

	return msg
}

// DialogueRecordFromDocuments 把构建好的 schema.Document 转换为 redis 可用对象
//
// 数据来源：摘要模型产物
func DialogueRecordFromDocument(doc *schema.Document) *DialogueRecord {
	if doc == nil || doc.ID == "" || doc.Content == "" || doc.MetaData == nil {
		slog.Warn("DialogueRecordFromDocuments: assistant message extracted, but doc data is nil")
		return nil
	}

	metaData := doc.MetaData
	record := &DialogueRecord{
		CreatedAt: time.Now().Unix(),
	}

	// 数据处理
	if v, ok := metaData[DialogueUseCaseKey]; ok {
		if record.UseCase, ok = v.(string); !ok {
			return nil
		}
	}
	if v, ok := metaData[DialogueUserIDKey]; ok {
		if record.UserID, ok = v.(string); !ok {
			return nil
		}
	}
	if v, ok := metaData[DialogueSessionIDKey]; ok {
		if record.SessionID, ok = v.(string); !ok {
			return nil
		}
	}
	if v, ok := metaData[DialogueTaskKey]; ok {
		if record.Task, ok = v.(string); !ok {
			return nil
		}
		record.TaskTokens = utils.EstimateTextLength(record.Task)
	}

	record.DialogueID = doc.ID

	if v, ok := metaData[DialogueAnswerKey]; ok {
		if record.Answer, ok = v.(string); !ok {
			return nil
		}
	}
	if v, ok := metaData[DialogueAnswerTokensKey]; ok {
		if record.AnswerTokens, ok = v.(int); !ok {
			return nil
		}
	}

	record.AnswerSummary = doc.Content
	if v, ok := metaData[DialogueAnswerSummaryTokensKey]; ok {
		if record.AnswerSummaryTokens, ok = v.(int); !ok {
			return nil
		}
	}

	return record
}
