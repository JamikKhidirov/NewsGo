# GoNews

**GoNews** — production-ready backend для новостного мобильного приложения
(Android/iOS). Сервис собирает новости из **26 бесплатных источников без API-ключей**,
объединяет их в единую ленту, убирает дубликаты, автоматически размечает
категории и теги и **переводит англоязычные материалы на русский язык**.

Стек: Go 1.26, chi, PostgreSQL 16, Redis 7, Kafka 3, Prometheus, Grafana, Loki.
JWT-авторизация, лайки, комментарии, закладки, подписки, push-уведомления
(Android FCM + iOS APNs) через Kafka, Swagger, OpenAPI, Postman, CI/CD и релизы
`.exe` по тегам.

---

## Содержание

- [Возможности](#возможности)
- [Архитектура](#архитектура)
- [Быстрый старт](#быстрый-старт)
- [Docker](#docker)
- [Конфигурация](#конфигурация)
- [API](#api)
- [Источники новостей](#источники-новостей)
- [Перевод на русский](#перевод-на-русский)
- [Push-уведомления](#push-уведомления)
- [Мониторинг и логи](#мониторинг-и-логи)
- [Нагрузочное тестирование](#нагрузочное-тестирование)
- [Сборка и релизы](#сборка-и-релизы)
- [Структура проекта](#структура-проекта)
- [Документация](#документация)
- [Лицензия](#лицензия)

---

## Возможности

### Новости
- 26 keyless-источников: RSS + Hacker News (Algolia), Reddit JSON, DEV.to,
  Spaceflight News API.
- Единая лента с фильтрами по категории, источнику, тегу, дате и сортировкой
  `latest / hot / trending / oldest`.
- Дедупликация по каноническому URL (SHA-256).
- Автоматические категории (13) и теги (48) по словарю RU/EN.
- Полнотекстовый поиск, тренды за 7 дней, похожие материалы.
- Плановый сбор (по умолчанию каждые 10 минут) и retention-политика (90 дней).

### Локализация
- Приоритетный язык — **русский** (`ru`), дополнительно `en`.
- Перевод заголовка, описания и текста через цепочку провайдеров:
  Google → Lingva → MyMemory → LibreTranslate.
- Durable-очередь переводов в PostgreSQL, `FOR UPDATE SKIP LOCKED`,
  circuit breaker на провайдера.
- Пока перевод не готов, клиент получает оригинал со статусом `pending`.
- `Accept-Language` и параметр `lang` управляют языком выдачи.

### Пользователи и социальные функции
- Регистрация/вход, bcrypt, JWT access (30 мин) + ротируемый refresh (30 дней).
- Профиль, смена пароля, статистика активности.
- Лайки, закладки, история просмотров.
- Комментарии с ответами (до 2000 символов), редактирование и удаление.
- Подписки на теги и персональная лента.
- Внутренний фид уведомлений + регистрация устройств Android/iOS.
- Push через FCM HTTP v1 и APNs (token-based auth); без ключей — dry-run.

### Эксплуатация
- Stateless HTTP-слой: реплики взаимозаменяемы, JWT проверяется локально.
- PostgreSQL с встроенными идемпотентными миграциями и пулом соединений.
- Redis: кэш анонимных GET-ответов и распределённый rate limit с graceful fallback.
- Kafka: события push-доставки и итогов ингеста; in-process шина без брокеров.
- Prometheus-метрики (HTTP, БД, кэш, Kafka, push, ингест, перевод).
- Grafana-дашборд, алерты, Loki + Promtail для логов.
- Docker Compose со всем стеком, distroless-образ, healthcheck.
- Встроенная веб-документация `/docs` (светлая/тёмная тема), Swagger UI,
  OpenAPI 3.0, Postman-коллекция.
- CORS, rate limiting, body limit, request ID, graceful shutdown.
- GitHub Actions: CI с PostgreSQL, Docker-образ в GHCR, релиз `.exe` по тегам `v*`.

---

## Архитектура

```
                       ┌──────────────────────────────┐
   Android / iOS ─────► │  GoNews API (chi, stateless) │
   Web / Swagger       └───────┬───────────┬──────────┘
                               │           │
             ┌─────────────────┘           └─────────────────┐
             ▼                                               ▼
      PostgreSQL 16                                    Redis 7
   новости, пользователи,                     кэш GET-ответов,
   устройства, очередь                        распределённый rate limit
             │
             │  push-события / итоги ингеста
             ▼
          Kafka 3  ──►  push.Worker  ──►  FCM (Android)
                                      └─►  APNs (iOS)

   Prometheus ◄── /metrics       Grafana ◄── Prometheus + Loki
   Promtail  ──► Loki
```

Подробнее: [docs/ru/ARCHITECTURE.md](docs/ru/ARCHITECTURE.md) и
[docs/ru/INFRASTRUCTURE.md](docs/ru/INFRASTRUCTURE.md).

---

## Быстрый старт

### Требования
- **Go 1.26+** — <https://go.dev/dl/>
- **Docker Desktop** (PostgreSQL, Redis, Kafka) — либо собственные сервисы.

### Вариант 1: всё в Docker (рекомендуется)

```bash
git clone https://github.com/JamikKhidirov/NewsGo.git
cd NewsGo
cp .env.example .env          # Windows: Copy-Item .env.example .env
docker compose up -d --build
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

Проверка:

```bash
curl -s localhost:8080/readyz
curl -s "localhost:8080/api/v1/news?limit=5&lang=ru"
```

### Вариант 2: локально

Поднимите инфраструктуру и запустите сервер:

```bash
docker compose up -d postgres redis kafka
cp .env.example .env
go run ./cmd/server
```

Или на Windows PowerShell:

```powershell
docker compose up -d postgres redis kafka
Copy-Item .env.example .env
$env:GONEWS_DATABASE_URL = "postgres://gonews:gonews@127.0.0.1:5432/gonews?sslmode=disable"
$env:GONEWS_ADMIN_TOKEN = "devadmin"
go run ./cmd/server
```

Сервис будет доступен на <http://localhost:8080>. Первый сбор новостей
занимает 1–2 минуты.

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
| `gonews` | 8080 | API (distroless, nonroot) |
| `postgres` | 5432 | основное хранилище |
| `redis` | 6379 | кэш и rate limit |
| `kafka` | 9092 (внутри сети) | события push и ингеста |
| `kafka-init` | — | создаёт топики `gonews.notifications`, `gonews.news` |
| `prometheus` | 9090 | сбор метрик |
| `grafana` | 3000 | дашборд и алерты |
| `loki` | 3100 | хранилище логов |
| `promtail` | — | сбор stdout контейнеров в Loki |
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
docker compose down            # остановить
docker compose down -v         # остановить и удалить данные
```

---

## Конфигурация

Все настройки — переменные окружения с префиксом `GONEWS_`. Полный список —
в [`.env.example`](.env.example) и
[docs/ru/DEPLOYMENT.md](docs/ru/DEPLOYMENT.md).

Ключевые параметры:

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `GONEWS_HTTP_ADDR` | `:8080` | Адрес HTTP-сервера |
| `GONEWS_DATABASE_URL` | `postgres://gonews:gonews@127.0.0.1:5432/gonews?sslmode=disable` | PostgreSQL DSN |
| `GONEWS_REDIS_URL` | пусто | Redis DSN (пусто = кэш отключён) |
| `GONEWS_CACHE_TTL` | `30s` | TTL кэша GET-ответов |
| `GONEWS_KAFKA_BROKERS` | пусто | Брокеры Kafka (пусто = in-process шина) |
| `GONEWS_JWT_SECRET` | dev-секрет | **Обязателен в production** |
| `GONEWS_ADMIN_TOKEN` | пусто | Сервисный токен админа |
| `GONEWS_ADMIN_EMAILS` | пусто | Email-ы с ролью `admin` |
| `GONEWS_INGEST_INTERVAL` | `10m` | Период сбора новостей |
| `GONEWS_PROVIDERS` | все | Ограничить список источников |
| `GONEWS_TRANSLATE_TARGETS` | `ru` | Языки перевода |
| `GONEWS_PUSH_ENABLED` | `true` | Включить push-доставку |
| `GONEWS_FCM_PROJECT_ID` | пусто | Firebase project ID (Android) |
| `GONEWS_FCM_SERVICE_ACCOUNT_FILE` | пусто | JSON сервисного аккаунта |
| `GONEWS_APNS_KEY_FILE` | пусто | `.p8` ключ APNs (iOS) |
| `GONEWS_APNS_KEY_ID` / `GONEWS_APNS_TEAM_ID` | пусто | Идентификаторы APNs |
| `GONEWS_RATE_LIMIT_PER_MINUTE` | `180` | Лимит запросов с IP |
| `GONEWS_RETENTION_DAYS` | `90` | Хранение новостей |

> **Production:** задайте `GONEWS_ENV=production`, надёжный `GONEWS_JWT_SECRET`,
> `GONEWS_ADMIN_TOKEN` и включите TLS на reverse proxy. Приложение откажется
> стартовать с dev-секретом.

---

## API

Базовый префикс: `/api/v1`.

Формат ответа:

```json
{ "data": { } }
```

Списки:

```json
{ "data": [], "meta": { "page": 1, "limit": 20, "total": 184, "totalPages": 10 } }
```

Ошибки:

```json
{ "error": { "code": "validation_error", "message": "invalid request body" } }
```

### Основные методы

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/news` | Лента с фильтрами и пагинацией |
| `GET` | `/news/trending` | Тренды за 7 дней |
| `GET` | `/news/{id}` | Карточка материала |
| `GET` | `/search?q=` | Поиск |
| `GET` | `/categories` | Категории |
| `GET` | `/sources` | Источники |
| `GET` | `/tags` | Теги |
| `POST` | `/auth/register` | Регистрация |
| `POST` | `/auth/login` | Вход |
| `POST` | `/auth/refresh` | Обновление токенов |
| `GET` | `/me` | Профиль и статистика |
| `GET` | `/me/feed` | Персональная лента |
| `POST` | `/news/{id}/like` | Лайк |
| `POST` | `/news/{id}/bookmark` | Закладка |
| `POST` | `/news/{id}/comments` | Комментарий |
| `POST` | `/me/devices` | Регистрация устройства |
| `GET` | `/me/notifications` | Уведомления |
| `POST` | `/admin/ingest` | Ручной сбор (admin) |
| `POST` | `/admin/push` | Push-рассылка (admin) |
| `GET` | `/admin/stats` | Статистика (admin) |

Полное описание — [docs/ru/API.md](docs/ru/API.md), веб-руководство `/docs`
и Swagger UI `/docs/swagger`.
Готовая коллекция Postman — [`docs/api/postman_collection.json`](docs/api/postman_collection.json).

---

## Источники новостей

26 источников, не требующих ключей:

- **Мир:** BBC, NPR, Al Jazeera, The Guardian, Lenta.ru, РИА Новости, ТАСС,
  Известия, Reddit r/worldnews.
- **Технологии:** Hacker News, Reddit r/technology, DEV.to, Хабр, CNews,
  TechCrunch, The Verge, Ars Technica, Lobsters.
- **Бизнес:** Коммерсантъ, РБК.
- **Наука и космос:** N+1, arXiv AI, NASA, Spaceflight News.
- **Безопасность:** BleepingComputer.
- **Криптовалюты:** CoinDesk.

Недоступность отдельного источника не останавливает ингест: ошибка
изолируется, попадает в `gonews_provider_errors_total` и лог.
Подробности — [docs/ru/AGGREGATORS.md](docs/ru/AGGREGATORS.md).

---

## Перевод на русский

Цепочка провайдеров с автоматическим fallback:

1. **Google** (`clients5.google.com`) — основной, без ключа.
2. **Lingva** — публичный фронтенд Google Translate.
3. **MyMemory** — бесплатный API с лимитами.
4. **LibreTranslate** — публичный или собственный сервер.

Перевод выполняется фоновыми воркерами из очереди в PostgreSQL
(`translation_jobs`, `FOR UPDATE SKIP LOCKED`). Провайдер, вернувший ошибку,
временно исключается на 2 минуты (circuit breaker). Для продакшена
рекомендуется собственный LibreTranslate в Docker Compose.

Подробности: [docs/ru/TRANSLATION.md](docs/ru/TRANSLATION.md).

---

## Push-уведомления

Схема: HTTP → PostgreSQL → Kafka `gonews.notifications` → push-worker →
FCM / APNs.

- Устройства регистрируются через `POST /api/v1/me/devices`
  (`platform`: `android` или `ios`).
- **Android:** FCM HTTP v1 при заданных `GONEWS_FCM_PROJECT_ID` и
  `GONEWS_FCM_SERVICE_ACCOUNT_FILE`.
- **iOS:** APNs token auth при заданных `GONEWS_APNS_KEY_FILE`,
  `GONEWS_APNS_KEY_ID`, `GONEWS_APNS_TEAM_ID`, `GONEWS_APNS_TOPIC`.
- Без ключей сервис работает в **dry-run**: события проходят весь путь через
  Kafka, а вместо сетевого вызова пишется лог `push (dry-run)`.
- Автоматическая рассылка топ-новостей: `GONEWS_PUSH_BROADCAST=true`,
  порог `GONEWS_PUSH_MIN_SCORE`, интервал `GONEWS_PUSH_INTERVAL`.
- Невалидные токены автоматически отключаются (`enabled=0`).

Подробности: [docs/ru/NOTIFICATIONS.md](docs/ru/NOTIFICATIONS.md).

---

## Мониторинг и логи

- `/metrics` — Prometheus: HTTP, PostgreSQL pool, Redis cache, Kafka,
  push, ингест, перевод, стандартные `go_*` и `process_*`.
- Prometheus собирает `gonews`, `postgres-exporter`, `redis-exporter`, себя.
- Grafana автоматически провижинит датасорсы и дашборд
  **GoNews — API, ingestion and delivery**.
- Алерты: доступность, 5xx, p99, насыщение пула БД, Redis, устаревший ингест,
  ошибки Kafka и push.
- Логи — JSON в stdout, Promtail → Loki; `X-Request-ID` связывает запрос и лог.

Подробности и PromQL: [docs/ru/OBSERVABILITY.md](docs/ru/OBSERVABILITY.md).

---

## Нагрузочное тестирование

Готовые сценарии k6 в `loadtest/k6`:

```bash
k6 run loadtest/k6/smoke.js
BASE_URL=http://localhost:8080 k6 run loadtest/k6/read.js
k6 run loadtest/k6/mixed.js
k6 run loadtest/k6/stress.js
k6 run loadtest/k6/spike.js
```

Windows: `.\loadtest\k6\run.ps1 read`.

Модель масштабирования до 1M RPS, поведение под перегрузкой и требования к
слоям: [docs/ru/PERFORMANCE.md](docs/ru/PERFORMANCE.md) и
[docs/ru/LOAD-TESTING.md](docs/ru/LOAD-TESTING.md).

---

## Сборка и релизы

### Локально

```bash
make build            # текущая платформа
make test             # тесты (нужен PostgreSQL)
make release-snapshot # все платформы через GoReleaser (без публикации)
```

Тесты используют отдельную схему в PostgreSQL. Если база недоступна,
интеграционные тесты пропускаются. DSN переопределяется через
`GONEWS_TEST_DATABASE_URL`.

### CI/CD

| Workflow | Триггер | Что делает |
|---|---|---|
| `.github/workflows/ci.yml` | push/PR | `gofmt`, `go vet`, тесты с PostgreSQL, сборка, smoke `.exe` |
| `.github/workflows/release.yml` | тег `v*` | GoReleaser: `.exe`, `.tar.gz`, checksums, GitHub Release |
| `.github/workflows/docker.yml` | тег `v*`, `main` | Сборка и публикация образа в GHCR |

Релиз:

```bash
git tag v1.0.0
git push origin v1.0.0
```

В GitHub Release появятся:

```
gonews_1.0.0_windows_amd64.zip   -> gonews.exe
gonews_1.0.0_windows_arm64.zip   -> gonews.exe
gonews_1.0.0_linux_amd64.tar.gz
gonews_1.0.0_darwin_arm64.tar.gz
checksums.txt
```

Подробности: [docs/ru/CI-CD.md](docs/ru/CI-CD.md).

---

## Структура проекта

```
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
  embed.go               встраивание документации в бинарник
deploy/                  конфиги Prometheus, Grafana, Loki, Promtail, Postgres
loadtest/k6/             нагрузочные сценарии
scripts/                 build.ps1 и вспомогательные скрипты
.github/workflows/       CI, release, docker
```

---

## Документация

- [docs/ru/ARCHITECTURE.md](docs/ru/ARCHITECTURE.md) — архитектура и потоки данных.
- [docs/ru/API.md](docs/ru/API.md) — справочник API с примерами.
- [docs/ru/DEPLOYMENT.md](docs/ru/DEPLOYMENT.md) — конфигурация, Docker, systemd, reverse proxy.
- [docs/ru/INFRASTRUCTURE.md](docs/ru/INFRASTRUCTURE.md) — PostgreSQL, Redis, Kafka.
- [docs/ru/OBSERVABILITY.md](docs/ru/OBSERVABILITY.md) — метрики, логи, алерты, SLO.
- [docs/ru/NOTIFICATIONS.md](docs/ru/NOTIFICATIONS.md) — push для Android и iOS.
- [docs/ru/PERFORMANCE.md](docs/ru/PERFORMANCE.md) — масштабирование до 1M RPS.
- [docs/ru/LOAD-TESTING.md](docs/ru/LOAD-TESTING.md) — сценарии k6 и методика.
- [docs/ru/TRANSLATION.md](docs/ru/TRANSLATION.md) — перевод, провайдеры, очередь.
- [docs/ru/AGGREGATORS.md](docs/ru/AGGREGATORS.md) — источники, парсеры, дедупликация.
- [docs/ru/CI-CD.md](docs/ru/CI-CD.md) — пайплайны и релизы.
- [Веб-документация](http://localhost:8080/docs) — все разделы в браузере,
  светлая и тёмная тема.
- [Swagger UI](http://localhost:8080/docs/swagger) — интерактивные запросы к API.
- [OpenAPI 3.0](http://localhost:8080/docs/openapi.yaml) — спецификация для генерации клиентов.

---

## Лицензия

MIT. См. [LICENSE](LICENSE).
