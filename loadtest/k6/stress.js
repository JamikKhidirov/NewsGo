// Stress test: ramps virtual users in steps to find the saturation point of a
// single instance. Watch p99, 5xx and the PostgreSQL pool in Grafana.
import { check, sleep } from 'k6';
import { firstArticleID, getArticle, getNews, getTrending, search } from './lib/common.js';

const MAX_VUS = Number(__ENV.STRESS_MAX_VUS || 400);

export const options = {
  scenarios: {
    ramp: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: '1m', target: Math.round(MAX_VUS * 0.1) },
        { duration: '2m', target: Math.round(MAX_VUS * 0.25) },
        { duration: '2m', target: Math.round(MAX_VUS * 0.5) },
        { duration: '2m', target: MAX_VUS },
        { duration: '2m', target: MAX_VUS },
        { duration: '1m', target: 0 },
      ],
      gracefulRampDown: '20s',
    },
  },
  thresholds: {
    // Stress is expected to degrade; fail only on catastrophic error rates.
    http_req_failed: ['rate<0.05'],
    checks: ['rate>0.95'],
  },
};

export function setup() {
  return { articleID: firstArticleID() };
}

export default function (data) {
  const roll = Math.random();
  if (roll < 0.75) {
    getNews(1 + Math.floor(Math.random() * 3), 20);
  } else if (roll < 0.9 && data.articleID) {
    getArticle(data.articleID);
  } else if (roll < 0.96) {
    getTrending();
  } else {
    search('наука');
  }
  sleep(Math.random() * 0.3);
}
