// GopherStack Enterprise — k6 Full Test Suite
// Comprehensive: smoke → load → stress → soak
// Output: HTML report + JSON summary
// Jalanin: k6 run tests/k6/full-suite.js --out json=results.json --out html=report.html

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

// ── Custom Metrics ──
const errorRate = new Rate('errors');
const phpDuration = new Trend('php_duration');
const staticDuration = new Trend('static_duration');
const totalReqs = new Counter('total_requests');
const byteRx = new Counter('bytes_received');

export const options = {
  stages: [
    // Smoke
    { duration: '30s', target: 2 },
    // Load test
    { duration: '1m', target: 20 },
    { duration: '2m', target: 50 },
    // Stress
    { duration: '1m', target: 100 },
    { duration: '1m', target: 200 },
    // Cool down
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_duration: [
      { threshold: 'p(95) < 5000', abortOnFail: false },
    ],
    http_req_failed: ['rate<0.10'],
    php_duration: ['p(95) < 4000'],
    errors: ['rate<0.10'],
  },
  // Summary
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'max', 'count'],
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:80';

export default function () {
  totalReqs.add(1);

  group('PHP Processing', function () {
    const res = http.get(`${BASE_URL}/index.php`, {
      tags: { name: 'php_processing' },
    });

    phpDuration.add(res.timings.duration);
    byteRx.add(res.body ? res.body.length : 0);

    const ok = check(res, {
      'php status 200': (r) => r.status === 200,
      'php duration < 8s': (r) => r.timings.duration < 8000,
    });

    if (!ok) errorRate.add(1);
  });

  group('Static Content', function () {
    const res = http.get(`${BASE_URL}/`, {
      tags: { name: 'static_content' },
    });

    staticDuration.add(res.timings.duration);

    check(res, {
      'static status 200': (r) => r.status === 200,
    });
  });

  group('Error Handling', function () {
    const res = http.get(`${BASE_URL}/missing-${__VU}.php`, {
      tags: { name: 'error_handling' },
    });

    check(res, {
      'missing page returns 404': (r) => r.status === 404,
    });
  });

  sleep(0.5);
}

// ── Custom Summary ──
export function handleSummary(data) {
  const summary = {
    meta: {
      test_name: 'GopherStack Enterprise — Full Suite',
      timestamp: new Date().toISOString(),
      duration: `${data.state.testRunDurationMs / 1000}s`,
    },
    summary: {
      total_requests: data.metrics.http_reqs.values.count,
      avg_rps: data.metrics.http_reqs.values.rate.toFixed(2),
      avg_latency_ms: data.metrics.http_req_duration.values.avg.toFixed(2),
      p95_latency_ms: data.metrics.http_req_duration.values['p(95)'].toFixed(2),
      error_rate: `${(data.metrics.http_req_failed.values.rate * 100).toFixed(2)}%`,
      bytes_received: data.metrics.data_received.values.count,
    },
    php: {
      avg_ms: data.metrics.php_duration?.values.avg?.toFixed(2) || 'N/A',
      p95_ms: data.metrics.php_duration?.values['p(95)']?.toFixed(2) || 'N/A',
    },
    thresholds: {},
  };

  // Threshold results
  for (const [name, checks] of Object.entries(data.metrics)) {
    if (checks.thresholds) {
      summary.thresholds[name] = {};
      for (const [t, result] of Object.entries(checks.thresholds)) {
        summary.thresholds[name][t] = result.ok ? '✅ PASS' : '❌ FAIL';
      }
    }
  }

  return {
    'stdout': JSON.stringify(summary, null, 2),
  };
}
