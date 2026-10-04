package config

import (
	"github.com/spf13/viper"
)

// setDefaults 设置配置项默认参数
func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "LeotureAI")
	v.SetDefault("app.version", "0.0.1")
	v.SetDefault("app.env", "prod")

	v.SetDefault("server.host", "127.0.0.1")
	v.SetDefault("server.port", 8088)
	v.SetDefault("server.mode", "release")

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
	v.SetDefault("log.add_source", false)
	v.SetDefault("log.file_path", "leoture.log")
	v.SetDefault("log.max_size", 10)
	v.SetDefault("log.max_backups", 10)
	v.SetDefault("log.max_age", 3)
	v.SetDefault("log.compress", false)

	v.SetDefault("redis.enabled", false)
	v.SetDefault("redis.addr", "127.0.0.1:6379")
	v.SetDefault("redis.password", "leoture_cache")
	v.SetDefault("redis.database", 0)
	v.SetDefault("redis.pool_size", 10)
	v.SetDefault("redis.min_idle_conns", 5)
	v.SetDefault("redis.conn_max_lifetime", "1h")
	v.SetDefault("redis.conn_max_idle_time", "5m")
	v.SetDefault("redis.dial_timeout", "5s")
	v.SetDefault("redis.read_timeout", "3s")
	v.SetDefault("redis.write_timeout", "3s")
	v.SetDefault("redis.pool_timeout", "4s")

	v.SetDefault("qdrant.host", "127.0.0.1")
	v.SetDefault("qdrant.grpc_port", "6334")
	v.SetDefault("qdrant.grpc_port", "6333")
	v.SetDefault("qdrant.api_key", "")
	v.SetDefault("qdrant.skip_compat_check", false)
	v.SetDefault("qdrant.pool_size", 4)
	v.SetDefault("qdrant.keep_alive_time", 10)
	v.SetDefault("qdrant.keep_alive_timeout", 2)
	v.SetDefault("qdrant.max_retries", 0)
	v.SetDefault("qdrant.dial_timeout", 3)
	v.SetDefault("qdrant.dial_idle_timeout", 45)
	v.SetDefault("qdrant.collections", make([]string, 0))

	v.SetDefault("langfuse.enabled", false)
	v.SetDefault("langfuse.host", false)
	v.SetDefault("langfuse.public_key", "")
	v.SetDefault("langfuse.secret_key", "")
	v.SetDefault("langfuse.threads", 4)
	v.SetDefault("langfuse.timeout", "10s")
	v.SetDefault("langfuse.max_task_queue_size", 2000)
	v.SetDefault("langfuse.max_event_size_bytes", 2000000)
	v.SetDefault("langfuse.flush_at", 15)
	v.SetDefault("langfuse.flush_interval", "3s")
	v.SetDefault("langfuse.sample_rate", 1.0)
	v.SetDefault("langfuse.max_retry", 3)
}
