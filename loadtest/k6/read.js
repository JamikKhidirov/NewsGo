// Read-heavy profile: mirrors a news app feed. 70% list, 20% article,
// 10% search/trending. Use this to measure cache and database read capacity.
import { check, sleep } from 'k6';
import { firstArticleID, getArticle, getNews, getTrending, search } from './lib/common.js';

const VUS = Number(__ENV.READ_VUS || 50);
const DURATION = __ENV.READ_DURATION || '3m';

export const options = {
  scenarios: {
    readers: {
      executor: 'constant-vus',
      vus: VUS,
      duration: DURATION,
      gracefulStop: '10s',
    },
  },
  thresholds: {
    checks: ['rate>0.99'],
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<300', 'p(99)<800'],
    news_list_duration: ['p(95)<150'],
    article_duration: ['p(95)<120'],
    search_duration: ['p(95)<500'],
  },
};

export function setup() {
  return { articleID: firstArticleID() };
}

export default function (data) {
  const roll = Math.random();

  if (roll < 0.7) {
    getNews(1 + Math.floor(Math.random() * 5), 20);
  } else if (roll < 0.9 && data.articleID) {
    getArticle(data.articleID);
  } else if (roll < 0.95) {
    getTrending();
  } else {
    search('новости');
  }

  sleep(Math.random() * 0.5);
}

export function teardown(data) {
  check(data, { 'setup: article id resolved': (d) => !!d.articleID });
}
