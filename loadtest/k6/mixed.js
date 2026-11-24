// Mixed profile: realistic traffic including authentication and writes.
// Requires a user (registered automatically on first run) and, for admin
// requests, ADMIN_TOKEN.
import { check, sleep } from 'k6';
import {
  ADMIN_TOKEN,
  TEST_EMAIL,
  TEST_PASSWORD,
  firstArticleID,
  getArticle,
  getNews,
  getTrending,
  like,
  listCategories,
  listTags,
  login,
  myFeed,
  register,
  search,
  view,
} from './lib/common.js';

const VUS = Number(__ENV.MIXED_VUS || 30);
const DURATION = __ENV.MIXED_DURATION || '3m';

export const options = {
  scenarios: {
    mixed: {
      executor: 'constant-vus',
      vus: VUS,
      duration: DURATION,
      gracefulStop: '10s',
    },
  },
  thresholds: {
    checks: ['rate>0.98'],
    http_req_failed: ['rate<0.02'],
    http_req_duration: ['p(95)<400', 'p(99)<1000'],
    news_list_duration: ['p(95)<200'],
    write_duration: ['p(95)<300'],
  },
};

export function setup() {
  register(TEST_EMAIL, TEST_PASSWORD);
  const token = login(TEST_EMAIL, TEST_PASSWORD);
  return { token, articleID: firstArticleID(), adminToken: ADMIN_TOKEN };
}

export default function (data) {
  const roll = Math.random();

  if (roll < 0.55) {
    getNews(1 + Math.floor(Math.random() * 5), 20);
  } else if (roll < 0.7 && data.articleID) {
    getArticle(data.articleID);
  } else if (roll < 0.78) {
    getTrending();
  } else if (roll < 0.85) {
    search('экономика');
  } else if (roll < 0.9) {
    listCategories();
  } else if (roll < 0.94) {
    listTags();
  } else if (data.token && data.articleID) {
    check(data.token, { 'setup: token present': (t) => t.length > 0 });
    view(data.articleID, data.token);
    if (Math.random() < 0.3) {
      like(data.articleID, data.token);
    }
    if (Math.random() < 0.2) {
      myFeed(data.token);
    }
  }

  sleep(Math.random() * 0.8);
}
