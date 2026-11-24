# Архитектура GoNews

Документ описывает устройство сервиса, потоки данных и принятые решения.

---

## 1. Общая схема

```
                    ┌──────────────────────────────────────────┐
                    │        Android / iOS / Web / Swagger     │
                    └───────────────────┬──────────────────────┘
                                        │ HTTPS + JSON
                                        ▼
                    ┌──────────────────────────────────────────┐
                    │        L7 LB / CDN (в production)        │
                    └───────────────────┬──────────────────────┘
                                        │
        ┌───────────────────────────────┼───────────────────────────────┐
        ▼                               ▼                               ▼
┌─────────────────┐           ┌─────────────────┐           ┌─────────────────┐
│ GoNews API #1   │           │ GoNews API #2   │    ...    │ GoNews API #N   │
│ (stateless)     │           │ (stateless)     │           │ (stateless)     │
└───┬─────────┬───┘           └─────────────────┘           └─────────────────┘
    │         │
    │         └──────────────► Redis 7
    │                         кэш GET-ответов,
    │                         распределённый rate limit
    ▼
PostgreSQL 16
новости, пользователи,
устройства, очередь
переводов, настройки
    │
    │  push-события / итоги ингеста
    ▼
 Kafka 3 ────► push.Worker (consumer group) ────► FCM (Android)
                                               └─► APNs (iOS)

 Prometheus ◄── /metrics        Grafana ◄── Prometheus + Loki
 Promtail  ──► Loki
```

GoNews — **stateless HTTP-слой**: JWT проверяется локально, кэш и лимиты
хранятся в Redis, данные — в PostgreSQL. Любую реплику можно заменить без
потери состояния. Тяжёлые операции (ингест, перевод, push) выполняются
фоновыми воркерами внутри процесса; их можно вынести в отдельные deployment'ы.

---

## 2. Пакеты

| Пакет | Ответственность |
|---|---|
| `cmd/server` | точка входа, graceful shutdown, `healthcheck` для контейнера |
| `internal/app` | сборка зависимостей, планировщик, публикация событий |
| `internal/config` | чтение env и `.env`, значения по умолчанию, валидация |
| `internal/auth` | регистрация, вход, JWT, refresh-токены, bcrypt |
| `internal/cache` | Redis: кэш ответов, rate limit, health, инвалидация |
| `internal/events` | Kafka-шина и in-process fallback с единым интерфейсом |
| `internal/httpapi` | роутер, handlers, DTO, middleware, OpenAPI, docs UI |
| `internal/metrics` | Prometheus registry и все метрики |
| `internal/store` | PostgreSQL, миграции, SQL-запросы, rebind `?` → `$n` |
| `internal/model` | доменные структуры |
| `internal/news` | провайдеры, агрегатор, категории, теги, sanitize |
| `internal/translate` | провайдеры перевода, очередь, воркеры |
| `internal/push` | FCM, APNs, воркер доставки, dry-run |
| `internal/langdetect` | определение языка и нормализация кодов |
| `internal/platform` | генерация идентификаторов |
| `internal/testutil` | изолированные схемы PostgreSQL для тестов |
| `internal/version` | версия, коммит, дата сборки |

Зависимости направлены внутрь: `httpapi → store/auth/news/translate/push/cache/events`,
`app` связывает всё, `model` не зависит ни от чего.

---

## 3. Поток сбора новостей

1. Планировщик (`app.scheduler`) запускает `Aggregator.Run` при старте
   (если `GONEWS_INGEST_ON_START=true`) и далее по таймеру
   `GONEWS_INGEST_INTERVAL` (по умолчанию 10 минут).
2. Агрегатор параллельно опрашивает включённые провайдеры с ограничением
   `GONEWS_PROVIDER_LIMIT` (25 материалов на источник).
3. Каждый провайдер возвращает `[]model.IngestItem`:
   URL, заголовок, описание, текст, картинка, автор, язык, дата, score.
4. Нормализация:
   - очистка HTML и опасных конструкций (`news/sanitize.go`);
   - определение языка (`langdetect`), если источник его не указал;
   - канонизация URL (удаление UTM-меток, якорей, трекинг-параметров);
   - вычисление `unique_hash = SHA-256(canonical_url)`;
   - определение категории и тегов по словарю.
5. Запись в PostgreSQL в транзакции:
   - новый `unique_hash` → `INSERT`;
   - существующий → обновление счётчиков/полей без дублирования.
6. Для материалов с языком, отличным от целевого (`ru`), создаётся задача
   перевода `(article_id, target_lang)`.
7. Сохраняется `ingest_runs` со статистикой по каждому провайдеру.
8. Публикуется событие `ingest.completed` в Kafka-топик `gonews.news`.
9. Кэш HTTP-ответов инвалидируется (`gonews:http*`), воркер переводов
   получает сигнал `Notify()`, при `GONEWS_PUSH_BROADCAST=true` может уйти
   push о топ-новости.

Ошибка одного источника не прерывает запуск: она попадает в
`providers[slug].error`, метрику `gonews_provider_errors_total` и лог,
остальные источники продолжают работу.

---

## 4. Поток перевода

```
articles (lang != target)
        │  enqueue
        ▼
translation_jobs (status=pending)  ← PostgreSQL
        │  worker: UPDATE ... FOR UPDATE SKIP LOCKED
        ▼
Translator: google → lingva → mymemory → libretranslate
        │  success
        ▼
article_translations (title, summary, content, provider)
        │
        ▼
GET /news?lang=ru отдаёт перевод, иначе оригинал + status=pending
```

- Очередь durable: задачи лежат в PostgreSQL и переживают перезапуск.
- `FOR UPDATE SKIP LOCKED` позволяет нескольким воркерам и репликам безопасно
  разбирать очередь без дублей.
- Повторные попытки — с экспоненциальной задержкой, максимум попыток
  ограничен; после исчерпания задача получает `failed`.
- Circuit breaker: провайдер, вернувший ошибку, пропускается 2 минуты.
- Перевод выполняется по частям с ограничением `GONEWS_TRANSLATE_MAX_CHARS`
  (4500 символов), чтобы не превышать лимиты публичных API.
- Если перевод не нужен (язык совпадает), статья сразу отдаётся как
  `translation.status=original`.

Подробнее: [TRANSLATION.md](TRANSLATION.md).

---

## 5. Поток HTTP-запроса

```
Request
  → recoverer            (panic → 500 JSON)
  → request-id           (X-Request-ID)
  → access-log           (slog JSON, duration, status, ip, bytes)
  → CORS                 (настраиваемые origin)
  → body-limit           (2 MiB)
  → rate-limit           (Redis INCR или локальный bucket)
  → response cache       (только анонимные GET из белого списка)
  → optional/require auth (JWT → пользователь в context)
  → handler
  → store / aggregator / translator / push
  → JSON {data} | {error}
  → метрики Prometheus
```

Особенности:

- **Публичные методы** (`/news`, `/categories`, `/tags`, `/sources`,
  `/search`, комментарии) не требуют токена.
- **Опциональная авторизация** на `/news` и `/news/{id}`: если токен передан,
  в ответе появляются `liked` и `bookmarked`.
- **Обязательная авторизация** — всё под `/me/*`, лайки, закладки,
  комментарии, устройства, уведомления.
- **Админ** — роль `admin` в JWT или заголовок `X-Admin-Token`
  (constant-time сравнение).
- **Rate limit** — Redis `INCR` + `EXPIRE` на 60 секунд; при недоступности
  Redis автоматически используется локальный limiter.
- **Кэш** — только анонимные GET: лента, карточка, тренды, поиск, категории,
  теги, источники. Заголовок `X-Cache: HIT|MISS|BYPASS` показывает результат.

---

## 6. Хранилище

PostgreSQL 16+, все временные метки — `TIMESTAMPTZ` (UTC). Миграции
встроены в бинарник (`internal/store/migrations/*.sql`), применяются при
старте и учитываются в `schema_migrations`.

### Таблицы

| Таблица | Назначение |
|---|---|
| `sources` | каталог источников |
| `articles` | материалы, уникальный `unique_hash` |
| `article_translations` | переводы по языкам |
| `translation_jobs` | очередь переводов и retry-состояние |
| `tags`, `article_tags` | теги и связь many-to-many |
| `users` | пользователи, bcrypt-хеш, роль |
| `refresh_tokens` | хеши refresh-токенов, отзыв, user-agent |
| `likes`, `bookmarks` | социальные связи |
| `follows` | подписки на теги |
| `comments` | комментарии с `parent_id`, soft delete |
| `article_views` | история просмотров (одна запись на пару) |
| `devices` | FCM/APNs-токены, платформа, темы |
| `notifications` | внутренний фид |
| `notification_reads` | прочтения per-user |
| `ingest_runs` | история запусков |
| `settings` | служебные значения |

### Индексы

Ключевые индексы: `articles(published_at DESC)`,
`articles(source_id, published_at DESC)`,
`articles(category, published_at DESC)`,
`articles(hot_score DESC)`,
`articles(lang, published_at DESC)`,
`translation_jobs(status, next_attempt_at)`,
`notifications(user_id, created_at DESC)`,
`devices(platform, enabled)`.

### Пул соединений

`SetMaxOpenConns(25)`, `SetMaxIdleConns(10)`, lifetime 30 минут.
Метрики `gonews_db_pool_*` показывают насыщение.

### Ранжирование

`hot_score` рассчитывается при вставке/обновлении:

- свежесть (экспоненциальное затухание),
- вес источника (`SourceDef.Weight`),
- вовлечённость (лайки, комментарии, просмотры),
- бонус за совпадение с трендовыми тегами.

Сортировка `hot` использует `hot_score`, `latest` — `published_at`,
`oldest` — обратный порядок.

---

## 7. Аутентификация

- Пароль: bcrypt (cost по умолчанию библиотеки).
- Access-токен: JWT HS256, TTL 30 минут, claims: `sub`, `role`, `typ=access`.
- Refresh-токен: случайная строка, в БД хранится только SHA-256 хеш.
- Ротация: при `/auth/refresh` старый токен отзывается, выдаётся новый.
- Logout отзывает refresh-токен.
- Роль `admin` назначается:
  - при регистрации, если email в `GONEWS_ADMIN_EMAILS`;
  - вручную в БД при необходимости.
- `X-Admin-Token` позволяет автоматизации не заводить пользователя.

---

## 8. Push и уведомления

```
POST /admin/push
      │
      ▼
notifications (PostgreSQL)
      │
      ▼
Kafka: gonews.notifications
      │
      ▼
push.Worker (consumer group gonews-push)
      ├── FCM HTTP v1  → Android
      └── APNs HTTP/2  → iOS
```

- `push.Service` умеет:
  - `Broadcast` — всем устройствам;
  - `SendToUser` — устройствам пользователя;
  - `BroadcastTopNews` — автоматическая рассылка топ-новости с антидублем
    по интервалу.
- HTTP-запрос возвращается после записи в БД и публикации события, не ожидая
  ответа FCM/APNs.
- Если Kafka недоступна, сервис деградирует к синхронной доставке.
- Если ключи FCM/APNs не заданы, используется `logSender` (dry-run):
  событие проходит весь путь, вместо сетевого вызова пишется лог.
- Внутренний фид работает независимо от транспорта: `/me/notifications`.
- Невалидные токены (FCM `UNREGISTERED`, APNs `410`/`400`) отключаются.

Подробнее: [NOTIFICATIONS.md](NOTIFICATIONS.md).

---

## 9. Кэш, события и наблюдаемость

### Redis

- Кэш анонимных GET-ответов, TTL 30 секунд (`GONEWS_CACHE_TTL`).
- Ключи `gonews:http:<uri>|<accept-language>`; инвалидация по префиксу
  после ингеста.
- Rate limit `gonews:rl:<scope>:<ip>`.
- При недоступности Redis приложение продолжает работать без кэша.

### Kafka

| Топик | Содержимое | Потребитель |
|---|---|---|
| `gonews.notifications` | события push | `push.Worker` |
| `gonews.news` | итоги ингеста | внешние потребители |

- Публикация синхронная, ошибка видна вызывающему коду.
- Без `GONEWS_KAFKA_BROKERS` используется in-process шина с теми же
  обработчиками.
- Партиции топика уведомлений задают параллелизм consumer group.

### Prometheus / Grafana / Loki

- `/metrics` — HTTP, БД, кэш, Kafka, push, ингест, перевод, `go_*`.
- Grafana-дашборд и алерты провижинятся из `deploy/`.
- JSON-логи в stdout, Promtail → Loki, `X-Request-ID` для корреляции.

Подробнее: [INFRASTRUCTURE.md](INFRASTRUCTURE.md) и
[OBSERVABILITY.md](OBSERVABILITY.md).

---

## 10. Надёжность

- Graceful shutdown по `SIGINT`/`SIGTERM` с таймаутом.
- Встроенные миграции применяются при старте и идемпотентны.
- Паника в handler перехватывается и превращается в JSON 500.
- Внешние HTTP-клиенты имеют таймауты; агрегатор ограничивает тело ответа.
- Ошибки источников изолированы.
- Retention-задача раз в сутки удаляет материалы старше
  `GONEWS_RETENTION_DAYS`.
- `/healthz` — liveness, `/readyz` — проверка PostgreSQL, Redis и Kafka.
- При недоступности PostgreSQL реплика исключается балансировщиком (503).

---

## 11. Масштабирование

API уже stateless, поэтому масштабируется горизонтально:

1. **Реплики API** за L7-балансировщиком; JWT проверяется локально.
2. **Redis Cluster** для кэша и распределённого rate limit.
3. **PostgreSQL read replicas + PgBouncer**; запись — в primary.
4. **Kafka**: 3+ брокера, RF=3, больше партиций, несколько реплик
   push-воркера в одной consumer group.
5. **CDN** перед API для анонимной ленты (`s-maxage=15`).
6. **Выделение воркеров**: ингест и перевод можно запускать отдельными
   deployment'ами с той же кодовой базой.
7. **Партиционирование** `articles` по месяцу при десятках миллионов строк.

Расчёт до 1M RPS и поведение под перегрузкой: [PERFORMANCE.md](PERFORMANCE.md).

---

## 12. Связанные документы

- [API.md](API.md) — методы и модели.
- [AGGREGATORS.md](AGGREGATORS.md) — источники и парсеры.
- [TRANSLATION.md](TRANSLATION.md) — перевод.
- [DEPLOYMENT.md](DEPLOYMENT.md) — эксплуатация.
- [INFRASTRUCTURE.md](INFRASTRUCTURE.md) — PostgreSQL, Redis, Kafka.
- [OBSERVABILITY.md](OBSERVABILITY.md) — метрики, логи, алерты.
- [NOTIFICATIONS.md](NOTIFICATIONS.md) — push для Android и iOS.
- [PERFORMANCE.md](PERFORMANCE.md) — масштабирование.
- [LOAD-TESTING.md](LOAD-TESTING.md) — нагрузочные сценарии.
- [CI-CD.md](CI-CD.md) — сборка и релизы.
