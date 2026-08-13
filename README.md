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
for f in migrations/*.sql; do mysql -u root -p sanctions < "$f"; done

# Configure
cp .env.example .env
# Edit .env with your DB credentials
```

Apply migration `006_ngram_fulltext.sql` before production screening. It requires `ngram_token_size=2` in MySQL (see **Infrastructure** below).

## Seed

```bash
./bin/seeder /path/to/sanctions.json
```

Feed files are often **multi‑GB (up to ~15 GB)**. The seeder does one sequential layout scan per file (byte-level, not full JSON parse), shows progress every few seconds, and writes a sidecar cache `sanctions.json.section-index` (size + mtime). Unchanged files reuse the cache on the next run so seeding skips the full scan.

### Inactivating records absent from the feed

Records missing from the feed are set to `active_status = 'Inactive'` only when the file is the whole universe of records. The seeder requires **both** of the following before it will do that:

1. `_meta.feed_scope` is exactly `complete`. Any other value, and any file without the field, is treated as a partial feed.
2. The incoming row count and the declared `_meta.record_count` are each at least 90% of the rows already in the database.

If either check fails, the seeder logs `WARN: skipping removed-from-feed inactivation` and applies only the adds and changes in the file. **Always rebuild** `bin/seeder` **from the current source before a load** — these checks live in the binary, so a stale build can still wipe the database with a delta file.

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
  "min_score": 60,
  "include_notes": false,
  "include_details": false
}
```

- `include_notes` (default `false`) — load `profile_notes` in each match (use `GET /api/records/{id}` for full detail).
- `include_details` (default `false`) — load dates, countries, images, descriptions, associations.

**Batch screen (up to 50 names)**

```
POST /api/screen/batch
```

```json
{
  "names": ["John Smith", "Jane Doe"],
  "search_type": "individual",
  "min_score": 60,
  "include_notes": false,
  "include_details": false
}
```

Each server log line for screening includes phase timings (`fetch`, `expand`, `score`, `like_retry`, `hydrate`, `total`).

### Shadow scoring

`SCREEN_SHADOW_SCORING=true` runs a candidate replacement scorer alongside the
live one. It is an **observation mode and changes nothing**: the same records
come back, scored and ordered by the live scorer exactly as before. Each result
gains `shadow_score` and `shadow_matched_name`, and every screen logs a
comparison line:

```
screen shadow query="..." type=individual min_score=75 live_alerts=8 agreed=3 suppressed=5 promoted=1 mean_delta=-14
```

- `suppressed` — the live scorer alerts, the candidate scorer does not.
- `promoted` — the candidate scorer alerts, the live scorer does not. These
  records are **not** returned; they are counted so a run can measure what the
  change would surface, not only what it would hide.

The candidate scorer weights name tokens by how rare they are, scores position
along the name chain rather than treating a name as a bag of tokens, and caps
matches assembled only from very common name parts. See
`analysis/SCORING_PLAN.md` for the measurements behind it.

At startup the service builds the token-frequency table with one pass over
`sanctions_names`, in the background. Until that finishes it falls back to a
static table of common Arabic name parts, so early requests are scored
sensibly but not identically — allow for the load to complete before taking
measurements. The table is only built when this flag is on.

**List records**

```
GET /api/records?page=1&per_page=25
GET /api/records?first_name=nouf&last_name=alkahtani&record_type=Individual&active_status=Active
```

Filters: `first_name`, `last_name` (matched against the primary name), `record_type`, `active_status`.

`first_name` and `last_name` are **prefix** matches — `alkahtani` finds `Alkahtani`, but `kahtani` does not. This is what lets migration `008_name_prefix_indexes.sql` serve them as an index range scan instead of scanning every name row. For fuzzy, mid-word, alias, or transliteration matching, use `POST /api/screen` instead; that is the endpoint built for recall.

**Get record**

```
GET /api/records/{id}
```

**Seed run history**

```
GET /api/historical_updates?page=1&per_page=25
```

Returns seeder run timestamps, aggregate change counts, interval since the previous run, and per-record snapshots (`display_name`, `countries`, `date_of_birth`). Query params: `include_records=false`, `records_limit` (default 100, max 500).

**Health check**

```
GET /health
```



## Infrastructure (minimum for production screening)

These are **server/DB settings**, not application code. Minimum recommendations for a Dow Jones–scale feed (millions of records):

### MySQL 8.0+


| Setting                     | Minimum           | Notes                                                                                                                                                                                                                                                                                                                                                        |
| --------------------------- | ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `ngram_token_size`          | `2`               | Required for migration `006_ngram_fulltext.sql`. Add to `my.cnf`: `[mysqld]` → `ngram_token_size=2`, then restart.                                                                                                                                                                                                                                           |
| `innodb_ft_enable_stopword` | `OFF`             | The default stopword list contains two-letter words (`as`, `in`, `is`, `of`, `on`, `or`, `to`, …). At `ngram_token_size=2` those are exactly the bigrams inside ordinary names — `Nasser` tokenizes to `na as ss se er`, and `as` would be dropped — so the ngram fallback silently misses names. Set this OFF and rebuild `sanctions_names_ngram_fulltext`. |
| RAM                         | **8 GB+**         | 16 GB+ preferred for FULLTEXT on multi‑million `sanctions_names` rows.                                                                                                                                                                                                                                                                                       |
| Storage                     | **SSD**           | LIKE/ngram/FULLTEXT on HDD will be slow.                                                                                                                                                                                                                                                                                                                     |
| `innodb_buffer_pool_size`   | **50–70% of RAM** | e.g. 4G on an 8G box.                                                                                                                                                                                                                                                                                                                                        |


After changing `ngram_token_size`, run migrations including `006_ngram_fulltext.sql`.

### Application server (Go API)


| Resource            | Minimum          | Notes                                                                 |
| ------------------- | ---------------- | --------------------------------------------------------------------- |
| CPU                 | **2 vCPU**       | Scoring is CPU-heavy; 4+ vCPU under concurrent load.                  |
| RAM                 | **512 MB–1 GB**  | Go process; DB holds the data.                                        |
| `DB_MAX_OPEN_CONNS` | **50** (default) | Raise if many concurrent screens; keep below MySQL `max_connections`. |




### Verifying screening uses the FULLTEXT index

`sanctions_names` carries two FULLTEXT indexes over the same column list
(`sanctions_names_fulltext` with the default word parser, `sanctions_names_ngram_fulltext`
with the ngram parser), so every candidate query pins one with `FORCE INDEX` and
filters on `MATCH ... AGAINST` in the **WHERE** clause. `EXPLAIN` on those queries
must report `type: fulltext` for `sn`. If it reports `ALL`, the index is not being
used and each screen degrades to a full scan of `sanctions_names` — tens of seconds
per request on a multi-million-row feed.

### Optional (recommended at scale)

- **Slow query log** — `long_query_time=1`, inspect FT/ngram queries.
- **MySQL** `max_connections` ≥ app pool × replica count + headroom (e.g. 100+).
- **Do not expose** `ENABLE_PPROF=true` on `:6060` to the public internet.
- **Read replica** — only if DB CPU/disk is saturated after app optimizations; screening is read-heavy.
- Set `SCREEN_USE_LIKE_FALLBACK=true` only if ngram index cannot be created (legacy fallback; slow).



### Seeder host (one-off / scheduled loads)


| Resource          | Minimum                                             |
| ----------------- | --------------------------------------------------- |
| RAM               | **8 GB+**                                           |
| Disk              | **2× feed size** (~30 GB for a 15 GB JSON)          |
| DB write timeouts | Seeder uses 7200s read/write timeouts automatically |


