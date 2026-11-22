package news

import (
	"strings"

	"github.com/gonews/gonews/internal/model"
)

// Category is a public news category with bilingual labels.
type Category struct {
	Slug   string `json:"slug"`
	NameRU string `json:"nameRu"`
	NameEN string `json:"nameEn"`
}

// Categories is the canonical category list exposed by the API.
var Categories = []Category{
	{"world", "В мире", "World"},
	{"politics", "Политика", "Politics"},
	{"business", "Бизнес", "Business"},
	{"technology", "Технологии", "Technology"},
	{"science", "Наука", "Science"},
	{"sports", "Спорт", "Sports"},
	{"health", "Здоровье", "Health"},
	{"culture", "Культура", "Culture"},
	{"security", "Безопасность", "Security"},
	{"climate", "Климат", "Climate"},
	{"crypto", "Криптовалюты", "Crypto"},
	{"space", "Космос", "Space"},
	{"auto", "Авто", "Auto"},
}

// TagDef describes an auto-tagging rule.
type TagDef struct {
	Slug     string
	NameRU   string
	NameEN   string
	Category string
	Keywords []string
}

// TagCatalog is the built-in topic dictionary used for automatic tagging.
var TagCatalog = []TagDef{
	{"world", "В мире", "World", "world", []string{"мир", "world", "global", "международн", "international"}},
	{"russia", "Россия", "Russia", "", []string{"росси", "russia", "russian", "кремл", "kremlin", "москв", "moscow"}},
	{"usa", "США", "USA", "", []string{"сша", "usa", "u.s.", "america", "американ", "white house", "белый дом", "вашингтон", "washington"}},
	{"europe", "Европа", "Europe", "", []string{"европ", "europe", "евросоюз", "european union", "brussels", "брюссел"}},
	{"china", "Китай", "China", "", []string{"китай", "china", "chinese", "пекин", "beijing"}},
	{"ukraine", "Украина", "Ukraine", "", []string{"украин", "ukraine", "kyiv", "киев", "zelensk", "зеленск"}},
	{"middle-east", "Ближний Восток", "Middle East", "", []string{"ближн", "middle east", "israel", "израил", "palestin", "палестин", "gaza", "газа", "iran", "иран"}},
	{"politics", "Политика", "Politics", "politics", []string{"политик", "politics", "президент", "president", "парламент", "parliament", "government", "правительств", "выбор", "election", "сенат", "senate", "министр", "minister"}},
	{"war", "Война и конфликты", "War & Conflict", "security", []string{"войн", "war", "conflict", "конфликт", "атак", "attack", "missile", "ракет", "military", "военн", "army", "арми"}},
	{"security", "Безопасность", "Security", "security", []string{"безопасн", "security", "terror", "террор", "полици", "police", "crime", "преступ", "спецслужб"}},
	{"business", "Бизнес", "Business", "business", []string{"бизнес", "business", "compan", "компани", "corporation", "корпорац", "рынок", "market", "предпринимат"}},
	{"economy", "Экономика", "Economy", "business", []string{"экономик", "economy", "economic", "ввп", "gdp", "инфляц", "inflation", "безработиц", "unemployment"}},
	{"finance", "Финансы", "Finance", "business", []string{"финанс", "finance", "bank", "банк", "stock", "акци", "dollar", "доллар", "рубл", "currency", "валют", "bond", "облигац"}},
	{"crypto", "Криптовалюты", "Crypto", "crypto", []string{"крипт", "crypto", "bitcoin", "биткоин", "ethereum", "эфириум", "blockchain", "блокчейн", "stablecoin", "стейблкоин"}},
	{"technology", "Технологии", "Technology", "technology", []string{"технолог", "technology", "tech", "гаджет", "gadget", "software", "софт", "computer", "компьютер", "интернет", "internet", "chip", "чип"}},
	{"ai", "Искусственный интеллект", "AI", "technology", []string{"искусственн", "artificial intelligence", "ai ", "нейросет", "neural", "machine learning", "chatgpt", "openai", "llm", "ии "}},
	{"cybersecurity", "Кибербезопасность", "Cybersecurity", "security", []string{"кибербез", "cybersecurity", "hacker", "хакер", "утечк", "data breach", "malware", "вирус", "ransomware", "шифровальщик"}},
	{"startups", "Стартапы", "Startups", "business", []string{"стартап", "startup", "venture", "венчур", "funding", "инвестиц", "investment"}},
	{"mobile", "Мобильные", "Mobile", "technology", []string{"смартфон", "smartphone", "android", "андроид", "iphone", "айфон", "ios", "mobile", "мобильн"}},
	{"apple", "Apple", "Apple", "technology", []string{"apple", "эппл", "iphone", "айфон", "macbook", "ipad"}},
	{"google", "Google", "Google", "technology", []string{"google", "гугл", "youtube", "ютуб", "alphabet"}},
	{"microsoft", "Microsoft", "Microsoft", "technology", []string{"microsoft", "майкрософт", "windows", "виндоус", "azure"}},
	{"linux", "Linux и Open Source", "Linux & Open Source", "technology", []string{"linux", "линукс", "ubuntu", "open source", "открытый исходн"}},
	{"nvidia", "NVIDIA", "NVIDIA", "technology", []string{"nvidia", "нвидиа", "gpu", "видеокарт"}},
	{"space", "Космос", "Space", "space", []string{"космос", "space", "nasa", "наса", "rocket", "ракет", "satellite", "спутник", "mars", "марс", "moon", "луна", "spacex"}},
	{"science", "Наука", "Science", "science", []string{"наука", "science", "research", "исследован", "учен", "scientist", "physics", "физик", "biolog", "биолог", "археолог"}},
	{"climate", "Климат", "Climate", "climate", []string{"климат", "climate", "global warming", "потеплен", "carbon", "углерод", "emissions", "выброс"}},
	{"energy", "Энергетика", "Energy", "business", []string{"энергет", "energy", "oil", "нефть", "gas", "газ", "electricity", "электроэнерг", "solar", "солнечн", "nuclear", "атомн"}},
	{"auto", "Авто", "Auto", "auto", []string{"авто", "car ", "cars", "автомобил", "electric vehicle", "электромобил", "tesla", "тесла", "bmw", "toyota"}},
	{"sports", "Спорт", "Sports", "sports", []string{"спорт", "sport", "football", "футбол", "soccer", "olympic", "олимпи", "tennis", "теннис", "basketball", "баскетбол", "хокке"}},
	{"health", "Здоровье", "Health", "health", []string{"здоров", "health", "medic", "медиц", "disease", "болезн", "vaccine", "вакцин", "hospital", "больниц", "cancer", "раков"}},
	{"culture", "Культура", "Culture", "culture", []string{"культур", "culture", "film", "фильм", "movie", "кино", "music", "музык", "art", "искусств", "book", "книг", "celebrity"}},
	{"gaming", "Игры", "Gaming", "technology", []string{"игр", "game", "gaming", "playstation", "xbox", "nintendo", "steam", "киберспорт", "esports"}},
	{"education", "Образование", "Education", "science", []string{"образован", "education", "school", "школ", "universit", "универ", "студент", "student"}},
	{"travel", "Путешествия", "Travel", "world", []string{"путешеств", "travel", "tourism", "туризм", "airline", "авиакомпан", "hotel", "отель"}},
	{"food", "Еда", "Food", "culture", []string{"еда", "food", "restaurant", "ресторан", "recipe", "рецепт", "кухн"}},
	{"weather", "Погода", "Weather", "climate", []string{"погод", "weather", "storm", "шторм", "hurricane", "ураган", "flood", "наводн", "snow", "снег"}},
	{"disaster", "Катастрофы", "Disasters", "world", []string{"катастроф", "disaster", "earthquake", "землетряс", "wildfire", "пожар", "tsunami", "цунами"}},
	{"elections", "Выборы", "Elections", "politics", []string{"выбор", "election", "vote", "голосован", "ballot", "campaign", "кампани"}},
	{"sanctions", "Санкции", "Sanctions", "politics", []string{"санкц", "sanctions", "embargo", "эмбарго", "tariff", "пошлин"}},
	{"migration", "Миграция", "Migration", "world", []string{"мигран", "migration", "refugee", "бежен", "asylum", "убежищ"}},
	{"protests", "Протесты", "Protests", "politics", []string{"протест", "protest", "демонстрац", "rally", "митинг", "strike", "забастов"}},
	{"media", "Медиа", "Media", "culture", []string{"медиа", "media", "journalist", "журналист", "press", "пресс", "newspaper", "газет", "television", "телевид"}},
	{"social", "Соцсети", "Social Media", "technology", []string{"соцсет", "social media", "facebook", "instagram", "tiktok", "twitter", "telegram", "телеграм"}},
	{"realestate", "Недвижимость", "Real Estate", "business", []string{"недвижим", "real estate", "housing", "жиль", "mortgage", "ипотек", "rent", "аренд"}},
	{"aviation", "Авиация", "Aviation", "world", []string{"авиац", "aviation", "plane", "самолет", "airport", "аэропорт", "boeing", "airbus"}},
	{"maritime", "Море и суда", "Maritime", "world", []string{"мор", "maritime", "ship", "корабл", "port", "порт", "shipping"}},
	{"agriculture", "Сельское хозяйство", "Agriculture", "business", []string{"сельск", "agriculture", "farm", "фермер", "crop", "урожай"}},
	{"religion", "Религия", "Religion", "culture", []string{"религи", "religion", "church", "церков", "mosque", "мечет", "pope", "папа римск"}},
	{"royal", "Королевские семьи", "Royalty", "world", []string{"королевск", "royal", "king", "король", "queen", "королев", "prince", "принц"}},
}

var tagBySlug = func() map[string]TagDef {
	m := make(map[string]TagDef, len(TagCatalog))
	for _, t := range TagCatalog {
		m[t.Slug] = t
	}
	return m
}()

// Tags returns the catalog as model tags (for DB seeding).
func Tags() []model.Tag {
	out := make([]model.Tag, 0, len(TagCatalog))
	for _, t := range TagCatalog {
		out = append(out, model.Tag{ID: TagID(t.Slug), Slug: t.Slug, NameRU: t.NameRU, NameEN: t.NameEN})
	}
	return out
}

// ExtractTags finds topics in the text, ordered by catalog priority.
func ExtractTags(text string) []model.Tag {
	text = " " + strings.ToLower(strings.ReplaceAll(text, "ё", "е")) + " "
	var out []model.Tag
	seen := map[string]bool{}
	for _, def := range TagCatalog {
		if seen[def.Slug] {
			continue
		}
		for _, kw := range def.Keywords {
			kw = strings.ToLower(strings.ReplaceAll(kw, "ё", "е"))
			if strings.Contains(text, kw) {
				out = append(out, model.Tag{ID: TagID(def.Slug), Slug: def.Slug, NameRU: def.NameRU, NameEN: def.NameEN})
				seen[def.Slug] = true
				break
			}
		}
		if len(out) >= 8 {
			break
		}
	}
	return out
}

// DetectCategory returns the best category slug for the text. Every matching
// tag contributes one point to its category, so "NASA запустила ракету"
// becomes space (4 signals) instead of security (1 signal from "ракета").
func DetectCategory(text, fallback string) string {
	lower := " " + strings.ToLower(strings.ReplaceAll(text, "ё", "е")) + " "
	scores := map[string]int{}
	for _, def := range TagCatalog {
		if def.Category == "" {
			continue
		}
		for _, kw := range def.Keywords {
			kw = strings.ToLower(strings.ReplaceAll(kw, "ё", "е"))
			if strings.Contains(lower, kw) {
				scores[def.Category]++
			}
		}
	}
	best, bestScore := "", 0
	for _, c := range Categories {
		if scores[c.Slug] > bestScore {
			best = c.Slug
			bestScore = scores[c.Slug]
		}
	}
	if best != "" {
		return best
	}
	if fallback != "" {
		return fallback
	}
	return "world"
}

// CategoryExists validates a category slug.
func CategoryExists(slug string) bool {
	for _, c := range Categories {
		if c.Slug == slug {
			return true
		}
	}
	return false
}

// TagBySlug returns a catalog entry.
func TagBySlug(slug string) (TagDef, bool) {
	t, ok := tagBySlug[slug]
	return t, ok
}
