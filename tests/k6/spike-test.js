// GopherStack Enterprise — k6 Spike Test
// Simulasi traffic spike mendadak (viral / flash sale)
// Jalanin: k6 run tests/k6/spike-test.js

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  stages: [
    { duration: '2m', target: 10 },    // Normal — 10 users
    { duration: '10s', target: 300 },  // SPIKE! langsung 300 users
    { duration: '1m', target: 300 },   // Bertahan di puncak
    { duration: '30s', target: 50 },   // Sedikit turun
    { duration: '1m', target: 10 },    // Kembali normal
  ],
  thresholds: {
    http_req_duration: [
      { threshold: 'p(90) < 3000', abortOnFail: true },
    ],
    http_req_failed: ['rate<0.15'],
    errors: ['rate<0.15'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:80';

export default function () {
  // Mix of requests
  const res = http.get(`${BASE_URL}/index.php`, {
    tags: { name: 'spike_php' },
  });

  const ok = check(res, {
    'status 200': (r) => r.status === 200,
  });

  if (!ok) errorRate.add(1);

  // Minimal delay to maximize concurrency during spike
  sleep(0.3);
}
