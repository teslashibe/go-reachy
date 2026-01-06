# Cloud Deployment Guide

Deploy Eva Cloud service to run the robot fleet management system.

## Quick Start

### Local Development

```bash
# Build and run locally
go run ./cmd/eva-cloud --debug

# Or with Docker
docker build -t eva-cloud .
docker run -p 8080:8080 eva-cloud
```

### Docker Compose

```bash
# Start with compose
docker-compose up -d

# View logs
docker-compose logs -f

# Stop
docker-compose down
```

## Production Deployment

### Fly.io (Recommended)

1. Install Fly CLI: https://fly.io/docs/hands-on/install-flyctl/

2. Login and deploy:
```bash
fly auth login
fly deploy
```

3. Set secrets:
```bash
fly secrets set OPENAI_API_KEY="sk-..."
fly secrets set ELEVENLABS_API_KEY="sk_..."
```

4. View status:
```bash
fly status
fly logs
```

### Other Platforms

The Docker image works with any container platform:

- **Railway**: Connect repo, it auto-detects Dockerfile
- **Render**: New Web Service → Docker → Select repo
- **Google Cloud Run**: `gcloud run deploy`
- **AWS ECS**: Push to ECR, create ECS service

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Eva Cloud Service                         │
│                                                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │  WebSocket  │  │    Hub      │  │   Face Detection    │  │
│  │   Server    │──│  (Routing)  │──│   + AI Processing   │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│         ▲                                     │              │
└─────────┼─────────────────────────────────────┼──────────────┘
          │ WebSocket                           │ Motor Commands
          │                                     ▼
     ┌────┴────┐                          ┌─────────────┐
     │  Robot  │◄─────────────────────────│   Commands  │
     │ (go-eva)│                          └─────────────┘
     └─────────┘
```

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/metrics` | GET | Prometheus metrics |
| `/ws/robot` | WS | Robot connection |
| `/ws/robot/:id` | WS | Robot with specific ID |
| `/api/robots` | GET | List connected robots |
| `/api/robots/stats` | GET | Hub statistics |
| `/api/robots/:id/motor` | POST | Send motor command |
| `/api/robots/:id/emotion` | POST | Send emotion |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | HTTP server port |
| `LOG_LEVEL` | info | Log level (debug, info, warn, error) |
| `OPENAI_API_KEY` | - | OpenAI API key for AI processing |
| `ELEVENLABS_API_KEY` | - | ElevenLabs API key for TTS |

## Scaling

For multiple robots:

1. **Horizontal Scaling**: Use Fly.io regions or multiple instances
2. **WebSocket Affinity**: Ensure robots reconnect to same instance (or use Redis for state)
3. **Resource Sizing**: ~512MB RAM per 10 concurrent robots

## Monitoring

```bash
# Health check
curl https://eva-cloud.fly.dev/health

# Metrics (Prometheus format)
curl https://eva-cloud.fly.dev/metrics

# Connected robots
curl https://eva-cloud.fly.dev/api/robots
```

## Troubleshooting

### Robot not connecting
- Check WebSocket URL is correct: `wss://eva-cloud.fly.dev/ws/robot`
- Verify firewall allows outbound WebSocket
- Check robot logs for connection errors

### High latency
- Deploy to region closest to robots
- Check robot network connection
- Consider reducing frame rate

### Memory issues
- Increase VM memory in fly.toml
- Check for frame processing backlog




