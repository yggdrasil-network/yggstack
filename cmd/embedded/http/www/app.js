const FETCH_TIMEOUT_MS = 3000;
const REFRESH_MS = 10000;
const HISTORY_LEN = 20;

// // // // // // // // // //

const bwHistory = {rx: [], tx: []};
let chartCanvas = null;

// //

function formatBytes(b) {
    if (b < 1024) return b + ' B';
    if (b < 1048576) return (b / 1024).toFixed(1) + ' KB';
    if (b < 1073741824) return (b / 1048576).toFixed(1) + ' MB';
    return (b / 1073741824).toFixed(2) + ' GB';
}

function formatRate(bps) {
    return formatBytes(bps) + '/s';
}

function formatUptime(s) {
    s = Math.floor(s);
    const d = Math.floor(s / 86400);
    const h = Math.floor((s % 86400) / 3600);
    const m = Math.floor((s % 3600) / 60);
    const sc = s % 60;
    if (d > 0) return `${d}d ${h}h ${m}m`;
    if (h > 0) return `${h}h ${m}m ${sc}s`;
    if (m > 0) return `${m}m ${sc}s`;
    return `${sc}s`;
}

// //

function pushHistory(rx, tx) {
    bwHistory.rx.push(rx);
    bwHistory.tx.push(tx);
    while (bwHistory.rx.length > HISTORY_LEN) {
        bwHistory.rx.shift();
        bwHistory.tx.shift();
    }
    while (bwHistory.rx.length < HISTORY_LEN) {
        bwHistory.rx.unshift(0);
        bwHistory.tx.unshift(0);
    }
}

function drawChart() {
    if (!chartCanvas) chartCanvas = document.getElementById('bw-chart');
    const ctx = chartCanvas.getContext('2d');
    chartCanvas.width = chartCanvas.offsetWidth || 600;
    const w = chartCanvas.width;
    const h = 80;
    ctx.clearRect(0, 0, w, h);

    const max = Math.max(...bwHistory.rx, ...bwHistory.tx, 1);
    const step = w / (HISTORY_LEN - 1);

    function drawLine(data, color) {
        ctx.beginPath();
        ctx.strokeStyle = color;
        ctx.lineWidth = 2;
        ctx.lineJoin = 'round';
        data.forEach((v, i) => {
            const x = i * step;
            const y = h - (v / max) * (h - 6) - 3;
            i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
        });
        ctx.stroke();
    }

    drawLine(bwHistory.rx, '#6366f1'); // RX — indigo
    drawLine(bwHistory.tx, '#22c55e'); // TX — green
}

// //

function setPeerList(peers) {
    const el = document.getElementById('peers-list');
    if (!peers || peers.length === 0) {
        el.innerHTML = '<p style="color:var(--muted);font-size:0.82rem">No peers connected</p>';
        return;
    }
    const rows = peers.map(p => {
        const errCell = (!p.up && p.last_error)
            ? `<span class="peer-error" title="${p.last_error}">${p.last_error}</span>`
            : '—';
        return `
    <tr>
      <td><span class="dot ${p.up ? 'up' : 'down'}"></span>${p.up ? 'Up' : 'Down'}</td>
      <td class="uri-cell" title="${p.uri}">${p.uri}</td>
      <td>${p.up ? p.latency_ms.toFixed(1) + ' ms' : errCell}</td>
      <td>${formatRate(p.rx_rate)}</td>
      <td>${formatRate(p.tx_rate)}</td>
    </tr>`;
    }).join('');
    el.innerHTML = `
    <table class="peers-table">
      <thead>
        <tr>
          <th>Status</th><th>URI</th><th>Latency / Error</th><th>↓ RX</th><th>↑ TX</th>
        </tr>
      </thead>
      <tbody>${rows}</tbody>
    </table>`;
}

// //

function updateUI(data) {
    // Connection badge.
    const badge = document.getElementById('connection-badge');
    if (data.is_yggdrasil) {
        badge.textContent = 'Connected via Yggdrasil';
        badge.className = 'badge ygg';
    } else {
        badge.textContent = 'Connected via regular web';
        badge.className = 'badge web';
    }

    // Node card.
    const addrEl = document.getElementById('ygg-addr');
    addrEl.textContent = data.ygg_address;

    const keyEl = document.getElementById('pub-key');
    keyEl.textContent = data.public_key;
    keyEl.title = data.public_key;

    document.getElementById('uptime').textContent = formatUptime(data.uptime_seconds);
    document.getElementById('sessions').textContent = data.sessions;

    // Bandwidth.
    const bw = data.bandwidth || {};
    document.getElementById('rx-rate').textContent = formatRate(bw.rx_rate || 0);
    document.getElementById('tx-rate').textContent = formatRate(bw.tx_rate || 0);
    document.getElementById('rx-total').textContent = formatBytes(bw.rx_bytes || 0);
    document.getElementById('tx-total').textContent = formatBytes(bw.tx_bytes || 0);

    pushHistory(bw.rx_rate || 0, bw.tx_rate || 0);
    drawChart();

    // Peers.
    setPeerList(data.peers);

    // Updated at — when the cached metrics snapshot was taken.
    const ts = new Date(data.cached_at);
    document.getElementById('updated-at').textContent =
        `— data from ${ts.toLocaleTimeString()}`;

    // Show content.
    document.getElementById('main-content').classList.remove('hidden');
    document.getElementById('status-unavailable').classList.add('hidden');
}

function showUnavailable() {
    const badge = document.getElementById('connection-badge');
    badge.textContent = 'Server unavailable';
    badge.className = 'badge loading';
    document.getElementById('status-unavailable').classList.remove('hidden');
    document.getElementById('main-content').classList.add('hidden');
}

// //

async function fetchInfo() {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);
    try {
        const resp = await fetch('/yggdrasil-server.json', {signal: controller.signal});
        clearTimeout(timer);
        if (!resp.ok) throw new Error('HTTP ' + resp.status);
        const data = await resp.json();
        updateUI(data);
    } catch (_) {
        clearTimeout(timer);
        showUnavailable();
    }
}

fetchInfo();
setInterval(fetchInfo, REFRESH_MS);
