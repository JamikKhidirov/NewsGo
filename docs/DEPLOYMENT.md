# Развёртывание GoNews

Руководство по установке, настройке и эксплуатации сервиса.

---

## 1. Требования

- Go 1.26+ (только для сборки из исходников) **или** готовый бинарник из
  GitHub Releases.
- Windows 10/11, Windows Server 2019+, Linux x86_64/arm64, macOS.
- 256 MB RAM минимум, 512 MB рекомендуется.
- 1 GB диска (база + бинарник + логи).
- Исходящий HTTPS к источникам новостей, провайдерам перевода и FCM.
- CGO не требуется: SQLite работает через `modernc.org/sqlite`.

---

## 2. Быстрая установка

### 2.1 Из релиза

1. Скачайте архив для своей платформы из GitHub Releases.
2. Распакуйте в каталог, например `C:\GoNews` или `/opt/gonews`.
3. Создайте `.env` на основе `.env.example`.
4. Запустите бинарник.

Windows:

```powershell
Expand-Archive gonews_1.0.0_windows_amd64.zip -DestinationPath C:\GoNews
cd C:\GoNews
Copy-Item .env.example .env
notepad .env
.\gonews.exe
```

Linux:

```bash
tar -xzf gonews_1.0.0_linux_amd64.tar.gz -C /opt/gonews
cd /opt/gonews
cp .env.example .env
nano .env
./gonews
```

### 2.2 Из исходников

```bash
git clone https://github.com/gonews/gonews.git
cd gonews
go mod download
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gonews ./cmd/server
```

Windows PowerShell:

```powershell
.\scripts\build.ps1
```

---

## 3. Конфигурация

Источники значений (по приоритету):

1. переменные окружения;
2. файл `.env` в рабочем каталоге;
3. значения по умолчанию.

### 3.1 Полный список переменных

| Переменная | По умолчанию | Описание |
|---|---|---|
| `GONEWS_ENV` | `development` | `development` или `production` |
| `GONEWS_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `GONEWS_HTTP_ADDR` | `:8080` | Адрес и порт |
| `GONEWS_PUBLIC_BASE_URL` | `http://localhost:8080` | Публичный URL для ссылок |
| `GONEWS_DB_PATH` | `data/gonews.db` | Путь к SQLite |
| `GONEWS_JWT_SECRET` | dev-секрет | Секрет подписи JWT |
| `GONEWS_ACCESS_TTL` | `30m` | Время жизни access-токена |
| `GONEWS_REFRESH_TTL` | `720h` | Время жизни refresh-токена |
| `GONEWS_ADMIN_TOKEN` | пусто | Токен для `X-Admin-Token` |
| `GONEWS_ADMIN_EMAILS` | пусто | Email-ы админов через запятую |
| `GONEWS_CORS_ORIGINS` | `*` | Разрешённые Origin через запятую |
| `GONEWS_RATE_LIMIT_PER_MINUTE` | `180` | Лимит запросов с IP |
| `GONEWS_INGEST_ON_START` | `true` | Сбор при старте |
| `GONEWS_INGEST_INTERVAL` | `10m` | Период сбора |
| `GONEWS_INGEST_TIMEOUT` | `90s` | Таймаут одного сбора |
| `GONEWS_PROVIDERS` | все | Список слагов источников |
| `GONEWS_PROVIDER_LIMIT` | `25` | Материалов с источника |
| `GONEWS_USER_AGENT` | `GoNewsBot/1.0 ...` | User-Agent при сборе |
| `GONEWS_RETENTION_DAYS` | `90` | Хранение новостей, `0` — вечно |
| `GONEWS_TRANSLATE_ENABLED` | `true` | Включить перевод |
| `GONEWS_TRANSLATE_PROVIDERS` | `google,lingva,mymemory,libretranslate` | Цепочка провайдеров |
| `GONEWS_TRANSLATE_TARGETS` | `ru` | Целевые языки |
| `GONEWS_TRANSLATE_WORKERS` | `2` | Число воркеров |
| `GONEWS_TRANSLATE_TIMEOUT` | `12s` | Таймаут запроса к провайдеру |
| `GONEWS_TRANSLATE_MAX_CHARS` | `4500` | Максимум символов на запрос |
| `GONEWS_LIBRETRANSLATE_URL` | `https://libretranslate.com` | LibreTranslate |
| `GONEWS_LINGVA_URL` | `https://lingva.ml` | Lingva |
| `GONEWS_PUSH_ENABLED` | `false` | Реальная отправка FCM |
| `GONEWS_FCM_PROJECT_ID` | пусто | Firebase project ID |
| `GONEWS_FCM_SERVICE_ACCOUNT_FILE` | пусто | Путь к JSON сервисного аккаунта |
| `GONEWS_PUSH_BROADCAST` | `true` | Авторассылка топ-новостей |
| `GONEWS_PUSH_INTERVAL` | `30m` | Антидубль-интервал рассылки |
| `GONEWS_PUSH_MIN_SCORE` | `20` | Минимальный score для рассылки |

### 3.2 Production-минимум

```dotenv
GONEWS_ENV=production
GONEWS_JWT_SECRET=<длинная случайная строка>
GONEWS_ADMIN_TOKEN=<длинный случайный токен>
GONEWS_PUBLIC_BASE_URL=https://news.example.com
GONEWS_CORS_ORIGINS=https://app.example.com
GONEWS_DB_PATH=/var/lib/gonews/gonews.db
GONEWS_HTTP_ADDR=127.0.0.1:8080
```

Сгенерировать секреты:

```bash
openssl rand -base64 48
```

PowerShell:

```powershell
[Convert]::ToBase64String((1..48 | ForEach-Object { Get-Random -Maximum 256 }))
```

> В `GONEWS_ENV=production` сервис **не запустится** с dev-секретом JWT.

---

## 4. Windows

### 4.1 Запуск как служба (NSSM)

1. Скачайте [NSSM](https://nssm.cc/download).
2. Установите службу:

```powershell
nssm install GoNews "C:\GoNews\gonews.exe"
nssm set GoNews AppDirectory "C:\GoNews"
nssm set GoNews AppEnvironmentExtra GONEWS_ENV=production GONEWS_DB_PATH=C:\GoNews\data\gonews.db
nssm set GoNews AppStdout "C:\GoNews\logs\gonews.log"
nssm set GoNews AppStderr "C:\GoNews\logs\gonews.err.log"
nssm start GoNews
```

### 4.2 Запуск как служба (встроенный `sc.exe`)

```powershell
New-Service -Name GoNews `
  -BinaryPathName '"C:\GoNews\gonews.exe"' `
  -DisplayName "GoNews API" `
  -StartupType Automatic
Start-Service GoNews
```

Переменные окружения для службы удобнее задать через `.env` в
`C:\GoNews` — сервис читает его из рабочего каталога.

### 4.3 Firewall

```powershell
New-NetFirewallRule -DisplayName "GoNews API" -Direction Inbound `
  -Protocol TCP -LocalPort 8080 -Action Allow
```

В production ставьте GoNews за reverse proxy и не открывайте порт наружу.

---

## 5. Linux (systemd)

Создайте пользователя и каталоги:

```bash
sudo useradd --system --home /opt/gonews --shell /usr/sbin/nologin gonews
sudo mkdir -p /opt/gonews /var/lib/gonews
sudo chown -R gonews:gonews /opt/gonews /var/lib/gonews
```

`/etc/systemd/system/gonews.service`:

```ini
[Unit]
Description=GoNews API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=gonews
Group=gonews
WorkingDirectory=/opt/gonews
EnvironmentFile=/opt/gonews/.env
ExecStart=/opt/gonews/gonews
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536

# Защита
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/gonews
AmbientCapabilities=
CapabilityBoundingSet=

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now gonews
sudo systemctl status gonews
sudo journalctl -u gonews -f
```

---

## 6. Docker и Docker Compose

### 6.1 Compose

```bash
cp .env.example .env
# отредактируйте .env
docker compose up -d --build
docker compose logs -f gonews
```

Данные — в томе `gonews-data`, база — `/data/gonews.db`.

Self-hosted переводчик:

```bash
GONEWS_LIBRETRANSLATE_URL=http://libretranslate:5000 \
  docker compose --profile translate up -d --build
```

### 6.2 Только Docker

```bash
docker build -t gonews:1.0.0 .
docker run -d --name gonews \
  -p 8080:8080 \
  -v gonews-data:/data \
  -e GONEWS_ENV=production \
  -e GONEWS_JWT_SECRET="$(openssl rand -base64 48)" \
  -e GONEWS_ADMIN_TOKEN="$(openssl rand -base64 32)" \
  gonews:1.0.0
```

Образ основан на distroless: внутри нет shell, поэтому healthcheck
настраивайте на стороне оркестратора (HTTP-проверка `/healthz`).

---

## 7. Reverse proxy

### 7.1 Nginx

```nginx
upstream gonews {
    server 127.0.0.1:8080;
    keepalive 32;
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
        proxy_read_timeout 180s;
    }
}
```

### 7.2 Caddy

```caddyfile
news.example.com {
    encode zstd gzip
    reverse_proxy 127.0.0.1:8080
}
```

### 7.3 Важно про IP клиента

Сервис определяет IP по `RemoteAddr`. За reverse proxy все клиенты будут
выглядеть одним IP, и rate limit станет общим. Для production:

- либо настройте лимит на уровне proxy (nginx `limit_req`);
- либо доработайте `clientIP` для доверенных `X-Forwarded-For`
  (учитывайте только доверенные прокси, иначе заголовок подделывается).

---

## 8. FCM

1. Создайте проект в Firebase.
2. Включите Cloud Messaging.
3. Создайте service account с ролью Firebase Cloud Messaging API.
4. Скачайте JSON-ключ.
5. Укажите:

```dotenv
GONEWS_PUSH_ENABLED=true
GONEWS_FCM_PROJECT_ID=my-project-id
GONEWS_FCM_SERVICE_ACCOUNT_FILE=/opt/gonews/fcm-service-account.json
```

Проверка:

```bash
curl -X POST http://localhost:8080/api/v1/admin/push \
  -H "X-Admin-Token: $GONEWS_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Тест","body":"Проверка push"}'
```

Без FCM сервис работает в dry-run: сообщение пишется в лог и во внутренний
фид.

---

## 9. Резервное копирование

SQLite в WAL-режиме. Корректный бэкап — через `VACUUM INTO` или остановку
сервиса.

```bash
sqlite3 /var/lib/gonews/gonews.db "VACUUM INTO '/backup/gonews-$(date +%F).db'"
```

PowerShell (если установлен `sqlite3`):

```powershell
sqlite3 C:\GoNews\data\gonews.db "VACUUM INTO 'C:\Backup\gonews-2026-01-01.db'"
```

Без `sqlite3`:

```powershell
Stop-Service GoNews
Copy-Item C:\GoNews\data\gonews.db C:\Backup\gonews-2026-01-01.db
Start-Service GoNews
```

Рекомендуется ежедневный бэкап и хранение 7–30 копий.

---

## 10. Мониторинг

| Проверка | Адрес | Ожидание |
|---|---|---|
| Liveness | `/healthz` | `200 {"status":"ok"}` |
| Readiness | `/readyz` | `200 {"status":"ready"}` |
| API health | `/api/v1/health` | `200` |
| Метрики | `/metrics` | текст Prometheus |

Ключевые метрики:

- `gonews_uptime_seconds`
- `gonews_http_requests_total`
- `gonews_http_responses_total{class="2xx|4xx|5xx"}`
- `gonews_ingest_runs_total`
- `gonews_pushes_total`

Логи — структурированные (`slog`), в production JSON в stdout.
Собирайте через journald, Docker logs или Loki.

Алерты:

- `5xx` > 5% за 5 минут;
- `/readyz` недоступен 2 минуты;
- `gonews_ingest_runs_total` не растёт > 30 минут;
- `translation.failed` растёт (см. `/admin/translation-stats`).

---

## 11. Обновление

1. Сделайте бэкап базы.
2. Остановите сервис.
3. Замените бинарник.
4. Запустите сервис — миграции применятся автоматически.
5. Проверьте `/api/v1/health` и `/admin/stats`.

Откат: верните предыдущий бинарник. Схема миграций только добавляет
таблицы/индексы, поэтому откат обычно безопасен, но бэкап обязателен.

---

## 12. Production checklist

- [ ] `GONEWS_ENV=production`
- [ ] Случайный `GONEWS_JWT_SECRET` (≥ 32 байта)
- [ ] Случайный `GONEWS_ADMIN_TOKEN`
- [ ] `GONEWS_CORS_ORIGINS` ограничен доменом приложения
- [ ] HTTPS через reverse proxy
- [ ] Порт приложения не открыт наружу
- [ ] Каталог данных доступен только пользователю сервиса
- [ ] Настроен бэкап SQLite
- [ ] Настроены health-проверки и алерты
- [ ] Ограничены `GONEWS_RATE_LIMIT_PER_MINUTE`
- [ ] Настроен `GONEWS_PROVIDERS` под нужные источники (при необходимости)
- [ ] Проверен перевод (`/admin/translation-stats`)
- [ ] Настроен FCM или осознанно оставлен dry-run
- [ ] Логи собираются и ротируются

---

## 13. Диагностика

| Симптом | Причина | Решение |
|---|---|---|
| `database is locked` | Конкуренция записи | Убедитесь, что WAL включён; не запускайте два процесса на одной БД |
| Пустая лента | Сбор не выполнен | `POST /admin/ingest`, проверьте логи |
| Много `pending` переводов | Провайдеры блокируют | Self-hosted LibreTranslate, см. TRANSLATION.md |
| Источник всегда с ошибкой | Блокировка/география | Уберите из `GONEWS_PROVIDERS` или используйте прокси |
| `401` на всех запросах | Истёк access-токен | `/auth/refresh` |
| `403` на admin | Нет роли/token | `GONEWS_ADMIN_EMAILS` или `X-Admin-Token` |
| `429` | Rate limit | Увеличьте лимит или настройте proxy |
| Push не приходит | FCM не настроен/неверный ключ | Проверьте `GONEWS_PUSH_*`, смотрите логи |

---

## 14. Связанные документы

- [ARCHITECTURE.md](ARCHITECTURE.md)
- [API.md](API.md)
- [TRANSLATION.md](TRANSLATION.md)
- [AGGREGATORS.md](AGGREGATORS.md)
- [CI-CD.md](CI-CD.md)
