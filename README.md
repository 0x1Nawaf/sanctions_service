# Sanctions Service

Go API for sanctions screening.

## Requirements

- Go 1.22+
- MySQL 8.0+

## Build

```bash
go mod tidy
go build -o bin/server ./cmd/server
go build -o bin/seeder ./cmd/seeder
```

## Setup

```bash
# Create database
mysql -u root -p -e "CREATE DATABASE sanctions CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
mysql -u root -p sanctions < migrations/001_schema.sql

# Configure
cp .env.example .env
# Edit .env with your DB credentials
```

## Seed

```bash
./bin/seeder /path/to/sanctions.json
```

## Run

```bash
./bin/server
# Listening on :8080
```

## API

**Screen a name**

```
POST /api/screen
```
```json
{
  "name": "Victor Stalony Brown",
  "search_type": "individual",
  "min_score": 60
}
```

**List records**

```
GET /api/records?page=1&per_page=25
```

**Get record**

```
GET /api/records/{id}
```

**Health check**

```
GET /health
```
