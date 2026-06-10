// GopherStack Enterprise — k6 Dashboard API Test
// Test semua endpoint REST API dashboard
// Jalanin: k6 run tests/k6/dashboard-test.js

import http from 'k6/http';
import { check, sleep, group } from 'k6';

export const options = {
  vus: 1,
  duration: '20s',
  thresholds: {
    http_req_duration: ['p(95) < 1000'],
    http_req_failed: ['rate<0.01'],
  },
};

const DASHBOARD_URL = __ENV.DASHBOARD_URL || 'http://localhost:8090';

export default function () {
  // Dashboard has SSE endpoint that streams — we don't test that with k6

  group('Dashboard API', function () {
    // Status endpoint
    const statusRes = http.get(`${DASHBOARD_URL}/api/status`, {
      tags: { name: 'api_status' },
    });
    check(statusRes, {
      'api/status returns 200': (r) => r.status === 200,
      'api/status has json body': (r) => r.headers['Content-Type']
        && r.headers['Content-Type'].includes('json'),
    });

    // Workers endpoint
    const workersRes = http.get(`${DASHBOARD_URL}/api/workers`, {
      tags: { name: 'api_workers' },
    });
    check(workersRes, {
      'api/workers returns 200': (r) => r.status === 200,
    });

    // Metrics endpoint
    const metricsRes = http.get(`${DASHBOARD_URL}/api/metrics`, {
      tags: { name: 'api_metrics' },
    });
    check(metricsRes, {
      'api/metrics returns 200': (r) => r.status === 200,
    });

    sleep(0.5);
  });

  group('Dashboard Static Assets', function () {
    const assets = ['/', '/index.html', '/static/style.css', '/static/app.js'];
    for (const asset of assets) {
      const res = http.get(`${DASHBOARD_URL}${asset}`, {
        tags: { name: `static_${asset.replace(/[^a-z]/g, '_')}` },
      });
      check(res, {
        [`static asset ${asset} returns 200-304`]: (r) =>
          r.status === 200 || r.status === 304,
      });
      sleep(0.3);
    }
  });
}
