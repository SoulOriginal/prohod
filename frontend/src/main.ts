import './style.css';
import {Connect, Disconnect, Logs, OpenLogs, SaveSettings, Settings, Stats, Status, Version} from '../wailsjs/go/main/App';

type Check = { name: string; ok: boolean; done: boolean; ms: number };
type St = { state: string; since: number; message: string; pinned: number; checks: Check[] | null };
type Svc = { name: string; active: number; ok: number; failed: number; bytesIn: number; bytesOut: number; lastOk: number; lastFail: number };
type Snap = { bytesIn: number; bytesOut: number; speedIn: number; speedOut: number; connections: number; total: number; failed: number; services: Svc[] | null };
type Ent = { time: string; level: string; msg: string };
type Cfg = { ipv4Only: boolean; autoConnect: boolean };

const icons = {
  power: '<svg viewBox="0 0 24 24" fill="none" stroke-width="2" stroke-linecap="round"><path d="M12 3v9"/><path d="M6.3 6.3a8 8 0 1 0 11.4 0"/></svg>',
  home: '<svg viewBox="0 0 24 24" fill="none" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l8 6v11h-5v-6H9v6H4V9z"/></svg>',
  grid: '<svg viewBox="0 0 24 24" fill="none" stroke-width="1.8" stroke-linejoin="round"><rect x="4" y="4" width="7" height="7" rx="2"/><rect x="13" y="4" width="7" height="7" rx="2"/><rect x="4" y="13" width="7" height="7" rx="2"/><rect x="13" y="13" width="7" height="7" rx="2"/></svg>',
  list: '<svg viewBox="0 0 24 24" fill="none" stroke-width="1.8" stroke-linecap="round"><path d="M8 6h12M8 12h12M8 18h12M4 6h.01M4 12h.01M4 18h.01"/></svg>',
  gear: '<svg viewBox="0 0 24 24" fill="none" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/></svg>',
  down: '<svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M12 5v14M6 13l6 6 6-6"/></svg>',
  up: '<svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M12 19V5M6 11l6-6 6 6"/></svg>',
};

const esc = (s: string) => s.replace(/[&<>"']/g, c => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[c]!));

function bytes(n: number): string {
  const u = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ'];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return (i === 0 ? n.toFixed(0) : n.toFixed(n < 10 ? 1 : 0)) + ' ' + u[i];
}
const speed = (n: number) => bytes(n) + '/с';
function dur(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(s / 3600), m = Math.floor(s / 60) % 60, ss = s % 60;
  const p = (x: number) => String(x).padStart(2, '0');
  return h ? `${h}:${p(m)}:${p(ss)}` : `${p(m)}:${p(ss)}`;
}
function ago(ms: number): string {
  if (!ms) return '';
  const s = Math.floor((Date.now() - ms) / 1000);
  if (s < 60) return 'только что';
  if (s < 3600) return `${Math.floor(s / 60)} мин назад`;
  return `${Math.floor(s / 3600)} ч назад`;
}

let tab = 'home';
let st: St = {state: 'off', since: Date.now(), message: '', pinned: 0, checks: null};
let snap: Snap | null = null;
let cfg: Cfg = {ipv4Only: false, autoConnect: false};
let version = '';
let busyClick = false;

const app = document.querySelector<HTMLDivElement>('#app')!;
app.innerHTML = `
  <div class="titlebar"></div>
  <main id="view"></main>
  <nav>
    <button data-tab="home">${icons.home}Главная</button>
    <button data-tab="services">${icons.grid}Сервисы</button>
    <button data-tab="log">${icons.list}Журнал</button>
    <button data-tab="settings">${icons.gear}Настройки</button>
  </nav>
  <div class="toast" id="toast"></div>`;
const view = document.getElementById('view')!;
const toastEl = document.getElementById('toast')!;

function toast(msg: string) {
  toastEl.textContent = msg;
  toastEl.classList.add('show');
  setTimeout(() => toastEl.classList.remove('show'), 4000);
}

document.querySelectorAll<HTMLButtonElement>('nav button').forEach(b => b.addEventListener('click', () => {
  tab = b.dataset.tab!;
  render(true);
}));

async function toggle() {
  if (busyClick) return;
  busyClick = true;
  try {
    const err = st.state === 'on' ? await Disconnect() : st.state === 'off' ? await Connect() : '';
    if (err) toast(err);
  } finally {
    busyClick = false;
    await refresh();
  }
}

function homeView(): string {
  const on = st.state === 'on';
  const busy = st.state === 'connecting' || st.state === 'stopping';
  const title = on ? 'Защищено' : busy ? (st.state === 'connecting' ? 'Подключение' : 'Отключение') : 'Не подключено';
  const sub = on ? dur(Date.now() - st.since) : (st.message || 'Нажмите, чтобы включить');
  const s = snap;
  const checks = (st.checks || []).map(c => {
    const cls = !c.done ? 'wait' : c.ok ? 'ok' : 'bad';
    const r = !c.done ? 'проверка…' : c.ok ? `доступен · ${c.ms} мс` : 'недоступен';
    return `<div class="check"><span>${esc(c.name)}</span><span class="r">${r}<span class="dot ${cls}"></span></span></div>`;
  }).join('');
  return `
    <div class="home">
      <div class="brand">Prohod</div>
      <button class="power ${on ? 'on' : ''} ${busy ? 'busy' : ''}" id="power" aria-label="toggle">${icons.power}</button>
      <div class="state"><div class="title">${title}</div><div class="sub">${esc(sub)}</div></div>
      <div class="metrics">
        <div class="metric"><div class="k">${icons.down} Получено</div><div class="v">${s ? bytes(s.bytesIn) : '—'}</div><div class="s">${s && on ? speed(s.speedIn) : ''}</div></div>
        <div class="metric"><div class="k">${icons.up} Отправлено</div><div class="v">${s ? bytes(s.bytesOut) : '—'}</div><div class="s">${s && on ? speed(s.speedOut) : ''}</div></div>
      </div>
      ${checks ? `<div class="checks">${checks}</div>` : ''}
    </div>`;
}

function servicesView(): string {
  const svcs = (snap?.services || []);
  const rows = svcs.map(v => {
    const used = v.ok + v.failed;
    let cls = '', meta = 'нет активности';
    if (v.active > 0 || used > 0) {
      const rate = used ? v.ok / used : 1;
      cls = v.lastOk >= v.lastFail ? 'ok' : rate >= 0.5 ? 'wait' : 'bad';
      meta = `${bytes(v.bytesIn + v.bytesOut)} · ${v.active ? 'активно' : ago(Math.max(v.lastOk, v.lastFail))}`;
    }
    return `<div class="row"><span class="dot ${cls}"></span><div class="name">${esc(v.name)}</div><div class="meta">${meta}</div></div>`;
  }).join('');
  return `
    <h2>Сервисы</h2>
    <div class="card">${rows || '<div class="empty">Подключитесь, чтобы увидеть активность</div>'}</div>
    <p class="hint">Зелёный — соединения проходят. Красный — сервис не отвечает в вашей сети.
      Оптимизировано маршрутов: ${st.pinned}. Остальные сайты подбираются автоматически.</p>
    ${snap ? `<p class="hint">Соединений: ${snap.connections} активно · ${snap.total} всего · ${snap.failed} без ответа</p>` : ''}`;
}

let logCache: Ent[] = [];
function logView(): string {
  const lines = logCache.map(e => {
    const t = new Date(e.time).toLocaleTimeString('ru-RU');
    return `<div class="ln"><span class="t">${t}</span> <span class="${esc(e.level)}">${esc(e.msg)}</span></div>`;
  }).join('');
  return `
    <h2>Журнал</h2>
    <div class="card"><div class="log" id="log">${lines || '<div class="empty">Пока пусто</div>'}</div></div>
    <div class="toolbar"><button class="btn" id="openlogs">Открыть папку с журналом</button></div>`;
}

function settingsView(): string {
  const sw = (id: string, on: boolean) => `<label class="switch"><input type="checkbox" id="${id}" ${on ? 'checked' : ''}><span></span></label>`;
  return `
    <h2>Настройки</h2>
    <div class="card">
      <div class="row"><div class="name">Подключаться при запуске<div class="desc">Включать защиту сразу после открытия приложения</div></div>${sw('autoConnect', cfg.autoConnect)}</div>
      <div class="row"><div class="name">Только IPv4<div class="desc">Если в вашей сети нестабилен IPv6. Применится при следующем подключении</div></div>${sw('ipv4Only', cfg.ipv4Only)}</div>
    </div>
    <h2>О программе</h2>
    <div class="card"><div class="row"><div class="name">Prohod</div><div class="meta">версия ${esc(version)}</div></div></div>
    <p class="hint">Работает локально на этом устройстве: без серверов, учётных записей и сбора данных.
      Сторонние компоненты и их лицензии перечислены в файле LICENSES внутри приложения.</p>`;
}

function bind() {
  document.getElementById('power')?.addEventListener('click', toggle);
  document.getElementById('openlogs')?.addEventListener('click', () => OpenLogs());
  for (const id of ['autoConnect', 'ipv4Only'] as const) {
    const el = document.getElementById(id) as HTMLInputElement | null;
    el?.addEventListener('change', async () => {
      cfg = {...cfg, [id]: el.checked};
      const err = await SaveSettings(cfg);
      if (err) toast(err);
    });
  }
}

let lastHTML = '';
function render(force = false) {
  document.querySelectorAll<HTMLButtonElement>('nav button').forEach(b => b.classList.toggle('active', b.dataset.tab === tab));
  const html = tab === 'home' ? homeView() : tab === 'services' ? servicesView() : tab === 'log' ? logView() : settingsView();
  if (!force && html === lastHTML) return;
  // keep the log scrolled to the bottom unless the user scrolled up
  const logEl = document.getElementById('log');
  const stick = !logEl || logEl.scrollTop + logEl.clientHeight >= logEl.scrollHeight - 20;
  // settings switches must not be re-rendered under the user's finger
  if (!force && tab === 'settings') return;
  view.innerHTML = html;
  lastHTML = html;
  bind();
  const nl = document.getElementById('log');
  if (nl && stick) nl.scrollTop = nl.scrollHeight;
}

async function refresh() {
  try {
    const [s, sn] = await Promise.all([Status(), Stats()]);
    st = s as St;
    snap = sn as Snap;
    if (tab === 'log') logCache = (await Logs(300)) as Ent[];
  } catch (e) {
    console.error(e);
  }
  render();
}

(async () => {
  version = await Version();
  cfg = (await Settings()) as Cfg;
  logCache = (await Logs(300)) as Ent[];
  render(true);
  await refresh();
  setInterval(refresh, 1000);
})();
