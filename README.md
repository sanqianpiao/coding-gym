Here’s a focused, “real-work” 12-week coding gym. it’s stack-agnostic but leans Go (primary) with Java alternatives. each week is a tiny, production-ish build with tests, observability, and an AI-assist angle.

# 12-Week Coding Gym (backend, realistic, AI-assisted)

## repo scaffolding (use for all weeks)

```
coding-gym/
  week01-redis-token-bucket/
  week02-outbox-kafka/
  week03-idempotency-dedupe/
  week04-notify-retry-dlq/
  week05-cache-aside-stampede/
  week06-cqrs-cdc-debezium/
  week07-circuit-breaker-bulkhead/
  week08-file-pipeline-s3-workers/
  week09-otel-metrics-tracing/
  week10-feature-flags-canary/
  week11-spec-driven-service/
  week12-capstone-pricing-feed/
common/
  docker-compose.yml  # kafka/redis/postgres/prometheus/grafana/zipkin
  Makefile            # up/down/test/lint
```

**Go:** `gin / fiber`, `sarama` or `franz-go` (Kafka), `pgx`, `redis/go-redis`, `OpenTelemetry`.
**Java (alt):** Spring Boot, Spring Kafka, JDBC, Lettuce/Redisson, Micrometer + OpenTelemetry.

---

## week 1 — redis token bucket rate limiter

**Goal:** implement per-key (user or IP) token bucket with atomicity.
**Tasks:** bucket state in Redis; Lua script (or MULTI+WATCH) for atomic take; TTL reset; a tiny HTTP demo endpoint.
**Tests:** concurrency test (100 goroutines/threads), boundary cases (0 tokens, burst).
**AI assist:** generate Lua, you review/trim.
**Stretch:** sliding-window log variant + benchmark.

## week 2 — outbox pattern (postgres → kafka)

**Goal:** exactly-once *effect* from DB to Kafka via outbox.
**Tasks:** transactional write to domain table + outbox; relay job scans outbox (status=NEW) → produce to Kafka → mark SENT.
**Tests:** crash between DB write and publish; relay idempotency.
**AI assist:** write SQL DDL + migration script.
**Stretch:** partition routing + backfill tool.

## week 3 — idempotency keys & dedupe store

**Goal:** safe “at-least once” APIs.
**Tasks:** POST /orders with `Idempotency-Key`; Redis store value+status; replay returns same result.
**Tests:** duplicate submits, concurrent same key, key TTL expiry.
**Stretch:** dedupe consumer on Kafka using `(key, partition, offset)` set with TTL.

## week 4 — notification service with retries, backoff, DLQ

**Goal:** robust async worker.
**Tasks:** Kafka topic `notify`; consumer with exponential backoff; after N attempts send to DLQ; small admin UI/endpoint to replay DLQ.
**Observability:** per-status metrics; failure rate alert.
**Stretch:** jittered backoff; poison-pill detection.

## week 5 — cache-aside + stampede control

**Goal:** fast reads without thundering herd.
**Tasks:** read-through with Redis; “mutex key” or single-flight; TTL + background refresh; negative caching.
**Tests:** 1k concurrent gets on cold key; latency distribution.
**Stretch:** stale-while-revalidate + request coalescing metrics.

## week  6 — CQRS read model with CDC (Debezium)

**Goal:** projection service.
**Tasks:** Postgres → Debezium → Kafka → projector builds a denormalized “search view” table; query API reads from view.
**Tests:** out-of-order updates; projector restart replays.
**Stretch:** add snapshot + compaction topic.

## week 7 — resilience: circuit breaker + bulkhead

**Goal:** defend upstream during provider slowness.
**Tasks:** wrap outbound HTTP client with CB (half-open probe); limit concurrent calls per host (semaphore).
**Tests:** inject latency/errors; verify open/half-open/closed transitions.
**Stretch:** fallback cache; per-tenant rate limits.

## week 8 — file pipeline: S3 → workers → results

**Goal:** durable, resumable batch.
**Tasks:** upload CSV to S3; enqueue job; worker pool processes rows with checkpoints every N records; progress API.
**Tests:** crash mid-job; resume; partial failure report.
**Stretch:** exactly-once row processing via id/key ledger.

## week 9 — observability deep-dive (OpenTelemetry)

**Goal:** traces + metrics + logs that tell a story.
**Tasks:** instrument two services + one worker; propagate context across HTTP/Kafka; Prometheus counters/histograms; Zipkin/Tempo traces.
**SLOs:** define p95 latency and error-budget burn math.
**Stretch:** exemplars linking metrics to traces.

## week 10 — feature flags & canary simulator

**Goal:** safe releases.
**Tasks:** simple flag store (DB/Redis); per-percentage rollout; canary evaluator compares key metrics between control vs canary and auto-rolls back.
**Tests:** synthetic traffic; guardrail triggers rollback.
**Stretch:** segment by tenant/geo.

## week 11 — spec-driven vibe coding mini-service

**Goal:** practice “spec → AI → refine.”
**Tasks:** write a 1-page spec (APIs, models, SLAs); ask AI to generate service skeleton, tests, Docker; you do the hard edges (error cases, perf).
**Deliverable:** PR diff with your edits + a README “what AI got wrong.”
**Stretch:** add contract tests (OpenAPI + golden files).

## week 12 — capstone: “pricing feed” (mini-Agoda)

**Goal:** stitch pieces together.
**Flow:** provider POST → ingest (idempotency) → outbox → Kafka → rules engine → cache-aside read API → projections → notify.
**Non-functionals:** freshness <2s, backpressure, retries, metrics, flags.
**Demo:** tiny web page that queries read API and shows live updates.
**Stretch:** run a chaos day (kill services, add latency) & write a postmortem.

---

## weekly cadence (repeatable checklist)

* **Design first:** short ADR (goal, constraints, alternatives).
* **Implement:** minimal end-to-end path before polish.
* **Tests:** unit + a small load/concurrency test.
* **Observe:** expose 3–5 key metrics + one trace.
* **Readme:** how to run, assumptions, known gaps.
* **Retro:** 5 bullets—what worked / pain points / next.

---

## warm-ups (10–15 min, 3× per week)

* **Parsing kata:** parse/validate a gnarly input (CSV with quotes, JSON lines).
* **Concurrency micro-drills:** bounded worker pool; cancellable context; timeouts.
* **Data transforms:** group/aggregate/filter a 50k-row CSV; produce top-N report.
  (keep these tiny; the point is keyboard fluency.)

---

## scoring rubric (self-review each week)

* **Correctness (25%)**: passes tests, handles edge cases.
* **Reliability (25%)**: retries, idempotency, backoff, DLQ present.
* **Clarity (20%)**: readable code, clear boundaries, docstrings.
* **Observability (15%)**: useful metrics/traces/logs.
* **Performance (10%)**: basic load test & findings.
* **AI leverage (5%)**: you steered AI, didn’t copy blindly.

---

## how AI fits (practical prompts)

* “Generate a Redis Lua script for an atomic token bucket (params: capacity, refill\_rate, now\_ms). Keep under 30 lines.”
* “Write table DDL and an outbox relay that is restart-safe; include idempotent Kafka publish.”
* “Produce OpenAPI spec + server stubs; then contract tests for 404/429/5xx.”