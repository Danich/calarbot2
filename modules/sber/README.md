# Sber Module for Calarbot2

This module implements the `/sber` command for calarbot2. It calls the sberify-service to find all nouns in a Russian text and add the "Сбер" prefix to them.

## Functionality

The module:
1. Listens for messages starting with `/sber`
2. Extracts the text after the command or from a replied message
3. Sends the text to the sberify-service for processing
4. Returns the processed text to the user

## Configuration

The module can be configured using environment variables:
- `MODULE_ORDER`: The priority order of the module (default: 500)
- `SBERIFY_URL`: The URL of the sberify-service (default: http://sberify-service:5000/sberify)
- `MODULE_PORT`: The port on which the module listens (default: 8080)

## Integration with sberify-service

This module depends on the sberify-service, which provides the NLTK functionality to find nouns in Russian text and add the "Сбер" prefix to them. The service is implemented in Python because NLTK does not have a Go equivalent.

## Usage

To use the `/sber` command, send a message to the bot in one of these formats:
- `/sber текст для обработки` - The text after the command will be processed
- Reply to a message with `/sber` - The text of the replied message will be processed

## Example

Input:
```
/sber Привет мир, как дела?
```

Output:
```
сберпривет сбермир, как сбердела?
```

## Docker

The module is built as part of the main calarbot2 image and is configured in docker-compose.yml to run alongside the other services.