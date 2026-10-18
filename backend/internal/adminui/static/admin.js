// Warranty Wallet admin panel. All data is rendered with textContent (no HTML string parsing),
// and the tokens live only in sessionStorage for the lifetime of the tab. An expired access token
// is renewed once with the (rotating) refresh token before asking the admin to sign in again.
'use strict';

const API = '/api/v1';
const $ = (id) => document.getElementById(id);
let token = sessionStorage.getItem('ww_admin_token') || '';
let refreshToken = sessionStorage.getItem('ww_admin_refresh') || '';
let me = null;
let users = [];
let centers = [];

function say(text, isError = false) {
  const m = $('message');
  m.textContent = text;
  m.className = isError ? 'error' : '';
}

function saveTokens(res) {
  token = res.token || '';
  refreshToken = res.refresh_token || '';
  sessionStorage.setItem('ww_admin_token', token);
  sessionStorage.setItem('ww_admin_refresh', refreshToken);
}

// Turkish texts for API error codes the panel commonly sees.
const MESSAGES = {
  invalid_credentials: 'E-posta veya şifre hatalı.',
  rate_limited: 'Çok fazla deneme yapıldı, biraz sonra tekrar deneyin.',
  forbidden: 'Bu işlem için yönetici yetkisi gerekiyor.',
  not_found: 'Kayıt bulunamadı.',
  validation_failed: 'Lütfen alanları kontrol edin (telefon, web sitesi, enlem/boylam birlikte).',
};

let refreshing = null;

// renew exchanges the refresh token once; concurrent callers share the same attempt.
function renew() {
  if (!refreshToken) return Promise.resolve(false);
  refreshing = refreshing || fetch(API + '/auth/refresh', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({ refresh_token: refreshToken }),
  }).then(async (res) => {
    if (!res.ok) return false;
    saveTokens(await res.json());
    return true;
  }).catch(() => false).finally(() => { refreshing = null; });
  return refreshing;
}

async function api(method, path, body, retried = false) {
  const res = await fetch(API + path, {
    method,
    headers: Object.assign(
      { Accept: 'application/json' },
      token ? { Authorization: 'Bearer ' + token } : {},
      body ? { 'Content-Type': 'application/json' } : {}),
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 401 && !path.startsWith('/auth/')) {
    if (!retried && (await renew())) return api(method, path, body, true);
    logout('Oturum süresi doldu, tekrar giriş yapın.');
    throw new Error('unauthorized');
  }
  const data = res.status === 204 ? null : await res.json().catch(() => null);
  if (!res.ok) {
    const code = data && data.error ? data.error.code : '';
    const msg = MESSAGES[code] || (data && data.error ? data.error.message || code : 'HTTP ' + res.status);
    const err = new Error(msg);
    err.fields = data && data.error ? data.error.fields : null;
    throw err;
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
    saveTokens(res);
    $('password').value = '';
    await start();
  } catch (e) {
    if (e.message !== 'unauthorized') say('Giriş başarısız: ' + e.message, true);
  }
}

function logout(text, revoke = false) {
  if (revoke && token && refreshToken) {
    // Best effort: revoke the refresh token on the server too.
    fetch(API + '/auth/logout', {
      method: 'POST',
      headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    }).catch(() => {});
  }
  token = '';
  refreshToken = '';
  me = null;
  sessionStorage.removeItem('ww_admin_token');
  sessionStorage.removeItem('ww_admin_refresh');
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
  for (const t of ['stats', 'users', 'logs', 'categories', 'centers']) $('tab-' + t).hidden = t !== tab;
  say('');
  try {
    await ({ stats: loadStats, users: loadUsers, logs: loadLogs, categories: loadCategories, centers: loadCenters })[tab]();
  } catch (e) {
    if (e.message !== 'unauthorized') say('Yüklenemedi: ' + e.message, true);
  }
}

async function loadStats() {
  const s = await api('GET', '/admin/stats');
  const ex = s.extractions || {};
  const dg = s.diagnoses || {};
  const rate = ex.sample_size ? Math.round((ex.ok / ex.sample_size) * 100) + '%' : '-';
  const cards = $('stats-cards');
  cards.replaceChildren();
  for (const [label, value] of [
    ['Kullanıcı', s.users], ['Ürün', s.products],
    ['Başarılı fiş okuma', ex.ok], ['Başarısız fiş okuma', ex.failed], ['Başarı oranı', rate],
    ['Arıza asistanı kullanımı', dg.total], ['Gemini yanıtı', dg.ai], ['Kural tabanı yanıtı', dg.rules],
    ['AI hatası (kurala düştü)', dg.ai_failed],
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
  const mrows = $('models');
  mrows.replaceChildren();
  const seen = new Set([...Object.keys(ex.by_model || {}), ...Object.keys(dg.by_model || {})]);
  for (const m of seen) {
    const tr = document.createElement('tr');
    tr.append(cell(m), cell((ex.by_model || {})[m] || 0), cell((dg.by_model || {})[m] || 0));
    mrows.append(tr);
  }
  if (!seen.size) {
    const tr = document.createElement('tr');
    const td = cell('Henüz Gemini yanıtı yok.');
    td.colSpan = 3;
    tr.append(td);
    mrows.append(tr);
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
  const q = new URLSearchParams();
  if ($('failures-only').checked) q.set('failures', '1');
  if ($('log-kind').value) q.set('kind', $('log-kind').value);
  const logs = (await api('GET', '/admin/extraction-logs?' + q)).items || [];
  const rows = $('logs');
  rows.replaceChildren();
  if (!logs.length) {
    const tr = document.createElement('tr');
    const td = cell('Kayıt yok.');
    td.colSpan = 5;
    tr.append(td);
    rows.append(tr);
  }
  for (const l of logs) {
    const tr = document.createElement('tr');
    tr.append(cell(fmtDate(l.created_at)), cell(l.kind === 'diagnosis' ? 'Arıza asistanı' : 'Fiş okuma'),
      cell(l.model ? l.provider + ' (' + l.model + ')' : l.provider),
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

// ---- service centers ----

const CENTER_FIELDS = ['brand', 'name', 'city', 'district', 'address', 'phone', 'website'];

async function loadCenters() {
  centers = (await api('GET', '/admin/service-centers')).items || [];
  renderCenters();
}

function renderCenters() {
  const q = $('center-filter').value.trim().toLocaleLowerCase('tr');
  const rows = $('centers');
  rows.replaceChildren();
  let shown = 0;
  for (const c of centers) {
    if (q && !(c.brand + ' ' + c.city + ' ' + c.name).toLocaleLowerCase('tr').includes(q)) continue;
    shown++;
    const tr = document.createElement('tr');
    const actions = document.createElement('td');
    actions.append(button('Düzenle', () => editCenter(c)), document.createTextNode(' '),
      button('Sil', () => removeCenter(c), 'danger'));
    const where = c.lat != null && c.lng != null ? c.lat.toFixed(4) + ', ' + c.lng.toFixed(4) : '-';
    tr.append(cell(c.brand), cell(c.name), cell(c.district ? c.city + ' / ' + c.district : c.city), cell(c.phone), cell(where), actions);
    rows.append(tr);
  }
  if (!shown) {
    const tr = document.createElement('tr');
    const td = cell('Kayıt yok.');
    td.colSpan = 6;
    tr.append(td);
    rows.append(tr);
  }
}

function resetCenterForm() {
  $('center-form').reset();
  $('center-id').value = '';
  $('center-submit').textContent = 'Ekle';
  $('center-cancel').hidden = true;
}

function editCenter(c) {
  $('center-id').value = c.id;
  for (const f of CENTER_FIELDS) $('center-' + f).value = c[f] || '';
  $('center-lat').value = c.lat == null ? '' : c.lat;
  $('center-lng').value = c.lng == null ? '' : c.lng;
  $('center-submit').textContent = 'Kaydet';
  $('center-cancel').hidden = false;
  $('center-brand').focus();
}

async function saveCenter(ev) {
  ev.preventDefault();
  const body = {};
  for (const f of CENTER_FIELDS) body[f] = $('center-' + f).value.trim();
  const lat = $('center-lat').value.trim();
  const lng = $('center-lng').value.trim();
  body.lat = lat === '' ? null : Number(lat);
  body.lng = lng === '' ? null : Number(lng);
  const id = $('center-id').value;
  try {
    if (id) await api('PUT', '/admin/service-centers/' + encodeURIComponent(id), body);
    else await api('POST', '/admin/service-centers', body);
    say(id ? 'Servis merkezi güncellendi.' : 'Servis merkezi eklendi.');
    resetCenterForm();
    await loadCenters();
  } catch (e) {
    if (e.message !== 'unauthorized') say('Kaydedilemedi: ' + e.message, true);
  }
}

async function removeCenter(c) {
  if (!confirm('"' + c.name + '" silinsin mi?')) return;
  try {
    await api('DELETE', '/admin/service-centers/' + encodeURIComponent(c.id));
    say('Servis merkezi silindi.');
    await loadCenters();
  } catch (e) {
    if (e.message !== 'unauthorized') say('Silinemedi: ' + e.message, true);
  }
}

// importCenters sends a CSV file; the server validates every row before writing anything.
async function importCenters(ev) {
  ev.preventDefault();
  const file = $('center-file').files[0];
  if (!file) return;
  if (file.size > 900 * 1024) {
    say('Dosya çok büyük (en fazla 900 KB). Listeyi parçalara bölün.', true);
    return;
  }
  try {
    const res = await api('POST', '/admin/service-centers/import', { csv: await file.text() });
    say(res.created + ' servis merkezi eklendi' + (res.skipped ? ', ' + res.skipped + ' kayıt zaten vardı.' : '.'));
    $('center-import').reset();
    await loadCenters();
  } catch (e) {
    if (e.message === 'unauthorized') return;
    const tr = { brand: 'marka', name: 'ad', city: 'şehir', district: 'ilçe', address: 'adres', phone: 'telefon',
      website: 'web sitesi', lat: 'enlem/boylam' };
    const rows = e.fields ? Object.entries(e.fields).map(([k, v]) => k === 'csv' ? v
      : k.replace('row', 'satır') + ': hatalı ' + v.replace('invalid ', '').split(', ').map((f) => tr[f] || f).join(', ')).join('; ') : '';
    say('İçe aktarılamadı, hiçbir kayıt eklenmedi. ' + (rows || e.message), true);
  }
}

// ---- wiring ----

$('login-form').addEventListener('submit', login);
$('logout').addEventListener('click', () => logout('Çıkış yapıldı.', true));
$('category-form').addEventListener('submit', saveCategory);
$('user-filter').addEventListener('input', renderUsers);
$('center-form').addEventListener('submit', saveCenter);
$('center-cancel').addEventListener('click', resetCenterForm);
$('center-filter').addEventListener('input', renderCenters);
$('center-import').addEventListener('submit', importCenters);
$('failures-only').addEventListener('change', () => show('logs'));
$('log-kind').addEventListener('change', () => show('logs'));
for (const b of document.querySelectorAll('nav button')) b.addEventListener('click', () => show(b.dataset.tab));

if (token) start().catch(() => logout());
