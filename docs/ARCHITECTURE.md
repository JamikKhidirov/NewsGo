# Архитектура GoNews

Документ описывает устройство сервиса, потоки данных и принятые решения.

---

## 1. Общая схема

```
                        ┌──────────────────────────────────────────┐
                        │              Мобильные клиенты            │
                        │            Android / iOS / Web            │
                        └───────────────────┬──────────────────────┘
                                            │ HTTPS + JSON
                                            ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                              GoNews (один процесс)                        │
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │ HTTP API (chi)                                                      │  │
│  │  middleware: recoverer, request-id, access-log, CORS, body-limit,   │  │
│  │              rate-limit, auth, admin                                │  │
│  │  handlers: news, auth, social, personalization, notifications,      │  │
│  │            admin, docs, metrics                                     │  │
│  └───────┬───────────────────────┬──────────────────────┬──────────────┘  │
│          │                       │                      │                 │
│          ▼                       ▼                      ▼                 │
│  ┌───────────────┐      ┌────────────────┐     ┌────────────────┐         │
│  │ Store (SQLite)│◄────►│ Aggregator     │     │ Translate      │         │
│  │ migrations    │      │ 26 providers   │     │ worker + queue │         │
│  └───────┬───────┘      └───────┬────────┘     └───────┬────────┘         │
│          │                      │                      │                 │
│          │              ┌───────▼────────┐     ┌───────▼────────┐         │
│          │              │ RSS / JSON     │     │ Google, Lingva │         │
│          │              │ внешние API    │     │ MyMemory, Libre│         │
│          │              └────────────────┘     └────────────────┘         │
│          │                                                                │
│          │              ┌────────────────┐     ┌────────────────┐         │
│          └─────────────►│ Push service   │────►│ FCM HTTP v1    │         │
│                         │ + notification │     │ (или dry-run)  │         │
│                         │   feed         │     └────────────────┘         │
│                         └────────────────┘                                │
└───────────────────────────────────────────────────────────────────────────┘
                                            │
                                            ▼
                                   data/gonews.db (SQLite WAL)
```

Сервис — **монолит с одним бинарником**. Это осознанный выбор: он проще в
эксплуатации, не требует CGO и внешней БД, а SQLite в WAL-режиме выдерживает
нагрузку небольшого и среднего новостного приложения.

---

## 2. Пакеты

| Пакет | Ответственность |
|---|---|
| `cmd/server` | точка входа, флаги, graceful shutdown, сигналы |
| `internal/app` | сборка зависимостей, планировщик сбора и retention |
| `internal/config` | чтение env и `.env`, значения по умолчанию, валидация |
| `internal/auth` | регистрация, вход, JWT, refresh-токены, bcrypt |
| `internal/httpapi` | роутер, handlers, DTO, middleware, OpenAPI, метрики |
| `internal/store` | SQLite, миграции, все SQL-запросы |
| `internal/model` | доменные структуры |
| `internal/news` | провайдеры, агрегатор, категории, теги, sanitize |
| `internal/translate` | провайдеры перевода, очередь, воркеры |
| `internal/push` | FCM HTTP v1, dry-run sender, сервис рассылки |
| `internal/langdetect` | определение языка и нормализация кодов |
| `internal/platform` | генерация идентификаторов |
| `internal/version` | версия, коммит, дата сборки |

Зависимости направлены внутрь: `httpapi → store/auth/news/translate/push`,
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
5. Запись в SQLite в транзакции:
   - новый `unique_hash` → `INSERT`;
   - существующий → обновление счётчиков/полей, но без дублирования.
6. Для материалов с языком, отличным от целевого (`ru`), создаётся задача
   перевода `(article_id, target_lang)`.
7. Сохраняется `ingest_runs` со статистикой по каждому провайдеру.
8. После успешного запуска воркер переводов получает сигнал `Notify()`,
   а при `GONEWS_PUSH_BROADCAST=true` может уйти push о топ-новости.

Ошибка одного источника не прерывает запуск: она попадает в
`providers[slug].error`, остальные источники продолжают работу.

---

## 4. Поток перевода

```
articles (lang != target)
        │  enqueue
        ▼
translation_jobs (status=pending)
        │  worker loop (N воркеров)
        ▼
Translator: google → lingva → mymemory → libretranslate
        │  success
        ▼
article_translations (title, summary, content, provider)
        │
        ▼
GET /news?lang=ru отдаёт перевод, иначе оригинал + status=pending
```

- Очередь durable: задачи лежат в SQLite и переживают перезапуск.
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
  → access-log           (slog, duration, status, ip)
  → CORS                 (настраиваемые origin)
  → body-limit           (2 MiB)
  → rate-limit           (180/мин на IP)
  → optional/require auth (JWT → пользователь в context)
  → handler
  → store / aggregator / translator / push
  → JSON {data} | {error}
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
- Лимит запросов — token bucket в памяти процесса; для нескольких инстансов
  нужен внешний limiter (см. раздел «Масштабирование»).

---

## 6. Хранилище

SQLite, файл `data/gonews.db`, режим WAL, `busy_timeout`, foreign keys.

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
| `devices` | FCM-токены и темы |
| `notifications` | внутренний фид |
| `notification_reads` | прочтения per-user |
| `ingest_runs` | история запусков |
| `settings` | служебные значения |

### Индексы

Ключевые индексы: `articles(published_at DESC)`,
`articles(source_id, published_at DESC)`,
`articles(category, published_at DESC)`,
`articles(hot_score DESC)`,
`translation_jobs(status, next_attempt_at)`,
`notifications(user_id, created_at DESC)`.

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

- `push.Service` умеет:
  - `Broadcast` — всем устройствам;
  - `SendToUser` — устройствам пользователя;
  - `BroadcastTopNews` — автоматическая рассылка топ-новости с антидублем
    по интервалу.
- Реальный sender — FCM HTTP v1 (OAuth2 service account).
- Если FCM не настроен, используется `logSender` (dry-run): сообщение
  логируется, но запись в `notifications` создаётся всегда.
- Внутренний фид работает независимо от FCM: `/me/notifications`.

---

## 9. Надёжность

- Graceful shutdown по `SIGINT`/`SIGTERM` с таймаутом.
- Встроенные миграции применяются при старте.
- Паника в handler перехватывается и превращается в JSON 500.
- Внешние HTTP-клиенты имеют таймауты; агрегатор ограничивает тело ответа.
- Ошибки источников изолированы.
- Retention-задача раз в сутки удаляет материалы старше
  `GONEWS_RETENTION_DAYS`.
- Health/readiness проверяют БД.
- Метрики Prometheus: uptime, запросы, классы ответов, сборы, push.

---

## 10. Масштабирование

Текущая архитектура рассчитана на один инстанс. При росте нагрузки:

1. **Read replicas / внешняя БД.** Заменить SQLite на PostgreSQL:
   `store` инкапсулирует SQL, но потребуется адаптация диалекта.
2. **Горизонтальное масштабирование API.** Вынести rate limit в Redis,
   сделать агрегатор отдельным worker-процессом (leader election),
   переводы — в отдельный пул.
3. **CDN** для изображений и статики, кэширование лент.
4. **Очередь** (NATS/Redis Streams) вместо SQLite-таблицы задач.
5. **Push** через собственный сервис с батчингом.

До этого момента один бинарник + SQLite остаются оптимальным выбором.

---

## 11. Связанные документы

- [API.md](API.md) — методы и модели.
- [AGGREGATORS.md](AGGREGATORS.md) — источники и парсеры.
- [TRANSLATION.md](TRANSLATION.md) — перевод.
- [DEPLOYMENT.md](DEPLOYMENT.md) — эксплуатация.
- [CI-CD.md](CI-CD.md) — сборка и релизы.
