/**
 * GopherStack Enterprise — Dashboard App (Tailwind UI)
 * Real-time monitoring and worker management
 */

const API_BASE = '';
const SSE_URL = '/ws/metrics';

// ---- State ----
let metricsHistory = [];
let lastRequestCount = 0;
let lastRequestTime = Date.now();
let eventSource = null;
let metricsChart = null;

// ---- DOM References ----
const dom = {
    systemStatus: document.getElementById('system-status'),
    statusDot: document.querySelector('#system-status .status-dot'),
    statusText: document.querySelector('#system-status .status-text'),
    uptimeDisplay: document.getElementById('uptime-display'),
    activeWorkers: document.getElementById('metric-active-workers'),
    totalWorkers: document.getElementById('metric-total-workers'),
    totalRequests: document.getElementById('metric-total-requests'),
    rps: document.getElementById('metric-rps'),
    memory: document.getElementById('metric-memory'),
    nginxStatus: document.getElementById('metric-nginx-status'),
    nginxPid: document.getElementById('metric-nginx-pid'),
    nginxPort: document.getElementById('metric-nginx-port-display'),
    phpVersion: document.getElementById('php-version-display'),
    workersList: document.getElementById('workers-list'),

    chart: document.getElementById('metrics-chart'),
    opcacheToggle: document.getElementById('toggle-opcache'),
    memoryLimitInput: document.getElementById('input-memory-limit'),
    memoryLimitValue: document.getElementById('memory-limit-value'),
    btnApplyPHP: document.getElementById('btn-apply-php'),
};

// ---- Initialize ----
document.addEventListener('DOMContentLoaded', () => {
    initChart();
    fetchStatus();
    fetchWorkers();
    connectSSE();
    setupButtons();
    fetchPHPConfig();
    setInterval(fetchWorkers, 5000);
});

// ---- API Calls ----
async function fetchStatus() {
    try {
        const res = await fetch(`${API_BASE}/api/status`);
        const data = await res.json();
        updateStatus(data);
    } catch (err) {
        setOffline();
    }
}

async function fetchWorkers() {
    try {
        const res = await fetch(`${API_BASE}/api/workers`);
        const workers = await res.json();
        renderWorkers(workers);
    } catch (err) {
        console.error('Failed to fetch workers:', err);
    }
}

async function scaleWorkers(action, count = 1) {
    try {
        const res = await fetch(`${API_BASE}/api/workers/scale`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ action, count }),
        });
        const data = await res.json();
        if (data.success) {
            showToast(`Scaled ${action}: now ${data.active_workers} workers`, 'success');
            fetchWorkers();
        } else {
            showToast(`Scale failed`, 'error');
        }
    } catch (err) {
        showToast(`Error: ${err.message}`, 'error');
    }
}

async function updateNginxPort() {
    const currentPort = dom.nginxPort.textContent.replace('PORT: ', '');
    const newPortStr = prompt('Enter new Nginx Port (1-65535):', currentPort);
    
    if (newPortStr === null) return; // Cancelled
    
    const newPort = parseInt(newPortStr);
    if (isNaN(newPort) || newPort < 1 || newPort > 65535) {
        showToast('Invalid port number', 'error');
        return;
    }

    if (newPort === parseInt(currentPort)) return;

    try {
        const res = await fetch(`${API_BASE}/api/settings/nginx_port`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ port: newPort }),
        });
        
        const data = await res.json();
        if (data.success) {
            showToast(`Nginx port updated to ${newPort}. Nginx is restarting...`, 'success');
            // Data will update via SSE/next poll
        } else {
            showToast(`Failed to update port: ${data.message || 'Unknown error'}`, 'error');
        }
    } catch (err) {
        showToast(`Error: ${err.message}`, 'error');
    }
}

async function restartWorker(id) {
    try {
        const res = await fetch(`${API_BASE}/api/workers/restart?id=${id}`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
            showToast(`Worker ${id} restarted`, 'success');
            fetchWorkers();
        }
    } catch (err) {
        showToast(`Error restarting worker: ${err.message}`, 'error');
    }
}

async function fetchPHPConfig() {
    try {
        const res = await fetch(`${API_BASE}/api/config`);
        const data = await res.json();
        dom.opcacheToggle.checked = data.enable_opcache;
        dom.memoryLimitInput.value = data.max_memory_mb;
        dom.memoryLimitValue.textContent = `${data.max_memory_mb} MB`;
    } catch (err) {
        console.error('Failed to fetch PHP config:', err);
    }
}

async function updatePHPConfig() {
    const opcache = dom.opcacheToggle.checked;
    const memory = parseInt(dom.memoryLimitInput.value);

    dom.btnApplyPHP.disabled = true;
    dom.btnApplyPHP.innerHTML = `<svg class="animate-spin h-4 w-4 mr-2" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg> Applying...`;

    try {
        const res = await fetch(`${API_BASE}/api/settings/php_config`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ enable_opcache: opcache, max_memory_mb: memory }),
        });
        const data = await res.json();
        if (data.success) {
            showToast(`PHP Config updated! Workers are restarting...`, 'success');
            setTimeout(fetchWorkers, 2000);
        } else {
            showToast(`Failed to update PHP config`, 'error');
        }
    } catch (err) {
        showToast(`Error: ${err.message}`, 'error');
    } finally {
        dom.btnApplyPHP.disabled = false;
        dom.btnApplyPHP.innerHTML = `<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path></svg> Apply & Restart Workers`;
    }
}


// ---- SSE Connection ----
function connectSSE() {
    if (eventSource) eventSource.close();

    eventSource = new EventSource(SSE_URL);

    eventSource.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            updateMetrics(data);
        } catch (err) {
            console.error('SSE parse error:', err);
        }
    };

    eventSource.onopen = () => {
        setOnline('Connected');
    };

    eventSource.onerror = () => {
        setOffline();
        setTimeout(connectSSE, 5000);
    };
}

// ---- Status UI Mappings ----
function setOffline() {
    if (!dom.systemStatus) return;
    dom.systemStatus.className = 'flex items-center gap-2 px-3 py-1.5 rounded-full bg-rose-950/40 border border-rose-900/50 text-xs font-medium transition-colors duration-300';
    dom.statusDot.className = 'status-dot w-2 h-2 rounded-full bg-rose-500 shadow-[0_0_8px_rgba(244,63,94,0.5)]';
    dom.statusText.textContent = 'Offline';
    dom.statusText.className = 'status-text text-rose-300';
}

function setOnline(text = 'Running') {
    if (!dom.systemStatus) return;
    dom.systemStatus.className = 'flex items-center gap-2 px-3 py-1.5 rounded-full bg-emerald-950/40 border border-emerald-900/50 text-xs font-medium transition-colors duration-300';
    dom.statusDot.className = 'status-dot w-2 h-2 rounded-full bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.5)]';
    dom.statusText.textContent = text;
    dom.statusText.className = 'status-text text-emerald-300';
}

// ---- Update UI ----
function updateStatus(data) {
    setOnline(data.status === 'running' ? 'Running' : data.status);
    dom.activeWorkers.textContent = data.active_workers;
    dom.totalWorkers.textContent = `/ ${data.total_workers} total`;
    dom.totalRequests.textContent = formatNumber(data.total_requests);
    dom.nginxStatus.textContent = data.nginx_running ? 'Running' : 'Stopped';
    dom.nginxStatus.className = data.nginx_running ? 'text-xl font-bold text-emerald-400 tracking-tight mt-1' : 'text-xl font-bold text-rose-400 tracking-tight mt-1';
    dom.nginxPid.textContent = data.nginx_running ? `PID: ${data.nginx_pid}` : 'Not running';
    dom.nginxPort.textContent = `PORT: ${data.nginx_port || '-'}`;
    dom.uptimeDisplay.textContent = data.uptime;
    if (data.php_version) {
        dom.phpVersion.textContent = data.php_version;
    }
}


function updateMetrics(data) {
    // Update cards
    dom.activeWorkers.textContent = data.active_workers;
    dom.totalWorkers.textContent = `/ ${data.total_workers} total`;
    dom.totalRequests.textContent = formatNumber(data.total_requests);
    dom.memory.textContent = data.mem_alloc_mb.toFixed(1);
    
    dom.nginxStatus.textContent = data.nginx_running ? 'Running' : 'Stopped';
    dom.nginxStatus.className = data.nginx_running ? 'text-xl font-bold text-emerald-400 tracking-tight mt-1' : 'text-xl font-bold text-rose-400 tracking-tight mt-1';
    dom.nginxPid.textContent = data.nginx_running ? `PID: ${data.nginx_pid}` : 'Not running';
    dom.nginxPort.textContent = `PORT: ${data.nginx_port || '-'}`;
    
    dom.uptimeDisplay.textContent = formatUptime(data.uptime_seconds);

    // Calculate requests per second
    const now = Date.now();
    const elapsed = (now - lastRequestTime) / 1000;
    let currentRps = 0;
    
    // Only calculate RPS if this isn't the very first data point
    if (elapsed > 0 && lastRequestCount > 0) {
        currentRps = Math.max(0, (data.total_requests - lastRequestCount) / elapsed);
    }
    
    dom.rps.textContent = `${currentRps.toFixed(1)} req/s`;
    
    lastRequestCount = data.total_requests;
    lastRequestTime = now;

    setOnline();

    // Add to chart history
    metricsHistory.push({
        time: new Date(),
        requests: currentRps, // We now plot RPS instead of cumulative total
        workers: data.active_workers,
        memory: data.mem_alloc_mb,
    });
    if (metricsHistory.length > 60) metricsHistory.shift();

    updateChart();
}

// ---- Render Workers ----
function renderWorkers(workers) {
    if (!workers || workers.length === 0) {
        dom.workersList.innerHTML = `<div class="absolute inset-0 flex items-center justify-center text-sm text-slate-500">No workers found</div>`;
        return;
    }

    dom.workersList.innerHTML = workers.map(w => {
        let statusColor = w.status === 'running' ? 'bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.5)]' : 
                          w.status === 'dead' ? 'bg-rose-500 shadow-[0_0_8px_rgba(244,63,94,0.5)]' : 
                          'bg-slate-500';
                          
        return `
            <div class="glass-card p-4 flex flex-col justify-between group">
                <div class="flex items-start justify-between mb-3">
                    <div class="flex items-center gap-2">
                        <div class="w-2 h-2 rounded-full ${statusColor} ${w.status === 'running' ? 'animate-pulse' : ''}"></div>
                        <span class="text-xs font-bold text-slate-200">Worker #${w.id}</span>
                    </div>
                    <span class="text-[0.6rem] font-mono text-slate-500 bg-slate-900/50 px-1.5 py-0.5 rounded border border-white/5">:${w.port}</span>
                </div>
                
                <div class="space-y-1">
                    <div class="flex justify-between items-baseline">
                        <span class="text-[0.65rem] text-slate-500 uppercase font-bold tracking-tighter">Requests</span>
                        <span class="text-sm font-black text-indigo-400 tracking-tight">${formatNumber(w.request_count)}</span>
                    </div>
                    <div class="flex justify-between items-baseline">
                        <span class="text-[0.65rem] text-slate-500 uppercase font-bold tracking-tighter">PID</span>
                        <span class="text-[0.65rem] font-mono text-slate-300">${w.pid}</span>
                    </div>
                </div>

                <div class="mt-4 pt-3 border-t border-white/5 flex justify-between items-center">
                    <span class="text-[0.6rem] text-slate-500 font-medium">${w.uptime || '0s'}</span>
                    <button onclick="restartWorker(${w.id})" class="p-1.5 rounded-lg bg-slate-800 text-slate-400 hover:bg-indigo-500 hover:text-white transition-all opacity-0 group-hover:opacity-100 focus:opacity-100 shadow-sm" title="Restart">
                        <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path></svg>
                    </button>
                </div>
            </div>
        `;
    }).join('');
}

// ---- Chart Drawing ----
function initChart() {
    const canvas = dom.chart;
    if (!canvas) return;
    
    // Set default font
    Chart.defaults.font.family = 'Inter, sans-serif';
    Chart.defaults.color = '#94a3b8'; // text-slate-400

    metricsChart = new Chart(canvas, {
        type: 'line',
        data: {
            labels: [],
            datasets: [
                {
                    label: 'Req/s',
                    data: [],
                    borderColor: '#818cf8', // indigo-400
                    backgroundColor: 'rgba(129, 140, 248, 0.1)',
                    borderWidth: 2,
                    tension: 0.4,
                    fill: true,
                    yAxisID: 'y'
                },
                {
                    label: 'Workers',
                    data: [],
                    borderColor: '#34d399', // emerald-400
                    backgroundColor: 'rgba(52, 211, 153, 0.05)',
                    borderWidth: 2,
                    tension: 0.4,
                    fill: true,
                    yAxisID: 'y1'
                },
                {
                    label: 'Memory (MB)',
                    data: [],
                    borderColor: '#fb923c', // orange-400
                    backgroundColor: 'rgba(251, 146, 60, 0.05)',
                    borderWidth: 2,
                    tension: 0.4,
                    fill: true,
                    yAxisID: 'y2'
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: {
                mode: 'index',
                intersect: false,
            },
            plugins: {
                legend: { display: false },
                tooltip: {
                    backgroundColor: 'rgba(15, 23, 42, 0.9)', // slate-900
                    titleColor: '#f8fafc',
                    bodyColor: '#cbd5e1',
                    borderColor: 'rgba(255,255,255,0.1)',
                    borderWidth: 1,
                    padding: 10,
                }
            },
            scales: {
                x: {
                    grid: { color: 'rgba(255, 255, 255, 0.05)' },
                    ticks: { maxTicksLimit: 8 }
                },
                y: {
                    type: 'linear',
                    display: true,
                    position: 'left',
                    title: { display: true, text: 'Req/s', color: '#818cf8' },
                    grid: { color: 'rgba(255, 255, 255, 0.05)' },
                    beginAtZero: true
                },
                y1: {
                    type: 'linear',
                    display: true,
                    position: 'right',
                    title: { display: true, text: 'Workers', color: '#34d399' },
                    grid: { drawOnChartArea: false },
                    beginAtZero: true,
                    suggestedMax: 10
                },
                y2: {
                    type: 'linear',
                    display: true,
                    position: 'right',
                    title: { display: true, text: 'Memory', color: '#fb923c' },
                    grid: { drawOnChartArea: false },
                    beginAtZero: true
                }
            }
        }
    });
}

function updateChart() {
    if (!metricsChart || metricsHistory.length === 0) return;
    
    // Extract data
    const labels = metricsHistory.map(d => {
        const t = d.time;
        return `${t.getHours().toString().padStart(2, '0')}:${t.getMinutes().toString().padStart(2, '0')}:${t.getSeconds().toString().padStart(2, '0')}`;
    });
    
    const reqs = metricsHistory.map(d => d.requests);
    const workers = metricsHistory.map(d => d.workers);
    const mem = metricsHistory.map(d => d.memory);
    
    metricsChart.data.labels = labels;
    metricsChart.data.datasets[0].data = reqs;
    metricsChart.data.datasets[1].data = workers;
    metricsChart.data.datasets[2].data = mem;
    
    metricsChart.update('none'); // Update without animation for smoother live data
}


async function reloadConfig() {
    const btn = document.getElementById('btn-reload-config');
    const originalHTML = btn.innerHTML;
    
    btn.disabled = true;
    btn.innerHTML = `<svg class="animate-spin h-4 w-4 mr-2" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg> Reloading...`;
    
    try {
        const res = await fetch(`${API_BASE}/api/reload`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
            showToast('Nginx configuration reloaded!', 'success');
        } else {
            showToast(`Reload failed: ${data.errors ? data.errors.join(', ') : 'Unknown error'}`, 'error');
        }
    } catch (err) {
        showToast(`Error: ${err.message}`, 'error');
    } finally {
        btn.disabled = false;
        btn.innerHTML = originalHTML;
    }
}

let isServerRunning = true;

async function toggleServerState() {
    const btn = document.getElementById('btn-shutdown');
    const isStopping = isServerRunning;
    
    if (isStopping) {
        if (!confirm('Are you sure you want to stop PHP and Nginx workers?')) return;
    }
    
    const originalHTML = btn.innerHTML;
    btn.disabled = true;
    btn.innerHTML = `<svg class="animate-spin h-4 w-4 mr-2" inline-block viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg> ${isStopping ? 'Stopping...' : 'Starting...'}`;
    
    try {
        const endpoint = isStopping ? '/api/shutdown' : '/api/start';
        const res = await fetch(`${API_BASE}${endpoint}`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
            showToast(`Server is ${isStopping ? 'stopping' : 'starting'}...`, 'success');
            setTimeout(() => {
                isServerRunning = !isStopping;
                btn.disabled = false;
                
                if (isServerRunning) {
                    btn.className = 'btn px-5 bg-rose-600 hover:bg-rose-500 text-white shadow-rose-500/25';
                    btn.innerHTML = `<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 10a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z"></path></svg>\n                Stop Server`;
                    fetchStatus();
                } else {
                    btn.className = 'btn px-5 bg-emerald-600 hover:bg-emerald-500 text-white shadow-emerald-500/25';
                    btn.innerHTML = `<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z"></path><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>\n                Start Server`;
                    setOffline();
                }
            }, 1000);
        } else {
            showToast(`${isStopping ? 'Shutdown' : 'Start'} failed`, 'error');
            btn.disabled = false;
            btn.innerHTML = originalHTML;
        }
    } catch (err) {
        showToast(`Error: ${err.message}`, 'error');
        btn.disabled = false;
        btn.innerHTML = originalHTML;
    }
}

// ---- Button Setup ----
function setupButtons() {
    document.getElementById('btn-scale-up').addEventListener('click', () => scaleWorkers('up'));
    document.getElementById('btn-scale-down').addEventListener('click', () => scaleWorkers('down'));
    document.getElementById('btn-reload-config').addEventListener('click', reloadConfig);
    document.getElementById('btn-edit-port').addEventListener('click', updateNginxPort);
    dom.btnApplyPHP.addEventListener('click', updatePHPConfig);
    dom.memoryLimitInput.addEventListener('input', (e) => {
        dom.memoryLimitValue.textContent = `${e.target.value} MB`;
    });
    document.getElementById('btn-refresh').addEventListener('click', () => {
        fetchStatus();
        fetchWorkers();
        fetchPHPConfig();
        showToast('Dashboard refreshed', 'success');
    });
    const btnShutdown = document.getElementById('btn-shutdown');
    if (btnShutdown) {
        btnShutdown.addEventListener('click', toggleServerState);
    }
}

// ---- Toast Notifications ----
function showToast(message, type = 'info') {
    const container = document.getElementById('toast-container');
    const toast = document.createElement('div');
    
    let colors = '';
    let icon = '';
    
    if (type === 'success') {
        colors = 'bg-emerald-950/80 border-emerald-900/50 text-emerald-200';
        icon = '<svg class="w-4 h-4 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path></svg>';
    } else if (type === 'error') {
        colors = 'bg-rose-950/80 border-rose-900/50 text-rose-200';
        icon = '<svg class="w-4 h-4 text-rose-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path></svg>';
    } else {
        colors = 'bg-indigo-950/80 border-indigo-900/50 text-indigo-200';
        icon = '<svg class="w-4 h-4 text-indigo-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>';
    }
    
    toast.className = `flex items-center gap-2 px-4 py-3 rounded-xl border backdrop-blur-md shadow-xl text-sm font-medium toast-animate-in pointer-events-auto ${colors}`;
    toast.innerHTML = `${icon} <span>${message}</span>`;
    
    container.appendChild(toast);

    setTimeout(() => {
        toast.classList.replace('toast-animate-in', 'toast-animate-out');
        setTimeout(() => toast.remove(), 300);
    }, 3000);
}

// ---- Utility ----
function formatNumber(n) {
    if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
    if (n >= 1000) return (n / 1000).toFixed(1) + 'K';
    return n.toString();
}

function formatUptime(seconds) {
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const s = Math.floor(seconds % 60);
    return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
}

// Handle window resize for chart
window.addEventListener('resize', () => {
    if (metricsChart) metricsChart.resize();
});
