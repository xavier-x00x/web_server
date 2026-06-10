// GopherStack Enterprise — k6 Smoke Test
// Verifikasi basic functionality: 1-2 VUs, bbrp request aja
// Jalanin: k6 run tests/k6/smoke-test.js

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  vus: 1,
  iterations: 5,
  thresholds: {
    http_req_duration: ['p(95) < 500'], // 95% request di bawah 500ms
    http_req_failed: ['rate<0.01'],      // error rate < 1%
    errors: ['rate<0.01'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:80';

export default function () {
  // 1. Test homepage (static / index.php)
  const responses = http.batch([
    ['GET', `${BASE_URL}/`, null, { tags: { name: 'homepage' } }],
    ['GET', `${BASE_URL}/index.php`, null, { tags: { name: 'index_php' } }],
    ['GET', `${BASE_URL}/info.php`, null, { tags: { name: 'info_php' } }],
  ]);

  for (const res of responses) {
    const result = check(res, {
      'status is 200': (r) => r.status === 200,
      'response time < 500ms': (r) => r.timings.duration < 500,
    });
    if (!result) {
      errorRate.add(1);
    }
  }

  sleep(1);
}
