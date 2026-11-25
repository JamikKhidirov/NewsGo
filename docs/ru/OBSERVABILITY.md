# Мониторинг, логи и алерты

GoNews экспортирует метрики в формате Prometheus, пишет структурированные JSON-логи
в stdout и поставляет их в Loki через Promtail. Grafana содержит готовый дашборд.

## Быстрый доступ

| Сервис | URL | Логин по умолчанию |
|---|---|---|
| Grafana | http://localhost:3000 | `admin` / `admin` (`GRAFANA_USER`, `GRAFANA_PASSWORD`) |
| Prometheus | http://localhost:9090 | — |
| Loki | http://localhost:3100 | — |
| Метрики приложения | http://localhost:8080/metrics | — |

Дашборд **GoNews — API, ingestion and delivery** провижинится автоматически
(`deploy/grafana/dashboards/gonews.json`).

## Каталог метрик

### HTTP

| Метрика | Тип | Метки | Смысл |
|---|---|---|---|
| `gonews_http_requests_total` | counter | `method`, `route`, `status` | все запросы |
| `gonews_http_request_duration_seconds` | histogram | `method`, `route` | задержка, 17 бакетов от 1 мс |
| `gonews_http_in_flight_requests` | gauge | — | одновременные запросы |
| `gonews_http_response_bytes_total` | counter | `route` | объём ответов |

`route` — шаблон chi (`/api/v1/news/{id}`), поэтому кардинальность ограничена
и не растёт от количества статей.

### База и кэш

| Метрика | Тип | Метки |
|---|---|---|
| `gonews_db_pool_open_connections` | gauge | — |
| `gonews_db_pool_in_use` | gauge | — |
| `gonews_db_pool_wait_total` | gauge | — |
| `gonews_cache_operations_total` | counter | `op=get|set|delete`, `result=hit|miss|ok|error` |

### Kafka и push

| Метрика | Тип | Метки |
|---|---|---|
| `gonews_kafka_messages_total` | counter | `direction=produce|consume`, `topic`, `result=ok|error|dropped` |
| `gonews_push_deliveries_total` | counter | `platform=android|ios`, `result=sent|failed` |
| `gonews_push_queue_depth` | gauge | — (локальная шина) |

### Ингест и перевод

| Метрика | Тип | Метки |
|---|---|---|
| `gonews_ingest_runs_total` | counter | — |
| `gonews_ingest_articles_total` | counter | `result=inserted|updated` |
| `gonews_ingest_duration_seconds` | histogram | — |
| `gonews_ingest_run_last_finished_timestamp_seconds` | gauge | — |
| `gonews_provider_fetched_total` | counter | `source` |
| `gonews_provider_errors_total` | counter | `source` |
| `gonews_translation_jobs_total` | counter | `result` |

Стандартные `go_*` и `process_*` метрики включены автоматически.

## Полезные запросы

```promql
# RPS
sum(rate(gonews_http_requests_total[5m]))

# доля 5xx
sum(rate(gonews_http_requests_total{status=~"5.."}[5m]))
  / clamp_min(sum(rate(gonews_http_requests_total[5m])), 0.001)

# p95 и p99 (без админских batch-маршрутов: ингест и purge длятся десятки секунд)
histogram_quantile(0.95, sum(rate(gonews_http_request_duration_seconds_bucket{route!~"/api/v1/admin/.*"}[5m])) by (le))
histogram_quantile(0.99, sum(rate(gonews_http_request_duration_seconds_bucket{route!~"/api/v1/admin/.*"}[5m])) by (le))

# самые медленные маршруты
topk(5, histogram_quantile(0.99,
  sum(rate(gonews_http_request_duration_seconds_bucket{route!~"/api/v1/admin/.*"}[5m])) by (route, le)))

# cache hit ratio
sum(rate(gonews_cache_operations_total{op="get",result="hit"}[5m]))
  / clamp_min(sum(rate(gonews_cache_operations_total{op="get"}[5m])), 0.001)

# ошибки провайдеров новостей
topk(10, sum by (source) (rate(gonews_provider_errors_total[15m])))

# насыщение пула PostgreSQL
gonews_db_pool_in_use / clamp_min(gonews_db_pool_open_connections, 1)
```

## Алерты

Файл `deploy/prometheus/alerts.yml` содержит правила:

| Алерт | Условие | Severity |
|---|---|---|
| `GoNewsDown` | `up{job="gonews"} == 0` 1 мин | critical |
| `GoNewsHighErrorRate` | 5xx > 5% за 5 мин | critical |
| `GoNewsHighLatencyP99` | p99 > 1 c за 10 мин | warning |
| `GoNewsDatabasePoolSaturated` | in-use > 90% open | warning |
| `GoNewsCacheDown` | Redis exporter недоступен | warning |
| `GoNewsIngestStale` | ингест не завершался > 1 ч | warning |
| `GoNewsKafkaPublishFailures` | > 0.1 ошибок/с | warning |
| `GoNewsPushFailures` | > 10% неудачных доставок | warning |

Проверка синтаксиса:

```bash
docker compose exec prometheus promtool check config /etc/prometheus/prometheus.yml
docker compose exec prometheus promtool check rules /etc/prometheus/alerts.yml
```

## Логи

Формат — JSON (`GONEWS_LOG_FORMAT=json` или `GONEWS_ENV=production`):

```json
{"time":"2026-11-24T09:00:00.123Z","level":"INFO","msg":"http request",
 "method":"GET","path":"/api/v1/news","status":200,"bytes":18422,
 "duration_ms":7,"ip":"172.18.0.1","request_id":"req_01J..."}
```

Поля: `time`, `level`, `msg`, `method`, `path`, `status`, `bytes`,
`duration_ms`, `ip`, `request_id`. Для ошибок добавляются `error` и контекст
(`provider`, `topic`, `notification_id` и т.п.). `X-Request-ID` возвращается
клиенту и позволяет связать запрос с логами.

Promtail собирает stdout всех контейнеров через Docker socket и добавляет метки
`container`, `service`, `level`. Примеры запросов в Grafana Explore:

```logql
{service="gonews"} |= "error"
{service="gonews"} | json | status >= 500
{service="gonews"} | json | duration_ms > 500
{service="gonews"} |= "push delivered"
```

## SLO и ёмкость

Базовые цели для одного инстанса (4 vCPU, 2 GB):

| Показатель | Цель |
|---|---|
| Доступность API | 99.9% в месяц |
| p95 чтения новостей | < 150 мс |
| p99 чтения новостей | < 500 мс |
| p99 записи (лайк/комментарий) | < 300 мс |
| Доля 5xx | < 0.1% |

Метрики для расчёта error budget:

```promql
1 - (
  sum(rate(gonews_http_requests_total{status=~"5.."}[30d]))
  / sum(rate(gonews_http_requests_total[30d]))
)
```

## Дежурство

1. `GoNewsDown` → проверьте `docker compose ps`, `docker compose logs gonews`, `/readyz`.
2. Рост 5xx → ищите `internal_error` в Loki, проверьте PostgreSQL и внешние API.
3. p99 растёт → смотрите пул БД, cache hit ratio, медленные маршруты, GC (`go_gc_duration_seconds`).
4. Ингест не идёт → `gonews_provider_errors_total`, сетевые таймауты источников.
5. Push-ошибки → проверьте срок действия ключей FCM/APNs и долю `Unregistered`.
