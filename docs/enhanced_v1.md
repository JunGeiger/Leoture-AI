## 一、SystemMessage（系统指令模板）

### Role string
定义 LLM 在当前 useCase 下扮演的身份或角色。
该字段决定模型回答的视角、专业领域与行为基调。
属于 System 消息的核心字段，应保持稳定且长期不变。
通过 YAML 预设值热更新，但模板本身固定。
渲染后作为 system 消息的 Content 传递给模型。

### Context []string
提供 useCase 级别的静态背景信息，如项目简介、技术栈或业务域知识。
每个元素是一条独立的背景描述，模板中循环渲染。
与 User 消息中的动态检索上下文（References）明确区分。
仅包含不随请求频繁变化的背景，避免破坏提示词缓存。
若背景为空，模板可跳过该段渲染。

### Constraints []string
列出模型回答时必须遵守的行为限制与边界。
可包含拒绝回答的条件、格式约束、安全规则等。
也承担错误处理的兜底策略，例如无法回答时的固定回复。
模板中以列表形式渲染，每条约束独立清晰。
是 System 消息中控制模型输出的关键字段。

### OutputFormat []string
规定模型输出内容的结构化格式要求。
可包含 JSON Schema、Markdown 代码块、字段顺序等具体指令。
每个元素描述一种格式约束或示例。
确保模型返回结果易于下游解析与展示。
与 Constraints 配合，共同限定输出行为。

---

## 二、UserMessage（用户输入模板）

### Task string
当前轮次的用户问题或任务描述，是 User 消息的核心内容。
可包含简短的错误处理提示，例如问题不明确时的引导话术。
由 handler 层接收请求后注入，经模板渲染为 user 消息的 Content。
应避免包含检索上下文或历史对话，保持单一职责。
与 Assistant 消息中的历史摘要形成问答对应关系。

### Example string
提供 few-shot 示例，引导模型遵循特定回答模式。
内容可为多轮对话示例或输入-输出对，长度需做个数或 token 限制。
复杂场景下可省略，由检索到的 References 替代。
渲染时置于 Task 之前或之后，增强格式一致性。
避免过多示例导致 token 浪费。

### References map[string]string
存放根据 Task 检索并经过重排序后的知识库内容。
键为文档标识，值为文本片段，由 retriever 组件从 qdrant 获取。
是 RAG 场景的核心上下文，直接支撑模型回答。
模板中遍历渲染，作为模型生成答案的依据。
与 System 的 Context 区分：此处为动态、请求级上下文。

### Instruction map[int]string
定义 Task 回答的思考链路或推理步骤，按执行顺序编号。
用于复杂场景的错误处理与流程控制，引导模型逐步推理。
键为步骤序号，值为该步骤的具体指令或问题描述。
模板中按序号排序渲染，形成 Chain-of-Thought 提示。
简单任务可留空，避免不必要的 token 消耗。

---

## 三、AssistantMessage（历史上下文模板）

### DialogueRecord map[string]string
存储对话历史记录，键为轮次序号，值为该轮摘要或截取文本。
包含最近 N 轮摘要以及上一轮对话全文。
上一轮全文超长时，首尾各截取 512 token 后拼接。
由 memory 组件从 redis 读取并注入，不通过模板热更新。
在请求中作为 assistant 消息单独传递，为模型提供历史上下文。

---

粗粒度 useCase
  ↓ 积累足够多成功 case
沉淀出可复用的 solution
  ↓ 抽象稳定 Instructions / Example
新定义更细化的 useCase
  ↓
SystemMessage 固化
