package spark

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewGoogleDocsClientMissingCredentials(t *testing.T) {
	_, err := NewGoogleDocsClient(GoogleDocsConfig{
		ClientID:     "",
		ClientSecret: "",
	})

	if err == nil {
		t.Error("expected error for missing credentials")
	}
}

func TestNewGoogleDocsClientWithCredentials(t *testing.T) {
	client, err := NewGoogleDocsClient(GoogleDocsConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/callback",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client == nil {
		t.Fatal("expected non-nil client")
	}

	// Should not be authenticated without token
	if client.IsAuthenticated() {
		t.Error("expected not authenticated without token")
	}
}

func TestGetAuthURL(t *testing.T) {
	client, err := NewGoogleDocsClient(GoogleDocsConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/callback",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	authURL := client.GetAuthURL()

	if authURL == "" {
		t.Error("expected non-empty auth URL")
	}

	// Should contain Google OAuth URL
	if len(authURL) < 50 {
		t.Errorf("auth URL seems too short: %s", authURL)
	}
}

func TestGetStatus(t *testing.T) {
	client, err := NewGoogleDocsClient(GoogleDocsConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := client.GetStatus()

	// Should not be connected
	if status.Connected {
		t.Error("expected not connected")
	}

	// Should have auth URL
	if status.AuthURL == "" {
		t.Error("expected auth URL when not connected")
	}
}

func TestDisconnectWithoutToken(t *testing.T) {
	client, err := NewGoogleDocsClient(GoogleDocsConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not error when disconnecting without token
	err = client.Disconnect()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTokenPathDefault(t *testing.T) {
	client, err := NewGoogleDocsClient(GoogleDocsConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		TokenPath:    "", // Empty - should use default
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Token path should be set to default
	homeDir, _ := os.UserHomeDir()
	expected := filepath.Join(homeDir, ".eva", "google_token.json")
	if client.tokenPath != expected {
		t.Errorf("expected token path %s, got %s", expected, client.tokenPath)
	}
}

func TestRedirectURLDefault(t *testing.T) {
	client, err := NewGoogleDocsClient(GoogleDocsConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "", // Empty - should use default
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Config should have default redirect URL
	if client.config.RedirectURL != "http://localhost:8181/api/spark/callback" {
		t.Errorf("expected default redirect URL, got %s", client.config.RedirectURL)
	}
}

func TestFormatSparkForDoc(t *testing.T) {
	spark := &Spark{
		ID:         "test-id",
		Title:      "Test Spark",
		RawContent: "This is the original idea",
		Tags:       []string{"test", "example"},
		Context: []Context{
			{
				Content: "Additional context",
				AddedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
				Source:  SourceVoice,
			},
		},
		Plan: &Plan{
			Summary:   "A brief plan",
			Steps:     []string{"Step 1", "Step 2"},
			Resources: []string{"Resource 1"},
		},
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
	}

	content := formatSparkForDoc(spark)

	// Check title
	if !containsString(content, "Test Spark") {
		t.Error("expected title in content")
	}

	// Check raw content
	if !containsString(content, "This is the original idea") {
		t.Error("expected raw content")
	}

	// Check tags
	if !containsString(content, "test") || !containsString(content, "example") {
		t.Error("expected tags in content")
	}

	// Check context
	if !containsString(content, "Additional context") {
		t.Error("expected context in content")
	}

	// Check plan
	if !containsString(content, "A brief plan") {
		t.Error("expected plan summary in content")
	}
	if !containsString(content, "Step 1") {
		t.Error("expected steps in content")
	}
	if !containsString(content, "Resource 1") {
		t.Error("expected resources in content")
	}
}

func TestFormatSparkForDocMinimal(t *testing.T) {
	spark := &Spark{
		ID:         "test-id",
		Title:      "Minimal Spark",
		RawContent: "Just an idea",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	content := formatSparkForDoc(spark)

	// Should have title and content
	if !containsString(content, "Minimal Spark") {
		t.Error("expected title")
	}
	if !containsString(content, "Just an idea") {
		t.Error("expected content")
	}
}

func TestGetDocURL(t *testing.T) {
	docID := "abc123xyz"
	url := GetDocURL(docID)

	expected := "https://docs.google.com/document/d/abc123xyz/edit"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestCreateDocWithoutAuth(t *testing.T) {
	client, err := NewGoogleDocsClient(GoogleDocsConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = client.CreateDoc("Test", "Content")
	if err == nil {
		t.Error("expected error when not authenticated")
	}
}

func TestUpdateDocWithoutAuth(t *testing.T) {
	client, err := NewGoogleDocsClient(GoogleDocsConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = client.UpdateDoc("doc-id", "Content")
	if err == nil {
		t.Error("expected error when not authenticated")
	}
}

func TestGetDocWithoutAuth(t *testing.T) {
	client, err := NewGoogleDocsClient(GoogleDocsConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = client.GetDoc("doc-id")
	if err == nil {
		t.Error("expected error when not authenticated")
	}
}

func TestSyncSparkWithoutAuth(t *testing.T) {
	client, err := NewGoogleDocsClient(GoogleDocsConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	spark := NewSpark("test-id", "Test content")
	err = client.SyncSpark(spark)
	if err == nil {
		t.Error("expected error when not authenticated")
	}
}

func TestHTTPHandlers(t *testing.T) {
	client, err := NewGoogleDocsClient(GoogleDocsConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test that handlers return non-nil functions
	authStart := client.HandleAuthStart()
	if authStart == nil {
		t.Error("expected non-nil auth start handler")
	}

	authCallback := client.HandleAuthCallback()
	if authCallback == nil {
		t.Error("expected non-nil auth callback handler")
	}

	status := client.HandleStatus()
	if status == nil {
		t.Error("expected non-nil status handler")
	}

	disconnect := client.HandleDisconnect()
	if disconnect == nil {
		t.Error("expected non-nil disconnect handler")
	}
}

func TestSaveAndLoadToken(t *testing.T) {
	// Create temp directory for token
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "test_token.json")

	client, err := NewGoogleDocsClient(GoogleDocsConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		TokenPath:    tokenPath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Try to save without token (should fail)
	err = client.saveToken()
	if err == nil {
		t.Error("expected error saving nil token")
	}

	// Try to load non-existent token (should fail)
	err = client.loadToken()
	if err == nil {
		t.Error("expected error loading non-existent token")
	}
}

func TestInitServiceWithoutToken(t *testing.T) {
	client, err := NewGoogleDocsClient(GoogleDocsConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Try to init service without token
	err = client.initService()
	if err == nil {
		t.Error("expected error initializing service without token")
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

