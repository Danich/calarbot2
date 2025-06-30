# Calarbot2

Calarbot2 is a Telegram bot written in Go, designed with a modular architecture. It's a rewrite of the original Calarbot project.

## Features

- Modular architecture allowing easy addition of new functionality
- Docker support for easy deployment
- Multiple modules:
  - SimpleReply: A basic module that responds to messages
  - Skazka: A storytelling game module

## Setup

### Prerequisites

- Go 1.23 or higher
- Docker and Docker Compose (for containerized deployment)

### Configuration

1. Create a Telegram bot using BotFather and get your bot token
2. Save your token to a file named `.tgtoken` in `/opt/calarbot/tokens/`

### Running with Docker

```bash
docker-compose up
```

### Development

To add a new module:

1. Create a new directory under `modules/`
2. Implement the BotModule interface
3. Add your module to the `includeModules` map in `engine/runBot.go`

## License

[MIT License](LICENSE)