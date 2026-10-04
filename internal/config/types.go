package config

import (
	"time"

	"github.com/spf13/viper"
)

// 项目环境模式
const (
	EnvDev     = "dev"
	EnvTest    = "test"
	EnvStaging = "staging"
	EnvProd    = "prod"
)

// Config 项目配置根结构
type Config struct {
	App              App               `mapstructure:"app" yaml:"app"`
	Server           Server            `mapstructure:"server" yaml:"server"`
	Log              Log               `mapstructure:"log" yaml:"log"`
	Redis            Redis             `mapstructure:"redis" yaml:"redis"`
	Qdrant           Qdrant            `mapstructure:"qdrant" yaml:"qdrant"`
	Langfuse         Langfuse          `mapstructure:"langfuse" yaml:"langfuse"`
	ModelServices    []ModelService    `mapstructure:"model_services" yaml:"model_services"`
	ChatModelPrompts []ChatModelPrompt `mapstructure:"chat_model_prompts" yaml:"chat_model_prompts"`
}

// App 项目配置
type App struct {
	// 项目名称
	Name string `mapstructure:"name" yaml:"name"`
	// 项目版本
	Version string `mapstructure:"version" yaml:"version"`
	// 环境模式 dev | test | staging | prod
	Env string `mapstructure:"env" yaml:"env"`
}

// Server HTTP服务配置
type Server struct {
	// 服务监听地址
	Host string `mapstructure:"host" yaml:"host"`
	// 服务监听端口
	Port int `mapstructure:"port" yaml:"port"`
	// Gin 运行模式: debug | release | test
	Mode string `mapstructure:"mode" yaml:"mode"`
}

// Log 项目日志配置 (slog)
type Log struct {
	// 日志级别: debug | info | warn | error
	Level string `mapstructure:"level" yaml:"level"`
	// 日志格式: json | text
	Format string `mapstructure:"format" yaml:"format"`
	// 是否记录源码位置（文件名 + 行号）
	AddSource bool `mapstructure:"add_source" yaml:"add_source"`
	// 日志文件路径，空值表示输出到 stdout
	FilePath string `mapstructure:"file_path" yaml:"file_path"`
	// 单个日志文件最大大小（MB）
	MaxSize int `mapstructure:"max_size" yaml:"max_size"`
	// 保留的旧日志文件数量
	MaxBackups int `mapstructure:"max_backups" yaml:"max_backups"`
	// 日志保留天数
	MaxAge int `mapstructure:"max_age" yaml:"max_age"`
	// 是否压缩旧日志文件
	Compress bool `mapstructure:"compress" yaml:"compress"`
}

// Redis 缓存配置 (go-redis)
type Redis struct {
	// 地址（host:port）
	Addr string `mapstructure:"addr" yaml:"addr"`
	// 密码
	Password string `mapstructure:"password" yaml:"password"`
	// 数据库编号
	Database int `mapstructure:"database" yaml:"database"`
	// 连接池最大连接数
	PoolSize int `mapstructure:"pool_size" yaml:"pool_size"`
	// 最小空闲连接数
	MinIdleConns int `mapstructure:"min_idle_conns" yaml:"min_idle_conns"`
	// 空闲连接最大存活时间
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time" yaml:"conn_max_idle_time"`
	// 连接最大存活时间
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime" yaml:"conn_max_lifetime"`
	// 建立连接超时
	DialTimeout time.Duration `mapstructure:"dial_timeout" yaml:"dial_timeout"`
	// 读超时
	ReadTimeout time.Duration `mapstructure:"read_timeout" yaml:"read_timeout"`
	// 写超时
	WriteTimeout time.Duration `mapstructure:"write_timeout" yaml:"write_timeout"`
	// 连接池获取连接超时
	PoolTimeout time.Duration `mapstructure:"pool_timeout" yaml:"pool_timeout"`
}

// Qdrant 数据库配置
type Qdrant struct {
	// qdrant 服务主机地址
	Host string `mapstructure:"host" yaml:"host"`
	// Qdrant gRPC 端口，默认 6334
	GRPCPort int `mapstructure:"grpc_port" yaml:"grpc_port"`
	// Qdrant REST API 端口，默认 6333
	RESTPort int `mapstructure:"rest_port" yaml:"rest_port"`
	// 服务端鉴权 key，空表示不启用
	APIKey string `mapstructure:"api_key" yaml:"api_key"`
	// 跳过客户端与服务端版本兼容性校验，生产环境建议关闭
	SkipCompatCheck bool `mapstructure:"skip_compat_check" yaml:"skip_compat_check"`
	// gRPC 连接池大小，0 使用默认值 3，推荐值: 预期并发数 / 2
	PoolSize uint `mapstructure:"pool_size" yaml:"pool_size"`
	// TCP KeepAlive 探测间隔（秒）
	KeepAliveTime int `mapstructure:"keep_alive_time" yaml:"keep_alive_time"`
	// TCP KeepAlive 探测超时（秒）
	KeepAliveTimeout uint `mapstructure:"keep_alive_timeout" yaml:"keep_alive_timeout"`
	// gRPC 调用最大重试次数，0 不重试，由业务逻辑自行处理
	MaxRetries uint `mapstructure:"max_retries" yaml:"max_retries"`
	// gRPC 拨号超时（秒）
	DialTimeout time.Duration `mapstructure:"dial_timeout" yaml:"dial_timeout"`
	// gRPC 空闲连接释放超时（秒）
	DialIdleTimeout time.Duration `mapstructure:"dial_idle_timeout" yaml:"dial_idle_timeout"`
	// 集合列表
	Collections []string `mapstructure:"collections" yaml:"collections"`
}

// Langfuse 观测组件配置
type Langfuse struct {
	// 是否开启
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
	// Langfuse 服务地址，自托管填内网域名
	Host string `mapstructure:"host" yaml:"host"`
	// 项目公钥
	PublicKey string `mapstructure:"public_key" yaml:"public_key"`
	// 项目私钥
	SecretKey string `mapstructure:"secret_key" yaml:"secret_key"`
	// 异步上报 worker 数，RAG 灌库期建议 4~8
	Threads int `mapstructure:"threads" yaml:"threads"`
	// 单次 HTTP 上报超时，防 Langfuse 抖动拖死主链路
	Timeout time.Duration `mapstructure:"timeout" yaml:"timeout"`
	// 内存队列上限，灌库期防丢 trace
	MaxTaskQueueSize int `mapstructure:"max_task_queue_size" yaml:"max_task_queue_size"`
	// 内存队列上限，灌库期防丢 trace
	MaxEventSizeBytes int `mapstructure:"max_event_size_bytes" yaml:"max_event_size_bytes"`
	// 攒够 N 条事件批量发送，降低请求数
	FlushAt int `mapstructure:"flush_at" yaml:"flush_at"`
	// 定时刷盘间隔，拉长减少空转
	FlushInterval time.Duration `mapstructure:"flush_interval" yaml:"flush_interval"`
	// 采样率，压测可调低
	SampleRate float64 `mapstructure:"sample_rate" yaml:"sample_rate"`
	// 上报失败重试次数
	MaxRetry uint64 `mapstructure:"max_retry" yaml:"max_retry"`
}

// 大模型
type ModelService struct {
	// 模型供应商，prompt 模板需要严格对应，否则无法正常调用
	Provider string `mapstructure:"provider" yaml:"provider"`
	// 是否启用
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
	// 可用模型标识符，模型类型 chat、embedding、reranker 走openai api，agent 走 anthropic api
	ChatModels      []string `mapstructure:"chat_models" yaml:"chat_models"`
	EmbeddingModels []string `mapstructure:"embedding_models" yaml:"embedding_models"`
	RerankerModels  []string `mapstructure:"reranker_models" yaml:"reranker_models"`
	AgentModels     []string `mapstructure:"agent_models" yaml:"agent_models"`
	// 供应商提供的 API Key
	APIKey string `mapstructure:"api_key" yaml:"api_key"`
	// 供应商提供的 API 端点
	BaseURL string `mapstructure:"base_url" yaml:"base_url"`
	// 请求失败最大重试次数
	MaxRetries *int `mapstructure:"max_retries" yaml:"max_retries"`
	// 请求超时时长
	Timeout time.Duration `mapstructure:"timeout" yaml:"timeout"`
	// 业务语义标签，自动合并后上报 Langfuse 用于过滤和聚合
	Tags []string `mapstructure:"tags" yaml:"tags"`
}

// Chat场景应用配置
type ChatModelPrompt struct {
	// 场景标识符，即对话分组title
	UseCase string `mapstructure:"use_case" yaml:"use_case"`
	// 是否启用该场景应用
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
	// 提示词与模型参数配置文件路径
	PromptPath string `mapstructure:"prompt_path" yaml:"prompt_path"`
	// prompt 渲染模板文件路径
	MessagesAssistantPath string `mapstructure:"messages_assistant_path" yaml:"messages_assistant_path"`
	MessagesSystemPath    string `mapstructure:"messages_system_path" yaml:"messages_system_path"`
	MessagesUserPath      string `mapstructure:"messages_user_path" yaml:"messages_user_path"`
	// 是否外挂知识库
	UseKnowledge bool `mapstructure:"use_knowledge" yaml:"use_knowledge"`
	// 除了 dialogue_memory 之外其他知识库集合名称
	KnowledgeCollections []string `mapstructure:"knowledge_collections" yaml:"knowledge_collections"`
	// 提示词与模型参数配置版本，在配置文件中进行定义和管理，允许加载多个版本的配置
	PromptVersion string `mapstructure:"prompt_version" yaml:"prompt_version"`
	// 业务语义标签，自动合并后上报 Langfuse 用于过滤和聚合
	Tags []string `mapstructure:"tags" yaml:"tags"`
	// 描述信息，用于文档说明和 Langfuse trace 展示
	Description string `mapstructure:"description" yaml:"description"`
	// prompt 预设值，key是版本编号，分开加载，使用指针是为了直接接收 Unmarshal 返回的指针结果，避免多余的解引用拷贝
	PresetValues map[string]*ChatModelPromptParams
}

type PromptParams struct {
	// 角色
	Role string `mapstructure:"role" yaml:"role"`
	// 上下文
	Context []string `mapstructure:"context" yaml:"context"`
	// 回答限制
	Constraints []string `mapstructure:"constraints" yaml:"constraints"`
	// 输出格式限制
	OutputFormat []string `mapstructure:"output_format" yaml:"output_format"`
}

// ModelPrompts 绑定模型
type ModelParams struct {
	// 模型供应商
	Provider string `mapstructure:"provider" yaml:"provider"`
	// 只能设置模型供应商已提供的模型
	Model string `mapstructure:"model" yaml:"model"`
	// 模型上下文大小，max_completion_tokens必须小于context_window
	ContextWindow *int `mapstructure:"context_window" yaml:"context_window"`
	// 采样温度，nil 说明未设置
	Temperature *float32 `mapstructure:"temperature" yaml:"temperature"`
	// 核采样参数，nil 说明未设置
	TopP *float32 `mapstructure:"top_p" yaml:"top_p"`
	// 最大生成 token 数
	MaxCompletionTokens *int `mapstructure:"max_completion_tokens" yaml:"max_completion_tokens"`
	// 供应商模型特有参数，使用供应商参数名直接定义，不一致将会导致参数不生效或者api调用报错
	ExtraParams *map[string]any `mapstructure:"extra_params" yaml:"extra_params"`
	// RAG 可调参数
	RagParams RagParams `mapstructure:"rag_params" yaml:"rag_params"`
}

// RAG 拓展参数
type RagParams struct {
	// 向量库检索结果粗排
	RetrievalTopK int32 `mapstructure:"retrieval_top_k" yaml:"retrieval_top_k"`
	// Rerank 精排
	RerankTopK int32 `mapstructure:"rerank_top_k" yaml:"rerank_top_k"`
	// 相关性阈值, 低于此分数的直接过滤掉
	ScoreThreshold float32 `mapstructure:"score_threshold" yaml:"score_threshold"`
}

type PresetValues struct {
	Values map[string]*ChatModelPromptParams `mapstructure:"preset_values" yaml:"preset_values"`
}

// 提示词与模型参数配置
type ChatModelPromptParams struct {
	PromptParams PromptParams `mapstructure:"prompt_params" yaml:"prompt_params"`
	ModelParams  ModelParams  `mapstructure:"model_params" yaml:"model_params"`
}

func bindExceptionEnv(v *viper.Viper) {
	v.BindEnv("model_services", "")
}
