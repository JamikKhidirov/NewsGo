# GoNews

**GoNews** — самодостаточный backend для новостного мобильного приложения
(Android/iOS). Сервис собирает новости из **26 бесплатных источников без API-ключей**,
объединяет их в единую ленту, убирает дубликаты, автоматически размечает
категории и теги и **переводит англоязычные материалы на русский язык**.

Один бинарник, встроенные миграции SQLite, JWT-авторизация, лайки, комментарии,
закладки, подписки на теги, push-уведомления, Swagger, метрики Prometheus,
CI/CD и релизы `.exe` по тегам.

---

## Содержание

- [Возможности](#возможности)
- [Быстрый старт (Windows)](#быстрый-старт-windows)
- [Быстрый старт (Linux/macOS)](#быстрый-старт-linuxmacos)
- [Docker](#docker)
- [Конфигурация](#конфигурация)
- [API](#api)
- [Источники новостей](#источники-новостей)
- [Перевод на русский](#перевод-на-русский)
- [Push-уведомления](#push-уведомления)
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
- Асинхронная durable-очередь переводов в SQLite, circuit breaker на провайдера.
- Пока перевод не готов, клиент получает оригинал со статусом `pending`.
- `Accept-Language` и параметр `lang` управляют языком выдачи.

### Пользователи и социальные функции
- Регистрация/вход, bcrypt, JWT access (30 мин) + ротируемый refresh (30 дней).
- Профиль, смена пароля, статистика активности.
- Лайки, закладки, история просмотров.
- Комментарии с ответами (до 2000 символов), редактирование и удаление.
- Подписки на теги и персональная лента.
- Внутренний фид уведомлений + регистрация FCM-устройств.
- Push через FCM HTTP v1; без ключей работает dry-run и внутренний фид.

### Эксплуатация
- Единый бинарник, `CGO_ENABLED=0`, pure-Go SQLite (`modernc.org/sqlite`).
- Встроенные миграции — база создаётся автоматически.
- Swagger UI: `/docs`, OpenAPI: `/docs/openapi.yaml`.
- Postman-коллекция и окружение в `docs/`.
- Health/readiness, структурированные логи, Prometheus-метрики.
- CORS, rate limiting, body limit, request ID, graceful shutdown.
- GitHub Actions: CI, Docker, релиз `.exe` по тегам `v*`.

---

## Быстрый старт (Windows)

### Требования
- **Go 1.26+** — <https://go.dev/dl/>
- PowerShell 5.1+ (встроен в Windows)

### Запуск

```powershell
git clone https://github.com/gonews/gonews.git
cd gonews

# зависимости
go mod download

# переменные окружения для локальной разработки
$env:GONEWS_ADMIN_TOKEN = "devadmin"
$env:GONEWS_INGEST_ON_START = "true"

# запуск
go run ./cmd/server
```

Сервис будет доступен на <http://localhost:8080>.

| Что | Адрес |
|---|---|
| Swagger UI | <http://localhost:8080/docs> |
| OpenAPI YAML | <http://localhost:8080/docs/openapi.yaml> |
| Health | <http://localhost:8080/api/v1/health> |
| Метрики | <http://localhost:8080/metrics> |

Первый сбор новостей занимает 1–2 минуты. Проверить результат:

```powershell
Invoke-RestMethod "http://localhost:8080/api/v1/news?limit=5&lang=ru"
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

### Переменные через `.env`

Скопируйте `.env.example` в `.env` — сервис читает его автоматически при старте.
Существующие переменные окружения имеют приоритет над `.env`.

---

## Быстрый старт (Linux/macOS)

```bash
git clone https://github.com/gonews/gonews.git
cd gonews
go mod download

export GONEWS_ADMIN_TOKEN=devadmin
go run ./cmd/server
```

Сборка:

```bash
make build      # bin/gonews
make test
make run
```

---

## Docker

```bash
docker compose up -d --build
```

Сервис: <http://localhost:8080>, данные в томе `gonews-data`.

Опциональный **self-hosted LibreTranslate** (рекомендуется для стабильного
перевода без внешних лимитов):

```bash
$env:GONEWS_LIBRETRANSLATE_URL = "http://libretranslate:5000"
docker compose --profile translate up -d --build
```

На Linux/macOS:

```bash
GONEWS_LIBRETRANSLATE_URL=http://libretranslate:5000 docker compose --profile translate up -d --build
```

---

## Конфигурация

Все настройки — переменные окружения с префиксом `GONEWS_`. Полный список с
описаниями — в [`.env.example`](.env.example) и
[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

Ключевые параметры:

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `GONEWS_HTTP_ADDR` | `:8080` | Адрес HTTP-сервера |
| `GONEWS_DB_PATH` | `data/gonews.db` | Путь к SQLite |
| `GONEWS_JWT_SECRET` | dev-секрет | **Обязателен в production** |
| `GONEWS_ADMIN_TOKEN` | пусто | Сервисный токен админа |
| `GONEWS_ADMIN_EMAILS` | пусто | Email-ы с ролью `admin` |
| `GONEWS_INGEST_INTERVAL` | `10m` | Период сбора новостей |
| `GONEWS_INGEST_ON_START` | `true` | Сбор при старте |
| `GONEWS_PROVIDERS` | все | Ограничить список источников |
| `GONEWS_TRANSLATE_TARGETS` | `ru` | Языки перевода |
| `GONEWS_TRANSLATE_PROVIDERS` | `google,lingva,mymemory,libretranslate` | Цепочка провайдеров |
| `GONEWS_LIBRETRANSLATE_URL` | `https://libretranslate.com` | Свой LibreTranslate |
| `GONEWS_PUSH_ENABLED` | `false` | Включить реальную отправку FCM |
| `GONEWS_FCM_PROJECT_ID` | пусто | Firebase project ID |
| `GONEWS_FCM_SERVICE_ACCOUNT_FILE` | пусто | JSON сервисного аккаунта |
| `GONEWS_RATE_LIMIT_PER_MINUTE` | `180` | Лимит запросов с IP |
| `GONEWS_RETENTION_DAYS` | `90` | Хранение новостей |

> **Production:** обязательно задайте `GONEWS_ENV=production` и надёжный
> `GONEWS_JWT_SECRET`. Приложение откажется стартовать с dev-секретом.

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
| `POST` | `/me/devices` | Регистрация FCM-устройства |
| `GET` | `/me/notifications` | Уведомления |
| `POST` | `/admin/ingest` | Ручной сбор (admin) |
| `GET` | `/admin/stats` | Статистика (admin) |

Полное описание — [docs/API.md](docs/API.md) и Swagger UI `/docs`.
Готовая коллекция Postman — [`docs/postman_collection.json`](docs/postman_collection.json).

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

Подробности, ограничения и рекомендации по self-hosting —
[docs/AGGREGATORS.md](docs/AGGREGATORS.md).

---

## Перевод на русский

Цепочка провайдеров с автоматическим fallback:

1. **Google** (`clients5.google.com`) — основной, без ключа.
2. **Lingva** — публичный фронтенд Google Translate.
3. **MyMemory** — бесплатный API с лимитами.
4. **LibreTranslate** — публичный или собственный сервер.

Перевод выполняется фоновыми воркерами из очереди в SQLite. Провайдер,
который вернул ошибку, временно исключается на 2 минуты (circuit breaker).
Для продакшена рекомендуется собственный LibreTranslate в Docker Compose.

Подробности: [docs/TRANSLATION.md](docs/TRANSLATION.md).

---

## Push-уведомления

- Устройства регистрируются через `POST /api/v1/me/devices`.
- FCM HTTP v1 используется при заданных `GONEWS_FCM_PROJECT_ID` и
  `GONEWS_FCM_SERVICE_ACCOUNT_FILE`.
- Без FCM сервис работает в **dry-run**: сообщения логируются и всегда
  попадают во внутренний фид `/me/notifications`.
- Автоматическая рассылка топ-новостей: `GONEWS_PUSH_BROADCAST=true`,
  порог `GONEWS_PUSH_MIN_SCORE`, интервал `GONEWS_PUSH_INTERVAL`.

---

## Сборка и релизы

### Локально

```bash
make build            # текущая платформа
make release-snapshot # все платформы через GoReleaser (без публикации)
```

### CI/CD

| Workflow | Триггер | Что делает |
|---|---|---|
| `.github/workflows/ci.yml` | push/PR | `gofmt`, `go vet`, тесты, сборка |
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

Подробности: [docs/CI-CD.md](docs/CI-CD.md).

---

## Структура проекта

```
cmd/server/              точка входа, graceful shutdown
internal/
  app/                   сборка зависимостей, планировщик
  auth/                  JWT, refresh-токены, bcrypt
  config/                конфигурация из env/.env
  httpapi/               роутер, handlers, DTO, middleware, OpenAPI
  langdetect/            определение и нормализация языка
  model/                 доменные модели
  news/                  провайдеры, агрегатор, категории, теги, sanitize
  platform/              генерация ID
  push/                  FCM HTTP v1 + dry-run
  store/                 SQLite, миграции, запросы
  translate/             провайдеры перевода, очередь, воркеры
  version/               версия сборки
docs/                    API, архитектура, деплой, Postman
scripts/                 build.ps1 и вспомогательные скрипты
.github/workflows/       CI, release, docker
```

---

## Документация

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — архитектура и потоки данных.
- [docs/API.md](docs/API.md) — справочник API с примерами.
- [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) — конфигурация, Docker, systemd, reverse proxy.
- [docs/TRANSLATION.md](docs/TRANSLATION.md) — перевод, провайдеры, очередь.
- [docs/AGGREGATORS.md](docs/AGGREGATORS.md) — источники, парсеры, дедупликация.
- [docs/CI-CD.md](docs/CI-CD.md) — пайплайны и релизы.
- [Swagger UI](http://localhost:8080/docs) — интерактивная документация.

---

## Лицензия

MIT. См. [LICENSE](LICENSE).
