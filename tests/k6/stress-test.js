// GopherStack Enterprise — k6 Stress Test
// Cari breaking point: terus naikin sampai server collapse
// Jalanin: k6 run tests/k6/stress-test.js

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  stages: [
    { duration: '2m', target: 10 },    // Normal
    { duration: '2m', target: 50 },    // Lumayan berat
    { duration: '2m', target: 100 },   // Berat
    { duration: '2m', target: 200 },   // Sangat berat
    { duration: '2m', target: 500 },   // Extreme — cari breaking point
    { duration: '1m', target: 0 },     // Turun
  ],
  thresholds: {
    http_req_duration: ['p(90) < 5000'],   // 90% di bawah 5s
    http_req_failed: ['rate<0.10'],         // Max 10% error
    errors: ['rate<0.10'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:80';

export default function () {
  const res = http.get(`${BASE_URL}/index.php`, {
    tags: { name: 'php_index' },
  });

  const ok = check(res, {
    'status 200': (r) => r.status === 200,
    'response time < 10s': (r) => r.timings.duration < 10000,
  });

  if (!ok) errorRate.add(1);

  // Report current stage info
  if (__ITER % 50 === 0) {
    console.log(
      `[VU ${__VU}] Iter ${__ITER} — ` +
      `Status: ${res.status}, Duration: ${res.timings.duration}ms`
    );
  }

  sleep(0.5); // 0.5s delay between requests (high concurrency)
}
