package deltaapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"YHagent/internal/config"
	"YHagent/internal/data/cache"
)

const (
	// DefaultAPIBase 三角洲开放平台默认API地址
	DefaultAPIBase = "https://api.delta行动.com/v1" // 实际地址根据文档替换
	// RequestTimeout HTTP请求超时
	RequestTimeout = 5 * time.Second
	// CacheExpire 数据缓存时间，静态数据缓存1小时
	CacheExpire = 1 * time.Hour
)

// Client 三角洲API客户端
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
	cache   *cache.Cache
}

// NewClient 创建三角洲API客户端
func NewClient(apiKey, baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultAPIBase
	}
	return &Client{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout: RequestTimeout,
		},
		cache: cache.NewCache(),
	}
}

// doRequest 统一HTTP请求封装，自动带认证头、解析响应、缓存
func (c *Client) doRequest(method, path string, result interface{}) error {
	cacheKey := method + ":" + path
	// GET请求优先读缓存
	if method == http.MethodGet {
		if v, ok := c.cache.Get(cacheKey); ok {
			// 缓存命中直接反序列化返回
			jsonBytes, err := json.Marshal(v)
			if err == nil {
				if err := json.Unmarshal(jsonBytes, result); err == nil {
					return nil
				}
			}
		}
	}

	url := c.baseURL + path
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	// 认证头
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API返回错误 %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	// GET请求写入缓存
	if method == http.MethodGet {
		var cacheVal interface{}
		_ = json.Unmarshal(body, &cacheVal)
		c.cache.Set(cacheKey, cacheVal, CacheExpire)
	}

	return nil
}
