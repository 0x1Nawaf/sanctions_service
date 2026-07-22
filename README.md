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
mysql -u root -p sanctions < migrations/004_seed_history.sql
mysql -u root -p sanctions < migrations/005_seed_run_record_details.sql

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

**Seed run history**

```
GET /api/historical_updates?page=1&per_page=25
```

Returns seeder run timestamps, aggregate change counts, interval since the previous run, and per-record snapshots (`display_name`, `countries`, `date_of_birth`). Query params: `include_records=false`, `records_limit` (default 100, max 500).

See [docs/INTEGRATION_HISTORICAL_UPDATES.md](docs/INTEGRATION_HISTORICAL_UPDATES.md) for a full integration guide.

**Health check**

```
GET /health
```
