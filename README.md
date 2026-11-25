<div align="center">

<img src="docs/assets/banner.svg" alt="GoNews — Russian-first news aggregation backend" width="100%" />

<a href="https://github.com/JamikKhidirov/NewsGo">
  <img src="https://readme-typing-svg.demolab.com?font=JetBrains+Mono&weight=600&size=21&pause=1200&color=38BDF8&center=true&vCenter=true&width=860&lines=26+%D0%B8%D1%81%D1%82%D0%BE%D1%87%D0%BD%D0%B8%D0%BA%D0%BE%D0%B2+%D0%B1%D0%B5%D0%B7+API-%D0%BA%D0%BB%D1%8E%D1%87%D0%B5%D0%B9;%D0%9F%D0%B5%D1%80%D0%B5%D0%B2%D0%BE%D0%B4+%D0%B0%D0%BD%D0%B3%D0%BB%D0%B8%D0%B9%D1%81%D0%BA%D0%B8%D1%85+%D0%BD%D0%BE%D0%B2%D0%BE%D1%81%D1%82%D0%B5%D0%B9+%D0%BD%D0%B0+%D1%80%D1%83%D1%81%D1%81%D0%BA%D0%B8%D0%B9;PostgreSQL+%2B+Redis+%2B+Kafka;47+%D0%BC%D0%B5%D1%82%D0%BE%D0%B4%D0%BE%D0%B2+REST+API;Prometheus+%2B+Grafana+%2B+Loki" alt="GoNews highlights" />
</a>

<br/>

<a href="https://github.com/JamikKhidirov/NewsGo/actions/workflows/ci.yml"><img src="https://github.com/JamikKhidirov/NewsGo/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI" /></a>
<a href="https://github.com/JamikKhidirov/NewsGo"><img src="https://img.shields.io/badge/status-production--ready-22c55e?style=flat-square" alt="Production ready" /></a>
<a href="https://github.com/JamikKhidirov/NewsGo/stargazers"><img src="https://img.shields.io/github/stars/JamikKhidirov/NewsGo?style=flat-square&logo=github" alt="Stars" /></a>
<a href="https://github.com/JamikKhidirov/NewsGo/commits/main"><img src="https://img.shields.io/github/last-commit/JamikKhidirov/NewsGo?style=flat-square&logo=git" alt="Last commit" /></a>
<a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-22c55e?style=flat-square" alt="MIT" /></a>

<br/>

<img src="https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.26" />
<img src="https://img.shields.io/badge/PostgreSQL-16-4169E1?style=flat-square&logo=postgresql&logoColor=white" alt="PostgreSQL 16" />
<img src="https://img.shields.io/badge/Redis-7-DC382D?style=flat-square&logo=redis&logoColor=white" alt="Redis 7" />
<img src="https://img.shields.io/badge/Kafka-3.9-231F20?style=flat-square&logo=apachekafka&logoColor=white" alt="Kafka 3.9" />
<img src="https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker&logoColor=white" alt="Docker" />
<img src="https://img.shields.io/badge/Prometheus-3.2-E6522C?style=flat-square&logo=prometheus&logoColor=white" alt="Prometheus" />
<img src="https://img.shields.io/badge/Grafana-11.5-F46800?style=flat-square&logo=grafana&logoColor=white" alt="Grafana" />
<img src="https://img.shields.io/badge/Loki-3.4-F5A800?style=flat-square&logo=grafanaloki&logoColor=white" alt="Loki" />

<br/><br/>

<a href="#возможности">Возможности</a> ·
<a href="#демо">Демо</a> ·
<a href="#архитектура">Архитектура</a> ·
<a href="#быстрый-старт">Быстрый старт</a> ·
<a href="#api">API</a> ·
<a href="#мониторинг-и-логи">Мониторинг</a> ·
<a href="#производительность">Масштабирование</a> ·
<a href="#документация">Документация</a>

</div>

---

**GoNews** — самодостаточный backend для новостного мобильного приложения
(Android/iOS) и веба. Сервис собирает материалы из **26 бесплатных источников
без API-ключей**, объединяет их в единую ленту, удаляет дубликаты, автоматически
размечает категории и теги и **переводит англоязычные новости на русский язык**.

Один Go-бинарник + PostgreSQL + Redis + Kafka. Внутри — JWT-авторизация,
лайки, комментарии, закладки, подписки, история, персональная лента,
push-уведомления Android/iOS, Prometheus-метрики, Grafana, Loki, Swagger,
OpenAPI, Postman, CI/CD и релизы `.exe` по тегам.

| | |
|---|---|
| **26** источников без ключей | **50** тем и **13** категорий |
| **47** методов REST API | **85** E2E-проверок в Docker |
| **1M RPS** — модель масштабирования | **0** обязательных внешних API-ключей |

---

## Возможности

<table>
<tr>
<td width="50%" valign="top">

### Новости и лента
- 26 keyless-источников: RSS, Hacker News (Algolia), Reddit JSON,
  DEV.to, Spaceflight News API.
- Фильтры по категории, источнику, тегу и дате.
- Сортировки `latest`, `hot`, `trending`, `oldest`.
- Дедупликация по каноническому URL (SHA-256).
- Полнотекстовый поиск и тренды за 7 дней.
- Похожие материалы и персональная лента по подпискам.
- Плановый сбор каждые 10 минут и retention 90 дней.

### Локализация
- Приоритетный язык — **русский** (`ru`), дополнительно `en`.
- Перевод заголовка, описания и текста.
- Цепочка Google → Lingva → MyMemory → LibreTranslate.
- Durable-очередь переводов в PostgreSQL.
- `FOR UPDATE SKIP LOCKED` и circuit breaker на провайдера.
- Пока перевод не готов — оригинал со статусом `pending`.
- `Accept-Language` и `?lang=` управляют языком выдачи.

</td>
<td width="50%" valign="top">

### Пользователи и социальное
- Регистрация и вход, bcrypt.
- JWT access 30 минут + ротируемый refresh 30 дней.
- Профиль, смена пароля, статистика активности.
- Лайки, закладки, история просмотров.
- Комментарии с ответами до 2000 символов.
- Подписки на теги и персональная лента.
- Внутренний фид уведомлений.
- Push через FCM HTTP v1 и APNs token auth.

### Эксплуатация
- Stateless HTTP-слой: реплики взаимозаменяемы.
- PostgreSQL с идемпотентными миграциями и пулом.
- Redis: кэш GET-ответов и распределённый rate limit.
- Kafka: события push и итогов ингеста.
- Prometheus, Grafana, Loki, Promtail, алерты.
- Distroless-образ, nonroot, healthcheck.
- Веб-документация `/docs` со светлой и тёмной темой.
- Swagger UI, OpenAPI 3.0, Postman, k6, GitHub Actions.

</td>
</tr>
</table>

---

## Демо

<div align="center">

<a href="http://localhost:8080/docs"><img src="docs/assets/images/docs-light.png" alt="Веб-документация GoNews — светлая тема" width="49%" /></a>
<a href="http://localhost:8080/docs"><img src="docs/assets/images/docs-dark.png" alt="Веб-документация GoNews — тёмная тема" width="49%" /></a>

<br/><br/>

<a href="http://localhost:8080/docs/swagger"><img src="docs/assets/images/swagger.png" alt="Swagger UI" width="82%" /></a>

<br/><br/>

<a href="http://localhost:3000"><img src="docs/assets/images/grafana.png" alt="Grafana: GoNews — API, ingestion and delivery" width="82%" /></a>

<sub>Скриншоты сняты с локального Docker-стека: документация, Swagger UI и дашборд Grafana.</sub>

</div>

---

## Архитектура

```mermaid
flowchart LR
    subgraph Clients["Клиенты"]
        direction TB
        AND["Android"]
        IOS["iOS"]
        WEB["Web / Swagger / Postman"]
    end

    LB["L7 LB / CDN<br/>TLS, HTTP/2, кэш"]

    subgraph API["GoNews API — stateless"]
        direction TB
        HTTP["chi router<br/>middleware + handlers"]
        ING["Aggregator<br/>26 providers"]
        TRW["Translation<br/>workers"]
        PWW["Push worker"]
    end

    PG[("PostgreSQL 16<br/>новости, пользователи,<br/>устройства, очередь")]
    RD[("Redis 7<br/>кэш + rate limit")]
    KF[["Kafka 3.9<br/>gonews.notifications<br/>gonews.news"]]
    FCM["FCM HTTP v1<br/>Android"]
    APNS["APNs HTTP/2<br/>iOS"]

    subgraph OBS["Наблюдаемость"]
        direction TB
        PROM["Prometheus"]
        GRAF["Grafana"]
        LOKI["Loki"]
        PT["Promtail"]
    end

    AND --> LB
    IOS --> LB
    WEB --> LB
    LB --> HTTP

    HTTP --> PG
    HTTP --> RD
    HTTP --> KF
    ING --> PG
    ING --> KF
    TRW --> PG
    KF --> PWW
    PWW --> FCM
    PWW --> APNS

    HTTP -. "/metrics" .-> PROM
    PG -. exporter .-> PROM
    RD -. exporter .-> PROM
    PROM --> GRAF
    LOKI --> GRAF
    PT --> LOKI
```

**Ключевые решения**

| Решение | Почему |
|---|---|
| Stateless API | JWT проверяется локально, реплики можно менять и масштабировать независимо |
| PostgreSQL как источник истины | транзакции, идемпотентные миграции, durable-очередь переводов |
| Redis как ускоритель, а не зависимость | при падении Redis сервис работает без кэша и с локальным лимитером |
| Kafka для push | HTTP-запрос не ждёт FCM/APNs; consumer group масштабирует доставку |
| In-process шина без брокеров | локальная разработка и тесты не требуют Kafka |
| Dry-run push | полный путь через Kafka без ключей FCM/APNs |

Подробнее: [Архитектура](docs/ru/ARCHITECTURE.md) ·
[PostgreSQL, Redis и Kafka](docs/ru/INFRASTRUCTURE.md).

---

## Как это работает

### Сбор и перевод новостей

```mermaid
sequenceDiagram
    autonumber
    participant S as Планировщик
    participant A as Aggregator
    participant P as 26 провайдеров
    participant DB as PostgreSQL
    participant K as Kafka

    S->>A: Run() при старте и каждые 10 минут
    par Параллельный опрос
        A->>P: Fetch(limit)
    and
        A->>P: Fetch(limit)
    end
    P-->>A: []IngestItem
    A->>A: sanitize, язык, canonical URL,<br/>категория, теги, hot_score
    A->>DB: upsert articles + enqueue translation
    A->>DB: запись ingest_runs
    A->>K: ingest.completed
    A->>DB: инвалидация кэша, Notify() воркеров
```

Ошибка одного источника не прерывает запуск: она попадает в
`providers[slug].error`, метрику `gonews_provider_errors_total` и лог.

### HTTP-запрос

```mermaid
sequenceDiagram
    autonumber
    participant C as Клиент
    participant API as GoNews API
    participant R as Redis
    participant DB as PostgreSQL

    C->>API: GET /api/v1/news?lang=ru
    API->>API: request-id, CORS, body-limit,<br/>rate-limit, auth
    API->>R: GET gonews:http:...
    alt cache HIT
        R-->>API: готовый JSON
        API-->>C: 200 + X-Cache: HIT
    else cache MISS
        API->>DB: SELECT articles + translations
        DB-->>API: строки
        API->>R: SETEX 30s
        API-->>C: 200 + X-Cache: MISS
    end
```

### Push-уведомления

```mermaid
sequenceDiagram
    autonumber
    participant AD as Admin
    participant API as GoNews API
    participant DB as PostgreSQL
    participant K as Kafka
    participant W as push.Worker
    participant F as FCM
    participant A as APNs

    AD->>API: POST /api/v1/admin/push
    API->>DB: INSERT notifications
    API->>K: gonews.notifications
    API-->>AD: 200 {"queued": 1}
    K->>W: consumer group gonews-push
    W->>F: Android
    W->>A: iOS
    W->>DB: отключить невалидные токены
```

---

## Технологии

| Слой | Технология | Версия | Роль |
|---|---|---|---|
| Язык | Go | 1.26 | API, воркеры, CLI |
| HTTP | chi | v5 | роутер и middleware |
| БД | PostgreSQL | 16 | источник истины |
| Драйвер | pgx (`stdlib`) | v5 | пул, `?` → `$n` rebind |
| Кэш | Redis | 7 | GET-кэш, rate limit |
| События | Kafka (franz-go) | 3.9 | push и итоги ингеста |
| Метрики | Prometheus | 3.2 | сбор и алерты |
| Дашборды | Grafana | 11.5 | визуализация |
| Логи | Loki + Promtail | 3.4 | JSON-логи контейнеров |
| Документация | goldmark + OpenAPI 3.0 | — | `/docs`, Swagger UI |
| Push | FCM HTTP v1 / APNs | — | Android и iOS |
| Нагрузка | k6 | — | smoke/read/mixed/stress/spike |
| CI/CD | GitHub Actions + GoReleaser | — | тесты, образы, `.exe` |

---

## Быстрый старт

### Требования

- **Docker Desktop** — рекомендуется, поднимает весь стек.
- **Go 1.26+** — только для локальной сборки.

### Всё в Docker

```bash
git clone https://github.com/JamikKhidirov/NewsGo.git
cd NewsGo
cp .env.example .env          # Windows: Copy-Item .env.example .env
docker compose up -d --build
```

Через 20–30 секунд:

```bash
curl -s localhost:8080/readyz
# {"components":{"kafka":"kafka","postgres":"ok","redis":"ok"},"status":"ready"}

curl -s "localhost:8080/api/v1/news?limit=3&lang=ru"
```

| Что | Адрес |
|---|---|
| API | <http://localhost:8080> |
| Документация (светлая/тёмная тема) | <http://localhost:8080/docs> |
| Swagger UI | <http://localhost:8080/docs/swagger> |
| OpenAPI YAML | <http://localhost:8080/docs/openapi.yaml> |
| Postman-коллекция | <http://localhost:8080/docs/postman_collection.json> |
| Health / readiness | <http://localhost:8080/healthz>, <http://localhost:8080/readyz> |
| Метрики | <http://localhost:8080/metrics> |
| Prometheus | <http://localhost:9090> |
| Grafana | <http://localhost:3000> (`admin` / `admin`) |
| Loki | <http://localhost:3100> |

### Локально без контейнера API

```bash
docker compose up -d postgres redis kafka
cp .env.example .env
go run ./cmd/server
```

Windows PowerShell:

```powershell
docker compose up -d postgres redis kafka
Copy-Item .env.example .env
$env:GONEWS_DATABASE_URL = "postgres://gonews:gonews@127.0.0.1:5432/gonews?sslmode=disable"
$env:GONEWS_ADMIN_TOKEN = "devadmin"
go run ./cmd/server
```

### Сборка `.exe`

```powershell
.\scripts\build.ps1
.\bin\gonews.exe
```

Или вручную:

```powershell
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o bin\gonews.exe .\cmd\server
```

---

## Docker

`docker compose up -d --build` поднимает:

| Сервис | Порт | Назначение |
|---|---|---|
| `gonews` | 8080 | API, distroless, nonroot, healthcheck |
| `postgres` | 5432 | основное хранилище |
| `redis` | 6379 | кэш и rate limit |
| `kafka` | 9092 (внутри сети) | события push и ингеста |
| `kafka-init` | — | создаёт топики `gonews.notifications`, `gonews.news` |
| `prometheus` | 9090 | сбор метрик |
| `grafana` | 3000 | дашборд и алерты |
| `loki` | 3100 | хранилище логов |
| `promtail` | — | stdout контейнеров → Loki |
| `postgres-exporter` | 9187 | метрики PostgreSQL |
| `redis-exporter` | 9121 | метрики Redis |

Опциональный **self-hosted LibreTranslate**:

```bash
GONEWS_LIBRETRANSLATE_URL=http://libretranslate:5000 \
  docker compose --profile translate up -d --build
```

Полезные команды:

```bash
docker compose ps
docker compose logs -f gonews
docker compose exec postgres psql -U gonews -d gonews
docker compose exec redis redis-cli DBSIZE
docker compose exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list
docker compose down            # остановить, данные в томах
docker compose down -v         # остановить и удалить данные
```

---

## API

Базовый префикс: `/api/v1`. Всего **47 методов** и **14 служебных маршрутов**.

Форматы ответов:

```json
{ "data": { } }
```

```json
{ "data": [], "meta": { "page": 1, "limit": 20, "total": 184, "totalPages": 10 } }
```

```json
{ "error": { "code": "validation_error", "message": "invalid request body" } }
```

<details>
<summary><b>Публичные методы: новости, поиск, справочники</b></summary>

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/news` | Лента: `category`, `source`, `tag`, `q`, `sort`, `page`, `limit`, `lang` |
| `GET` | `/news/trending` | Тренды за 7 дней |
| `GET` | `/news/{id}` | Карточка материала, похожие |
| `GET` | `/news/{id}/comments` | Комментарии материала |
| `GET` | `/search?q=` | Полнотекстовый поиск |
| `GET` | `/categories` | 13 категорий |
| `GET` | `/sources` | Каталог источников |
| `GET` | `/tags` | 50 тем |
| `GET` | `/tags/{slug}` | Тег и материалы |
| `GET` | `/health` | Состояние API |

</details>

<details>
<summary><b>Авторизация</b></summary>

| Метод | Путь | Описание |
|---|---|---|
| `POST` | `/auth/register` | Регистрация, сразу выдаёт пару токенов |
| `POST` | `/auth/login` | Вход |
| `POST` | `/auth/refresh` | Ротация refresh-токена |
| `POST` | `/auth/logout` | Отзыв refresh-токена |

</details>

<details>
<summary><b>Профиль, лента и социальные функции</b></summary>

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/me` | Профиль и статистика |
| `PATCH` | `/me` | Изменить display name |
| `POST` | `/me/password` | Смена пароля |
| `GET` | `/me/stats` | Активность пользователя |
| `GET` | `/me/feed` | Персональная лента по подпискам |
| `GET` | `/me/bookmarks` | Закладки |
| `GET` | `/me/likes` | Лайки |
| `GET` | `/me/history` | История просмотров |
| `GET` | `/me/follows` | Подписки на теги |
| `PUT` | `/me/follows/{slug}` | Подписаться |
| `DELETE` | `/me/follows/{slug}` | Отписаться |
| `POST` | `/news/{id}/like` | Поставить лайк |
| `DELETE` | `/news/{id}/like` | Убрать лайк |
| `POST` | `/news/{id}/bookmark` | Добавить закладку |
| `DELETE` | `/news/{id}/bookmark` | Удалить закладку |
| `POST` | `/news/{id}/view` | Отметить просмотр |
| `POST` | `/news/{id}/comments` | Комментарий или ответ |
| `PATCH` | `/comments/{id}` | Редактировать комментарий |
| `DELETE` | `/comments/{id}` | Удалить комментарий |

</details>

<details>
<summary><b>Устройства и уведомления</b></summary>

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/me/devices` | Список устройств |
| `POST` | `/me/devices` | Регистрация Android/iOS-токена |
| `DELETE` | `/me/devices/{token}` | Удалить устройство |
| `GET` | `/me/notifications` | Внутренний фид |
| `GET` | `/me/notifications/unread-count` | Непрочитанные |
| `POST` | `/me/notifications/{id}/read` | Отметить одно |
| `POST` | `/me/notifications/read-all` | Отметить все |

</details>

<details>
<summary><b>Администрирование</b></summary>

| Метод | Путь | Описание |
|---|---|---|
| `POST` | `/admin/ingest` | Ручной сбор новостей |
| `GET` | `/admin/ingest-runs` | История запусков |
| `POST` | `/admin/translate/backfill` | Дозаполнить очередь переводов |
| `GET` | `/admin/translation-stats` | Статистика перевода |
| `POST` | `/admin/push` | Push-рассылка через Kafka |
| `GET` | `/admin/stats` | Статьи, пользователи, комментарии, теги |
| `POST` | `/admin/purge` | Удалить старые материалы |

Доступ: роль `admin` в JWT или заголовок `X-Admin-Token`
(constant-time сравнение).

</details>

### Примеры

```bash
# Лента на русском
curl "localhost:8080/api/v1/news?limit=10&lang=ru"

# Поиск
curl "localhost:8080/api/v1/search?q=россия"

# Регистрация
curl -X POST localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password123","displayName":"User"}'

# Лайк
curl -X POST localhost:8080/api/v1/news/<id>/like \
  -H "Authorization: Bearer <accessToken>"

# Ручной ингест
curl -X POST localhost:8080/api/v1/admin/ingest \
  -H "X-Admin-Token: devadmin"
```

Полный справочник — [docs/ru/API.md](docs/ru/API.md), интерактивные запросы —
<http://localhost:8080/docs/swagger>, Postman —
[`docs/api/postman_collection.json`](docs/api/postman_collection.json).

---

## Модель данных

```mermaid
erDiagram
    sources ||--o{ articles : publishes
    articles ||--o{ article_translations : has
    articles ||--o{ translation_jobs : queues
    articles ||--o{ article_tags : tagged
    tags ||--o{ article_tags : labels
    articles ||--o{ likes : receives
    articles ||--o{ bookmarks : saved
    articles ||--o{ article_views : viewed
    articles ||--o{ comments : has
    users ||--o{ likes : gives
    users ||--o{ bookmarks : creates
    users ||--o{ comments : writes
    users ||--o{ article_views : watches
    users ||--o{ devices : registers
    users ||--o{ follows : follows
    tags ||--o{ follows : followed
    users ||--o{ notifications : receives
    notifications ||--o{ notification_reads : read_by
    users ||--o{ refresh_tokens : owns
```

| Таблица | Назначение |
|---|---|
| `sources` | каталог источников и весов |
| `articles` | материалы, уникальный `unique_hash` |
| `article_translations` | переводы по языкам |
| `translation_jobs` | durable-очередь и retry-состояние |
| `tags`, `article_tags` | темы и связь many-to-many |
| `users` | пользователи, bcrypt, роль |
| `refresh_tokens` | SHA-256 хеши refresh-токенов |
| `likes`, `bookmarks`, `article_views` | социальные связи и история |
| `follows` | подписки на теги |
| `comments` | комментарии с `parent_id` и soft delete |
| `devices` | FCM/APNs-токены, платформа, темы |
| `notifications`, `notification_reads` | внутренний фид и прочтения |
| `ingest_runs` | история запусков и ошибки провайдеров |
| `settings` | служебные значения |

Ключевые индексы: `articles(published_at DESC)`,
`articles(source_id, published_at DESC)`,
`articles(category, published_at DESC)`,
`articles(hot_score DESC)`, `articles(lang, published_at DESC)`,
`translation_jobs(status, next_attempt_at)`,
`notifications(user_id, created_at DESC)`, `devices(platform, enabled)`.

Подробнее: [docs/ru/INFRASTRUCTURE.md](docs/ru/INFRASTRUCTURE.md).

---

## Перевод на русский

```mermaid
flowchart LR
    A["articles<br/>lang != target"] --> Q["translation_jobs<br/>status=pending"]
    Q --> W["worker<br/>FOR UPDATE SKIP LOCKED"]
    W --> P{"Google → Lingva →<br/>MyMemory → LibreTranslate"}
    P -->|success| T["article_translations"]
    P -->|error| B["circuit breaker 2 мин<br/>retry с backoff"]
    B --> Q
    T --> API["GET /news?lang=ru"]
```

- Очередь durable: задачи переживают перезапуск.
- Несколько воркеров и реплик разбирают очередь без дублей.
- Повторные попытки с экспоненциальной задержкой.
- Провайдер с ошибкой пропускается 2 минуты.
- Текст режется по `GONEWS_TRANSLATE_MAX_CHARS` (4500 символов).
- Если язык совпадает — `translation.status=original`.

Подробнее: [docs/ru/TRANSLATION.md](docs/ru/TRANSLATION.md).

---

## Push-уведомления

| Платформа | Транспорт | Настройка |
|---|---|---|
| Android | FCM HTTP v1 | `GONEWS_FCM_PROJECT_ID`, `GONEWS_FCM_SERVICE_ACCOUNT_FILE` |
| iOS | APNs token auth | `GONEWS_APNS_KEY_FILE`, `GONEWS_APNS_KEY_ID`, `GONEWS_APNS_TEAM_ID`, `GONEWS_APNS_TOPIC` |
| Без ключей | dry-run | событие проходит Kafka, вместо сети — лог `push (dry-run)` |

- Устройства: `POST /api/v1/me/devices` с `platform: android|ios`.
- Рассылка: `POST /api/v1/admin/push` → PostgreSQL → Kafka → worker.
- Автоматическая рассылка топ-новостей: `GONEWS_PUSH_BROADCAST=true`,
  `GONEWS_PUSH_MIN_SCORE`, `GONEWS_PUSH_INTERVAL`.
- Невалидные токены отключаются (`enabled=0`).
- Внутренний фид работает независимо от FCM/APNs.

Подробнее: [docs/ru/NOTIFICATIONS.md](docs/ru/NOTIFICATIONS.md).

---

## Мониторинг и логи

### Метрики

| Метрика | Что показывает |
|---|---|
| `gonews_http_requests_total` | запросы по методу, маршруту и статусу |
| `gonews_http_request_duration_seconds` | гистограмма latency |
| `gonews_http_in_flight_requests` | параллельные запросы |
| `gonews_http_response_bytes_total` | объём ответов |
| `gonews_cache_operations_total` | HIT/MISS/BYPASS |
| `gonews_db_pool_*` | пул PostgreSQL |
| `gonews_kafka_messages_total` | производство и потребление |
| `gonews_kafka_consumer_lag` | отставание consumer group |
| `gonews_push_deliveries_total` | доставки по платформе и статусу |
| `gonews_push_queue_depth` | очередь push |
| `gonews_ingest_runs_total` | запуски ингеста |
| `gonews_ingest_articles_total` | собранные статьи |
| `gonews_ingest_duration_seconds` | длительность ингеста |
| `gonews_provider_errors_total` | ошибки источников |
| `gonews_provider_fetched_total` | материалы по источникам |
| `gonews_translation_jobs_total` | переводы по статусу |

Плюс стандартные `go_*` и `process_*`.

### Grafana и алерты

- Датасорсы и дашборд **GoNews — API, ingestion and delivery**
  провижинятся автоматически из `deploy/grafana`.
- Алерты: доступность, 5xx, p99, насыщение пула БД, Redis, устаревший
  ингест, ошибки Kafka и push.
- Логи — JSON в stdout, Promtail → Loki.
- `X-Request-ID` связывает HTTP-ответ, лог и метрику.

Проверка:

```bash
curl -s localhost:8080/readyz
curl -s localhost:9090/-/ready
curl -s localhost:3100/ready
docker compose exec prometheus promtool check rules /etc/prometheus/alerts.yml
```

Подробнее и PromQL: [docs/ru/OBSERVABILITY.md](docs/ru/OBSERVABILITY.md).

---

## Производительность

Ориентиры на 4 vCPU / 2 GB для API-реплики (модель, не гарантия):

| Сценарий | RPS | p50 | p95 | p99 |
|---|---:|---:|---:|---:|
| `GET /news` cache HIT | 18 000 | 1.8 ms | 4 ms | 9 ms |
| `GET /news` cache MISS | 2 400 | 18 ms | 55 ms | 120 ms |
| `GET /news/{id}` HIT | 22 000 | 1.5 ms | 3 ms | 7 ms |
| `GET /search?q=` | 900 | 42 ms | 110 ms | 240 ms |
| `POST /auth/login` | 180 | 55 ms | 90 ms | 150 ms |
| `POST /news/{id}/like` | 1 100 | 9 ms | 25 ms | 60 ms |

Путь к **1M RPS**:

1. **CDN / L7 LB** перед API, кэш анонимной ленты.
2. **Горизонтальные реплики API** — JWT проверяется локально.
3. **Redis Cluster** для кэша и распределённого rate limit.
4. **PgBouncer + read replicas** для PostgreSQL.
5. **Kafka 3+ брокера**, RF=3, партиции под consumer group.
6. **Партиционирование** `articles` по месяцам.
7. **Выделение воркеров** ингеста, перевода и push в отдельные deployment'ы.

Расчёт: 1 000 000 / 18 000 ≈ 56 реплик, с запасом 1.5× — 84 реплики
(≈336 vCPU). Подробности: [docs/ru/PERFORMANCE.md](docs/ru/PERFORMANCE.md).

---

## Нагрузочное тестирование

Готовые сценарии k6 в `loadtest/k6`:

| Сценарий | Что проверяет |
|---|---|
| `smoke.js` | доступность и базовые ответы |
| `read.js` | чтение ленты и карточек |
| `mixed.js` | чтение + авторизация + социальные действия |
| `stress.js` | поведение при росте нагрузки |
| `spike.js` | резкий всплеск и восстановление |

```bash
k6 run loadtest/k6/smoke.js
BASE_URL=http://localhost:8080 k6 run loadtest/k6/read.js
k6 run loadtest/k6/mixed.js
k6 run loadtest/k6/stress.js
k6 run loadtest/k6/spike.js
```

Windows: `.\loadtest\k6\run.ps1 read`.

Методика и пороги: [docs/ru/LOAD-TESTING.md](docs/ru/LOAD-TESTING.md).

---

## Конфигурация

Все настройки — переменные окружения с префиксом `GONEWS_`. Полный список —
в [`.env.example`](.env.example) и
[docs/ru/DEPLOYMENT.md](docs/ru/DEPLOYMENT.md).

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `GONEWS_ENV` | `development` | `production` включает строгие проверки |
| `GONEWS_HTTP_ADDR` | `:8080` | адрес HTTP-сервера |
| `GONEWS_DATABASE_URL` | `postgres://gonews:gonews@127.0.0.1:5432/gonews?sslmode=disable` | PostgreSQL DSN |
| `GONEWS_REDIS_URL` | пусто | Redis DSN (пусто = кэш отключён) |
| `GONEWS_CACHE_TTL` | `30s` | TTL кэша GET-ответов |
| `GONEWS_KAFKA_BROKERS` | пусто | брокеры Kafka (пусто = in-process шина) |
| `GONEWS_JWT_SECRET` | dev-секрет | **обязателен в production** |
| `GONEWS_ADMIN_TOKEN` | пусто | сервисный токен админа |
| `GONEWS_ADMIN_EMAILS` | пусто | email-ы с ролью `admin` |
| `GONEWS_INGEST_INTERVAL` | `10m` | период сбора новостей |
| `GONEWS_PROVIDERS` | все | ограничить список источников |
| `GONEWS_PROVIDERS_LIMIT` | `25` | материалов на источник |
| `GONEWS_TRANSLATE_TARGETS` | `ru` | языки перевода |
| `GONEWS_TRANSLATE_WORKERS` | `2` | воркеры перевода |
| `GONEWS_PUSH_ENABLED` | `true` | включить push-доставку |
| `GONEWS_PUSH_WORKERS` | `4` | воркеры доставки |
| `GONEWS_FCM_PROJECT_ID` | пусто | Firebase project ID |
| `GONEWS_FCM_SERVICE_ACCOUNT_FILE` | пусто | JSON сервисного аккаунта |
| `GONEWS_APNS_KEY_FILE` | пусто | `.p8` ключ APNs |
| `GONEWS_RATE_LIMIT_PER_MINUTE` | `180` | лимит запросов с IP |
| `GONEWS_RETENTION_DAYS` | `90` | хранение новостей |

> **Production:** задайте `GONEWS_ENV=production`, надёжный
> `GONEWS_JWT_SECRET`, `GONEWS_ADMIN_TOKEN` и TLS на reverse proxy.
> Приложение откажется стартовать с dev-секретом.

---

## CI/CD

| Workflow | Триггер | Что делает |
|---|---|---|
| `.github/workflows/ci.yml` | push/PR | `gofmt`, `go vet`, тесты с PostgreSQL, сборка, smoke API, `.exe` artifact |
| `.github/workflows/release.yml` | тег `v*` | GoReleaser: `.exe`, `.tar.gz`, checksums, GitHub Release |
| `.github/workflows/docker.yml` | `main`, тег `v*` | сборка и публикация образа в GHCR |

Релиз:

```bash
git tag -a v1.0.0 -m "GoNews 1.0.0"
git push origin v1.0.0
```

Артефакты:

```
gonews_1.0.0_windows_amd64.zip   -> gonews.exe
gonews_1.0.0_windows_arm64.zip   -> gonews.exe
gonews_1.0.0_linux_amd64.tar.gz
gonews_1.0.0_linux_arm64.tar.gz
gonews_1.0.0_darwin_arm64.tar.gz
checksums.txt
```

Тесты используют изолированные схемы PostgreSQL; без базы интеграционные
тесты пропускаются. DSN переопределяется через `GONEWS_TEST_DATABASE_URL`.

Подробнее: [docs/ru/CI-CD.md](docs/ru/CI-CD.md).

---

## Структура проекта

```text
cmd/server/              точка входа, graceful shutdown, healthcheck
internal/
  app/                   сборка зависимостей, планировщик, события
  auth/                  JWT, refresh-токены, bcrypt
  cache/                 Redis: кэш ответов, rate limit, health
  config/                конфигурация из env/.env
  events/                Kafka и in-process шина событий
  httpapi/               роутер, handlers, DTO, middleware, OpenAPI, docs UI
  langdetect/            определение и нормализация языка
  metrics/               Prometheus registry и метрики
  model/                 доменные модели
  news/                  провайдеры, агрегатор, категории, теги, sanitize
  platform/              генерация ID
  push/                  FCM, APNs, воркер доставки, dry-run
  store/                 PostgreSQL, миграции, запросы
  testutil/              изолированные схемы PostgreSQL для тестов
  translate/             провайдеры перевода, очередь, воркеры
  version/               версия сборки
docs/
  ru/                    русская документация (Markdown)
  api/                   Postman collection и environment
  assets/images/         скриншоты для README
  embed.go               встраивание документации в бинарник
deploy/                  Prometheus, Grafana, Loki, Promtail, PostgreSQL
loadtest/k6/             нагрузочные сценарии
scripts/                 build.ps1 и вспомогательные скрипты
.github/workflows/       CI, release, docker
```

---

## Документация

| Документ | О чём |
|---|---|
| [Обзор](docs/ru/OVERVIEW.md) | краткий обзор и быстрый старт |
| [Архитектура](docs/ru/ARCHITECTURE.md) | пакеты, потоки данных, хранилище |
| [REST API](docs/ru/API.md) | методы, модели, коды ошибок |
| [Развёртывание](docs/ru/DEPLOYMENT.md) | конфигурация, Docker, systemd, NSSM, nginx |
| [PostgreSQL, Redis и Kafka](docs/ru/INFRASTRUCTURE.md) | схемы, индексы, топики, пулы |
| [Мониторинг, логи и алерты](docs/ru/OBSERVABILITY.md) | метрики, PromQL, SLO, Loki |
| [Push: Android и iOS](docs/ru/NOTIFICATIONS.md) | FCM, APNs, dry-run, воркеры |
| [Производительность](docs/ru/PERFORMANCE.md) | расчёт до 1M RPS |
| [Нагрузочное тестирование](docs/ru/LOAD-TESTING.md) | k6-сценарии и методика |
| [Перевод](docs/ru/TRANSLATION.md) | провайдеры, очередь, circuit breaker |
| [Источники и агрегация](docs/ru/AGGREGATORS.md) | парсеры, дедупликация, категории |
| [CI/CD и релизы](docs/ru/CI-CD.md) | GitHub Actions, GoReleaser, GHCR |

**Веб-документация** — <http://localhost:8080/docs> (светлая и тёмная тема,
поиск по разделам, навигация). **Swagger UI** —
<http://localhost:8080/docs/swagger>. **OpenAPI 3.0** —
<http://localhost:8080/docs/openapi.yaml>.

---

## Участие

1. Форкните репозиторий и создайте ветку от `main`.
2. Запустите `gofmt -w .`, `go vet ./...`, `go test ./...`.
3. Убедитесь, что `docker compose up -d --build` и
   `k6 run loadtest/k6/smoke.js` проходят.
4. Откройте pull request с описанием изменений.

Идеи и баги — в [Issues](https://github.com/JamikKhidirov/NewsGo/issues).

---

## Лицензия

MIT. См. [LICENSE](LICENSE).

<div align="center">

<img src="docs/assets/footer.svg" alt="GoNews" width="100%" />

</div>
