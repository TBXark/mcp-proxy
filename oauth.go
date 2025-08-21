package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// OAuth 2.1 Server Implementation for MCP

type OAuthServer struct {
	baseURL         string
	clients         map[string]*OAuthClient
	authCodes       map[string]*AuthorizationCode
	accessTokens    map[string]*AccessToken
	mutex           sync.RWMutex
	tokenExpiration time.Duration
	persistenceFile string
	accessConfig    *OAuth2Config
	templates       *template.Template
}

type OAuthClient struct {
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret,omitempty"`
	RedirectURIs []string  `json:"redirect_uris"`
	GrantTypes   []string  `json:"grant_types"`
	CreatedAt    time.Time `json:"client_id_issued_at"`
	ClientName   string    `json:"client_name,omitempty"`
}

type AuthorizationCode struct {
	Code          string
	ClientID      string
	RedirectURI   string
	Scope         string
	CodeChallenge string // PKCE challenge
	ExpiresAt     time.Time
	Resource      string
}

type AccessToken struct {
	Token        string
	RefreshToken string
	ClientID     string
	Scope        string
	Resource     string
	ExpiresAt    time.Time
}

// OAuth Server Metadata Response
type ServerMetadata struct {
	Issuer                             string   `json:"issuer"`
	AuthorizationEndpoint              string   `json:"authorization_endpoint"`
	TokenEndpoint                      string   `json:"token_endpoint"`
	RegistrationEndpoint               string   `json:"registration_endpoint"`
	ScopesSupported                    []string `json:"scopes_supported"`
	ResponseTypesSupported             []string `json:"response_types_supported"`
	GrantTypesSupported                []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported  []string `json:"token_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported      []string `json:"code_challenge_methods_supported"`
}

// Dynamic Client Registration Request
type ClientRegistrationRequest struct {
	RedirectURIs []string `json:"redirect_uris"`
	GrantTypes   []string `json:"grant_types,omitempty"`
	Scope        string   `json:"scope,omitempty"`
	ClientName   string   `json:"client_name,omitempty"`
}

// Dynamic Client Registration Response
type ClientRegistrationResponse struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret,omitempty"`
	RedirectURIs []string `json:"redirect_uris"`
	GrantTypes   []string `json:"grant_types"`
	CreatedAt    int64    `json:"client_id_issued_at"`
}

// Token Request
type TokenRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code,omitempty"`
	RedirectURI  string `json:"redirect_uri,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	CodeVerifier string `json:"code_verifier,omitempty"`
	Resource     string `json:"resource,omitempty"`
}

// Token Response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// Error Response
type OAuthError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// Template data structures
type AuthPageData struct {
	ClientID        string
	ClientName      string
	ResourceName    string
	RedirectURI     string
	ResponseType    string
	Scope           string
	State           string
	CodeChallenge   string
	Resource        string
	ErrorMessage    string
}

type SuccessPageData struct {
	RedirectURL string
	Username    string
}


func NewOAuthServer(baseURL string, accessConfig *OAuth2Config) *OAuthServer {
	var mcpProxyDir string
	var persistenceFile string
	
	// Check if persistence directory is specified in config
	if accessConfig != nil && accessConfig.PersistenceDir != "" {
		// Use the configured directory
		mcpProxyDir = accessConfig.PersistenceDir
	} else {
		// Use default $HOME/.mcpproxy
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Printf("OAuth: Could not determine home directory, using current directory: %v", err)
			homeDir = "."
		}
		mcpProxyDir = filepath.Join(homeDir, ".mcpproxy")
	}
	
	persistenceFile = filepath.Join(mcpProxyDir, "oauth_clients.json")
	
	// Create directory if it doesn't exist
	if err := os.MkdirAll(mcpProxyDir, 0700); err != nil {
		log.Printf("OAuth: Could not create directory %s: %v", mcpProxyDir, err)
		// Fall back to current directory
		persistenceFile = "oauth_clients.json"
	}
	
	
	// Set token expiration from config or default to 1 hour
	tokenExpiration := time.Hour // Default 1 hour
	if accessConfig != nil && accessConfig.TokenExpirationMinutes > 0 {
		tokenExpiration = time.Duration(accessConfig.TokenExpirationMinutes) * time.Minute
		log.Printf("OAuth: Using custom token expiration: %v", tokenExpiration)
	}

	// Load templates with fallback mechanism
	var templates *template.Template
	var err error
	
	// Determine template directory
	templateDir := "templates/oauth"
	if accessConfig != nil && accessConfig.TemplateDir != "" {
		templateDir = filepath.Join(accessConfig.TemplateDir, "oauth")
	}
	
	// First try to load external templates (for customization)
	if _, statErr := os.Stat(templateDir); statErr == nil {
		templateGlob := filepath.Join(templateDir, "*.html")
		log.Printf("OAuth: Found external templates directory at '%s', loading custom templates", templateDir)
		templates, err = template.ParseGlob(templateGlob)
		if err != nil {
			log.Printf("OAuth: Failed to load external templates from '%s': %v", templateDir, err)
			log.Printf("OAuth: Falling back to built-in templates")
		} else {
			log.Printf("OAuth: Successfully loaded external templates from '%s'", templateDir)
		}
	}
	
	// Fall back to built-in templates if external ones failed or don't exist
	if templates == nil {
		log.Printf("OAuth: Loading built-in templates")
		templates = template.New("")
		
		// Parse built-in authorize template
		_, err = templates.New("authorize.html").Parse(defaultAuthorizePage)
		if err != nil {
			log.Printf("OAuth: Failed to parse built-in authorize template: %v", err)
			return nil
		}
		
		// Parse built-in success template  
		_, err = templates.New("success.html").Parse(defaultSuccessPage)
		if err != nil {
			log.Printf("OAuth: Failed to parse built-in success template: %v", err)
			return nil
		}
		
		log.Printf("OAuth: Successfully loaded built-in templates")
	}

	server := &OAuthServer{
		baseURL:         baseURL,
		clients:         make(map[string]*OAuthClient),
		authCodes:       make(map[string]*AuthorizationCode),
		accessTokens:    make(map[string]*AccessToken),
		tokenExpiration: tokenExpiration,
		persistenceFile: persistenceFile,
		accessConfig:    accessConfig,
		templates:       templates,
	}
	
	// Load persisted clients
	server.loadClients()
	
	return server
}

// OAuth persistence data structure
type OAuthPersistenceData struct {
	Clients      map[string]*OAuthClient `json:"clients"`
	AccessTokens map[string]*AccessToken `json:"accessTokens"`
	SavedAt      time.Time               `json:"savedAt"`
}

func (s *OAuthServer) loadClients() {
	if _, err := os.Stat(s.persistenceFile); os.IsNotExist(err) {
		return
	}
	
	data, err := os.ReadFile(s.persistenceFile)
	if err != nil {
		log.Printf("OAuth: Failed to read persistence file: %v", err)
		return
	}
	
	// Try to load new format first (with tokens)
	var persistenceData OAuthPersistenceData
	if err := json.Unmarshal(data, &persistenceData); err == nil && persistenceData.Clients != nil {
		s.mutex.Lock()
		s.clients = persistenceData.Clients
		
		// Load tokens, filtering out expired ones
		validAccessTokens := make(map[string]*AccessToken)
		
		now := time.Now()
		for token, accessToken := range persistenceData.AccessTokens {
			if accessToken.ExpiresAt.After(now) {
				validAccessTokens[token] = accessToken
			}
		}
		
		s.accessTokens = validAccessTokens
		s.mutex.Unlock()
		
		log.Printf("OAuth: Loaded %d clients, %d active access tokens", 
			len(persistenceData.Clients), len(validAccessTokens))
		return
	}
	
	// Fallback to old format (clients only) for backward compatibility
	var clients map[string]*OAuthClient
	if err := json.Unmarshal(data, &clients); err != nil {
		log.Printf("OAuth: Failed to unmarshal persistence data: %v", err)
		return
	}
	
	s.mutex.Lock()
	s.clients = clients
	s.mutex.Unlock()
	
	log.Printf("OAuth: Loaded %d persisted clients (legacy format)", len(clients))
}

func (s *OAuthServer) saveClients() {
	s.mutex.RLock()
	
	// Copy all data for persistence
	clients := make(map[string]*OAuthClient)
	for k, v := range s.clients {
		clients[k] = v
	}
	
	accessTokens := make(map[string]*AccessToken)
	for k, v := range s.accessTokens {
		accessTokens[k] = v
	}
	
	s.mutex.RUnlock()
	
	// Create persistence data structure
	persistenceData := OAuthPersistenceData{
		Clients:      clients,
		AccessTokens: accessTokens,
		SavedAt:      time.Now(),
	}
	
	data, err := json.MarshalIndent(persistenceData, "", "  ")
	if err != nil {
		log.Printf("OAuth: Failed to marshal persistence data: %v", err)
		return
	}
	
	if err := os.WriteFile(s.persistenceFile, data, 0600); err != nil {
		log.Printf("OAuth: Failed to save persistence data: %v", err)
		return
	}
	
	log.Printf("OAuth: Saved %d clients, %d access tokens to persistence file", 
		len(clients), len(accessTokens))
}

func (s *OAuthServer) generateRandomString(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)[:length]
}





// Server Metadata Discovery Handler - Per MCP Server
func (s *OAuthServer) handleServerMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract server name from path if present
	// Path format: /.well-known/oauth-authorization-server/server-name
	path := r.URL.Path
	serverName := ""
	if path != "/.well-known/oauth-authorization-server" {
		parts := strings.Split(strings.TrimPrefix(path, "/.well-known/oauth-authorization-server/"), "/")
		if len(parts) > 0 && parts[0] != "" {
			serverName = parts[0]
		}
	}

	metadata := ServerMetadata{
		Issuer:                s.baseURL,
		AuthorizationEndpoint: s.baseURL + "/oauth/authorize",
		TokenEndpoint:         s.baseURL + "/oauth/token",
		RegistrationEndpoint:  s.baseURL + "/oauth/register",
		ScopesSupported:       []string{"mcp"},
		ResponseTypesSupported: []string{"code"},
		GrantTypesSupported:   []string{"authorization_code", "refresh_token"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_post", "none"},
		CodeChallengeMethodsSupported:     []string{"S256"},
	}

	// If this is for a specific server, add server-specific metadata
	if serverName != "" {
		// Add server-specific resource URI
		metadata.Issuer = s.baseURL + "/" + serverName
		// Update endpoints to include server context
		metadata.AuthorizationEndpoint = s.baseURL + "/oauth/authorize?resource=" + url.QueryEscape(s.baseURL+"/"+serverName)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metadata)
}

// Protected Resource Metadata Handler
func (s *OAuthServer) handleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract server name from path
	// Path format: /.well-known/oauth-protected-resource/server-name
	path := r.URL.Path
	serverName := ""
	parts := strings.Split(strings.TrimPrefix(path, "/.well-known/oauth-protected-resource/"), "/")
	if len(parts) > 0 && parts[0] != "" {
		serverName = parts[0]
	}

	if serverName == "" {
		http.Error(w, "Server name required", http.StatusBadRequest)
		return
	}

	resourceMetadata := map[string]interface{}{
		"resource":                    s.baseURL + "/" + serverName,
		"authorization_servers":       []string{s.baseURL},
		"scopes_supported":           []string{"mcp"},
		"bearer_methods_supported":   []string{"header"},
		"resource_documentation":     s.baseURL + "/" + serverName + "/mcp",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resourceMetadata)
}

// Dynamic Client Registration Handler
func (s *OAuthServer) handleClientRegistration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate client IP against allowlist
	if s.accessConfig != nil && len(s.accessConfig.AllowedIPs) > 0 {
		clientIP := s.getClientIP(r)
		if !s.isIPAllowed(clientIP, s.accessConfig.AllowedIPs) {
			log.Printf("OAuth: Client registration blocked - IP %s not in allowlist %v", clientIP, s.accessConfig.AllowedIPs)
			s.writeOAuthError(w, "access_denied", "Client registration not allowed from this IP", http.StatusForbidden)
			return
		}
		log.Printf("OAuth: Client registration allowed from IP %s", clientIP)
	}

	var req ClientRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("OAuth: Client registration failed - invalid JSON: %v", err)
		s.writeOAuthError(w, "invalid_request", "Invalid JSON request", http.StatusBadRequest)
		return
	}

	log.Printf("OAuth: Client registration request: %+v", req)

	// Validate redirect URIs
	if len(req.RedirectURIs) == 0 {
		log.Printf("OAuth: Client registration failed - no redirect URIs")
		s.writeOAuthError(w, "invalid_redirect_uri", "At least one redirect URI is required", http.StatusBadRequest)
		return
	}
	
	// Validate that redirect URIs are from Claude (allowlist)
	allowedCallbackURLs := []string{
		"https://claude.ai/api/mcp/auth_callback",
		"https://claude.com/api/mcp/auth_callback", // Future URL
	}
	
	for _, uri := range req.RedirectURIs {
		validURI := false
		for _, allowed := range allowedCallbackURLs {
			if uri == allowed {
				validURI = true
				break
			}
		}
		if !validURI {
			log.Printf("OAuth: Client registration failed - invalid redirect URI: %s", uri)
			s.writeOAuthError(w, "invalid_redirect_uri", "Redirect URI not allowed", http.StatusBadRequest)
			return
		}
	}


	// Generate client credentials
	clientID := s.generateRandomString(32)
	clientSecret := s.generateRandomString(48)

	client := &OAuthClient{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURIs: req.RedirectURIs,
		GrantTypes:   []string{"authorization_code"},
		CreatedAt:    time.Now(),
		ClientName:   req.ClientName,
	}

	if len(req.GrantTypes) > 0 {
		client.GrantTypes = req.GrantTypes
	}

	s.mutex.Lock()
	s.clients[clientID] = client
	s.mutex.Unlock()

	// Save clients to persistence file
	s.saveClients()

	log.Printf("OAuth: Registered client ID: %s, redirect URIs: %v", clientID, client.RedirectURIs)

	response := ClientRegistrationResponse{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURIs: client.RedirectURIs,
		GrantTypes:   client.GrantTypes,
		CreatedAt:    client.CreatedAt.Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// Authorization Endpoint Handler
func (s *OAuthServer) handleAuthorization(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.handleAuthorizationGET(w, r)
	} else if r.Method == http.MethodPost {
		s.handleAuthorizationPOST(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *OAuthServer) handleAuthorizationGET(w http.ResponseWriter, r *http.Request) {
	// Parse authorization request
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	responseType := r.URL.Query().Get("response_type")
	scope := r.URL.Query().Get("scope")
	state := r.URL.Query().Get("state")
	codeChallenge := r.URL.Query().Get("code_challenge")
	resource := r.URL.Query().Get("resource")

	log.Printf("OAuth: Authorization request - client_id=%s, redirect_uri=%s, response_type=%s, resource=%s", 
		clientID, redirectURI, responseType, resource)

	// Validate request
	if clientID == "" || redirectURI == "" || responseType != "code" {
		log.Printf("OAuth: Authorization failed - missing parameters")
		s.writeOAuthError(w, "invalid_request", "Missing or invalid required parameters", http.StatusBadRequest)
		return
	}

	// Show authorization/consent page instead of auto-approving
	s.showAuthorizationPage(w, r, clientID, redirectURI, responseType, scope, state, codeChallenge, resource, "")
}

func (s *OAuthServer) showAuthorizationPage(w http.ResponseWriter, r *http.Request, clientID, redirectURI, responseType, scope, state, codeChallenge, resource, errorMsg string) {
	// Skip client validation at authorization endpoint per Claude DCR spec
	// Client validation will happen at token endpoint where invalid_client triggers re-registration
	log.Printf("OAuth: Authorization request for client_id '%s' - proceeding to login", clientID)

	// Show login page for authentication
	clientName := "Claude" // Default to Claude since that's the expected client
	
	resourceName := "MCP Proxy"
	if resource != "" {
		// Extract resource name from URL
		if u, err := url.Parse(resource); err == nil {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) > 0 && parts[len(parts)-1] != "" {
				resourceName = parts[len(parts)-1]
			}
		}
	}

	// Prepare template data
	data := AuthPageData{
		ClientID:      clientID,
		ClientName:    clientName,
		ResourceName:  resourceName,
		RedirectURI:   redirectURI,
		ResponseType:  responseType,
		Scope:         scope,
		State:         state,
		CodeChallenge: codeChallenge,
		Resource:      resource,
		ErrorMessage:  errorMsg,
	}

	// Execute authorization template
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	if err := s.templates.ExecuteTemplate(w, "authorize.html", data); err != nil {
		log.Printf("OAuth: Failed to execute authorization template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *OAuthServer) handleAuthorizationPOST(w http.ResponseWriter, r *http.Request) {
	// Parse form data from login page
	err := r.ParseForm()
	if err != nil {
		s.writeOAuthError(w, "invalid_request", "Failed to parse form", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	scope := r.FormValue("scope")
	state := r.FormValue("state")
	codeChallenge := r.FormValue("code_challenge")
	resource := r.FormValue("resource")

	log.Printf("OAuth: Login attempt - username=%s, client_id=%s", username, clientID)

	// Validate credentials against configuration
	if s.accessConfig == nil || s.accessConfig.Users == nil {
		log.Printf("OAuth: No users configured in OAuth2 config")
		s.writeOAuthError(w, "server_error", "Authentication not configured", http.StatusInternalServerError)
		return
	}

	expectedPassword, exists := s.accessConfig.Users[username]
	if !exists || subtle.ConstantTimeCompare([]byte(expectedPassword), []byte(password)) != 1 {
		log.Printf("OAuth: Authentication failed for username: %s", username)
		
		// Show login page again with error message
		s.showAuthorizationPage(w, r, clientID, redirectURI, "code", scope, state, codeChallenge, resource, "Invalid username or password. Please try again.")
		return
	}

	log.Printf("OAuth: Authentication successful for username: %s", username)

	// Generate authorization code after successful authentication
	code := s.generateRandomString(32)
	authCode := &AuthorizationCode{
		Code:          code,
		ClientID:      clientID,
		RedirectURI:   redirectURI,
		Scope:         scope,
		CodeChallenge: codeChallenge,
		ExpiresAt:     time.Now().Add(10 * time.Minute),
		Resource:      resource,
	}

	s.mutex.Lock()
	s.authCodes[code] = authCode
	s.mutex.Unlock()

	// Show success page before redirecting
	s.showSuccessPage(w, r, redirectURI, code, state, username)

	log.Printf("OAuth: User authenticated successfully, showing success page for code: %s", code)
}

func (s *OAuthServer) showSuccessPage(w http.ResponseWriter, r *http.Request, redirectURI, code, state, username string) {
	// Build redirect URL
	redirectURL, _ := url.Parse(redirectURI)
	params := redirectURL.Query()
	params.Set("code", code)
	if state != "" {
		params.Set("state", state)
	}
	redirectURL.RawQuery = params.Encode()

	// Prepare template data
	data := SuccessPageData{
		RedirectURL: redirectURL.String(),
		Username:    username,
	}

	// Execute success template
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	if err := s.templates.ExecuteTemplate(w, "success.html", data); err != nil {
		log.Printf("OAuth: Failed to execute success template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// Token Endpoint Handler
func (s *OAuthServer) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Printf("OAuth: Token request received - Method: %s, Content-Type: %s", r.Method, r.Header.Get("Content-Type"))

	var grantType, code, redirectURI, clientID, codeVerifier, resource string

	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		// Handle JSON request body
		log.Printf("OAuth: Parsing JSON request body")
		var req TokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("OAuth: Failed to parse JSON body: %v", err)
			s.writeOAuthError(w, "invalid_request", "Invalid JSON request", http.StatusBadRequest)
			return
		}
		
		grantType = req.GrantType
		code = req.Code
		redirectURI = req.RedirectURI
		clientID = req.ClientID
		codeVerifier = req.CodeVerifier
		resource = req.Resource
		
		log.Printf("OAuth: JSON request params - grant_type=%s, code=%s, redirect_uri=%s, client_id=%s, resource=%s", 
			grantType, code, redirectURI, clientID, resource)
	} else {
		// Handle form data
		log.Printf("OAuth: Parsing form data")
		if err := r.ParseForm(); err != nil {
			log.Printf("OAuth: Failed to parse form data: %v", err)
			s.writeOAuthError(w, "invalid_request", "Invalid form data", http.StatusBadRequest)
			return
		}

		// Log all form values for debugging
		log.Printf("OAuth: Token request form data:")
		for key, values := range r.PostForm {
			log.Printf("  %s: %v", key, values)
		}

		grantType = r.FormValue("grant_type")
		code = r.FormValue("code")
		redirectURI = r.FormValue("redirect_uri")
		clientID = r.FormValue("client_id")
		codeVerifier = r.FormValue("code_verifier")
		resource = r.FormValue("resource")
		
		log.Printf("OAuth: Form request params - grant_type=%s, code=%s, redirect_uri=%s, client_id=%s, resource=%s", 
			grantType, code, redirectURI, clientID, resource)
	}

	if grantType == "refresh_token" {
		s.handleRefreshToken(w, r, clientID)
		return
	}
	
	if grantType != "authorization_code" {
		s.writeOAuthError(w, "unsupported_grant_type", "Only authorization_code and refresh_token grant types are supported", http.StatusBadRequest)
		return
	}

	if code == "" || redirectURI == "" || clientID == "" {
		s.writeOAuthError(w, "invalid_request", "Missing required parameters", http.StatusBadRequest)
		return
	}

	// First, validate that the client exists
	s.mutex.RLock()
	_, clientExists := s.clients[clientID]
	s.mutex.RUnlock()
	
	if !clientExists {
		log.Printf("OAuth: Client ID '%s' not found in token endpoint, returning invalid_client", clientID)
		s.writeOAuthError(w, "invalid_client", "Client not found", http.StatusUnauthorized)
		return
	}

	// Validate authorization code
	s.mutex.Lock()
	authCode, exists := s.authCodes[code]
	if exists {
		delete(s.authCodes, code) // Use authorization code only once
	}
	s.mutex.Unlock()

	if !exists {
		s.writeOAuthError(w, "invalid_grant", "Invalid or expired authorization code", http.StatusBadRequest)
		return
	}

	if time.Now().After(authCode.ExpiresAt) {
		s.writeOAuthError(w, "invalid_grant", "Authorization code expired", http.StatusBadRequest)
		return
	}

	if authCode.ClientID != clientID || authCode.RedirectURI != redirectURI {
		s.writeOAuthError(w, "invalid_grant", "Authorization code does not match client", http.StatusBadRequest)
		return
	}

	// PKCE verification (if code_verifier provided)
	if codeVerifier != "" && authCode.CodeChallenge != "" {
		hash := sha256.Sum256([]byte(codeVerifier))
		challenge := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[:])
		log.Printf("OAuth: PKCE verification - code_verifier='%s', stored_challenge='%s', computed_challenge='%s'", codeVerifier, authCode.CodeChallenge, challenge)
		if challenge != authCode.CodeChallenge {
			s.writeOAuthError(w, "invalid_grant", "PKCE verification failed", http.StatusBadRequest)
			return
		}
		log.Printf("OAuth: PKCE verification passed")
	}

	// Generate access token and refresh token
	accessToken := s.generateRandomString(48)
	refreshToken := s.generateRandomString(48)
	token := &AccessToken{
		Token:        accessToken,
		RefreshToken: refreshToken,
		ClientID:     clientID,
		Scope:        authCode.Scope,
		Resource:     resource,
		ExpiresAt:    time.Now().Add(s.tokenExpiration),
	}

	s.mutex.Lock()
	s.accessTokens[accessToken] = token
	s.mutex.Unlock()
	
	// Persist tokens to disk
	s.saveClients()

	response := TokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.tokenExpiration.Seconds()),
		RefreshToken: refreshToken,
		Scope:        authCode.Scope,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper function to get the real client IP address
func (s *OAuthServer) getClientIP(r *http.Request) string {
	// Priority order for different proxy scenarios:
	
	// 1. CF-Connecting-IP (Cloudflare)
	if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
		if net.ParseIP(cfIP) != nil {
			return cfIP
		}
	}
	
	// 2. True-Client-IP (Cloudflare Enterprise, some CDNs)
	if tcIP := r.Header.Get("True-Client-IP"); tcIP != "" {
		if net.ParseIP(tcIP) != nil {
			return tcIP
		}
	}
	
	// 3. X-Real-IP (nginx, some proxies)
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		if net.ParseIP(xrip) != nil {
			return xrip
		}
	}
	
	// 4. X-Forwarded-For (most proxies/load balancers)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain (the original client)
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	
	// 5. X-Cluster-Client-IP (some Kubernetes ingresses)
	if ccIP := r.Header.Get("X-Cluster-Client-IP"); ccIP != "" {
		if net.ParseIP(ccIP) != nil {
			return ccIP
		}
	}
	
	// 6. X-Forwarded (less common, but some proxies use it)
	if xf := r.Header.Get("X-Forwarded"); xf != "" {
		// Format: X-Forwarded: for=192.0.2.60;proto=http;by=203.0.113.43
		if strings.HasPrefix(xf, "for=") {
			forPart := strings.Split(xf, ";")[0]
			ip := strings.TrimPrefix(forPart, "for=")
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	
	// 7. Forwarded (RFC 7239 standard)
	if fwd := r.Header.Get("Forwarded"); fwd != "" {
		// Format: Forwarded: for=192.0.2.60;proto=http;by=203.0.113.43
		if strings.Contains(fwd, "for=") {
			parts := strings.Split(fwd, ";")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "for=") {
					ip := strings.TrimPrefix(part, "for=")
					// Handle quoted IPs: for="192.0.2.60"
					ip = strings.Trim(ip, "\"")
					if net.ParseIP(ip) != nil {
						return ip
					}
				}
			}
		}
	}
	
	// 8. Fall back to RemoteAddr (direct connection or unknown proxy)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Validate IP against allowlist
func (s *OAuthServer) isIPAllowed(clientIP string, allowedIPs []string) bool {
	if len(allowedIPs) == 0 {
		return true // No restrictions if allowlist is empty
	}
	
	for _, allowedIP := range allowedIPs {
		if clientIP == allowedIP {
			return true
		}
	}
	return false
}

func (s *OAuthServer) handleRefreshToken(w http.ResponseWriter, r *http.Request, clientID string) {
	var refreshToken string
	
	// Parse refresh token from request
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeOAuthError(w, "invalid_request", "Invalid JSON request", http.StatusBadRequest)
			return
		}
		refreshToken = req.RefreshToken
	} else {
		refreshToken = r.FormValue("refresh_token")
	}
	
	if refreshToken == "" {
		s.writeOAuthError(w, "invalid_request", "Missing refresh_token", http.StatusBadRequest)
		return
	}
	
	// Validate client exists
	s.mutex.RLock()
	_, clientExists := s.clients[clientID]
	s.mutex.RUnlock()
	
	if !clientExists {
		log.Printf("OAuth: Client ID '%s' not found in refresh token endpoint", clientID)
		s.writeOAuthError(w, "invalid_client", "Client not found", http.StatusUnauthorized)
		return
	}
	
	// Find the access token that has this refresh token
	s.mutex.Lock()
	var oldToken *AccessToken
	var oldAccessTokenKey string
	exists := false
	
	for accessTokenKey, token := range s.accessTokens {
		if token.RefreshToken == refreshToken {
			oldToken = token
			oldAccessTokenKey = accessTokenKey
			exists = true
			break
		}
	}
	
	if exists {
		// Remove the old access token (which also removes the refresh token)
		delete(s.accessTokens, oldAccessTokenKey)
	}
	s.mutex.Unlock()
	
	// Persist token deletions to disk
	if exists {
		s.saveClients()
	}
	
	if !exists {
		s.writeOAuthError(w, "invalid_grant", "Invalid refresh token", http.StatusBadRequest)
		return
	}
	
	if oldToken.ClientID != clientID {
		s.writeOAuthError(w, "invalid_grant", "Refresh token does not belong to client", http.StatusBadRequest)
		return
	}
	
	// Generate new access token and refresh token
	newAccessToken := s.generateRandomString(48)
	newRefreshToken := s.generateRandomString(48)
	token := &AccessToken{
		Token:        newAccessToken,
		RefreshToken: newRefreshToken,
		ClientID:     clientID,
		Scope:        oldToken.Scope,
		Resource:     oldToken.Resource,
		ExpiresAt:    time.Now().Add(s.tokenExpiration),
	}
	
	s.mutex.Lock()
	s.accessTokens[newAccessToken] = token
	s.mutex.Unlock()
	
	// Persist tokens to disk
	s.saveClients()
	
	log.Printf("OAuth: Refreshed tokens for client %s", clientID)
	
	response := TokenResponse{
		AccessToken:  newAccessToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.tokenExpiration.Seconds()),
		RefreshToken: newRefreshToken,
		Scope:        oldToken.Scope,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Token Validation
func (s *OAuthServer) ValidateToken(tokenString string) (*AccessToken, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	token, exists := s.accessTokens[tokenString]
	if !exists {
		return nil, false
	}

	if time.Now().After(token.ExpiresAt) {
		// Token expired, clean it up
		go func() {
			s.mutex.Lock()
			delete(s.accessTokens, tokenString)
			s.mutex.Unlock()
			
			// Persist cleanup to disk
			s.saveClients()
		}()
		return nil, false
	}

	return token, true
}

func (s *OAuthServer) writeOAuthError(w http.ResponseWriter, error, description string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(OAuthError{
		Error:            error,
		ErrorDescription: description,
	})
}

// Register OAuth routes
func (s *OAuthServer) RegisterRoutes(mux *http.ServeMux) {
	// Global OAuth endpoints
	mux.HandleFunc("/.well-known/oauth-authorization-server", s.handleServerMetadata)
	mux.HandleFunc("/oauth/register", s.handleClientRegistration)
	mux.HandleFunc("/oauth/authorize", s.handleAuthorization)
	mux.HandleFunc("/oauth/token", s.handleToken)
	
	// Per-server OAuth discovery endpoints
	mux.HandleFunc("/.well-known/oauth-authorization-server/", s.handleServerMetadata)
	mux.HandleFunc("/.well-known/oauth-protected-resource/", s.handleProtectedResourceMetadata)
}