# Источники новостей и агрегация

GoNews собирает новости из **26 бесплатных источников без API-ключей**:
RSS/Atom, Hacker News (Algolia), Reddit JSON, DEV.to и Spaceflight News API.
Все они сводятся к единой модели и проходят нормализацию, дедупликацию и
автоматическую разметку.

---

## 1. Каталог источников

Определён в `internal/news/providers.go` (`SourceDefs`).

| Слаг | Название | Язык | Категория | Вес | Тип |
|---|---|---|---|---|---|
| `hackernews` | Hacker News | en | technology | 12 | Algolia JSON |
| `reddit-worldnews` | Reddit r/worldnews | en | world | 9 | Reddit JSON |
| `reddit-technology` | Reddit r/technology | en | technology | 8 | Reddit JSON |
| `devto` | DEV Community | en | technology | 6 | DEV.to API |
| `spaceflight` | Spaceflight News | en | space | 8 | Spaceflight API |
| `bbc-world` | BBC News | en | world | 15 | RSS |
| `npr` | NPR | en | world | 11 | RSS |
| `aljazeera` | Al Jazeera | en | world | 11 | RSS |
| `guardian-world` | The Guardian | en | world | 13 | RSS |
| `lenta` | Lenta.ru | ru | world | 13 | RSS |
| `ria` | РИА Новости | ru | world | 13 | RSS |
| `habr` | Хабр | ru | technology | 9 | RSS |
| `kommersant` | Коммерсантъ | ru | business | 12 | RSS |
| `cnews` | CNews | ru | technology | 8 | RSS |
| `coindesk` | CoinDesk | en | crypto | 7 | RSS |
| `nasa` | NASA | en | space | 10 | RSS |
| `techcrunch` | TechCrunch | en | technology | 8 | RSS |
| `theverge` | The Verge | en | technology | 8 | RSS |
| `arstechnica` | Ars Technica | en | technology | 8 | RSS |
| `tass` | ТАСС | ru | world | 12 | RSS |
| `rbc` | РБК | ru | business | 12 | RSS |
| `nplus1` | N+1 | ru | science | 8 | RSS |
| `lobsters` | Lobsters | en | technology | 6 | RSS |
| `bleepingcomputer` | BleepingComputer | en | security | 7 | RSS |
| `izvestia` | Известия | ru | world | 10 | RSS |
| `arxiv-ai` | arXiv AI | en | science | 6 | RSS (Atom) |

### 1.1 URL-адреса

| Слаг | URL |
|---|---|
| `hackernews` | `https://hn.algolia.com/api/v1/search?tags=front_page&hitsPerPage=%d` |
| `reddit-worldnews` | `https://www.reddit.com/r/worldnews/hot.json?limit=%d&raw_json=1` |
| `reddit-technology` | `https://www.reddit.com/r/technology/hot.json?limit=%d&raw_json=1` |
| `devto` | `https://dev.to/api/articles?per_page=%d&top=7` |
| `spaceflight` | `https://api.spaceflightnewsapi.net/v4/articles/?limit=%d` |
| `bbc-world` | `https://feeds.bbci.co.uk/news/world/rss.xml` |
| `npr` | `https://feeds.npr.org/1001/rss.xml` |
| `aljazeera` | `https://www.aljazeera.com/xml/rss/all.xml` |
| `guardian-world` | `https://www.theguardian.com/world/rss` |
| `lenta` | `https://lenta.ru/rss/news` |
| `ria` | `https://ria.ru/export/rss2/archive/index.xml` |
| `habr` | `https://habr.com/ru/rss/news/?fl=ru` |
| `kommersant` | `https://www.kommersant.ru/RSS/news.xml` |
| `cnews` | `https://www.cnews.ru/inc/rss/news.xml` |
| `coindesk` | `https://www.coindesk.com/arc/outboundfeeds/rss/` |
| `nasa` | `https://www.nasa.gov/feed/` |
| `techcrunch` | `https://techcrunch.com/feed/` |
| `theverge` | `https://www.theverge.com/rss/index.xml` |
| `arstechnica` | `https://feeds.arstechnica.com/arstechnica/index` |
| `tass` | `https://tass.ru/rss/v2.xml` |
| `rbc` | `https://rssexport.rbc.ru/rbcnews/news/30/full.rss` |
| `nplus1` | `https://nplus1.ru/rss` |
| `lobsters` | `https://lobste.rs/rss` |
| `bleepingcomputer` | `https://www.bleepingcomputer.com/feed/` |
| `izvestia` | `https://iz.ru/xml/rss/all.xml` |
| `arxiv-ai` | `http://export.arxiv.org/api/query?search_query=cat:cs.AI&sortBy=submittedDate&sortOrder=descending&max_results=25` |

> URL могут меняться владельцами источников. Если источник стабильно
> возвращает ошибку — проверьте актуальность адреса и обновите каталог.

---

## 2. Парсеры

### 2.1 RSS / Atom

Используется `github.com/mmcdole/gofeed`.

Извлекается:

- `title` — очищается от HTML и пробелов, обрезается до 300 символов;
- `summary` — `description` без HTML, до 600 символов;
- `content` — `content:encoded`, иначе `description`, до 20 000 символов;
- `image` — `it.Image`, enclosure с `image/*` или первый `<img src>`;
- `author` — имя автора;
- `published` — `published`, иначе `updated`, иначе текущее время;
- `lang` — определяется по тексту, иначе язык источника.

Будущие даты (более +2 часов) заменяются текущим временем.

### 2.2 Hacker News (Algolia)

`https://hn.algolia.com/api/v1/search?tags=front_page`.

- `objectID`, `title`, `url`, `author`, `created_at`, `points`;
- если `url` пуст — ссылка на обсуждение `news.ycombinator.com/item?id=...`;
- `score = points` (влияет на `hot_score`);
- язык — `en`.

### 2.3 Reddit JSON

`/r/{subreddit}/hot.json?raw_json=1`.

- `id`, `title`, `url`, `permalink`, `selftext`, `author`, `created_utc`,
  `score`, `preview.images[0].source.url`;
- self-post или относительная ссылка → `reddit.com` + `permalink`;
- `score` влияет на `hot_score`;
- язык — `en`.

Публичный Reddit API часто отвечает `403` для дата-центров. Источник
можно отключить через `GONEWS_PROVIDERS`.

### 2.4 DEV.to

`/api/articles?per_page=N&top=7`.

- `id`, `title`, `description`, `url`, `cover_image`, `published_at`,
  `tag_list`, `positive_reactions_count`, `user.name`;
- `score = positive_reactions_count`;
- язык — `en`.

### 2.5 Spaceflight News API

`/v4/articles/?limit=N`.

- `id`, `title`, `url`, `image_url`, `news_site`, `summary`, `published_at`;
- категория всегда `space`;
- язык — `en`.

---

## 3. Нормализация

`internal/news/aggregator.go` → `normalizeItem`:

1. **Заголовок.** `CleanTitle`: удаление HTML, схлопывание пробелов,
   ограничение 300 символов. Пустой заголовок → материал отбрасывается.
2. **URL.** `CanonicalURL`:
   - нижний регистр scheme и host;
   - удаление fragment;
   - удаление трекинг-параметров (`utm_*`, `fbclid`, `gclid`, `ref`,
     `source`, `mc_cid`, `_ga` и др.);
   - удаление завершающего `/`.
3. **Хеш.** `SHA-256(canonical_url)` в нижнем регистре. Если URL пуст —
   `source_slug|external_id`.
4. **Язык.** `langdetect.Normalize` + `Detect` по заголовку и описанию;
   fallback — язык источника.
5. **Текст.** `StripHTML`, ограничения: summary 600, content 20 000.
   Если summary пуст — берётся начало content; если content пуст —
   summary.
6. **Дата.** Пустая → `now`; будущая (> +2 ч) → `now`.
7. **Категория.** Если провайдер задал валидную — используется она; иначе
   `DetectCategory` по словарю.
8. **Теги.** `ExtractTags` (до 8) + тег категории, если его нет.
9. **hot_score.**

---

## 4. Дедупликация

Дедупликация двухуровневая:

1. **Внутри запуска.** Все нормализованные материалы складываются в map по
   `unique_hash`. При коллизии остаётся версия с более длинным `content`.
2. **В базе.** `articles.unique_hash` имеет `UNIQUE`. `UpsertArticle`
   обновляет существующую запись, не создавая дубликат.

Это отсекает:

- один и тот же материал из разных RSS;
- повторные публикации;
- URL с UTM-метками;
- агрегаторы, ссылающиеся на первоисточник.

ID статьи детерминирован: `a_` + первые 20 символов SHA-256. Поэтому
повторный сбор не создаёт новый ID.

---

## 5. Категории и теги

### 5.1 Категории

13 категорий (`internal/news/catalog.go`):

`world`, `politics`, `business`, `technology`, `science`, `sports`,
`health`, `culture`, `security`, `climate`, `crypto`, `space`, `auto`.

`DetectCategory` считает совпадения ключевых слов по всем тегам и выбирает
категорию с максимальным счётом. Fallback — категория источника, затем
`world`.

### 5.2 Теги

48 тегов с русскими и английскими названиями. Каждый тег имеет:

- `slug`, `nameRu`, `nameEn`;
- `category` — вклад в определение категории;
- `keywords` — подстроки на русском и английском.

Примеры: `ai`, `crypto`, `space`, `cybersecurity`, `startups`, `nvidia`,
`sanctions`, `elections`, `climate`, `gaming`.

Правила:

- текст приводится к нижнему регистру, `ё` → `е`;
- ищутся подстроки (не слова), поэтому «нейросет» ловит «нейросети»;
- максимум 8 тегов на статью;
- тег категории добавляется принудительно.

---

## 6. Ранжирование

`hotScore`:

```go
score = weight + min(log1p(engagement)*3, 25) + 60*exp(-ageHours/36)
```

Где:

- `weight` — вес источника (6–15);
- `engagement` — score из HN/Reddit/DEV.to;
- `ageHours` — возраст публикации.

Свежесть даёт до 60 баллов, вес — до 15, вовлечённость — до 25.
Сортировка `hot` использует `hot_score`, `trending` — то же окно за 7 дней.

---

## 7. Планирование и таймауты

- Сбор запускается при старте (`INGEST_ON_START`) и по таймеру
  (`INGEST_INTERVAL`, 10 минут).
- Одновременно выполняется только один сбор (`ErrAlreadyRunning`).
- Провайдеры опрашиваются параллельно, но не более **4** одновременно.
- На каждый источник — **35 секунд**.
- Общий таймаут запуска — `GONEWS_INGEST_TIMEOUT` (90 секунд).
- Тело ответа ограничено 8 MiB.
- Ошибка источника не прерывает остальные.
- Итог пишется в `ingest_runs` со статусом `ok`, `partial` или `failed`.

---

## 8. Ограничение набора источников

Все источники:

```dotenv
GONEWS_PROVIDERS=
```

Только русскоязычные и технологии:

```dotenv
GONEWS_PROVIDERS=lenta,ria,habr,tass,rbc,cnews,nplus1,izvestia
```

Проверка без ожидания таймера:

```bash
curl -X POST http://localhost:8080/api/v1/admin/ingest \
  -H "X-Admin-Token: $GONEWS_ADMIN_TOKEN"
```

Статистика по источникам — в ответе и в `GET /admin/ingest-runs`.

---

## 9. Добавление нового источника

1. Добавьте `SourceDef` в `internal/news/providers.go`:

```go
{
    Slug:     "example",
    Name:     "Example News",
    Homepage: "https://example.com",
    Kind:     "rss",
    Lang:     "en",
    Category: "world",
    Weight:   8,
    URL:      "https://example.com/feed.xml",
    Parser:   "rss",
},
```

2. Если нужен новый формат — реализуйте `Provider`:

```go
type exampleProvider struct{ providerBase }

func (p *exampleProvider) Fetch(ctx context.Context, limit int) ([]model.IngestItem, error) {
    body, err := p.fetch(ctx, fmt.Sprintf(p.def.URL, limit))
    if err != nil {
        return nil, err
    }
    // ... разбор ...
    return items, nil
}
```

3. Зарегистрируйте парсер в `BuildProviders`:

```go
case "example":
    out = append(out, &exampleProvider{providerBase: base})
```

4. Добавьте тест в `internal/news/news_test.go` (нормализация, дедуп).
5. Обновите таблицу в этом документе и в README.

Источник появится в `/api/v1/sources` после следующего `Seed` (при старте).

---

## 10. Надёжность и блокировки

Публичные источники могут:

- блокировать IP дата-центров (Reddit `403`, BBC/Guardian TLS timeout);
- отдавать устаревший или невалидный XML;
- менять URL;
- ограничивать частоту.

Меры:

- изоляция ошибок по провайдеру;
- таймауты;
- ограничение параллелизма;
- User-Agent `GONEWS_USER_AGENT`;
- возможность отключить источник;
- повторный сбор по расписанию.

Если источник критичен, но недоступен из вашей сети:

1. поднимите прокси/relay и укажите его в коде провайдера;
2. используйте зеркало RSS;
3. отключите источник и добавьте альтернативу.

---

## 11. Правовые и этические аспекты

- Сервис использует публичные RSS/JSON-ленты и не обходит авторизацию.
- Соблюдайте условия использования источников и `robots.txt`.
- Не публикуйте полные тексты чужих статей без прав: GoNews хранит
  заголовок, краткое описание и ссылку на оригинал; полный текст зависит
  от того, что отдаёт RSS.
- Указывайте источник и ссылку в клиенте.
- Для коммерческого использования проверьте лицензии источников.

---

## 12. Связанные документы

- [ARCHITECTURE.md](ARCHITECTURE.md)
- [TRANSLATION.md](TRANSLATION.md)
- [API.md](API.md)
- [DEPLOYMENT.md](DEPLOYMENT.md)
