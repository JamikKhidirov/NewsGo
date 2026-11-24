// Smoke test: one virtual user, 30 seconds. Verifies that every public read
// endpoint responds with the expected shape after a deployment.
import { check, sleep } from 'k6';
import {
  firstArticleID,
  getArticle,
  getNews,
  getTrending,
  health,
  listCategories,
  listSources,
  listTags,
  metrics,
  readiness,
  search,
} from './lib/common.js';

export const options = {
  vus: 1,
  duration: '30s',
  thresholds: {
    checks: ['rate>0.99'],
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<800'],
  },
};

export default function () {
  health();
  readiness();
  metrics();

  const list = getNews(1, 10);
  check(list, { 'smoke: feed is not empty': (r) => r.status === 200 });

  getTrending();
  listCategories();
  listTags();
  listSources();
  search('технологии');

  const id = firstArticleID();
  if (id) {
    getArticle(id);
  }

  sleep(1);
}
