# Eva Spark 🔥

AI-powered idea collection for Eva. "A spark of inspiration" - capture ideas, accumulate context, and develop plans.

## Features

- **Voice-first capture**: Say "I have an idea" or "new spark" to capture inspiration
- **Context accumulation**: Add notes and context to ideas over time
- **AI-powered titles**: Gemini generates concise titles automatically
- **Plan generation**: Get actionable steps for any idea
- **Google Docs sync**: Back up ideas to Google Docs for sharing

## Configuration

Spark can be configured via **config file**, **environment variables**, or **CLI flags**.

Priority (highest to lowest): CLI flags > Environment variables > Config file > Defaults

### Config File (`~/.eva/config.json`)

```json
{
  "spark": {
    "enabled": true,
    "auto_sync": true,
    "planning_enabled": true,
    "gemini_model": "gemini-2.0-flash"
  }
}
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SPARK_ENABLED` | Enable/disable Spark (`true`/`false`) | `true` |
| `GEMINI_MODEL` | Gemini model for AI generation | `gemini-2.0-flash` |
| `GOOGLE_API_KEY` | API key for Gemini (required for AI features) | - |
| `GOOGLE_CLIENT_ID` | OAuth2 client ID for Google Docs | - |
| `GOOGLE_CLIENT_SECRET` | OAuth2 client secret for Google Docs | - |

### CLI Flags

```bash
eva --spark=false  # Disable Spark
eva --spark=true   # Enable Spark (default)
```

## Voice Commands

Eva understands these commands for Spark:

| Action | Example Phrases |
|--------|-----------------|
| Save idea | "new spark", "I have an idea", "capture this" |
| Add context | "add to [spark]", "for [spark], also..." |
| Rename | "rename [spark] to...", "call it [name] instead" |
| List | "what sparks do I have?", "show my ideas" |
| View | "tell me about [spark]", "show me [spark]" |
| Delete | "delete [spark]", "remove [spark]" |
| Search | "find sparks about [topic]" |
| Sync | "sync [spark] to Google" |
| Plan | "make a plan for [spark]", "how do I build [spark]" |

## Tools

Spark provides 10 tools for Eva:

| Tool | Description |
|------|-------------|
| `save_spark` | Capture a new idea |
| `add_context` | Add context to existing spark |
| `update_title` | Rename a spark |
| `list_sparks` | List all sparks |
| `view_spark` | View spark details |
| `delete_spark` | Delete a spark |
| `search_sparks` | Search by keyword |
| `sync_spark` | Sync to Google Docs |
| `google_status` | Check Google connection |
| `generate_plan` | Create action plan |

## Storage

- **Local**: `~/.eva/sparks.json` (JSON file)
- **Google Docs**: One document per spark (optional)

## Gemini Models

Supported models (configure via `GEMINI_MODEL`):

| Model | Description |
|-------|-------------|
| `gemini-2.0-flash` | Fast, latest Flash model (default) |
| `gemini-1.5-flash` | Stable Flash model |
| `gemini-1.5-pro` | Higher quality, slower |

## Rate Limiting

- Gemini API: 10 requests/minute (configurable)
- Title/tag generation results are cached to avoid duplicate calls

## Google Docs Setup

1. Set `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET`
2. Open Eva dashboard at `http://localhost:8080`
3. Click "Connect Google" and authorize
4. Token is saved to `~/.eva/google_token.json`

OAuth2 callback URL: `http://localhost:8080/api/spark/callback`

