package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Global configuration object.
//
//	全局配置对象
var (
	cfg  = atomic.Value{}
	once sync.Once
)

// SetupViper Initialize Viper configuration data.
//
//	初始化配置数据
func SetupViper() (*Config, error) {
	var setupErr error
	var c *Config
	once.Do(func() {
		v := viper.New()

		// 配置文件类型
		v.SetConfigType("yaml")

		// 设置配置文件路径
		v.SetConfigFile("./config/config.yaml")

		// 校验并设置默认值 (优先级：最低)
		setDefaults(v)

		// 读取配置 (优先级：中)
		if err := v.ReadInConfig(); err != nil {
			setupErr = fmt.Errorf("viper: config read failed: %w", err)
			return
		}

		// 读取环境变量 (优先级：高)
		// 加载 .env（开发环境用，生产环境用系统环境变量）
		err := godotenv.Load(".env")
		if err != nil {
			slog.Warn("godotenv: not found .env file", slog.String("error", err.Error()))
			// 环境变量前缀
			v.SetEnvPrefix("LEOTURE")
		}
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		v.AutomaticEnv()

		// 加载配置数据
		if err := loadConfig(v); err != nil {
			setupErr = fmt.Errorf("viper: config unmarshal or verification failed: %w", err)
			return
		}
		c = cfg.Load().(*Config)

		// 开启热加载
		v.WatchConfig()
		v.OnConfigChange(func(e fsnotify.Event) {
			if e.Op&fsnotify.Write != 0 {
				slog.Warn("viper: config file changed", slog.String("path", e.Name))
				if err := loadConfig(v); err != nil {
					slog.Error("viper: failed to reload config", slog.String("error", err.Error()))
					return
				}
				slog.Warn("viper: config hot reloading is complete")
			}
		})
	})
	if setupErr != nil {
		return nil, setupErr
	}
	return c, nil
}

// verify 配置校验
func verify(c Config) error {
	for i, ms := range c.ModelServices {
		if ms.Provider == "" {
			return fmt.Errorf("viper: model_service provider is empty, index: %d", i)
		}
		if len(ms.ChatModels) == 0 {
			return fmt.Errorf("viper: model_service does not contain any models, provider: %s", ms.Provider)
		}
		if ms.APIKey == "" {
			return fmt.Errorf("viper: model_service api_key is empty, provider: %s", ms.Provider)
		}
		if ms.BaseURL == "" {
			return fmt.Errorf("viper: model_service base_url is empty, provider: %s", ms.Provider)
		}
		if ms.Timeout == 0 {
			return fmt.Errorf("viper: model_service api timeout can not set 0, provider: %s", ms.Provider)
		}
		if ms.MaxRetries == nil || *ms.MaxRetries > 3 {
			return fmt.Errorf("viper: model_service api max_retries can not set nil or > 3, provider: %s", ms.Provider)
		}
	}
	return nil
}

// loadConfig 加载配置
func loadConfig(v *viper.Viper) error {
	// 主配置数据
	c := &Config{}
	if err := v.Unmarshal(c); err != nil {
		return err
	}

	// 递归替换所有 ${VAR} 占位符
	c.ModelServices = expandModelServices(c.ModelServices)

	if err := verify(*c); err != nil {
		return err
	}

	// 加载预置提示词内容，当需要热加载内容时，
	// 请先对chat_model_prompts enabled设置为false，
	// 修改完毕以后改回 true，将会自动加载新内容
	for i := 0; i < len(c.ChatModelPrompts); i++ {
		if c.ChatModelPrompts[i].Enabled {
			pp, err := loadPromptPreset(c.ChatModelPrompts[i].PromptPath)
			if err != nil {
				return err
			}
			c.ChatModelPrompts[i].PresetValues = pp.Values
		}
	}

	// 原子替换，保证线上请求安全
	cfg.Store(c)
	return nil
}

func loadPromptPreset(path string) (*PresetValues, error) {
	if path == "" {
		return nil, errors.New("viper: model and prompt params template file path is empty")
	}
	v := viper.New()
	v.SetConfigFile(path)
	// 读取配置
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("viper: model and prompt config read failed: %w", err)
	}
	p := &PresetValues{}
	if err := v.Unmarshal(p); err != nil {
		return nil, err
	}
	return p, nil
}

func currentConfigs() Config {
	cfg := cfg.Load().(*Config)
	return *cfg
}

// 获取所有云端LLM预置提示词
func GetPropmptMap() map[string]*ChatModelPrompt {
	cfg := currentConfigs()
	chatModelPromptMap := make(map[string]*ChatModelPrompt, len(cfg.ChatModelPrompts))
	for i := 0; i < len(cfg.ChatModelPrompts); i++ {
		chatModelPromptMap[cfg.ChatModelPrompts[i].UseCase] = &cfg.ChatModelPrompts[i]
	}
	return chatModelPromptMap
}

func expandEnvOrError(s string) string {
	return os.Expand(s, func(key string) string {
		v, ok := os.LookupEnv(key)
		if !ok {
			panic(fmt.Sprintf("环境变量未设置: %s", key))
		}
		return v
	})
}

// 替换环境变量占位符为实际值，仅支持 string 类型的配置数据
func expandEnvVars(s string) string {
	envVarPattern := regexp.MustCompile(`\$\{([^}]+)\}`)
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := envVarPattern.FindStringSubmatch(match)[1]
		val, ok := os.LookupEnv(varName)
		if !ok {
			panic(fmt.Sprintf("环境变量未设置: %s", varName))
		}
		return val
	})
}

func expandModelServices(services []ModelService) []ModelService {
	for i := range services {
		services[i].APIKey = expandEnvVars(services[i].APIKey)
		services[i].BaseURL = expandEnvVars(services[i].BaseURL)
	}
	return services
}
