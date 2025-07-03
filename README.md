# Calarbot2

Calarbot2 is a Telegram bot written in Go, designed with a modular architecture. It's a cloud-ready rewrite of the original Calarbot project.

## Features

- Modular architecture allowing easy addition of new functionality
- Docker support for easy deployment
- Multiple modules:
  - **simpleReply**: A basic module that responds to messages
  - **skazka**: A storytelling game module
  - **sber**: A module that adds "Сбер" prefix to nouns in Russian text

## Sber Module

The `/sber` command is implemented as a combination of two services:

1. **sberify-service**: A Python REST API that uses NLTK and pymorphy3 to find nouns in Russian text and add "Сбер" prefix to them
2. **sber**: A Go module that calls the sberify-service and integrates with the bot

This approach was chosen because NLTK does not have a Go equivalent, so we need to use Python for the natural language processing functionality.

### How it works

1. User sends a message with the `/sber` command
2. The sber module extracts the text to process
3. The module sends the text to the sberify-service
4. The service processes the text and returns the result
5. The module sends the result back to the user

## Skazka Module

The `/skazka` command implements a collaborative storytelling game where multiple players take turns contributing to a story.

### How it works

1. A user starts the game in a group chat with the `/skazka` command
2. Players join the game by sending the `/play` command during a 5-minute registration period
3. Each player must have started a private chat with the bot to participate
4. After registration, if there are at least 2 players, the game begins
5. Players are randomly shuffled to determine the order
6. The bot sends private messages to each player for their turn:
   - The first player starts the story
   - Middle players continue the story, seeing the last part of the previous contribution
   - The last player is asked to finish the story
7. Each player has 3 minutes to respond, or their turn is skipped
8. The game continues for a maximum of 10 turns or until all players have had a chance to contribute
9. At the end, the complete story is posted to the chat with anonymous names for all contributors

### Features

- Players are assigned anonymous names (combinations of adjectives and animals)
- The game manages multiple sessions across different chats
- Stories can be automatically posted to a configured channel
- The game has configurable timeouts for registration and turns

## Setup

### Prerequisites

- Go 1.22 or higher (for development)
- Docker and Docker Compose (for deployment)

### Configuration

1. Create a Telegram bot using BotFather and get your bot token
2. Save your token to a file named `.tgtoken` in `/opt/calarbot/tokens/`

## Running the bot

### Starting the bot

```bash
docker-compose up -d
```

This will start all the services defined in docker-compose.yml, including the engine, modules, and the sberify-service.

### Stopping the bot

```bash
docker-compose down
```

## Development

### Adding a new module

1. Create a new directory in the `modules` directory
2. Implement the BotModule interface
3. Add a build stage to the Dockerfile
4. Add a service to docker-compose.yml
5. Add your module to the `includeModules` map in `engine/runBot.go` (for local development)

See the existing modules for examples.

## License

[MIT License](LICENSE)
