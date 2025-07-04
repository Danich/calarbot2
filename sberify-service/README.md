# Sberify Service

This service provides a REST API for the `/sber` command in calarbot2. It uses NLTK and pymorphy3 to find all nouns in a Russian text and add the "Сбер" prefix to them.

## API Endpoints

### POST /sberify

Processes text and adds "сбер" prefix to all nouns.

**Request:**
```json
{
  "text": "Текст для обработки"
}
```

**Response:**
```json
{
  "result": "сбертекст для обработки"
}
```

### GET /health

Health check endpoint.

**Response:**
```json
{
  "status": "ok"
}
```

## Development

### Requirements

- Python 3.11+
- Flask
- NLTK
- pymorphy3

### Installation

```bash
pip install -r requirements.txt
```

### Running locally

```bash
python app.py
```

## Docker

### Building the image

```bash
docker build -t sberify-service .
```

### Running the container

```bash
docker run -p 5000:5000 sberify-service
```

## Integration with calarbot2

This service is used by the `sber` module in calarbot2 to implement the `/sber` command. The module sends a request to this service with the text to process and receives the processed text in response.

The service is configured in docker-compose.yml to run alongside the calarbot2 services.