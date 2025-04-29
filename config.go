package main

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/TBXark/confstore"
	"github.com/TBXark/optional-go"
)

type StdioMCPClientConfig struct {
	Command string            `json:"command"`
	Env     map[string]string `json:"env"`
	Args    []string          `json:"args"`
}

type SSEMCPClientConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

type StreamableMCPClientConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Timeout time.Duration     `json:"timeout"`
}

type MCPClientType string

const (
	MCPClientTypeStdio      MCPClientType = "stdio"
	MCPClientTypeSSE        MCPClientType = "sse"
	MCPClientTypeStreamable MCPClientType = "streamable-http"
)

// ---- V1 ----

type MCPClientConfigV1 struct {
	Type           MCPClientType   `json:"type"`
	Config         json.RawMessage `json:"config"`
	PanicIfInvalid bool            `json:"panicIfInvalid"`
	LogEnabled     bool            `json:"logEnabled"`
	AuthTokens     []string        `json:"authTokens"`
}

type MCPProxyConfigV1 struct {
	BaseURL          string   `json:"baseURL"`
	Addr             string   `json:"addr"`
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	GlobalAuthTokens []string `json:"globalAuthTokens"`
}

func parseMCPClientConfigV1(conf *MCPClientConfigV1) (any, error) {
	switch conf.Type {
	case MCPClientTypeStdio:
		var config StdioMCPClientConfig
		err := json.Unmarshal(conf.Config, &config)
		if err != nil {
			return nil, err
		}
		return &config, nil
	case MCPClientTypeSSE:
		var config SSEMCPClientConfig
		err := json.Unmarshal(conf.Config, &config)
		if err != nil {
			return nil, err
		}
		return &config, nil
	case MCPClientTypeStreamable:
		var config StreamableMCPClientConfig
		err := json.Unmarshal(conf.Config, &config)
		if err != nil {
			return nil, err
		}
		return &config, nil
	default:
		return nil, errors.New("invalid client type")
	}
}

// ---- V2 ----

type ToolFilterConfig struct {
	Mode string   `json:"mode,omitempty"` // 过滤模式 ("allow" or "block")
	List []string `json:"list,omitempty"` // 用于过滤的工具列表
}

type OptionsV2 struct {
	PanicIfInvalid optional.Field[bool] `json:"panicIfInvalid,omitempty"`
	LogEnabled     optional.Field[bool] `json:"logEnabled,omitempty"`
	AuthTokens     []string             `json:"authTokens,omitempty"`
	ToolFilter     *ToolFilterConfig    `json:"toolFilter,omitempty"` // 工具过滤配置
}

type MCPProxyConfigV2 struct {
	BaseURL string     `json:"baseURL"`
	Addr    string     `json:"addr"`
	Name    string     `json:"name"`
	Version string     `json:"version"`
	Options *OptionsV2 `json:"options,omitempty"`
}

type MCPClientConfigV2 struct {
	TransportType MCPClientType `json:"transportType,omitempty"`

	// Stdio
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// SSE or Streamable HTTP
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Timeout time.Duration     `json:"timeout,omitempty"`

	Options *OptionsV2 `json:"options,omitempty"`
}

func parseMCPClientConfigV2(conf *MCPClientConfigV2) (any, error) {
	if conf.Command != "" || conf.TransportType == MCPClientTypeStdio {
		if conf.Command == "" {
			return nil, errors.New("command is required for stdio transport")
		}
		return &StdioMCPClientConfig{
			Command: conf.Command,
			Env:     conf.Env,
			Args:    conf.Args,
		}, nil
	}
	if conf.URL != "" {
		if conf.TransportType == MCPClientTypeStreamable {
			return &StreamableMCPClientConfig{
				URL:     conf.URL,
				Headers: conf.Headers,
				Timeout: conf.Timeout,
			}, nil
		} else {
			return &SSEMCPClientConfig{
				URL:     conf.URL,
				Headers: conf.Headers,
			}, nil
		}
	}
	return nil, errors.New("invalid server type")
}

// ---- Config ----

type Config struct {
	McpProxy   *MCPProxyConfigV2             `json:"mcpProxy"`
	McpServers map[string]*MCPClientConfigV2 `json:"mcpServers"`
}

func load(path string) (*Config, error) {
	type FullConfig struct {
		DeprecatedServerV1  *MCPProxyConfigV1             `json:"server"`
		DeprecatedClientsV1 map[string]*MCPClientConfigV1 `json:"clients"`

		McpProxy   *MCPProxyConfigV2             `json:"mcpProxy"`
		McpServers map[string]*MCPClientConfigV2 `json:"mcpServers"`
	}
	conf, err := confstore.Load[FullConfig](path)
	if err != nil {
		return nil, err
	}

	if conf.DeprecatedServerV1 != nil && conf.McpProxy == nil {
		v1 := conf.DeprecatedServerV1
		conf.McpProxy = &MCPProxyConfigV2{
			BaseURL: v1.BaseURL,
			Addr:    v1.Addr,
			Name:    v1.Name,
			Version: v1.Version,
			Options: &OptionsV2{
				AuthTokens: v1.GlobalAuthTokens,
			},
		}
	}

	if len(conf.DeprecatedClientsV1) > 0 && len(conf.McpServers) == 0 {
		conf.McpServers = make(map[string]*MCPClientConfigV2)
		for name, clientConfig := range conf.DeprecatedClientsV1 {
			clientInfo, cErr := parseMCPClientConfigV1(clientConfig)
			if cErr != nil {
				continue
			}
			options := &OptionsV2{
				AuthTokens: clientConfig.AuthTokens,
			}
			if conf.DeprecatedServerV1 != nil && len(conf.DeprecatedServerV1.GlobalAuthTokens) > 0 {
				options.AuthTokens = append(options.AuthTokens, conf.DeprecatedServerV1.GlobalAuthTokens...)
			}
			switch v := clientInfo.(type) {
			case *StdioMCPClientConfig:
				conf.McpServers[name] = &MCPClientConfigV2{
					Command: v.Command,
					Args:    v.Args,
					Env:     v.Env,
					Options: options,
				}
			case *SSEMCPClientConfig:
				conf.McpServers[name] = &MCPClientConfigV2{
					URL:     v.URL,
					Headers: v.Headers,
					Options: options,
				}
			case *StreamableMCPClientConfig:
				conf.McpServers[name] = &MCPClientConfigV2{
					URL:     v.URL,
					Headers: v.Headers,
					Timeout: v.Timeout,
					Options: options,
				}
			default:
				continue
			}
		}
	}
	if conf.McpProxy == nil {
		return nil, errors.New("mcpProxy is required")
	}
	if conf.McpProxy.Options == nil {
		conf.McpProxy.Options = &OptionsV2{}
	}
	for _, clientConfig := range conf.McpServers {
		if clientConfig.Options == nil {
			clientConfig.Options = &OptionsV2{}
		}
		if clientConfig.Options.AuthTokens == nil {
			clientConfig.Options.AuthTokens = conf.McpProxy.Options.AuthTokens
		}
		if !clientConfig.Options.PanicIfInvalid.Present() {
			clientConfig.Options.PanicIfInvalid = conf.McpProxy.Options.PanicIfInvalid
		}
		if !clientConfig.Options.LogEnabled.Present() {
			clientConfig.Options.LogEnabled = conf.McpProxy.Options.LogEnabled
		}
	}

	// remove deprecated fields
	conf.DeprecatedServerV1 = nil
	conf.DeprecatedClientsV1 = nil

	return &Config{
		McpProxy:   conf.McpProxy,
		McpServers: conf.McpServers,
	}, nil
}
