# Push-уведомления: Android и iOS через Kafka

GoNews доставляет уведомления асинхронно. HTTP-запрос только сохраняет
уведомление в PostgreSQL и публикует событие в Kafka; отдельный воркер
разбирает событие и отправляет его в FCM (Android) и APNs (iOS).

```
POST /api/v1/admin/push
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

Если Kafka не настроена (`GONEWS_KAFKA_BROKERS` пуст), используется
in-process шина с теми же обработчиками. Если публикация в Kafka не удалась,
сервис доставляет уведомление синхронно — событие не теряется.

## API устройств

Все методы требуют `Authorization: Bearer <access_token>`.

### Регистрация

```http
POST /api/v1/me/devices
Content-Type: application/json

{
  "token": "fcm-or-apns-device-token",
  "platform": "android",          // android | ios
  "locale": "ru",
  "topics": ["technology", "ai"]
}
```

Ответ `201`:

```json
{
  "data": {
    "id": "d_01J...",
    "userId": "u_01J...",
    "token": "fcm-or-apns-device-token",
    "platform": "android",
    "locale": "ru",
    "topics": ["technology", "ai"],
    "createdAt": "2026-11-24T09:00:00Z",
    "lastSeenAt": "2026-11-24T09:00:00Z"
  }
}
```

Повторная регистрация того же токена обновляет запись (upsert) и снова включает
устройство, если оно было отключено.

### Список и удаление

```http
GET /api/v1/me/devices
DELETE /api/v1/me/devices/{token}
```

`DELETE` возвращает `204`; неизвестный токен — `404`.

Валидация: токен 10–4096 символов, `platform` только `android` или `ios`.
Невалидные токены, отклонённые FCM/APNs (`404`, `400`, `410`), автоматически
помечаются `enabled=0` и больше не используются.

## Отправка

### Broadcast всем устройствам

```http
POST /api/v1/admin/push
X-Admin-Token: <token>
Content-Type: application/json

{
  "title": "Главное сейчас",
  "body": "Ключевая новость дня",
  "articleId": "a_01J..."
}
```

### Персональное уведомление

```json
{
  "title": "Новое по вашим темам",
  "body": "Свежий материал про AI",
  "userId": "u_01J...",
  "articleId": "a_01J..."
}
```

Ответ:

```json
{"data":{"queued":1,"targets":0,"sent":0,"failed":0}}
```

`queued=1` означает, что событие принято брокером. Счётчики `targets/sent/failed`
заполняются при синхронной деградации (Kafka недоступна). Результаты асинхронной
доставки видны в метрике `gonews_push_deliveries_total` и в логах
`push delivered`.

In-app уведомления доступны сразу:

```http
GET  /api/v1/me/notifications
GET  /api/v1/me/notifications/unread-count
POST /api/v1/me/notifications/{id}/read
POST /api/v1/me/notifications/read-all
```

## Android: FCM HTTP v1

1. Создайте проект в Firebase, добавьте Android-приложение.
2. Включите Firebase Cloud Messaging API.
3. Создайте сервисный аккаунт с ролью *Firebase Cloud Messaging API Admin*.
4. Скачайте JSON-ключ и смонтируйте его в контейнер.

```dotenv
GONEWS_FCM_PROJECT_ID=my-project
GONEWS_FCM_SERVICE_ACCOUNT_FILE=/run/secrets/fcm-service-account.json
```

В `docker-compose.yml` добавьте секрет:

```yaml
services:
  gonews:
    volumes:
      - ./secrets/fcm-service-account.json:/run/secrets/fcm-service-account.json:ro
```

Приложение должно получить FCM-токен и отправить его в `POST /me/devices`
с `platform: "android"`. Канал уведомлений `news`, приоритет `HIGH`, звук
`default` задаются сервером.

## iOS: APNs (token-based auth)

1. В Apple Developer создайте APNs Auth Key (`.p8`).
2. Запишите Key ID и Team ID.
3. Укажите Bundle ID приложения как `GONEWS_APNS_TOPIC`.

```dotenv
GONEWS_APNS_KEY_FILE=/run/secrets/AuthKey_ABC123.p8
GONEWS_APNS_KEY_ID=ABC123
GONEWS_APNS_TEAM_ID=TEAM123456
GONEWS_APNS_TOPIC=com.example.gonews
GONEWS_APNS_SANDBOX=false
```

Для TestFlight/development используйте `GONEWS_APNS_SANDBOX=true`.
Сервер подписывает JWT (ES256), кэширует его на 50 минут и отправляет
HTTP/2-запросы на `api.push.apple.com` (или sandbox-хост). Payload:

```json
{
  "aps": {"alert": {"title": "…", "body": "…"}, "sound": "default"},
  "articleId": "a_01J...",
  "kind": "breaking",
  "notificationId": "n_01J..."
}
```

Приложение отправляет APNs device token в `POST /me/devices` с `platform: "ios"`.

## Режим без ключей (dry-run)

Если ключи не заданы, оба транспорта работают как logging sender: события
проходят весь путь через Kafka и воркер, а вместо сетевого вызова пишется лог
`push (dry-run)` с платформой и числом токенов. Это позволяет проверять
интеграцию и нагрузочные сценарии без реальных credentials.

## Масштабирование и надёжность

* Число партиций `gonews.notifications` определяет параллелизм consumer group.
  В compose создаётся 6 партиций; запустите несколько реплик `gonews` с одной
  группой — партиции распределятся автоматически.
* Доставка идемпотентна на уровне устройства: повторный push с тем же payload
  безопасен, но может показаться дважды. Для строгой дедупликации храните
  `notificationId` и проверяйте его на клиенте.
* FCM/APNs вызовы ограничены 8 параллельными запросами на транспорт.
* Ошибки транспорта не блокируют API: HTTP-ответ возвращается после публикации.
* Соблюдайте квоты: FCM — 1 сообщение на токен, APNs — 4096 байт payload.
* Не логируйте полные device-токены в production; в текущих логах они не
  выводятся.

## Проверка end-to-end

```bash
# 1. зарегистрировать устройство
TOKEN=$(curl -s -X POST localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","password":"password123"}' | jq -r .data.accessToken)

curl -s -X POST localhost:8080/api/v1/me/devices \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"token":"test-device-token-123456","platform":"ios","locale":"ru"}'

# 2. отправить broadcast
curl -s -X POST localhost:8080/api/v1/admin/push \
  -H "X-Admin-Token: $GONEWS_ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Тест","body":"Проверка доставки"}'

# 3. увидеть событие в Kafka
docker compose exec kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic gonews.notifications --from-beginning --max-messages 1

# 4. проверить метрики
curl -s localhost:8080/metrics | grep gonews_push
```
