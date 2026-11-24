// Spike test: sudden burst from 10 to 500 users in 20 seconds, then recovery.
// Validates queueing behaviour, timeouts and whether the service returns to
// normal latency after the surge.
import { check, sleep } from 'k6';
import { firstArticleID, getArticle, getNews } from './lib/common.js';

export const options = {
  scenarios: {
    spike: {
      executor: 'ramping-vus',
      startVUs: 10,
      stages: [
        { duration: '20s', target: 500 },
        { duration: '1m', target: 500 },
        { duration: '20s', target: 10 },
        { duration: '1m', target: 10 },
        { duration: '10s', target: 0 },
      ],
      gracefulRampDown: '15s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.03'],
    checks: ['rate>0.97'],
  },
};

export function setup() {
  return { articleID: firstArticleID() };
}

export default function (data) {
  if (Math.random() < 0.85 || !data.articleID) {
    getNews(1, 20);
  } else {
    getArticle(data.articleID);
  }
  check(data, { 'setup: article id resolved': (d) => !!d.articleID });
  sleep(Math.random() * 0.2);
}
