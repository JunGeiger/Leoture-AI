package prompt

import (
	"fmt"
	"os"
	"sync"

	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
	"github.com/golang/groupcache/singleflight"
)

var (
	// templateCache 缓存已解析的 ChatTemplate
	// key 格式: "role:filepath"
	templateCache sync.Map

	// tmplGroup 防止缓存击穿
	templateGroup singleflight.Group
)

// GetChatTemplate 获取消息模板，模板语法固定为 schema.GoTemplate
// 模板本身不支持热更新和版本管理，缓存生命周期与进程一致
func GetChatTemplate(role schema.RoleType, tmplPath string) (prompt.ChatTemplate, error) {
	if role == "" || tmplPath == "" {
		return nil, fmt.Errorf("message role or template path is empty")
	}
	// 尝试从缓存加载
	if cached, ok := templateCache.Load(tmplPath); ok {
		if tmpl, ok := cached.(prompt.ChatTemplate); ok {
			// 类型异常，清除缓存，触发重新加载
			templateCache.Delete(tmplPath)
			return tmpl, nil
		}
		return nil, fmt.Errorf("invalid template type in cache, cleared")
	}

	// 使用 singleflight 确保同一路径只解析一次
	v, err := templateGroup.Do(tmplPath, func() (any, error) {
		// 读取模板文件内容，支持上下文取消
		content, err := readFileContent(tmplPath)
		if err != nil {
			return nil, fmt.Errorf("read template file %s failed: %w", tmplPath, err)
		}

		// 构建消息模板，模板语法固定为 schema.GoTemplate
		tmpl := prompt.FromMessages(schema.GoTemplate, &schema.Message{
			Role:    role,
			Content: content,
		})

		// 存入缓存
		templateCache.Store(tmplPath, tmpl)

		return tmpl, nil
	})

	if err != nil {
		return nil, err
	}

	if tmpl, ok := v.(prompt.ChatTemplate); ok {
		return tmpl, nil

	}
	return nil, fmt.Errorf("invalid template type returned from singleflight")
}

// readFileContent 读取文件内容
func readFileContent(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("template path is empty")
	}

	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read template file %s failed: %w", path, err)
	}

	return string(contentBytes), nil
}
