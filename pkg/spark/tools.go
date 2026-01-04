package spark

import (
	"fmt"
	"strings"
)

// Tool represents an AI tool that Eva can use for Spark operations.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]interface{}
	Handler     func(args map[string]interface{}) (string, error)
}

// ToolsConfig holds dependencies for Spark tools.
type ToolsConfig struct {
	Store      *JSONStore
	Gemini     *GeminiClient      // Gemini client for title/tag generation (optional)
	GoogleDocs *GoogleDocsClient  // Google Docs client for syncing (optional)
}

// Tools returns all Spark-related tools for Eva.
func Tools(cfg ToolsConfig) []Tool {
	return []Tool{
		// ============================================================
		// save_spark - Capture a new spark of inspiration
		// ============================================================
		{
			Name: "save_spark",
			Description: `Save a new spark of inspiration. Use when someone says "new spark", "I have an idea", "save this idea", "capture this", or shares something they want to remember and develop later. Eva will generate a title automatically.`,
			Parameters: map[string]interface{}{
				"content": map[string]interface{}{
					"type":        "string",
					"description": "The idea or inspiration to capture",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				content, _ := args["content"].(string)
				if content == "" {
					return "I need to know what spark of inspiration you want to save!", nil
				}

				if cfg.Store == nil {
					return "Spark storage not available", nil
				}

				// Create the spark
				spark, err := cfg.Store.CreateSpark(content)
				if err != nil {
					return fmt.Sprintf("Failed to save spark: %v", err), nil
				}

				// Generate title and tags with Gemini (if available)
				var title string
				var tags []string

				if cfg.Gemini != nil {
					// Use combined call for efficiency
					title, tags, err = cfg.Gemini.GenerateTitleAndTags(content)
					if err != nil {
						fmt.Printf("⚠️  Gemini title/tag generation failed: %v\n", err)
					}
				}

				// Fallback to default title if not generated
				if title == "" {
					title = generateDefaultTitle(content)
				}

				spark.SetTitle(title)
				if len(tags) > 0 {
					spark.SetTags(tags)
				}

				// Save with title and tags
				if err := cfg.Store.Update(spark); err != nil {
					return fmt.Sprintf("Failed to update spark: %v", err), nil
				}

				fmt.Printf("🔥 Spark saved: %s (ID: %s)\n", title, spark.ID)
				return fmt.Sprintf("Got it! I've captured your spark about '%s'. Just say 'add to %s' whenever you have more thoughts!", title, title), nil
			},
		},

		// ============================================================
		// add_context - Add context to an existing spark
		// ============================================================
		{
			Name: "add_context",
			Description: `Add more context or inspiration to an existing spark. Use when someone says "add to [spark]", "for [spark], also...", "regarding [spark]", or wants to add more details to a previously saved idea.`,
			Parameters: map[string]interface{}{
				"spark": map[string]interface{}{
					"type":        "string",
					"description": "The spark title or keyword to add context to",
				},
				"context": map[string]interface{}{
					"type":        "string",
					"description": "The additional context or inspiration to add",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				sparkID, _ := args["spark"].(string)
				context, _ := args["context"].(string)

				if sparkID == "" {
					return "Which spark do you want to add to?", nil
				}
				if context == "" {
					return "What context do you want to add?", nil
				}

				if cfg.Store == nil {
					return "Spark storage not available", nil
				}

				// Find the spark (fuzzy match)
				spark, ambiguous, err := findSparkFuzzy(cfg.Store, sparkID)
				if err != nil {
					return err.Error(), nil
				}
				if ambiguous != "" {
					return ambiguous, nil
				}

				// Add context
				spark.AddContext(context, SourceVoice)
				if err := cfg.Store.Update(spark); err != nil {
					return fmt.Sprintf("Failed to add context: %v", err), nil
				}

				fmt.Printf("🔥 Context added to '%s' (%d total)\n", spark.Title, spark.ContextCount())
				return fmt.Sprintf("Added to '%s' - you now have %d pieces of context!", spark.Title, spark.ContextCount()), nil
			},
		},

		// ============================================================
		// update_title - Rename a spark
		// ============================================================
		{
			Name: "update_title",
			Description: `Change the title of an existing spark. Use when someone says "rename [spark] to...", "change title of [spark]", "call it [new name] instead", or wants to give a spark a better name.`,
			Parameters: map[string]interface{}{
				"spark": map[string]interface{}{
					"type":        "string",
					"description": "The current spark title or keyword",
				},
				"new_title": map[string]interface{}{
					"type":        "string",
					"description": "The new title for the spark",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				sparkID, _ := args["spark"].(string)
				newTitle, _ := args["new_title"].(string)

				if sparkID == "" {
					return "Which spark do you want to rename?", nil
				}
				if newTitle == "" {
					return "What should I rename it to?", nil
				}

				if cfg.Store == nil {
					return "Spark storage not available", nil
				}

				// Find the spark
				spark, ambiguous, err := findSparkFuzzy(cfg.Store, sparkID)
				if err != nil {
					return err.Error(), nil
				}
				if ambiguous != "" {
					return ambiguous, nil
				}

				oldTitle := spark.Title
				spark.SetTitle(newTitle)
				if err := cfg.Store.Update(spark); err != nil {
					return fmt.Sprintf("Failed to rename: %v", err), nil
				}

				fmt.Printf("🔥 Renamed '%s' → '%s'\n", oldTitle, newTitle)
				return fmt.Sprintf("Renamed '%s' to '%s'", oldTitle, newTitle), nil
			},
		},

		// ============================================================
		// list_sparks - List all saved sparks
		// ============================================================
		{
			Name: "list_sparks",
			Description: `List all saved sparks of inspiration. Use when someone asks "what sparks do I have?", "show my ideas", "list sparks", or wants to see all their captured ideas.`,
			Parameters: map[string]interface{}{},
			Handler: func(args map[string]interface{}) (string, error) {
				if cfg.Store == nil {
					return "Spark storage not available", nil
				}

				sparks, err := cfg.Store.List()
				if err != nil {
					return fmt.Sprintf("Failed to list sparks: %v", err), nil
				}

				if len(sparks) == 0 {
					return "You don't have any sparks yet. Share an idea and I'll capture it!", nil
				}

				// Build response
				var lines []string
				for i, spark := range sparks {
					line := fmt.Sprintf("%d. %s", i+1, spark.Title)
					if spark.ContextCount() > 0 {
						line += fmt.Sprintf(" (%d context)", spark.ContextCount())
					}
					if spark.HasPlan() {
						line += " [planned]"
					}
					lines = append(lines, line)
				}

				summary := fmt.Sprintf("You have %d sparks:\n%s", len(sparks), strings.Join(lines, "\n"))
				return summary, nil
			},
		},

		// ============================================================
		// view_spark - View full details of a spark
		// ============================================================
		{
			Name: "view_spark",
			Description: `View the full details of a specific spark including all context. Use when someone asks "tell me about [spark]", "show me [spark]", "what's in [spark]", or wants to see the details of an idea.`,
			Parameters: map[string]interface{}{
				"spark": map[string]interface{}{
					"type":        "string",
					"description": "The spark title or keyword to view",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				sparkID, _ := args["spark"].(string)
				if sparkID == "" {
					return "Which spark do you want to view?", nil
				}

				if cfg.Store == nil {
					return "Spark storage not available", nil
				}

				// Find the spark
				spark, ambiguous, err := findSparkFuzzy(cfg.Store, sparkID)
				if err != nil {
					return err.Error(), nil
				}
				if ambiguous != "" {
					return ambiguous, nil
				}

				// Build detailed view
				var parts []string
				parts = append(parts, fmt.Sprintf("**%s**", spark.Title))
				parts = append(parts, fmt.Sprintf("Original: %s", spark.RawContent))

				if len(spark.Tags) > 0 {
					parts = append(parts, fmt.Sprintf("Tags: %s", strings.Join(spark.Tags, ", ")))
				}

				if spark.ContextCount() > 0 {
					parts = append(parts, fmt.Sprintf("\nContext (%d pieces):", spark.ContextCount()))
					for i, ctx := range spark.Context {
						parts = append(parts, fmt.Sprintf("  %d. %s", i+1, ctx.Content))
					}
				}

				if spark.HasPlan() {
					parts = append(parts, fmt.Sprintf("\nPlan: %s", spark.Plan.Summary))
					if len(spark.Plan.Steps) > 0 {
						parts = append(parts, "Steps:")
						for i, step := range spark.Plan.Steps {
							parts = append(parts, fmt.Sprintf("  %d. %s", i+1, step))
						}
					}
				}

				// Offer next steps
				if !spark.HasPlan() && spark.ContextCount() >= 2 {
					parts = append(parts, "\nWant me to start planning this one?")
				}

				return strings.Join(parts, "\n"), nil
			},
		},

		// ============================================================
		// delete_spark - Delete a spark
		// ============================================================
		{
			Name: "delete_spark",
			Description: `Delete a spark permanently. Use when someone says "delete [spark]", "remove [spark]", "forget about [spark]", or wants to remove an idea they no longer need.`,
			Parameters: map[string]interface{}{
				"spark": map[string]interface{}{
					"type":        "string",
					"description": "The spark title or keyword to delete",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				sparkID, _ := args["spark"].(string)
				if sparkID == "" {
					return "Which spark do you want to delete?", nil
				}

				if cfg.Store == nil {
					return "Spark storage not available", nil
				}

				// Find the spark
				spark, ambiguous, err := findSparkFuzzy(cfg.Store, sparkID)
				if err != nil {
					return err.Error(), nil
				}
				if ambiguous != "" {
					return ambiguous, nil
				}

				title := spark.Title
				if err := cfg.Store.Delete(spark.ID); err != nil {
					return fmt.Sprintf("Failed to delete: %v", err), nil
				}

				fmt.Printf("🔥 Deleted spark: %s\n", title)
				return fmt.Sprintf("Deleted '%s'. It's gone!", title), nil
			},
		},

		// ============================================================
		// search_sparks - Search sparks by keyword
		// ============================================================
		{
			Name: "search_sparks",
			Description: `Search sparks by keyword. Use when someone asks "find sparks about [topic]", "search for [keyword]", or wants to find specific ideas.`,
			Parameters: map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "The search query or keyword",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				query, _ := args["query"].(string)
				if query == "" {
					return "What do you want to search for?", nil
				}

				if cfg.Store == nil {
					return "Spark storage not available", nil
				}

				results, err := cfg.Store.Search(query)
				if err != nil {
					return fmt.Sprintf("Search failed: %v", err), nil
				}

				if len(results) == 0 {
					return fmt.Sprintf("No sparks found matching '%s'", query), nil
				}

				var lines []string
				for i, spark := range results {
					lines = append(lines, fmt.Sprintf("%d. %s", i+1, spark.Title))
				}

				return fmt.Sprintf("Found %d sparks matching '%s':\n%s", len(results), query, strings.Join(lines, "\n")), nil
			},
		},

		// ============================================================
		// sync_spark - Sync a spark to Google Docs
		// ============================================================
		{
			Name: "sync_spark",
			Description: `Sync a spark to Google Docs. Use when someone says "sync [spark] to Google", "save [spark] to docs", or wants to back up an idea to Google Docs.`,
			Parameters: map[string]interface{}{
				"spark": map[string]interface{}{
					"type":        "string",
					"description": "The spark title or keyword to sync",
				},
			},
			Handler: func(args map[string]interface{}) (string, error) {
				sparkID, _ := args["spark"].(string)
				if sparkID == "" {
					return "Which spark do you want to sync to Google Docs?", nil
				}

				if cfg.Store == nil {
					return "Spark storage not available", nil
				}

				if cfg.GoogleDocs == nil {
					return "Google Docs is not configured. Please set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET.", nil
				}

				if !cfg.GoogleDocs.IsAuthenticated() {
					return "Not connected to Google. Please connect via the web dashboard first.", nil
				}

				// Find the spark
				spark, ambiguous, err := findSparkFuzzy(cfg.Store, sparkID)
				if err != nil {
					return err.Error(), nil
				}
				if ambiguous != "" {
					return ambiguous, nil
				}

				// Sync to Google Docs
				if err := cfg.GoogleDocs.SyncSpark(spark); err != nil {
					return fmt.Sprintf("Failed to sync: %v", err), nil
				}

				// Save the updated spark with GoogleDocID
				if err := cfg.Store.Update(spark); err != nil {
					fmt.Printf("⚠️  Failed to save spark after sync: %v\n", err)
				}

				docURL := GetDocURL(spark.GoogleDocID)
				fmt.Printf("🔥 Synced spark to Google Docs: %s -> %s\n", spark.Title, docURL)
				return fmt.Sprintf("Synced '%s' to Google Docs! You can view it at: %s", spark.Title, docURL), nil
			},
		},

		// ============================================================
		// google_status - Check Google Docs connection status
		// ============================================================
		{
			Name: "google_status",
			Description: `Check if Google Docs is connected. Use when someone asks "is Google connected?", "am I connected to Google?", or wants to know the sync status.`,
			Parameters:  map[string]interface{}{},
			Handler: func(args map[string]interface{}) (string, error) {
				if cfg.GoogleDocs == nil {
					return "Google Docs is not configured. Please set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET environment variables.", nil
				}

				if cfg.GoogleDocs.IsAuthenticated() {
					return "Connected to Google Docs! Your sparks can be synced.", nil
				}

				return "Not connected to Google. You can connect via the web dashboard at /api/spark/auth", nil
			},
		},
	}
}

// findSparkFuzzy finds a spark by ID or fuzzy title match.
// Returns (spark, ambiguousMessage, error)
func findSparkFuzzy(store *JSONStore, identifier string) (*Spark, string, error) {
	// Try exact ID first
	if spark, err := store.Get(identifier); err == nil {
		return spark, "", nil
	}

	// Try exact title match
	if spark, err := store.GetByTitle(identifier); err == nil {
		return spark, "", nil
	}

	// Try fuzzy keyword match
	matches, err := store.FindByKeyword(identifier)
	if err != nil {
		return nil, "", fmt.Errorf("search failed: %v", err)
	}

	if len(matches) == 0 {
		return nil, "", fmt.Errorf("I couldn't find a spark matching '%s'. Try 'list sparks' to see what you have.", identifier)
	}

	if len(matches) == 1 {
		return matches[0], "", nil
	}

	// Multiple matches - ask for clarification
	var titles []string
	for _, m := range matches {
		titles = append(titles, m.Title)
	}
	ambiguous := fmt.Sprintf("I found %d sparks matching '%s'. Which one: %s?",
		len(matches), identifier, strings.Join(titles, " or "))
	return nil, ambiguous, nil
}

// generateDefaultTitle creates a simple title from content (first 50 chars).
func generateDefaultTitle(content string) string {
	// Clean up and truncate
	title := strings.TrimSpace(content)
	if len(title) > 50 {
		title = title[:47] + "..."
	}
	return title
}


