# GoNews API

Базовый URL: `http://localhost:8080` (локально) или ваш домен.
Префикс API: `/api/v1`.
Интерактивная документация: `/docs` (Swagger UI), спецификация:
`/docs/openapi.yaml`.

---

## 1. Общие правила

### 1.1 Формат ответа

Успех:

```json
{
  "data": { }
}
```

Список:

```json
{
  "data": [],
  "meta": {
    "page": 1,
    "limit": 20,
    "total": 184,
    "totalPages": 10
  }
}
```

Ошибка:

```json
{
  "error": {
    "code": "validation_error",
    "message": "invalid request body"
  }
}
```

HTTP-коды: `200`, `201`, `204`, `400`, `401`, `403`, `404`, `409`, `429`,
`500`, `502`, `503`.

### 1.2 Язык

Параметр `lang` (`ru` или `en`) или заголовок `Accept-Language`.
По умолчанию `ru`. Если перевод ещё не готов, возвращается оригинал,
а `translation.status` равен `pending` или `original`.

### 1.3 Пагинация

`page` — с 1, `limit` — от 1 до 100 (по умолчанию 20, для комментариев 30).
Некорректные значения нормализуются.

### 1.4 Авторизация

```
Authorization: Bearer <accessToken>
```

Административные методы дополнительно принимают:

```
X-Admin-Token: <GONEWS_ADMIN_TOKEN>
```

### 1.5 Лимиты

- 180 запросов в минуту с IP (настраивается).
- 20 попыток в минуту для `/auth/register`, `/auth/login`, `/auth/refresh`.
- Тело запроса — не более 2 MiB.
- При превышении: `429` и заголовок `Retry-After`.

### 1.6 Заголовки ответа

| Заголовок | Значение |
|---|---|
| `X-Request-ID` | ID запроса (можно передать свой) |
| `X-RateLimit-Limit` | Лимит в минуту |
| `X-RateLimit-Remaining` | Остаток |
| `Retry-After` | Секунды при 429 |

---

## 2. Системные методы

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/` | Название, версия, ссылки |
| `GET` | `/healthz` | Liveness |
| `GET` | `/readyz` | Readiness (проверяет БД) |
| `GET` | `/api/v1/health` | Health с версией |
| `GET` | `/metrics` | Prometheus |
| `GET` | `/docs` | Swagger UI |
| `GET` | `/docs/openapi.yaml` | OpenAPI 3.0 |

```bash
curl http://localhost:8080/api/v1/health
```

```json
{
  "status": "ok",
  "version": "1.0.0",
  "time": "2026-01-01T12:00:00Z"
}
```

---

## 3. Новости

### 3.1 Лента

`GET /api/v1/news`

| Параметр | Тип | Описание |
|---|---|---|
| `lang` | string | `ru` / `en` |
| `category` | string | Слаг категории |
| `source` | string | Слаг источника |
| `tag` | string | Слаг тега |
| `q` | string | Поиск по заголовку и описанию |
| `sort` | string | `latest`, `hot`, `trending`, `oldest` |
| `since` | string | `24h` или RFC3339 |
| `page` | int | Страница |
| `limit` | int | Размер страницы |

```bash
curl "http://localhost:8080/api/v1/news?lang=ru&category=technology&sort=hot&limit=10"
```

Фрагмент ответа:

```json
{
  "data": [
    {
      "id": "a_01J8Z5YQ4M6T7K8N9P0R1S2T3V",
      "title": "NASA запустила новый телескоп",
      "summary": "Аппарат будет изучать атмосферы экзопланет.",
      "url": "https://www.nasa.gov/news/example",
      "imageUrl": "https://www.nasa.gov/example.jpg",
      "lang": "ru",
      "originalLang": "en",
      "category": "space",
      "source": {
        "id": "s_nasa",
        "slug": "nasa",
        "name": "NASA",
        "homepage": "https://www.nasa.gov",
        "kind": "rss",
        "lang": "en"
      },
      "tags": [
        { "slug": "space", "name": "Космос", "nameRu": "Космос", "nameEn": "Space" }
      ],
      "publishedAt": "2026-01-01T10:00:00Z",
      "fetchedAt": "2026-01-01T10:05:00Z",
      "stats": { "likes": 12, "comments": 3, "views": 250 },
      "liked": false,
      "bookmarked": false,
      "translation": { "status": "ready", "provider": "google" }
    }
  ],
  "meta": { "page": 1, "limit": 10, "total": 184, "totalPages": 19 }
}
```

Если передан Bearer-токен, поля `liked` и `bookmarked` отражают состояние
пользователя.

### 3.2 Тренды

`GET /api/v1/news/trending?lang=ru&limit=10&since=7d`

Окно по умолчанию — 7 дней, сортировка `hot`.

### 3.3 Карточка материала

`GET /api/v1/news/{id}?lang=ru`

Возвращает полный текст (`content`) и `related` — до 5 похожих материалов.
Увеличивает счётчик просмотров.

### 3.4 Поиск

`GET /api/v1/search?q=космос&lang=ru&page=1&limit=20`

Минимальная длина запроса — 2 символа. Поддерживаются те же `sort`,
`page`, `limit`.

---

## 4. Категории, источники и теги

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/api/v1/categories?lang=ru` | 13 категорий |
| `GET` | `/api/v1/sources` | Каталог источников |
| `GET` | `/api/v1/tags?lang=ru&limit=100` | Теги с количеством |
| `GET` | `/api/v1/tags/{slug}?lang=ru` | Один тег |

```json
{
  "data": [
    { "slug": "world", "name": "В мире" },
    { "slug": "technology", "name": "Технологии" }
  ]
}
```

Теги:

```json
{
  "data": [
    {
      "slug": "ai",
      "name": "Искусственный интеллект",
      "nameRu": "Искусственный интеллект",
      "nameEn": "AI",
      "count": 17
    }
  ]
}
```

---

## 5. Аутентификация

### 5.1 Регистрация

`POST /api/v1/auth/register`

```json
{
  "email": "user@example.com",
  "password": "password123",
  "displayName": "Иван"
}
```

Пароль — от 8 символов, имя — 2–50 символов.

Ответ `201`:

```json
{
  "data": {
    "accessToken": "eyJhbGciOi...",
    "refreshToken": "rt_...",
    "tokenType": "Bearer",
    "expiresIn": 1800,
    "user": {
      "id": "u_...",
      "email": "user@example.com",
      "displayName": "Иван",
      "role": "user",
      "createdAt": "2026-01-01T12:00:00Z"
    }
  }
}
```

Ошибки: `409 email_taken`, `400 validation_error`.

### 5.2 Вход

`POST /api/v1/auth/login`

```json
{ "email": "user@example.com", "password": "password123" }
```

Ответ `200` — та же структура, что при регистрации.
Ошибка `401 invalid_credentials`.

### 5.3 Обновление токенов

`POST /api/v1/auth/refresh`

```json
{ "refreshToken": "rt_..." }
```

Refresh-токен **одноразовый**: возвращается новая пара, старая отзывается.
Ошибка `401 invalid_refresh_token`.

### 5.4 Выход

`POST /api/v1/auth/logout`

```json
{ "refreshToken": "rt_..." }
```

Ответ `204`.

---

## 6. Профиль

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/api/v1/me` | Пользователь + статистика |
| `PATCH` | `/api/v1/me` | Изменить `displayName` |
| `POST` | `/api/v1/me/password` | Сменить пароль |
| `GET` | `/api/v1/me/stats` | Счётчики |

`GET /me`:

```json
{
  "data": {
    "user": { "id": "u_...", "email": "...", "displayName": "Иван", "role": "user", "createdAt": "..." },
    "stats": { "likes": 5, "bookmarks": 2, "comments": 1, "follows": 4 }
  }
}
```

Смена пароля:

```json
{
  "currentPassword": "password123",
  "newPassword": "password456"
}
```

---

## 7. Социальные функции

### 7.1 Лайки

```
POST   /api/v1/news/{id}/like
DELETE /api/v1/news/{id}/like
```

Ответ:

```json
{ "data": { "liked": true, "likesCount": 13 } }
```

### 7.2 Закладки

```
POST   /api/v1/news/{id}/bookmark
DELETE /api/v1/news/{id}/bookmark
```

```json
{ "data": { "bookmarked": true } }
```

### 7.3 Просмотр

`POST /api/v1/news/{id}/view` → `204`.

Нужен, если клиент не вызывал `GET /news/{id}`.

### 7.4 Комментарии

`GET /api/v1/news/{id}/comments?page=1&limit=30` — публичный.

`POST /api/v1/news/{id}/comments`:

```json
{
  "body": "Отличная новость!",
  "parentId": null
}
```

`parentId` — ID родительского комментария для ответа.

`PATCH /api/v1/comments/{id}`:

```json
{ "body": "Обновлённый текст" }
```

`DELETE /api/v1/comments/{id}` → `204`. Автор или администратор.

Модель комментария:

```json
{
  "id": "c_...",
  "articleId": "a_...",
  "userId": "u_...",
  "parentId": null,
  "body": "Отличная новость!",
  "authorName": "Иван",
  "createdAt": "2026-01-01T12:00:00Z",
  "updatedAt": "2026-01-01T12:00:00Z",
  "deleted": false
}
```

---

## 8. Персонализация

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/api/v1/me/feed` | Лента по подпискам |
| `GET` | `/api/v1/me/bookmarks` | Закладки |
| `GET` | `/api/v1/me/likes` | Лайки |
| `GET` | `/api/v1/me/history` | История |
| `GET` | `/api/v1/me/follows` | Подписки |
| `PUT` | `/api/v1/me/follows/{slug}` | Подписаться |
| `DELETE` | `/api/v1/me/follows/{slug}` | Отписаться |

Все списки поддерживают `lang`, `page`, `limit`.

`GET /me/feed` сначала пытается собрать ленту по тегам подписок. Если
подписок нет или материалов не найдено, возвращается общая лента.

Подписка:

```json
{
  "data": {
    "following": true,
    "tag": { "slug": "ai", "name": "Искусственный интеллект", "nameRu": "...", "nameEn": "AI" }
  }
}
```

---

## 9. Устройства и уведомления

### 9.1 Устройства

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/api/v1/me/devices` | Список устройств |
| `POST` | `/api/v1/me/devices` | Зарегистрировать/обновить |
| `DELETE` | `/api/v1/me/devices/{token}` | Удалить |

Регистрация:

```json
{
  "token": "fcm-device-token-1234567890",
  "platform": "android",
  "locale": "ru",
  "topics": ["technology", "space"]
}
```

`platform`: `android`, `ios`, `web`. Токен — 10–4096 символов.
Ответ `201` — объект устройства.

### 9.2 Уведомления

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/api/v1/me/notifications?unread=true` | Фид |
| `GET` | `/api/v1/me/notifications/unread-count` | Непрочитанные |
| `POST` | `/api/v1/me/notifications/{id}/read` | Прочитать |
| `POST` | `/api/v1/me/notifications/read-all` | Прочитать все |

```json
{
  "data": [
    {
      "id": "n_...",
      "kind": "breaking",
      "title": "Главные новости дня",
      "body": "Что произошло в мире технологий",
      "data": { "score": 42 },
      "articleId": "a_...",
      "createdAt": "2026-01-01T12:00:00Z",
      "read": false
    }
  ],
  "meta": { "page": 1, "limit": 30, "total": 12, "totalPages": 1 }
}
```

Счётчик:

```json
{ "data": { "unread": 3 } }
```

---

## 10. Администрирование

Доступ: JWT пользователя с ролью `admin` **или** `X-Admin-Token`.

| Метод | Путь | Описание |
|---|---|---|
| `POST` | `/api/v1/admin/ingest` | Запустить сбор |
| `GET` | `/api/v1/admin/ingest-runs?limit=20` | История сборов |
| `POST` | `/api/v1/admin/translate/backfill` | Поставить переводы в очередь |
| `GET` | `/api/v1/admin/translation-stats` | Статистика переводов |
| `POST` | `/api/v1/admin/push` | Отправить push |
| `GET` | `/api/v1/admin/stats` | Сводная статистика |
| `POST` | `/api/v1/admin/purge` | Удалить старые материалы |

### 10.1 Сбор

```bash
curl -X POST http://localhost:8080/api/v1/admin/ingest \
  -H "X-Admin-Token: devadmin"
```

Ответ:

```json
{
  "data": {
    "runId": 42,
    "startedAt": "2026-01-01T12:00:00Z",
    "finishedAt": "2026-01-01T12:01:10Z",
    "durationMs": 70000,
    "fetched": 195,
    "inserted": 184,
    "updated": 3,
    "errors": 8,
    "providers": {
      "habr": { "fetched": 25, "inserted": 24, "updated": 0 },
      "bbc-world": { "fetched": 0, "inserted": 0, "updated": 0, "error": "timeout" }
    }
  }
}
```

Если сбор уже идёт — `409 ingest_running`.

### 10.2 Backfill переводов

```json
{ "lang": "ru", "limit": 500 }
```

Ответ:

```json
{ "data": { "queued": 120, "lang": "ru" } }
```

### 10.3 Статистика переводов

```json
{ "data": { "pending": 74, "failed": 0, "done": 11 } }
```

### 10.4 Push

```json
{
  "title": "Главные новости дня",
  "body": "Что произошло в мире технологий",
  "articleId": "a_...",
  "userId": ""
}
```

`userId` пустой — рассылка всем устройствам.
Ответ:

```json
{ "data": { "targets": 120, "sent": 118, "failed": 2 } }
```

### 10.5 Сводная статистика

```json
{
  "data": {
    "articles": 184,
    "users": 12,
    "comments": 5,
    "tags": 48,
    "translation": { "pending": 74, "failed": 0, "done": 11 },
    "lastIngest": { "id": 42, "status": "ok", "inserted": 184, "errors": 8 },
    "uptimeSeconds": 3600,
    "version": "see /"
  }
}
```

### 10.6 Очистка

```json
{ "beforeDays": 90 }
```

Минимум 7 дней. Ответ: `{ "data": { "deleted": 42 } }`.

---

## 11. Коды ошибок

| Код | HTTP | Значение |
|---|---|---|
| `invalid_json` | 400 | Некорректный JSON |
| `validation_error` | 400 | Ошибка валидации |
| `invalid_category` | 400 | Неизвестная категория |
| `invalid_sort` | 400 | Неизвестная сортировка |
| `invalid_parent` | 400 | Родительский комментарий не найден |
| `unauthorized` | 401 | Нет/неверный токен |
| `invalid_credentials` | 401 | Неверный email или пароль |
| `invalid_refresh_token` | 401 | Refresh недействителен |
| `forbidden` | 403 | Нет роли admin |
| `not_found` | 404 | Ресурс не найден |
| `email_taken` | 409 | Email занят |
| `ingest_running` | 409 | Сбор уже выполняется |
| `rate_limited` | 429 | Превышен лимит |
| `internal_error` | 500 | Внутренняя ошибка |
| `push_failed` | 502 | Ошибка FCM |
| `db_unavailable` | 503 | БД недоступна |

---

## 12. Postman

Импортируйте два файла:

- `docs/postman_collection.json`
- `docs/postman_environment.json`

Выберите окружение **GoNews Local**, выполните `Auth → Вход` — токены
сохранятся в переменные коллекции автоматически. Дальше можно выполнять
остальные запросы.

Для административных методов задайте `adminToken` (по умолчанию `devadmin`,
если сервер запущен с `GONEWS_ADMIN_TOKEN=devadmin`).
