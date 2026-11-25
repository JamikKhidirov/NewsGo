# GoNews — обзор

**GoNews** — самодостаточный backend для новостного мобильного приложения
(Android/iOS) и веба. Сервис собирает материалы из **26 бесплатных источников
без API-ключей**, объединяет их в единую ленту, удаляет дубликаты, автоматически
размечает категории и теги и **переводит англоязычные новости на русский язык**.

Один Go-бинарник + PostgreSQL + Redis + Kafka. Внутри: JWT-авторизация,
лайки, комментарии, закладки, подписки, история, персональная лента,
push-уведомления Android/iOS, Prometheus-метрики, Grafana, Loki, Swagger,
OpenAPI, Postman, CI/CD и релизы `.exe` по тегам.

---

## Ключевые возможности

| Блок | Что внутри |
|---|---|
| **Новости** | 26 keyless-источников, дедупликация по SHA-256, 13 категорий, 50 тегов, поиск, тренды, похожие материалы, retention 90 дней |
| **Локализация** | Приоритет `ru`; перевод заголовка, описания и текста через Google → Lingva → MyMemory → LibreTranslate; durable-очередь в PostgreSQL |
| **Пользователи** | Регистрация и вход, bcrypt, JWT access 30 минут + ротируемый refresh 30 дней, профиль, смена пароля, статистика |
| **Социальное** | Лайки, закладки, история просмотров, комментарии с ответами, подписки на теги, персональная лента |
| **Уведомления** | Внутренний фид, регистрация устройств Android/iOS, FCM HTTP v1, APNs token auth, dry-run без ключей, доставка через Kafka |
| **Эксплуатация** | Stateless API, PostgreSQL-миграции, Redis-кэш и rate limit, Kafka-события, Prometheus, Grafana, Loki, Promtail, health/readiness |
| **Разработка** | Swagger UI, OpenAPI 3.0, Postman, встроенная веб-документация со светлой и тёмной темой, k6-сценарии, GitHub Actions |

---

## Быстрый старт

```bash
git clone https://github.com/JamikKhidirov/NewsGo.git
cd NewsGo
cp .env.example .env
docker compose up -d --build
```

Через 20–30 секунд:

| Что | Адрес |
|---|---|
| API | <http://localhost:8080> |
| Документация | <http://localhost:8080/docs> |
| Swagger UI | <http://localhost:8080/docs/swagger> |
| Метрики | <http://localhost:8080/metrics> |
| Prometheus | <http://localhost:9090> |
| Grafana | <http://localhost:3000> (`admin` / `admin`) |
| Loki | <http://localhost:3100> |

Проверка:

```bash
curl -s localhost:8080/readyz
curl -s "localhost:8080/api/v1/news?limit=5&lang=ru"
```

Полные инструкции — в разделе [Развёртывание](/docs/deployment).

---

## Как устроен сервис

```
Клиенты ──► GoNews API (stateless) ──► PostgreSQL
                    │                └► Redis (кэш, лимиты)
                    └── Kafka ──► push-worker ──► FCM / APNs

Prometheus ◄── /metrics      Grafana ◄── Prometheus + Loki
Promtail ──► Loki
```

* **HTTP-слой** не хранит состояние: JWT проверяется локально, реплики
  взаимозаменяемы.
* **PostgreSQL** — источник истины: новости, переводы, пользователи,
  устройства, очередь переводов.
* **Redis** — кэш анонимных GET-ответов и распределённый rate limit;
  при недоступности сервис продолжает работать.
* **Kafka** — события push-доставки и итогов ингеста; без брокеров
  автоматически используется in-process шина.
* **Фоновые воркеры** — ингест, перевод, push и retention внутри процесса.

Подробнее: [Архитектура](/docs/architecture) и
[PostgreSQL, Redis и Kafka](/docs/infrastructure).

---

## API за минуту

Базовый префикс `/api/v1`. Ответы — `{"data": ...}`, списки дополнительно
содержат `meta`, ошибки — `{"error": {"code", "message"}}`.

```bash
# Лента на русском
curl "localhost:8080/api/v1/news?limit=10&lang=ru"

# Поиск
curl "localhost:8080/api/v1/search?q=россия"

# Регистрация
curl -X POST localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password123","displayName":"User"}'

# Лайк (нужен accessToken)
curl -X POST localhost:8080/api/v1/news/<id>/like \
  -H "Authorization: Bearer <accessToken>"
```

Полный справочник — [REST API](/docs/api), Swagger UI —
<http://localhost:8080/docs/swagger>, Postman-коллекция —
<http://localhost:8080/docs/postman_collection.json>.

---

## Наблюдаемость

* `/metrics` — HTTP, PostgreSQL pool, Redis, Kafka, push, ингест, перевод.
* Prometheus собирает `gonews`, `postgres-exporter`, `redis-exporter`, себя.
* Grafana автоматически провижинит датасорсы и дашборд
  **GoNews — API, ingestion and delivery**.
* Алерты: доступность, 5xx, p99, насыщение пула БД, Redis, устаревший ингест,
  ошибки Kafka и push.
* Логи — JSON в stdout, Promtail → Loki, `X-Request-ID` связывает запрос и лог.

Подробнее: [Мониторинг, логи и алерты](/docs/observability).

---

## Документация

| Раздел | Содержание |
|---|---|
| [Архитектура](/docs/architecture) | пакеты, потоки данных, хранилище, надёжность |
| [REST API](/docs/api) | методы, модели, коды ошибок, примеры |
| [Развёртывание](/docs/deployment) | конфигурация, Docker, systemd, NSSM, nginx |
| [PostgreSQL, Redis и Kafka](/docs/infrastructure) | схемы, индексы, топики, пулы |
| [Мониторинг, логи и алерты](/docs/observability) | метрики, PromQL, SLO, Loki |
| [Push: Android и iOS](/docs/notifications) | FCM, APNs, dry-run, воркеры |
| [Производительность](/docs/performance) | расчёт до 1M RPS |
| [Нагрузочное тестирование](/docs/load-testing) | k6-сценарии и методика |
| [Перевод](/docs/translation) | провайдеры, очередь, circuit breaker |
| [Источники и агрегация](/docs/aggregators) | парсеры, дедупликация, категории |
| [CI/CD и релизы](/docs/ci-cd) | GitHub Actions, GoReleaser, GHCR |

Исходные Markdown-файлы лежат в [`docs/ru`](https://github.com/JamikKhidirov/NewsGo/tree/main/docs/ru),
Postman — в [`docs/api`](https://github.com/JamikKhidirov/NewsGo/tree/main/docs/api).

---

## Лицензия

MIT. См. [LICENSE](/docs/LICENSE).
