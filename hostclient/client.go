package hostclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const AuthorizationHeader = "X-AIHub-Host-Authorization"

type User struct {
	ID     int `json:"id"`
	Role   int `json:"role"`
	Status int `json:"status"`
}
type dataHostURLKey struct{}

func WithDataHostBaseURL(ctx context.Context, value string) (context.Context, error) {
	if err := ValidateBaseURL(value); err != nil {
		return nil, err
	}
	return context.WithValue(ctx, dataHostURLKey{}, strings.TrimRight(value, "/")), nil
}

func (c *Client) DataHostBaseURLFor(ctx context.Context) string {
	if value, ok := ctx.Value(dataHostURLKey{}).(string); ok {
		return value
	}
	return c.DataHostBaseURL()
}

type Client struct {
	mu          sync.RWMutex
	BaseURL     string
	DataBaseURL string
	ConfigPath  string
	HTTP        *http.Client
}

func ValidateBaseURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("host_url 必须为 HTTP/HTTPS origin")
	}
	return nil
}

func New(baseURL, configPath string, dataBaseURLs ...string) (*Client, error) {
	if err := ValidateBaseURL(baseURL); err != nil {
		return nil, err
	}
	dataBaseURL := ""
	if len(dataBaseURLs) > 0 && dataBaseURLs[0] != "" {
		if err := ValidateBaseURL(dataBaseURLs[0]); err != nil {
			return nil, err
		}
		dataBaseURL = strings.TrimRight(dataBaseURLs[0], "/")
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), DataBaseURL: dataBaseURL, ConfigPath: configPath, HTTP: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) HostBaseURL() string { c.mu.RLock(); defer c.mu.RUnlock(); return c.BaseURL }
func (c *Client) DataHostBaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.DataBaseURL != "" {
		return c.DataBaseURL
	}
	return c.BaseURL
}

func (c *Client) SetBaseURL(value string) error {
	if err := ValidateBaseURL(value); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.setBaseURLsLocked(strings.TrimRight(value, "/"), c.DataBaseURL)
}

func (c *Client) SetBaseURLs(control, data string) error {
	if err := ValidateBaseURL(control); err != nil {
		return err
	}
	if data != "" {
		if err := ValidateBaseURL(data); err != nil {
			return err
		}
		data = strings.TrimRight(data, "/")
	}
	control = strings.TrimRight(control, "/")
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.setBaseURLsLocked(control, data)
}

func (c *Client) setBaseURLsLocked(control, data string) error {
	if c.BaseURL == control && c.DataBaseURL == data {
		return nil
	}
	if c.ConfigPath != "" {
		body, err := os.ReadFile(c.ConfigPath)
		if err != nil {
			return err
		}
		var config map[string]any
		if err = yaml.Unmarshal(body, &config); err != nil {
			return err
		}
		if config == nil {
			config = map[string]any{}
		}
		host, _ := config["host"].(map[string]any)
		if host == nil {
			host = map[string]any{}
		}
		host["base_url"] = control
		host["data_base_url"] = data
		config["host"] = host
		body, err = yaml.Marshal(config)
		if err != nil {
			return err
		}
		temporary := c.ConfigPath + ".runtime.tmp"
		if err = os.WriteFile(temporary, body, 0600); err != nil {
			return err
		}
		if err = os.Rename(temporary, c.ConfigPath); err != nil {
			_ = os.Remove(temporary)
			return err
		}
	}
	c.BaseURL, c.DataBaseURL = control, data
	return nil
}

// Self always uses the current request's delegated identity. No role cache or
// service credential can grant access to another user's execution records.
func (c *Client) Self(ctx context.Context, identity string) (User, error) {
	var user User
	if identity == "" {
		return user, errors.New("缺少宿主委托身份")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.HostBaseURL()+"/api/user/self", nil)
	if err != nil {
		return user, err
	}
	req.Header.Set("Authorization", "Bearer "+identity)
	req.Header.Set("X-AIHub-Plugin-ID", "example")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return user, errors.New("宿主权限查询不可用")
	}
	defer resp.Body.Close()
	var envelope struct {
		Success bool `json:"success"`
		Data    User `json:"data"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope) != nil || !envelope.Success {
		return user, errors.New("宿主权限查询失败")
	}
	return envelope.Data, nil
}
