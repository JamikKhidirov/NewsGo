# CI/CD и релизы

Проект использует GitHub Actions и GoReleaser. На каждый тег `v*`
собираются бинарники для Windows/Linux/macOS и публикуется GitHub Release
с `.exe` для Windows.

---

## 1. Обзор пайплайнов

| Workflow | Файл | Триггер | Результат |
|---|---|---|---|
| CI | `.github/workflows/ci.yml` | push/PR в `main`/`master`, вручную | fmt, vet, тесты, сборка, smoke-тест `.exe`, artifact |
| Release | `.github/workflows/release.yml` | тег `v*`, вручную | GitHub Release с архивами и checksums |
| Docker | `.github/workflows/docker.yml` | push в `main`/`master`, тег `v*`, вручную | образ в GHCR |

```
PR / push
   │
   ├── CI: gofmt → go vet → go test -race → go build
   │        └── Windows job: go build → запуск .exe → /api/v1/health
   │
   └── merge в main
          └── Docker: build & push ghcr.io/<owner>/<repo>:main

git tag v1.0.0 && git push origin v1.0.0
   │
   ├── Release: тесты → GoReleaser
   │      ├── gonews_1.0.0_windows_amd64.zip  → gonews.exe
   │      ├── gonews_1.0.0_windows_arm64.zip  → gonews.exe
   │      ├── gonews_1.0.0_linux_amd64.tar.gz
   │      ├── gonews_1.0.0_linux_arm64.tar.gz
   │      ├── gonews_1.0.0_darwin_arm64.tar.gz
   │      └── checksums.txt
   │
   └── Docker: ghcr.io/<owner>/<repo>:1.0.0
```

---

## 2. CI

### 2.1 Что проверяется

Job `test` (ubuntu-latest):

1. `gofmt -l .` — любой неформатированный файл ломает сборку.
2. `go vet ./...`.
3. `go test -race -count=1 ./...`.
4. `CGO_ENABLED=0 go build ./...`.

Job `build-windows` (windows-latest):

1. `go build -trimpath -ldflags "-s -w" -o bin/gonews.exe ./cmd/server`.
2. Запуск бинарника с временной БД и `INGEST_ON_START=false`.
3. Проверка `GET /api/v1/health`.
4. Загрузка `gonews.exe` как artifact.

### 2.2 Локальный запуск тех же проверок

Windows PowerShell:

```powershell
gofmt -l .
go vet ./...
go test -count=1 ./...
$env:CGO_ENABLED = "0"
go build -trimpath -o bin\gonews.exe .\cmd\server
```

Linux/macOS:

```bash
make fmt-check
make vet
make test
make build
```

С race detector:

```bash
go test -race -count=1 ./...
```

> На Windows для `-race` нужен CGO и установленный GCC (MinGW). В CI race
> запускается на Linux. Основные тесты работают и без race.

---

## 3. Release

### 3.1 Процесс

1. Убедитесь, что `main` зелёный.
2. Обновите версию (при необходимости) и changelog.
3. Создайте тег:

```bash
git tag -a v1.0.0 -m "GoNews 1.0.0"
git push origin v1.0.0
```

4. Workflow `Release`:
   - запускает `go test`;
   - запускает GoReleaser `release --clean`;
   - публикует GitHub Release;
   - прикладывает архивы и `checksums.txt`;
   - загружает артефакты в Actions.

### 3.2 Артефакты

GoReleaser (`.goreleaser.yaml`):

| Платформа | Архив | Внутри |
|---|---|---|
| Windows amd64 | `.zip` | `gonews.exe` |
| Windows arm64 | `.zip` | `gonews.exe` |
| Linux amd64/arm64 | `.tar.gz` | `gonews` |
| macOS arm64 | `.tar.gz` | `gonews` |

В архивы также включены `README.md`, `LICENSE`, `.env.example`.
`checksums.txt` содержит SHA-256.

### 3.3 Версия в бинарнике

GoReleaser передаёт:

```
-X github.com/gonews/gonews/internal/version.Version={{ .Version }}
-X github.com/gonews/gonews/internal/version.Commit={{ .ShortCommit }}
-X github.com/gonews/gonews/internal/version.BuildDate={{ .Date }}
```

Проверка:

```bash
./gonews
curl http://localhost:8080/
```

В ответе будет `version` и `build`.

### 3.4 Локальная проверка релиза

Установите GoReleaser:

```bash
go install github.com/goreleaser/goreleaser/v2@latest
```

Соберите snapshot без публикации:

```bash
goreleaser release --snapshot --clean
```

Артефакты появятся в `dist/`.

Windows:

```powershell
goreleaser release --snapshot --clean
Get-ChildItem dist
```

### 3.5 Ручной запуск

В GitHub: **Actions → Release → Run workflow**. При ручном запуске
GoReleaser создаст релиз для текущего состояния; тег лучше указывать
явно через `workflow_dispatch` не поддерживается — используйте push тега.

---

## 4. Docker

Workflow собирает образ и публикует в GitHub Container Registry:

```
ghcr.io/<owner>/<repo>:main
ghcr.io/<owner>/<repo>:sha-<commit>
ghcr.io/<owner>/<repo>:1.0.0      # для тега v1.0.0
ghcr.io/<owner>/<repo>:1.0
ghcr.io/<owner>/<repo>:latest     # для default branch
```

Локально:

```bash
docker build -t gonews:dev .
docker run --rm -p 8080:8080 -v gonews-data:/data gonews:dev
```

Compose:

```bash
docker compose up -d --build
```

### 4.1 Права

- `GITHUB_TOKEN` имеет `packages: write` (указано в workflow).
- Если пакет приватный, для pull нужен `docker login ghcr.io`.
- Имя образа должно быть в нижнем регистре. Если в `owner/repo` есть
  заглавные буквы, замените `${{ github.repository }}` на явное значение
  в нижнем регистре.

---

## 5. Секреты и настройки репозитория

Обязательные:

| Секрет | Назначение |
|---|---|
| `GITHUB_TOKEN` | Автоматически предоставляется Actions |

Опциональные (для деплоя):

| Секрет | Назначение |
|---|---|
| `DEPLOY_HOST` | SSH-хост |
| `DEPLOY_USER` | SSH-пользователь |
| `DEPLOY_SSH_KEY` | Приватный ключ |
| `GONEWS_JWT_SECRET` | Секрет для production |
| `GONEWS_ADMIN_TOKEN` | Админ-токен |

Секреты приложения **не** коммитятся. `.env` и `service-account*.json`
исключены через `.gitignore`.

---

## 6. Защита ветки

Рекомендуется для `main`:

- Require pull request before merging.
- Require status checks: `Test & vet`, `Build gonews.exe`.
- Require branches to be up to date.
- Require linear history (squash/rebase).
- Запретить force push.
- Теги `v*` создавать только через PR/релизный процесс.

---

## 7. Версионирование

Используется [Semantic Versioning](https://semver.org/lang/ru/):

- `MAJOR` — несовместимые изменения API;
- `MINOR` — новые возможности обратносовместимо;
- `PATCH` — исправления.

Примеры тегов: `v1.0.0`, `v1.1.0`, `v1.1.1`, `v2.0.0`.
Префикс `v` обязателен для workflow.

---

## 8. Changelog

GoReleaser формирует changelog из коммитов и группирует:

- `feat:` → Новые возможности;
- `fix:` → Исправления;
- остальное → Прочее.

Исключаются `docs:`, `test:`, `chore:`, `ci:`, merge-коммиты.
Рекомендуется [Conventional Commits](https://www.conventionalcommits.org/ru/).

---

## 9. Диагностика

| Проблема | Причина | Решение |
|---|---|---|
| CI падает на `gofmt` | Файлы не отформатированы | `gofmt -w .` и коммит |
| `go test -race` падает только в CI | Гонка/тайминг | Воспроизведите локально на Linux/Docker |
| Release не создаётся | Тег без `v` или нет прав | Тег `v1.0.0`, `permissions: contents: write` |
| GoReleaser: `git is in a dirty state` | Незакоммиченные файлы | Закоммитьте или используйте `--snapshot` |
| GoReleaser: `no commits found` | Мелкий clone | `fetch-depth: 0` |
| Docker push 403 | Нет `packages: write` | Проверьте `permissions` |
| Образ не пуллится | Приватный пакет | `docker login ghcr.io` |
| Windows smoke test не стартует | Порт занят | В CI используется `18080`; локально выберите другой |
| `GONEWS_JWT_SECRET must be set` | `ENV=production` с dev-секретом | Задайте секрет |

---

## 10. Связанные документы

- [DEPLOYMENT.md](DEPLOYMENT.md)
- [ARCHITECTURE.md](ARCHITECTURE.md)
- [API.md](API.md)
