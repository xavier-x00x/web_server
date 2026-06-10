import http from 'k6/http';
import { check, sleep } from 'k6';

// Konfigurasi Stress Test K6
export const options = {
    // Menjalankan skenario secara bertahap (ramp up & ramp down)
    stages: [
        { duration: '10s', target: 100 }, // Fase 1: Naik ke 100 VU
        { duration: '20s', target: 500 }, // Fase 2: Ramp up ke 500 VU (Load testing)
        { duration: '25s', target: 500 }, // Fase 3: Tahan di 500 VU (Stress testing)
        { duration: '10s', target: 0 },   // Fase 4: Turun bertahap ke 0 (Cooldown)
    ],
    // Target yang diharapkan dari performa server
    thresholds: {
        http_req_duration: ['p(95)<500'], // 95% dari seluruh request harus selesai di bawah 500ms
        http_req_failed: ['rate<0.01'],   // Tingkat kegagalan (error) harus di bawah 1%
    },
};

const TARGET_URL = 'http://localhost:8088/index.php';

export default function () {
    // 1. Melakukan HTTP GET request ke target
    const res = http.get(TARGET_URL);

    // 2. Verifikasi respons
    check(res, {
        'status is 200 (OK)': (r) => r.status === 200,
        'response contains GopherStack text': (r) => r.body && r.body.includes('GopherStack'),
    });

    // 3. Jeda sangat singkat antar request (10ms) untuk mensimulasikan serbuan trafik yang intensif namun tidak memblokir thread K6
    sleep(0.01);
}
