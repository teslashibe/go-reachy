package spark

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"
)

// GoogleDocsClient handles OAuth2 authentication and Google Docs API operations.
type GoogleDocsClient struct {
	config      *oauth2.Config
	token       *oauth2.Token
	tokenPath   string
	docsService *docs.Service

	mu sync.RWMutex

	// Channels for OAuth flow
	authCodeChan chan string
	authErrChan  chan error
}

// GoogleDocsConfig configures the Google Docs client.
type GoogleDocsConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string // e.g., "http://localhost:8080/api/spark/callback"
	TokenPath    string // Path to store token (default: ~/.eva/google_token.json)
}

// NewGoogleDocsClient creates a new Google Docs client.
func NewGoogleDocsClient(cfg GoogleDocsConfig) (*GoogleDocsClient, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET are required")
	}

	if cfg.RedirectURL == "" {
		cfg.RedirectURL = "http://localhost:8080/api/spark/callback"
	}

	if cfg.TokenPath == "" {
		homeDir, _ := os.UserHomeDir()
		cfg.TokenPath = filepath.Join(homeDir, ".eva", "google_token.json")
	}

	oauthConfig := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/documents",
			"https://www.googleapis.com/auth/drive.file",
		},
		Endpoint: google.Endpoint,
	}

	client := &GoogleDocsClient{
		config:       oauthConfig,
		tokenPath:    cfg.TokenPath,
		authCodeChan: make(chan string, 1),
		authErrChan:  make(chan error, 1),
	}

	// Try to load existing token
	if err := client.loadToken(); err == nil {
		// Token loaded, try to initialize service
		if err := client.initService(); err != nil {
			// Token might be expired, will need re-auth
			client.token = nil
		}
	}

	return client, nil
}

// IsAuthenticated returns true if the client has a valid token.
func (g *GoogleDocsClient) IsAuthenticated() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.token != nil && g.token.Valid()
}

// GetAuthURL returns the OAuth2 authorization URL for user consent.
func (g *GoogleDocsClient) GetAuthURL() string {
	return g.config.AuthCodeURL("spark-state", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

// HandleCallback processes the OAuth2 callback with the authorization code.
func (g *GoogleDocsClient) HandleCallback(code string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	token, err := g.config.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("failed to exchange code for token: %w", err)
	}

	g.mu.Lock()
	g.token = token
	g.mu.Unlock()

	// Save token for future use
	if err := g.saveToken(); err != nil {
		fmt.Printf("⚠️  Failed to save token: %v\n", err)
	}

	// Initialize the docs service
	if err := g.initService(); err != nil {
		return fmt.Errorf("failed to initialize docs service: %w", err)
	}

	return nil
}

// Disconnect clears the authentication and removes the stored token.
func (g *GoogleDocsClient) Disconnect() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.token = nil
	g.docsService = nil

	// Remove token file
	if err := os.Remove(g.tokenPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove token file: %w", err)
	}

	return nil
}

// CreateDoc creates a new Google Doc with the given title and content.
func (g *GoogleDocsClient) CreateDoc(title, content string) (string, error) {
	g.mu.RLock()
	service := g.docsService
	g.mu.RUnlock()

	if service == nil {
		return "", fmt.Errorf("not authenticated - please connect to Google first")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create the document
	doc := &docs.Document{
		Title: title,
	}

	createdDoc, err := service.Documents.Create(doc).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("failed to create document: %w", err)
	}

	// Add content if provided
	if content != "" {
		requests := []*docs.Request{
			{
				InsertText: &docs.InsertTextRequest{
					Location: &docs.Location{
						Index: 1,
					},
					Text: content,
				},
			},
		}

		_, err = service.Documents.BatchUpdate(createdDoc.DocumentId, &docs.BatchUpdateDocumentRequest{
			Requests: requests,
		}).Context(ctx).Do()
		if err != nil {
			return createdDoc.DocumentId, fmt.Errorf("created doc but failed to add content: %w", err)
		}
	}

	return createdDoc.DocumentId, nil
}

// UpdateDoc updates an existing Google Doc with new content.
func (g *GoogleDocsClient) UpdateDoc(docID, content string) error {
	g.mu.RLock()
	service := g.docsService
	g.mu.RUnlock()

	if service == nil {
		return fmt.Errorf("not authenticated - please connect to Google first")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get current document to find the end index
	doc, err := service.Documents.Get(docID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}

	// Calculate end index (content length minus 1 for the newline)
	endIndex := doc.Body.Content[len(doc.Body.Content)-1].EndIndex - 1

	requests := []*docs.Request{}

	// Delete existing content if any (keep at least index 1)
	if endIndex > 1 {
		requests = append(requests, &docs.Request{
			DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{
					StartIndex: 1,
					EndIndex:   endIndex,
				},
			},
		})
	}

	// Insert new content
	requests = append(requests, &docs.Request{
		InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{
				Index: 1,
			},
			Text: content,
		},
	})

	_, err = service.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to update document: %w", err)
	}

	return nil
}

// GetDoc retrieves the content of a Google Doc.
func (g *GoogleDocsClient) GetDoc(docID string) (string, error) {
	g.mu.RLock()
	service := g.docsService
	g.mu.RUnlock()

	if service == nil {
		return "", fmt.Errorf("not authenticated - please connect to Google first")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	doc, err := service.Documents.Get(docID).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("failed to get document: %w", err)
	}

	// Extract text content
	var content string
	for _, elem := range doc.Body.Content {
		if elem.Paragraph != nil {
			for _, pe := range elem.Paragraph.Elements {
				if pe.TextRun != nil {
					content += pe.TextRun.Content
				}
			}
		}
	}

	return content, nil
}

// SyncSpark syncs a local spark to Google Docs.
// Creates a new doc if GoogleDocID is empty, updates existing doc otherwise.
func (g *GoogleDocsClient) SyncSpark(spark *Spark) error {
	if !g.IsAuthenticated() {
		return fmt.Errorf("not authenticated - please connect to Google first")
	}

	// Format spark content for Google Doc
	content := formatSparkForDoc(spark)

	if spark.GoogleDocID == "" {
		// Create new document
		docID, err := g.CreateDoc(spark.Title, content)
		if err != nil {
			spark.MarkSyncError()
			return err
		}
		spark.MarkSynced(docID)
	} else {
		// Update existing document
		if err := g.UpdateDoc(spark.GoogleDocID, content); err != nil {
			spark.MarkSyncError()
			return err
		}
		spark.SyncStatus = SyncSynced
		spark.UpdatedAt = time.Now()
	}

	return nil
}

// GetDocURL returns the URL to view/edit a Google Doc.
func GetDocURL(docID string) string {
	return fmt.Sprintf("https://docs.google.com/document/d/%s/edit", docID)
}

// initService initializes the Google Docs service with the current token.
func (g *GoogleDocsClient) initService() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.token == nil {
		return fmt.Errorf("no token available")
	}

	ctx := context.Background()
	client := g.config.Client(ctx, g.token)

	service, err := docs.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return fmt.Errorf("failed to create docs service: %w", err)
	}

	g.docsService = service
	return nil
}

// loadToken loads the OAuth token from disk.
func (g *GoogleDocsClient) loadToken() error {
	data, err := os.ReadFile(g.tokenPath)
	if err != nil {
		return err
	}

	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return err
	}

	g.mu.Lock()
	g.token = &token
	g.mu.Unlock()

	return nil
}

// saveToken saves the OAuth token to disk.
func (g *GoogleDocsClient) saveToken() error {
	g.mu.RLock()
	token := g.token
	g.mu.RUnlock()

	if token == nil {
		return fmt.Errorf("no token to save")
	}

	// Ensure directory exists
	dir := filepath.Dir(g.tokenPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(g.tokenPath, data, 0600)
}

// formatSparkForDoc formats a spark for display in a Google Doc.
func formatSparkForDoc(spark *Spark) string {
	var content string

	// Title as heading
	content += fmt.Sprintf("%s\n\n", spark.Title)

	// Original idea
	content += "💡 Original Idea\n"
	content += fmt.Sprintf("%s\n\n", spark.RawContent)

	// Tags
	if len(spark.Tags) > 0 {
		content += "🏷️ Tags: "
		for i, tag := range spark.Tags {
			if i > 0 {
				content += ", "
			}
			content += tag
		}
		content += "\n\n"
	}

	// Context
	if len(spark.Context) > 0 {
		content += "📝 Context & Notes\n"
		for _, ctx := range spark.Context {
			content += fmt.Sprintf("• [%s] %s\n", ctx.AddedAt.Format("Jan 2"), ctx.Content)
		}
		content += "\n"
	}

	// Plan
	if spark.Plan != nil {
		content += "📋 Plan\n"
		content += fmt.Sprintf("%s\n\n", spark.Plan.Summary)
		if len(spark.Plan.Steps) > 0 {
			content += "Steps:\n"
			for i, step := range spark.Plan.Steps {
				content += fmt.Sprintf("%d. %s\n", i+1, step)
			}
			content += "\n"
		}
		if len(spark.Plan.Resources) > 0 {
			content += "Resources:\n"
			for _, resource := range spark.Plan.Resources {
				content += fmt.Sprintf("• %s\n", resource)
			}
		}
	}

	// Metadata
	content += "\n---\n"
	content += fmt.Sprintf("Created: %s\n", spark.CreatedAt.Format("January 2, 2006"))
	content += fmt.Sprintf("Last Updated: %s\n", spark.UpdatedAt.Format("January 2, 2006 3:04 PM"))

	return content
}

// GoogleDocsStatus returns the current connection status.
type GoogleDocsStatus struct {
	Connected bool   `json:"connected"`
	AuthURL   string `json:"auth_url,omitempty"`
}

// GetStatus returns the current Google Docs connection status.
func (g *GoogleDocsClient) GetStatus() GoogleDocsStatus {
	status := GoogleDocsStatus{
		Connected: g.IsAuthenticated(),
	}

	if !status.Connected {
		status.AuthURL = g.GetAuthURL()
	}

	return status
}

// HTTP Handler helpers for web integration

// HandleAuthStart returns a handler that redirects to Google OAuth.
func (g *GoogleDocsClient) HandleAuthStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authURL := g.GetAuthURL()
		http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
	}
}

// HandleAuthCallback returns a handler that processes the OAuth callback.
func (g *GoogleDocsClient) HandleAuthCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			return
		}

		if err := g.HandleCallback(code); err != nil {
			http.Error(w, fmt.Sprintf("Authentication failed: %v", err), http.StatusInternalServerError)
			return
		}

		// Success! Show a nice message and close the popup
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>Spark - Connected!</title>
    <style>
        body { 
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            display: flex; 
            justify-content: center; 
            align-items: center; 
            height: 100vh; 
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: white;
        }
        .container { 
            text-align: center; 
            padding: 40px;
            background: rgba(255,255,255,0.1);
            border-radius: 16px;
            backdrop-filter: blur(10px);
        }
        h1 { margin-bottom: 10px; }
        p { opacity: 0.9; }
        .emoji { font-size: 48px; margin-bottom: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="emoji">🔥</div>
        <h1>Spark Connected!</h1>
        <p>Your ideas will now sync to Google Docs.</p>
        <p><small>You can close this window.</small></p>
    </div>
    <script>
        setTimeout(function() { window.close(); }, 3000);
    </script>
</body>
</html>
`)
	}
}

// HandleStatus returns a handler that returns the connection status as JSON.
func (g *GoogleDocsClient) HandleStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(g.GetStatus())
	}
}

// HandleDisconnect returns a handler that disconnects from Google.
func (g *GoogleDocsClient) HandleDisconnect() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := g.Disconnect(); err != nil {
			http.Error(w, fmt.Sprintf("Failed to disconnect: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}
}

