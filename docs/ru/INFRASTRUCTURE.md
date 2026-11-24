# Инфраструктура: PostgreSQL, Redis и Kafka

GoNews работает как stateless HTTP-слой и использует три внешних сервиса:

| Компонент | Назначение | Обязателен | Деградация без него |
|---|---|---|---|
| PostgreSQL 16 | основное хранилище (новости, пользователи, устройства, задания перевода) | да | сервис не стартует |
| Redis 7 | кэш ответов, распределённый rate limit | нет | кэш и лимиты работают в памяти процесса |
| Kafka 3.x | события пуш-уведомлений и итогов ингеста | нет | события доставляются через in-process шину |

Схема подключения задаётся переменными окружения:

```dotenv
GONEWS_DATABASE_URL=postgres://gonews:gonews@postgres:5432/gonews?sslmode=disable
GONEWS_REDIS_URL=redis://redis:6379/0
GONEWS_KAFKA_BROKERS=kafka:9092
GONEWS_KAFKA_PUSH_TOPIC=gonews.notifications
GONEWS_KAFKA_NEWS_TOPIC=gonews.news
GONEWS_KAFKA_GROUP=gonews-push
```

## PostgreSQL

### Схема

Все таблицы создаются миграцией `internal/store/migrations/0001_init.sql` при старте.
Миграции идемпотентны (`CREATE TABLE IF NOT EXISTS`) и учитываются в `schema_migrations`.

Ключевые таблицы:

| Таблица | Содержимое |
|---|---|
| `sources` | каталог RSS/JSON-источников |
| `articles` | нормализованные новости, уникальность по `unique_hash` |
| `article_translations` | переводы заголовка/анонса/текста |
| `translation_jobs` | очередь переводов с backoff |
| `tags`, `article_tags` | теги и связи |
| `users`, `refresh_tokens` | аккаунты и ротация refresh-токенов |
| `likes`, `bookmarks`, `follows`, `comments`, `article_views` | социальные данные |
| `devices` | FCM/APNs токены, платформа, темы |
| `notifications`, `notification_reads` | in-app уведомления |
| `ingest_runs` | история запусков агрегатора |
| `settings` | служебные ключи (например, троттлинг breaking news) |

Все временные метки — `TIMESTAMPTZ` в UTC. Индексы покрывают выдачу по дате,
источнику, категории, тегам и пользовательские выборки.

### Пул соединений

Приложение открывает до 25 соединений на инстанс (`SetMaxOpenConns(25)`),
10 idle, время жизни 30 минут. Для N реплик API держите
`max_connections >= 25*N + 20` (резерв для миграций и экспортёра).

### Бэкапы

```bash
# логический дамп
docker compose exec -T postgres pg_dump -U gonews -Fc gonews > gonews.dump

# восстановление
docker compose exec -T postgres pg_restore -U gonews -d gonews --clean < gonews.dump
```

Рекомендуется ежедневный `pg_dump` + WAL-архивирование (pgBackRest / wal-g) для
point-in-time recovery. Проверяйте восстановление на отдельной базе не реже
раза в месяц.

### Диагностика

```sql
-- размер базы и таблиц
SELECT relname, pg_size_pretty(pg_total_relation_size(relid)) AS size
FROM pg_catalog.pg_statio_user_tables ORDER BY pg_total_relation_size(relid) DESC;

-- долгие запросы
SELECT pid, now() - query_start AS duration, query
FROM pg_stat_activity WHERE state = 'active' ORDER BY duration DESC;

-- неиспользуемые индексы
SELECT relname, indexrelname, idx_scan FROM pg_stat_user_indexes WHERE idx_scan = 0;
```

## Redis

Используется для двух задач:

1. **Кэш HTTP-ответов.** Кэшируются только анонимные `GET`:
   `/api/v1/news`, `/api/v1/news/{id}`, `/api/v1/news/trending`, `/api/v1/search`,
   `/api/v1/categories`, `/api/v1/tags`, `/api/v1/sources`.
   TTL — `GONEWS_CACHE_TTL` (по умолчанию 30 секунд). После ингеста кэш
   инвалидируется по префиксу `gonews:http*`.
   Заголовок `X-Cache: HIT|MISS|BYPASS` показывает результат.
2. **Распределённый rate limit.** `INCR` + `EXPIRE` на ключ
   `gonews:rl:<scope>:<ip>` с окном 60 секунд. Лимиты:
   `GONEWS_RATE_LIMIT_PER_MINUTE` (общий) и 20/мин на авторизацию.
   Если Redis недоступен, автоматически включается локальный limiter.

Кэш никогда не влияет на корректность: при ошибке Redis запрос выполняется
обычным путём. Пароли, токены и персональные ответы (`/api/v1/me/*`) не кэшируются.

Политика вытеснения в compose: `allkeys-lru`, лимит 512 MB.

## Kafka

### Топики

| Топик | Ключ | Содержимое | Потребитель |
|---|---|---|---|
| `gonews.notifications` | `notification.id` | события пуш-доставки | `push.Worker` (группа `gonews-push`) |
| `gonews.news` | `ingest` | итоги запуска агрегатора | внешние аналитические потребители |

Событие уведомления:

```json
{
  "id": "n_01J...",
  "userId": "u_01J...",
  "title": "Главное сейчас",
  "body": "Заголовок новости",
  "articleId": "a_01J...",
  "kind": "breaking",
  "createdAt": "2026-11-24T09:00:00Z"
}
```

`userId` отсутствует у broadcast-событий — воркер рассылает их на все активные
устройства.

### Гарантии

* Публикация синхронная (`ProduceSync`), ошибка видна вызывающему коду.
* Если Kafka недоступна, `push.Service` деградирует к прямой доставке, поэтому
  уведомление не теряется.
* Потребитель коммитит смещения после обработки батча. Ошибка обработки
  логируется; событие не блокирует очередь (at-least-once на уровне батча).
* Для горизонтального масштабирования увеличьте число партиций и реплик
  приложения: участники одной consumer group делят партиции.

### Проверка

```bash
# список топиков
docker compose exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list

# чтение событий уведомлений
docker compose exec kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic gonews.notifications --from-beginning --max-messages 5

# лаг группы
docker compose exec kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 --describe --group gonews-push
```

### Надёжность в production

* Минимум 3 брокера, `replication.factor=3`, `min.insync.replicas=2`.
* `acks=all` (в коде используется синхронная отправка).
* Отдельные диски под лог, мониторинг свободного места.
* Включите SASL/SCRAM + TLS и передавайте брокеры через секреты.
* Для строгой гарантии «ровно один раз» используйте транзакционный outbox:
  запись события в ту же транзакцию PostgreSQL, что и уведомление, и отдельный
  публикатор. Текущая реализация даёт at-least-once и fallback-доставку.

## Порядок запуска

```bash
docker compose up -d postgres redis kafka
docker compose up -d --build gonews prometheus grafana loki promtail
docker compose ps
curl -s localhost:8080/readyz | jq
```

`/readyz` возвращает состояние каждого компонента:

```json
{"status":"ready","components":{"postgres":"ok","redis":"ok","kafka":"kafka"}}
```

Если компонент недоступен, статус — `503`, а в `components` будет текст ошибки.
