# Meli URL Shortener

# Choices

**Redis** to store data

**OpenCensus** for metrics

## Run

```
docker-compose up --build
```

## Endpoint

| URL | Method | Body | Desctiption |
|-----|--------|------|-------------|
|/|POST|`{"url":"https://felipeweb.dev"}`|Create short URL|
|/{short}|GET|nil|Redirect to full URL|
|/{short}|DELETE|nil|Remove short URL|
|/search/{short}|GET|nil|Get full URL based on short|
|/metrics|GET|nil|Metrics of the service|