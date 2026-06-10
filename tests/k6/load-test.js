// GopherStack Enterprise — k6 Load Test
// Simulasi traffic realistis dengan ramp-up bertahap
// Jalanin: k6 run tests/k6/load-test.js

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('errors');
const phpLatency = new Trend('php_latency');
const staticLatency = new Trend('static_latency');

export const options = {
  stages: [
    { duration: '1m', target: 10 },   // Ramp up ke 10 users
    { duration: '2m', target: 25 },   // Naik ke 25 users
    { duration: '2m', target: 50 },   // Naik ke 50 users
    { duration: '1m', target: 0 },    // Turun
  ],
  thresholds: {
    http_req_duration: ['p(95) < 2000'],    // 95% di bawah 2s
    http_req_failed: ['rate<0.05'],          // Max 5% error
    php_latency: ['p(95) < 3000'],           // PHP request p95 < 3s
    static_latency: ['p(95) < 500'],         // Static file p95 < 500ms
    errors: ['rate<0.05'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:80';
const SLEEP_DURATION = 1;

export default function () {
  group('Static Files', function () {
    const res = http.get(`${BASE_URL}/`, {
      tags: { name: 'static_home' },
    });
    staticLatency.add(res.timings.duration);
    check(res, {
      'homepage status 200': (r) => r.status === 200,
    });
    sleep(SLEEP_DURATION);
  });

  group('PHP Pages', function () {
    const endpoints = ['/index.php', '/info.php', '/test.php'];
    for (const ep of endpoints) {
      const res = http.get(`${BASE_URL}${ep}`, {
        tags: { name: `php_${ep.replace(/[^a-z]/g, '_')}` },
      });
      phpLatency.add(res.timings.duration);
      const ok = check(res, {
        'php status is 200': (r) => r.status === 200,
        'php response time < 5s': (r) => r.timings.duration < 5000,
      });
      if (!ok) errorRate.add(1);
      sleep(SLEEP_DURATION);
    }
  });

  group('404 Handling', function () {
    const res = http.get(`${BASE_URL}/nonexistent-${__VU}-${__ITER}.php`, {
      tags: { name: '404_page' },
    });
    check(res, {
      '404 returns 404': (r) => r.status === 404,
    });
    sleep(SLEEP_DURATION);
  });
}
