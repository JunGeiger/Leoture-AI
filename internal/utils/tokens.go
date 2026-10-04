package utils

const (
	TaskTrimHeadLength   = 32
	TaskTrimTailLength   = 32
	AnswerTrimHeadLength = 256
	AnswerTrimTailLength = 256
)

// estimateTextLength 返回字符数估算值，不是精确 LLM Token 数。
// 精确 Token 应优先使用模型返回的 ResponseMeta.Usage。
// 估算策略：UTF-8 字节数 / 4（经验近似，中英文混合场景下的粗略兜底）。
func EstimateTextLength(text string) int {
	if text == "" {
		return 0
	}
	// UTF-8：平均 4 字节 ≈ 1 token（经验近似）
	n := len(text) / 4
	if n == 0 {
		return 1 // 非空文本至少 1 token
	}
	return n
}

// headTailRunes 按 rune 截取首尾，中间插入省略标记。
// 调用方应已判断是否需要截断（即 len(runes) > headRunes+tailRunes）。
func HeadTailRunes(text string, headRunes, tailRunes int) string {
	runes := []rune(text)
	length := EstimateTextLength(text)
	if length <= headRunes+tailRunes {
		return text
	}
	return string(runes[:headRunes]) + string(runes[length-tailRunes:])
}
