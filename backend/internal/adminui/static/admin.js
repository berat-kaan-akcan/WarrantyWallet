// Warranty Wallet admin panel. All data is rendered with textContent (no HTML string parsing),
// and the access token lives only in sessionStorage for the lifetime of the tab.
'use strict';

const API = '/api/v1';
const $ = (id) => document.getElementById(id);
let token = sessionStorage.getItem('ww_admin_token') || '';
let me = null;
let users = [];

function say(text, isError = false) {
  const m = $('message');
  m.textContent = text;
  m.className = isError ? 'error' : '';
}

async function api(method, path, body) {
  const res = await fetch(API + path, {
    method,
    headers: Object.assign(
      { Accept: 'application/json' },
      token ? { Authorization: 'Bearer ' + token } : {},
      body ? { 'Content-Type': 'application/json' } : {}),
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 401) {
    logout('Oturum süresi doldu, tekrar giriş yapın.');
    throw new Error('unauthorized');
  }
  const data = res.status === 204 ? null : await res.json().catch(() => null);
  if (!res.ok) {
    const msg = data && data.error ? data.error.message || data.error.code : 'HTTP ' + res.status;
    throw new Error(msg);
  }
  return data;
}

function cell(text, cls) {
  const td = document.createElement('td');
  td.textContent = text == null ? '' : String(text);
  if (cls) td.className = cls;
  return td;
}

function button(text, onClick, cls) {
  const b = document.createElement('button');
  b.type = 'button';
  b.textContent = text;
  if (cls) b.className = cls;
  b.addEventListener('click', onClick);
  return b;
}

const fmtDate = (s) => (s ? new Date(s).toLocaleString('tr-TR') : '');

// ---- auth ----

async function login(ev) {
  ev.preventDefault();
  say('');
  try {
    token = '';
    const res = await api('POST', '/auth/login', { email: $('email').value.trim(), password: $('password').value });
    if (!res.user || res.user.role !== 'admin') {
      say('Bu hesabın yönetici yetkisi yok.', true);
      return;
    }
    token = res.token;
    sessionStorage.setItem('ww_admin_token', token);
    $('password').value = '';
    await start();
  } catch (e) {
    if (e.message !== 'unauthorized') say('Giriş başarısız: ' + e.message, true);
  }
}

function logout(text) {
  token = '';
  me = null;
  sessionStorage.removeItem('ww_admin_token');
  $('app').hidden = true;
  $('who').hidden = true;
  $('login-view').hidden = false;
  say(text || '');
}

async function start() {
  me = await api('GET', '/me');
  if (me.role !== 'admin') {
    logout('Bu hesabın yönetici yetkisi yok.');
    return;
  }
  $('who-email').textContent = me.email;
  $('who').hidden = false;
  $('login-view').hidden = true;
  $('app').hidden = false;
  await show('stats');
}

// ---- tabs ----

async function show(tab) {
  for (const b of document.querySelectorAll('nav button')) {
    if (b.dataset.tab === tab) b.setAttribute('aria-current', 'page');
    else b.removeAttribute('aria-current');
  }
  for (const t of ['stats', 'users', 'logs', 'categories']) $('tab-' + t).hidden = t !== tab;
  say('');
  try {
    await ({ stats: loadStats, users: loadUsers, logs: loadLogs, categories: loadCategories })[tab]();
  } catch (e) {
    if (e.message !== 'unauthorized') say('Yüklenemedi: ' + e.message, true);
  }
}

async function loadStats() {
  const s = await api('GET', '/admin/stats');
  const ex = s.extractions || {};
  const rate = ex.sample_size ? Math.round((ex.ok / ex.sample_size) * 100) + '%' : '-';
  const cards = $('stats-cards');
  cards.replaceChildren();
  for (const [label, value] of [
    ['Kullanıcı', s.users], ['Ürün', s.products],
    ['Başarılı fiş okuma', ex.ok], ['Başarısız fiş okuma', ex.failed], ['Başarı oranı', rate],
  ]) {
    const d = document.createElement('div');
    d.className = 'card';
    const strong = document.createElement('strong');
    strong.textContent = value == null ? '0' : String(value);
    d.append(strong, document.createTextNode(label));
    cards.append(d);
  }
  const rows = $('providers');
  rows.replaceChildren();
  for (const [p, n] of Object.entries(ex.by_provider || {})) {
    const tr = document.createElement('tr');
    tr.append(cell(p), cell(n));
    rows.append(tr);
  }
}

async function loadUsers() {
  users = (await api('GET', '/admin/users')).items || [];
  renderUsers();
}

function renderUsers() {
  const q = $('user-filter').value.trim().toLocaleLowerCase('tr');
  const rows = $('users');
  rows.replaceChildren();
  for (const u of users) {
    if (q && !(u.email + ' ' + u.name).toLocaleLowerCase('tr').includes(q)) continue;
    const tr = document.createElement('tr');
    const actions = document.createElement('td');
    if (u.id !== me.id) {
      const next = u.role === 'admin' ? 'user' : 'admin';
      actions.append(
        button(next === 'admin' ? 'Yönetici yap' : 'Yöneticiliği kaldır', () => setRole(u, next)),
        document.createTextNode(' '),
        button('Sil', () => removeUser(u), 'danger'));
    } else {
      actions.textContent = '(siz)';
    }
    tr.append(cell(u.email), cell(u.name), cell(u.role === 'admin' ? 'Yönetici' : 'Kullanıcı'), cell(fmtDate(u.created_at)), actions);
    rows.append(tr);
  }
}

async function setRole(u, role) {
  try {
    await api('PATCH', '/admin/users/' + encodeURIComponent(u.id), { role });
    say(u.email + ' güncellendi.');
    await loadUsers();
  } catch (e) {
    if (e.message !== 'unauthorized') say('Güncellenemedi: ' + e.message, true);
  }
}

async function removeUser(u) {
  if (!confirm(u.email + ' ve tüm ürünleri, belgeleri kalıcı olarak silinsin mi?')) return;
  try {
    await api('DELETE', '/admin/users/' + encodeURIComponent(u.id));
    say(u.email + ' silindi.');
    await loadUsers();
  } catch (e) {
    if (e.message !== 'unauthorized') say('Silinemedi: ' + e.message, true);
  }
}

async function loadLogs() {
  const only = $('failures-only').checked ? '?failures=1' : '';
  const logs = (await api('GET', '/admin/extraction-logs' + only)).items || [];
  const rows = $('logs');
  rows.replaceChildren();
  if (!logs.length) {
    const tr = document.createElement('tr');
    const td = cell('Kayıt yok.');
    td.colSpan = 4;
    tr.append(td);
    rows.append(tr);
  }
  for (const l of logs) {
    const tr = document.createElement('tr');
    tr.append(cell(fmtDate(l.created_at)), cell(l.provider),
      cell(l.success ? 'Başarılı' : 'Başarısız', l.success ? 'ok' : 'bad'), cell(l.error, 'error'));
    rows.append(tr);
  }
}

async function loadCategories() {
  const cats = (await api('GET', '/categories')).items || [];
  const rows = $('categories');
  rows.replaceChildren();
  for (const c of cats) {
    const tr = document.createElement('tr');
    const actions = document.createElement('td');
    if (c.slug !== 'other') actions.append(button('Sil', () => removeCategory(c), 'danger'));
    tr.append(cell(c.slug), cell(c.name_tr), actions);
    rows.append(tr);
  }
}

async function saveCategory(ev) {
  ev.preventDefault();
  try {
    await api('POST', '/admin/categories', { slug: $('cat-slug').value.trim(), name_tr: $('cat-name').value.trim() });
    say('Kategori kaydedildi.');
    $('category-form').reset();
    await loadCategories();
  } catch (e) {
    if (e.message !== 'unauthorized') say('Kaydedilemedi: ' + e.message, true);
  }
}

async function removeCategory(c) {
  if (!confirm('"' + c.name_tr + '" kategorisi silinsin mi?')) return;
  try {
    await api('DELETE', '/admin/categories/' + encodeURIComponent(c.slug));
    say('Kategori silindi.');
    await loadCategories();
  } catch (e) {
    if (e.message !== 'unauthorized') say('Silinemedi: ' + e.message, true);
  }
}

// ---- wiring ----

$('login-form').addEventListener('submit', login);
$('logout').addEventListener('click', () => logout('Çıkış yapıldı.'));
$('category-form').addEventListener('submit', saveCategory);
$('user-filter').addEventListener('input', renderUsers);
$('failures-only').addEventListener('change', () => show('logs'));
for (const b of document.querySelectorAll('nav button')) b.addEventListener('click', () => show(b.dataset.tab));

if (token) start().catch(() => logout());
