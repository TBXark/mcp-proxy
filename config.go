package main

import (
	"crypto/tls"
	"errors"
	"net/http"
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

type MCPServerType string

const (
	MCPServerTypeSSE        MCPServerType = "sse"
	MCPServerTypeStreamable MCPServerType = "streamable-http"
)

// ---- V2 ----

type ToolFilterMode string

const (
	ToolFilterModeAllow ToolFilterMode = "allow"
	ToolFilterModeBlock ToolFilterMode = "block"
)

type ToolFilterConfig struct {
	Mode ToolFilterMode `json:"mode,omitempty"`
	List []string       `json:"list,omitempty"`
}

type UserFilterMode string

const (
	UserFilterModeAllow UserFilterMode = "allow"
	UserFilterModeBlock UserFilterMode = "block"
)

type UserFilterConfig struct {
	Mode UserFilterMode `json:"mode,omitempty"`
	List []string       `json:"list,omitempty"`
}

// IsUserAllowed checks if a user is allowed based on the user filter configuration
func (ufc *UserFilterConfig) IsUserAllowed(username string) bool {
	if ufc == nil || username == "" {
		// No filter configured or empty username - allow by default
		return true
	}

	// Check if username is in the list
	userInList := false
	for _, user := range ufc.List {
		if user == username {
			userInList = true
			break
		}
	}

	switch ufc.Mode {
	case UserFilterModeAllow:
		// Allow mode: user must be in the list to be allowed
		return userInList
	case UserFilterModeBlock:
		// Block mode: user must NOT be in the list to be allowed
		return !userInList
	default:
		// No mode specified or unknown mode - allow by default
		return true
	}
}

type OAuth2Config struct {
	Enabled           bool              `json:"enabled,omitempty"`
	Users             map[string]string `json:"users,omitempty"`
	PersistenceDir    string            `json:"persistenceDir,omitempty"`
	AllowedIPs        []string          `json:"allowedIPs,omitempty"`
	TokenExpirationMinutes int           `json:"tokenExpirationMinutes,omitempty"`
	DisableTokenExpiration bool          `json:"disableTokenExpiration,omitempty"`
	TemplateDir       string            `json:"templateDir,omitempty"`
}

type OptionsV2 struct {
	PanicIfInvalid optional.Field[bool] `json:"panicIfInvalid,omitempty"`
	LogEnabled     optional.Field[bool] `json:"logEnabled,omitempty"`
	AuthTokens     []string             `json:"authTokens,omitempty"`
	OAuth2         *OAuth2Config        `json:"oauth2,omitempty"`
	ToolFilter     *ToolFilterConfig    `json:"toolFilter,omitempty"`
	UserFilter     *UserFilterConfig    `json:"userFilter,omitempty"`
}

type MCPProxyConfigV2 struct {
	BaseURL string        `json:"baseURL"`
	Addr    string        `json:"addr"`
	Name    string        `json:"name"`
	Version string        `json:"version"`
	Type    MCPServerType `json:"type,omitempty"`
	Options *OptionsV2    `json:"options,omitempty"`
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

type FullConfig struct {
	DeprecatedServerV1  *MCPProxyConfigV1             `json:"server"`
	DeprecatedClientsV1 map[string]*MCPClientConfigV1 `json:"clients"`

	McpProxy   *MCPProxyConfigV2             `json:"mcpProxy"`
	McpServers map[string]*MCPClientConfigV2 `json:"mcpServers"`
}

func load(path string, insecure bool) (*Config, error) {
	httpClient := http.DefaultClient
	if insecure {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		httpClient = &http.Client{Transport: transport}
	}

	conf, err := confstore.Load[FullConfig](
		path,
		confstore.WithHTTPClientOption(httpClient),
	)
	if err != nil {
		return nil, err
	}
	adaptMCPClientConfigV1ToV2(conf)

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
		if clientConfig.Options.OAuth2 == nil && conf.McpProxy.Options.OAuth2 != nil {
			clientConfig.Options.OAuth2 = conf.McpProxy.Options.OAuth2
		}
		if !clientConfig.Options.PanicIfInvalid.Present() {
			clientConfig.Options.PanicIfInvalid = conf.McpProxy.Options.PanicIfInvalid
		}
		if !clientConfig.Options.LogEnabled.Present() {
			clientConfig.Options.LogEnabled = conf.McpProxy.Options.LogEnabled
		}
	}

	if conf.McpProxy.Type == "" {
		conf.McpProxy.Type = MCPServerTypeSSE // default to SSE
	}

	return &Config{
		McpProxy:   conf.McpProxy,
		McpServers: conf.McpServers,
	}, nil
}
