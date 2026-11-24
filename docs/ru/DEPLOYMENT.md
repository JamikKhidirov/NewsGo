# Развёртывание GoNews

Документ описывает запуск сервиса локально, в Docker и на production-сервере.

---

## 1. Требования

| Компонент | Версия | Обязателен |
|---|---|---|
| Go (для сборки) | 1.26+ | только сборка |
| PostgreSQL | 16+ | да |
| Redis | 7+ | нет (кэш и лимиты в памяти) |
| Kafka | 3.x | нет (in-process шина) |
| Docker / Compose | 24+ / v2 | для контейнерного запуска |

Сервис собирается с `CGO_ENABLED=0` и работает из distroless-образа под
непривилегированным пользователем.

---

## 2. Конфигурация

Все настройки — переменные окружения с префиксом `GONEWS_`. Локальный `.env`
читается автоматически; существующие переменные окружения имеют приоритет.

### 2.1. Основное

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `GONEWS_ENV` | `development` | `production` включает строгие проверки и JSON-логи |
| `GONEWS_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `GONEWS_LOG_FORMAT` | — | `json` для Loki |
| `GONEWS_HTTP_ADDR` | `:8080` | адрес HTTP-сервера |
| `GONEWS_PUBLIC_BASE_URL` | `http://localhost:8080` | внешний URL для ссылок |
| `GONEWS_CORS_ORIGINS` | `*` | разрешённые origin через запятую |
| `GONEWS_RATE_LIMIT_PER_MINUTE` | `180` | лимит запросов с IP |

### 2.2. Хранилища

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `GONEWS_DATABASE_URL` | `postgres://gonews:gonews@127.0.0.1:5432/gonews?sslmode=disable` | PostgreSQL DSN |
| `GONEWS_REDIS_URL` | пусто | Redis DSN; пусто = кэш отключён |
| `GONEWS_CACHE_TTL` | `30s` | TTL кэша GET-ответов |
| `GONEWS_CACHE_PREFIX` | `gonews` | префикс ключей |
| `GONEWS_KAFKA_BROKERS` | пусто | брокеры через запятую |
| `GONEWS_KAFKA_PUSH_TOPIC` | `gonews.notifications` | топик push |
| `GONEWS_KAFKA_NEWS_TOPIC` | `gonews.news` | топик ингеста |
| `GONEWS_KAFKA_GROUP` | `gonews-push` | consumer group |

### 2.3. Безопасность

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `GONEWS_JWT_SECRET` | dev-секрет | **обязателен в production** |
| `GONEWS_ACCESS_TTL` | `30m` | TTL access-токена |
| `GONEWS_REFRESH_TTL` | `720h` | TTL refresh-токена |
| `GONEWS_ADMIN_TOKEN` | пусто | `X-Admin-Token` для admin API |
| `GONEWS_ADMIN_EMAILS` | пусто | email-ы с ролью `admin` |

### 2.4. Ингест, перевод, push

| Переменная | По умолчанию |
|---|---|
| `GONEWS_INGEST_ON_START` | `true` |
| `GONEWS_INGEST_INTERVAL` | `10m` |
| `GONEWS_INGEST_TIMEOUT` | `90s` |
| `GONEWS_PROVIDERS` | все |
| `GONEWS_PROVIDER_LIMIT` | `25` |
| `GONEWS_RETENTION_DAYS` | `90` |
| `GONEWS_TRANSLATE_ENABLED` | `true` |
| `GONEWS_TRANSLATE_PROVIDERS` | `google,lingva,mymemory,libretranslate` |
| `GONEWS_TRANSLATE_TARGETS` | `ru` |
| `GONEWS_TRANSLATE_WORKERS` | `2` |
| `GONEWS_PUSH_ENABLED` | `true` |
| `GONEWS_PUSH_WORKERS` | `4` |
| `GONEWS_PUSH_BROADCAST` | `true` |
| `GONEWS_PUSH_INTERVAL` | `30m` |
| `GONEWS_PUSH_MIN_SCORE` | `20` |
| `GONEWS_FCM_PROJECT_ID` | пусто |
| `GONEWS_FCM_SERVICE_ACCOUNT_FILE` | пусто |
| `GONEWS_APNS_KEY_FILE` | пусто |
| `GONEWS_APNS_KEY_ID` | пусто |
| `GONEWS_APNS_TEAM_ID` | пусто |
| `GONEWS_APNS_TOPIC` | `com.gonews.app` |
| `GONEWS_APNS_SANDBOX` | `false` |

Полный пример — [`.env.example`](../../.env.example).

### 2.5. Production-пример

```dotenv
GONEWS_ENV=production
GONEWS_LOG_LEVEL=info
GONEWS_LOG_FORMAT=json
GONEWS_HTTP_ADDR=:8080
GONEWS_PUBLIC_BASE_URL=https://news.example.com
GONEWS_CORS_ORIGINS=https://app.example.com,https://news.example.com

GONEWS_DATABASE_URL=postgres://gonews:STRONG_PASSWORD@postgres:5432/gonews?sslmode=disable
GONEWS_REDIS_URL=redis://redis:6379/0
GONEWS_KAFKA_BROKERS=kafka:9092

GONEWS_JWT_SECRET=<openssl rand -base64 48>
GONEWS_ADMIN_TOKEN=<openssl rand -hex 24>
GONEWS_ADMIN_EMAILS=admin@example.com

GONEWS_RATE_LIMIT_PER_MINUTE=600
GONEWS_PUSH_ENABLED=true
```

> В `production` приложение откажется стартовать с dev-секретом JWT.

---

## 3. Docker Compose (рекомендуется)

```bash
cp .env.example .env
# отредактируйте .env: пароли, JWT_SECRET, ADMIN_TOKEN
docker compose up -d --build
docker compose ps
curl -s localhost:8080/readyz
```

Состав стека описан в [README](../../README.md#docker). Ключевые моменты:

* `postgres`, `redis`, `kafka` имеют healthcheck; `gonews` стартует только
  после их готовности.
* `kafka-init` создаёт топики `gonews.notifications` и `gonews.news`.
* Образ `gonews` — distroless, `nonroot`, с `HEALTHCHECK` через подкоманду
  `healthcheck`.
* Логи ограничены (`json-file`, 10 MB × 5) и собираются Promtail в Loki.

### Обновление

```bash
git pull
docker compose build gonews
docker compose up -d gonews
```

Миграции применяются автоматически при старте. Откат — вернуться на предыдущий
тег и пересобрать образ.

### Остановка

```bash
docker compose down       # данные сохраняются в томах
docker compose down -v    # удалить данные (осторожно)
```

---

## 4. Запуск без Docker

### 4.1. Подготовка инфраструктуры

```bash
docker compose up -d postgres redis kafka
```

Или используйте собственные серверы и укажите DSN в `.env`.

### 4.2. Сборка и запуск

```bash
go build -trimpath -ldflags "-s -w" -o bin/gonews ./cmd/server
./bin/gonews
```

Windows:

```powershell
.\scripts\build.ps1
.\bin\gonews.exe
```

---

## 5. Linux (systemd)

```ini
# /etc/systemd/system/gonews.service
[Unit]
Description=GoNews API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=gonews
Group=gonews
WorkingDirectory=/opt/gonews
EnvironmentFile=/etc/gonews/gonews.env
ExecStart=/opt/gonews/gonews
Restart=always
RestartSec=5
LimitNOFILE=65536

# безопасность
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/opt/gonews

[Install]
WantedBy=multi-user.target
```

```bash
sudo useradd --system --home /opt/gonews --shell /usr/sbin/nologin gonews
sudo install -m 0755 bin/gonews /opt/gonews/gonews
sudo install -m 0640 .env /etc/gonews/gonews.env
sudo systemctl daemon-reload
sudo systemctl enable --now gonews
sudo journalctl -u gonews -f
```

`journalctl` отдаёт JSON-логи; Promtail может читать их через
`/var/log/journal` либо используйте Docker-развёртывание.

---

## 6. Windows (NSSM)

```powershell
nssm install GoNews "C:\GoNews\gonews.exe"
nssm set GoNews AppDirectory "C:\GoNews"
nssm set GoNews AppEnvironmentExtra `
  GONEWS_ENV=production `
  GONEWS_DATABASE_URL=postgres://gonews:pass@127.0.0.1:5432/gonews?sslmode=disable `
  GONEWS_REDIS_URL=redis://127.0.0.1:6379/0 `
  GONEWS_JWT_SECRET=<secret> `
  GONEWS_ADMIN_TOKEN=<token>
nssm set GoNews AppStdout "C:\GoNews\logs\gonews.log"
nssm set GoNews AppStderr "C:\GoNews\logs\gonews.log"
nssm start GoNews
```

Firewall:

```powershell
New-NetFirewallRule -DisplayName "GoNews" -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow
```

---

## 7. Reverse proxy

### 7.1. Nginx

```nginx
upstream gonews {
    server 127.0.0.1:8080;
    keepalive 64;
}

server {
    listen 443 ssl http2;
    server_name news.example.com;

    ssl_certificate     /etc/letsencrypt/live/news.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/news.example.com/privkey.pem;

    client_max_body_size 4m;

    location / {
        proxy_pass http://gonews;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 90s;
    }

    location /metrics {
        allow 10.0.0.0/8;
        deny all;
        proxy_pass http://gonews;
    }
}
```

Важно: `X-Forwarded-For` используется для rate limit. Если прокси не
передаёт реальный IP, все клиенты попадут в один bucket.

### 7.2. Caddy

```caddyfile
news.example.com {
    encode zstd gzip
    reverse_proxy 127.0.0.1:8080
}
```

---

## 8. Push-ключи

### Android (FCM)

1. Firebase → Project settings → Service accounts → Generate new private key.
2. Смонтируйте JSON в контейнер:
   `./secrets/fcm.json:/run/secrets/fcm.json:ro`.
3. Задайте `GONEWS_FCM_PROJECT_ID` и
   `GONEWS_FCM_SERVICE_ACCOUNT_FILE=/run/secrets/fcm.json`.

### iOS (APNs)

1. Apple Developer → Keys → APNs Auth Key (`.p8`).
2. Задайте `GONEWS_APNS_KEY_FILE`, `GONEWS_APNS_KEY_ID`,
   `GONEWS_APNS_TEAM_ID`, `GONEWS_APNS_TOPIC`.
3. Для TestFlight — `GONEWS_APNS_SANDBOX=true`.

Без ключей работает dry-run: события проходят через Kafka и логируются.
Подробности: [NOTIFICATIONS.md](NOTIFICATIONS.md).

---

## 9. Мониторинг

```bash
docker compose up -d prometheus grafana loki promtail \
  postgres-exporter redis-exporter
```

| Сервис | Адрес |
|---|---|
| Prometheus | <http://localhost:9090> |
| Grafana | <http://localhost:3000> (`admin`/`admin`) |
| Loki | <http://localhost:3100> |
| Метрики | <http://localhost:8080/metrics> |

Проверка:

```bash
curl -s localhost:8080/readyz
curl -s localhost:9090/-/ready
curl -s localhost:3100/ready
docker compose exec prometheus promtool check rules /etc/prometheus/alerts.yml
```

Дашборд и алерты описаны в [OBSERVABILITY.md](OBSERVABILITY.md).

---

## 10. Бэкапы PostgreSQL

```bash
# логический дамп
docker compose exec -T postgres pg_dump -U gonews -Fc gonews > gonews-$(date +%F).dump

# восстановление
docker compose exec -T postgres pg_restore -U gonews -d gonews --clean < gonews-2026-11-24.dump
```

Рекомендации:

* ежедневный `pg_dump` + WAL-архивирование (pgBackRest / wal-g) для PITR;
* хранение копий вне сервера (S3, другой регион);
* проверка восстановления на отдельной базе не реже раза в месяц;
* Redis и Kafka не требуют бэкапа: кэш восстанавливается, события
  краткоживущие. Для критичных push-событий используйте outbox в PostgreSQL.

---

## 11. Production checklist

- [ ] `GONEWS_ENV=production`, случайный `GONEWS_JWT_SECRET`
- [ ] `GONEWS_ADMIN_TOKEN` задан и хранится в секретах
- [ ] PostgreSQL: сильный пароль, отдельный пользователь, `max_connections >= 25*N+20`
- [ ] Redis: пароль/ACL, `maxmemory-policy allkeys-lru`
- [ ] Kafka: 3 брокера, RF=3, `min.insync.replicas=2`
- [ ] TLS на reverse proxy, HSTS
- [ ] `/metrics` закрыт от внешней сети
- [ ] Prometheus + Grafana + Loki запущены, алерты проверены
- [ ] Бэкапы PostgreSQL настроены и проверены
- [ ] `GONEWS_RATE_LIMIT_PER_MINUTE` соответствует ожидаемой нагрузке
- [ ] FCM/APNs ключи смонтированы, dry-run отключён
- [ ] Логи в JSON, `X-Request-ID` доходит до клиента
- [ ] Health/readiness подключены к балансировщику
- [ ] Нагрузочный smoke (`k6 run loadtest/k6/smoke.js`) проходит

---

## 12. Диагностика

| Симптом | Причина | Решение |
|---|---|---|
| Контейнер `unhealthy` | нет PostgreSQL | `docker compose logs gonews`, проверить DSN |
| `readyz` 503 | Redis/Kafka недоступны | проверить `components` в ответе |
| Пустая лента | ингест не выполнен | `POST /admin/ingest`, логи провайдеров |
| 429 | низкий rate limit | поднять `GONEWS_RATE_LIMIT_PER_MINUTE` |
| Медленные ответы | промахи кэша, пул БД | смотреть Grafana, `gonews_db_pool_*` |
| Push не приходит | нет ключей или Kafka | логи `push (dry-run)` / `push delivered` |
| Дубли новостей | изменилась канонизация URL | проверить `unique_hash`, миграцию |
| Ошибки миграций | нет прав на схему | выдать `CREATE` на базу |

Полезные команды:

```bash
docker compose logs -f gonews
docker compose exec postgres psql -U gonews -d gonews -c "select count(*) from articles;"
docker compose exec redis redis-cli DBSIZE
docker compose exec kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 --describe --group gonews-push
curl -s localhost:8080/metrics | grep gonews_
```

---

## 13. Связанные документы

- [ARCHITECTURE.md](ARCHITECTURE.md) — устройство сервиса.
- [INFRASTRUCTURE.md](INFRASTRUCTURE.md) — PostgreSQL, Redis, Kafka.
- [OBSERVABILITY.md](OBSERVABILITY.md) — мониторинг и алерты.
- [NOTIFICATIONS.md](NOTIFICATIONS.md) — push.
- [PERFORMANCE.md](PERFORMANCE.md) — масштабирование.
- [LOAD-TESTING.md](LOAD-TESTING.md) — нагрузочные тесты.
- [CI-CD.md](CI-CD.md) — пайплайны.
