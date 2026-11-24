// Shared helpers for the GoNews k6 scenarios.
import http from 'k6/http';
import { check } from 'k6';
import { Trend, Rate } from 'k6/metrics';

export const BASE_URL = (__ENV.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
export const LANG = __ENV.LANG || 'ru';
export const ADMIN_TOKEN = __ENV.ADMIN_TOKEN || '';
export const TEST_EMAIL = __ENV.TEST_EMAIL || 'loadtest@example.com';
export const TEST_PASSWORD = __ENV.TEST_PASSWORD || 'password123';

export const newsListDuration = new Trend('news_list_duration', true);
export const articleDuration = new Trend('article_duration', true);
export const searchDuration = new Trend('search_duration', true);
export const writeDuration = new Trend('write_duration', true);
export const cacheHits = new Rate('cache_hits');

export const jsonHeaders = {
  'Content-Type': 'application/json',
  Accept: 'application/json',
};

export function authHeaders(token) {
  const h = { ...jsonHeaders };
  if (token) {
    h.Authorization = `Bearer ${token}`;
  }
  return h;
}

function recordCache(res) {
  const value = res.headers['X-Cache'] || res.headers['x-cache'];
  if (value) {
    cacheHits.add(value === 'HIT');
  }
}

export function getNews(page = 1, limit = 20, extra = '') {
  const url = `${BASE_URL}/api/v1/news?page=${page}&limit=${limit}&lang=${LANG}${extra}`;
  const res = http.get(url, { headers: { Accept: 'application/json' }, tags: { name: 'news_list' } });
  newsListDuration.add(res.timings.duration);
  recordCache(res);
  check(res, {
    'news: status 200': (r) => r.status === 200,
    'news: has data array': (r) => {
      try {
        return Array.isArray(r.json('data'));
      } catch (e) {
        return false;
      }
    },
    'news: has meta': (r) => {
      try {
        return r.json('meta') !== null;
      } catch (e) {
        return false;
      }
    },
  });
  return res;
}

export function getArticle(id) {
  const res = http.get(`${BASE_URL}/api/v1/news/${id}?lang=${LANG}`, {
    headers: { Accept: 'application/json' },
    tags: { name: 'article' },
  });
  articleDuration.add(res.timings.duration);
  recordCache(res);
  check(res, {
    'article: status 200 or 404': (r) => r.status === 200 || r.status === 404,
  });
  return res;
}

export function firstArticleID() {
  if (__ENV.ARTICLE_ID) {
    return __ENV.ARTICLE_ID;
  }
  const res = http.get(`${BASE_URL}/api/v1/news?limit=1&lang=${LANG}`, {
    headers: { Accept: 'application/json' },
    tags: { name: 'bootstrap' },
  });
  if (res.status !== 200) {
    return '';
  }
  try {
    const data = res.json('data');
    return Array.isArray(data) && data.length > 0 ? data[0].id : '';
  } catch (e) {
    return '';
  }
}

export function getTrending() {
  const res = http.get(`${BASE_URL}/api/v1/news/trending?lang=${LANG}`, {
    headers: { Accept: 'application/json' },
    tags: { name: 'trending' },
  });
  recordCache(res);
  check(res, { 'trending: status 200': (r) => r.status === 200 });
  return res;
}

export function search(term = 'россия') {
  const res = http.get(`${BASE_URL}/api/v1/search?q=${encodeURIComponent(term)}&lang=${LANG}`, {
    headers: { Accept: 'application/json' },
    tags: { name: 'search' },
  });
  searchDuration.add(res.timings.duration);
  recordCache(res);
  check(res, { 'search: status 200': (r) => r.status === 200 });
  return res;
}

export function listCategories() {
  const res = http.get(`${BASE_URL}/api/v1/categories`, {
    headers: { Accept: 'application/json' },
    tags: { name: 'categories' },
  });
  check(res, { 'categories: status 200': (r) => r.status === 200 });
  return res;
}

export function listTags() {
  const res = http.get(`${BASE_URL}/api/v1/tags?lang=${LANG}`, {
    headers: { Accept: 'application/json' },
    tags: { name: 'tags' },
  });
  check(res, { 'tags: status 200': (r) => r.status === 200 });
  return res;
}

export function listSources() {
  const res = http.get(`${BASE_URL}/api/v1/sources`, {
    headers: { Accept: 'application/json' },
    tags: { name: 'sources' },
  });
  check(res, { 'sources: status 200': (r) => r.status === 200 });
  return res;
}

export function login(email = TEST_EMAIL, password = TEST_PASSWORD) {
  const res = http.post(
    `${BASE_URL}/api/v1/auth/login`,
    JSON.stringify({ email, password }),
    { headers: jsonHeaders, tags: { name: 'login' } },
  );
  writeDuration.add(res.timings.duration);
  if (res.status !== 200) {
    return '';
  }
  try {
    return res.json('data.accessToken') || '';
  } catch (e) {
    return '';
  }
}

export function register(email = TEST_EMAIL, password = TEST_PASSWORD) {
  return http.post(
    `${BASE_URL}/api/v1/auth/register`,
    JSON.stringify({ email, password, displayName: 'Load Test' }),
    { headers: jsonHeaders, tags: { name: 'register' } },
  );
}

export function like(articleID, token) {
  const res = http.post(`${BASE_URL}/api/v1/news/${articleID}/like`, null, {
    headers: authHeaders(token),
    tags: { name: 'like' },
  });
  writeDuration.add(res.timings.duration);
  check(res, { 'like: status 2xx or 409': (r) => (r.status >= 200 && r.status < 300) || r.status === 409 });
  return res;
}

export function bookmark(articleID, token) {
  const res = http.post(`${BASE_URL}/api/v1/news/${articleID}/bookmark`, null, {
    headers: authHeaders(token),
    tags: { name: 'bookmark' },
  });
  writeDuration.add(res.timings.duration);
  check(res, { 'bookmark: status 2xx or 409': (r) => (r.status >= 200 && r.status < 300) || r.status === 409 });
  return res;
}

export function view(articleID, token) {
  const res = http.post(`${BASE_URL}/api/v1/news/${articleID}/view`, null, {
    headers: authHeaders(token),
    tags: { name: 'view' },
  });
  check(res, { 'view: status 2xx': (r) => r.status >= 200 && r.status < 300 });
  return res;
}

export function myFeed(token) {
  const res = http.get(`${BASE_URL}/api/v1/me/feed?lang=${LANG}`, {
    headers: authHeaders(token),
    tags: { name: 'my_feed' },
  });
  check(res, { 'feed: status 200': (r) => r.status === 200 });
  return res;
}

export function health() {
  const res = http.get(`${BASE_URL}/healthz`, { tags: { name: 'health' } });
  check(res, { 'health: status 200': (r) => r.status === 200 });
  return res;
}

export function readiness() {
  const res = http.get(`${BASE_URL}/readyz`, { tags: { name: 'readyz' } });
  check(res, { 'readyz: status 200': (r) => r.status === 200 });
  return res;
}

export function metrics() {
  const res = http.get(`${BASE_URL}/metrics`, { tags: { name: 'metrics' } });
  check(res, {
    'metrics: status 200': (r) => r.status === 200,
    'metrics: has request counter': (r) => r.body.includes('gonews_http_requests_total'),
  });
  return res;
}

export function randomItem(items) {
  if (!items || items.length === 0) {
    return null;
  }
  return items[Math.floor(Math.random() * items.length)];
}
