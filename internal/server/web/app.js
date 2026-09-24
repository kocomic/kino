'use strict';
const $ = s => document.querySelector(s);
const state = { offset: 0, limit: 36, total: 0, platforms: [], token: sessionStorage.getItem('kino-owner-token') || '', preview: null, request: null, detail: null, urls: [] };
function node(tag, text, cls) { const n = document.createElement(tag); if (text !== undefined) n.textContent = text; if (cls) n.className = cls; return n; }
function message(value) { $('#message').textContent = value; $('#message').hidden = !value; }
async function api(path, method = 'GET', body) {
 const headers = {}; if (state.token) headers.Authorization = `Bearer ${state.token}`;
 if (body !== undefined && !(body instanceof FormData)) { headers['Content-Type'] = 'application/json'; body = JSON.stringify(body); }
 const res = await fetch('/api/v1' + path, { method, headers, body });
 if (!res.ok) { const error = await res.json().catch(() => null); if (res.status === 401) $('#connection').textContent = '请在设置中输入访问令牌'; throw new Error(error?.error?.message || `请求失败 (${res.status})`); }
 return res.status === 204 ? null : res.json();
}
function run(fn) { return async (...args) => { message(''); try { await fn(...args); } catch (e) { message(e.message); } }; }
function view(id) { document.querySelectorAll('.view').forEach(n => n.hidden = n.id !== id); document.querySelectorAll('nav button').forEach(n => n.classList.toggle('active', n.dataset.view === id)); $('#heading').textContent = { library: '游戏库', import: '导入 ROM', settings: '设置与备份' }[id]; }
function selectPlatforms(select, all) { select.replaceChildren(); if (all) { const o = node('option', '全部平台'); o.value = ''; select.append(o); } for (const p of state.platforms) { const o = node('option', p.name || p.id); o.value = p.id; select.append(o); } }
async function load() {
 const platforms = await api('/platforms?limit=200'); state.platforms = platforms.data || platforms;
 const selected = $('#platform-filter').value; selectPlatforms($('#platform-filter'), true); $('#platform-filter').value = selected; selectPlatforms($('#import-platform'), true); await library(); $('#connection').textContent = '服务已连接';
}
let generation = 0;
async function library() {
 const epoch = ++generation; const params = new URLSearchParams({limit:state.limit, offset:state.offset, q:$('#search').value, platform:$('#platform-filter').value, locale:'zh-CN'});
 const result = await api('/games?' + params); if (epoch !== generation) return;
 state.total = result.pagination.total; state.urls.forEach(URL.revokeObjectURL); state.urls = []; $('#games').replaceChildren(); $('#count').textContent = `${state.total} 个游戏`;
 for (const game of result.data || []) {
  const card = node('button', undefined, 'game'); const cover = node('div', (game.display_title || game.default_title).slice(0,1), 'cover'); const info = node('div', undefined, 'game-info'); info.append(node('strong', game.display_title || game.default_title), node('small', `${game.platform} · ${(game.editions || []).length} 个版本`)); card.append(cover, info); card.onclick = run(() => detail(game.id)); $('#games').append(card);
  const media = (game.media || []).find(m => m.kind === 'cover' && m.content_status === 'available');
  if (media) coverImage(media.id, cover, epoch).catch(() => {});
 }
 if (!state.total) $('#games').append(node('div', '暂无游戏。添加游戏，或从服务器目录导入收藏。', 'empty'));
 $('#previous').disabled = state.offset === 0; $('#next').disabled = state.offset + state.limit >= state.total; $('#page-label').textContent = `${Math.floor(state.offset/state.limit)+1} / ${Math.max(1,Math.ceil(state.total/state.limit))}`;
}
async function coverImage(id, placeholder, epoch) { const headers = state.token ? {Authorization:`Bearer ${state.token}`} : {}; const r = await fetch(`/api/v1/media/${encodeURIComponent(id)}/thumbnail`, {headers}); if (!r.ok) return; const blob = await r.blob(); if (epoch !== generation) return; const url = URL.createObjectURL(blob); state.urls.push(url); const img = node('img', undefined, 'cover'); img.alt = ''; img.src = url; placeholder.replaceWith(img); }
function field(form, label, name, value = '', required = false) { const l = node('label', label); const input = node('input'); input.name = name; input.value = value; input.required = required; l.append(input); form.append(l); return input; }
function action(parent, title, fn, cls) { const b = node('button', title, cls); b.type = 'button'; b.onclick = run(fn); parent.append(b); return b; }
function openDialog() { if (!$('#detail').open) $('#detail').showModal(); }
async function detail(id) {
 const game = await api('/games/' + encodeURIComponent(id) + '?locale=zh-CN'); state.detail = game; const body = $('#detail-body'); body.replaceChildren(node('h2', game.display_title || game.default_title)); const form = node('form'); field(form, '游戏名称', 'default_title', game.default_title, true); field(form, '平台', 'platform', game.platform, true); const save = node('button', '保存修改', 'primary'); form.append(save); form.onsubmit = run(async e => { e.preventDefault(); const data = Object.fromEntries(new FormData(form)); data.titles = game.titles || {}; await api('/games/' + id, 'PUT', data); await library(); await detail(id); }); body.append(form);
 for (const edition of game.editions || []) {
  const section = node('div', undefined, 'edition'); section.append(node('h3', edition.display_title || edition.default_title), node('p', `${edition.edition_type} · ${edition.region || '未指定地区'}`, 'muted'));
  for (const artifact of edition.artifacts || []) section.append(node('p', `${artifact.path} · ${artifact.content_status || '已收录'}`, 'muted'));
  const actions = node('div', undefined, 'actions'); action(actions, '设为主要版本', async () => { await api('/games/'+id+'/primary', 'PUT', {edition_id:edition.id}); await detail(id); }); action(actions, '添加 ROM 文件', async () => { const path = prompt('服务器 ROM 路径（相对于 ROM 根目录）'); if (!path) return; await api('/artifacts', 'POST', {edition_id:edition.id,path,role:'rom'}); await detail(id); }); action(actions, '删除版本', async () => { if (!confirm('删除该版本的目录记录？原始 ROM 文件会保留。')) return; await api('/editions/'+edition.id,'DELETE'); await detail(id); await library(); }, 'danger'); section.append(actions); body.append(section);
 }
 const actions = node('div', undefined, 'actions'); action(actions, '添加版本', async () => { const title = prompt('版本名称', game.default_title); if (!title) return; await api('/editions','POST',{game_id:id,default_title:title,edition_type:'original'}); await detail(id); await library(); }); action(actions, '删除游戏', async () => { if (!confirm('删除游戏及其版本记录？原始 ROM 文件会保留。')) return; await api('/games/'+id,'DELETE'); $('#detail').close(); await library(); }, 'danger'); body.append(actions);
 const upload = node('form'); const fileLabel = node('label', '上传封面'); const file = node('input'); file.type = 'file'; file.accept = 'image/*'; file.required = true; file.name = 'file'; fileLabel.append(file); upload.append(fileLabel, node('button', '上传')); upload.onsubmit = run(async e => { e.preventDefault(); const data = new FormData(upload); data.set('game_id',id); data.set('kind','cover'); await api('/media/upload','POST',data); await library(); $('#detail').close(); }); body.append(upload); openDialog();
}
document.querySelectorAll('nav button').forEach(b => b.onclick = () => view(b.dataset.view));
$('#filters').onsubmit = run(async e => {e.preventDefault(); state.offset = 0; await library();});
$('#previous').onclick = run(async () => {state.offset = Math.max(0,state.offset-state.limit); await library();});
$('#next').onclick = run(async () => {state.offset += state.limit; await library();});
$('.close').onclick = () => $('#detail').close();
$('#new-game').onclick = () => { const body = $('#detail-body'); body.replaceChildren(node('h2','添加游戏')); const form = node('form'); field(form,'游戏名称','default_title','',true); field(form,'平台','platform','gba',true); form.append(node('button','创建','primary')); form.onsubmit = run(async e => {e.preventDefault(); const game = await api('/games','POST',Object.fromEntries(new FormData(form))); await library(); await detail(game.id);}); body.append(form); openDialog(); };
$('#auth-form').onsubmit = run(async e => {e.preventDefault(); state.token = $('#token').value.trim(); sessionStorage.setItem('kino-owner-token',state.token); await load(); $('#token').value = ''; view('library');});
$('#logout').onclick = () => {sessionStorage.removeItem('kino-owner-token'); state.token = ''; ++generation; state.urls.forEach(URL.revokeObjectURL); state.urls = []; $('#games').replaceChildren(); $('#connection').textContent = '令牌已清除';};
$('#import-form').onsubmit = run(async e => {e.preventDefault(); const button = e.submitter; button.disabled = true; $('#preview').hidden = true; try {state.request = Object.fromEntries(new FormData(e.target)); const rom = state.request.format === 'rom'; if (rom) {delete state.request.format; delete state.request.content_root;} state.request.locale = 'zh-CN'; state.request.media_storage = 'copy'; if (rom) {delete state.request.locale; delete state.request.media_storage;} state.importPath = rom ? '/imports/roms' : '/imports'; state.preview = await api(state.importPath+'/preview','POST',state.request); $('#candidates').replaceChildren(); for (const item of state.preview.candidates || []) { const label = node('label', undefined, 'candidate'); const check = node('input'); check.type = 'checkbox'; check.value = item.token || ''; check.disabled = item.status !== 'new' || !item.token; check.checked = !check.disabled; const title = node('span', item.game?.default_title || item.game?.title || '未命名'); title.append(node('small', [item.status,item.reason].filter(Boolean).join(' · '))); label.append(check,title); $('#candidates').append(label); } if (!state.preview.candidates?.length) $('#candidates').append(node('p','没有可导入的项目。请检查路径和平台。')); $('#preview').hidden = false; } finally {button.disabled = false;} });
$('#commit-import').onclick = run(async () => { const selected = [...$('#candidates').querySelectorAll('input:checked')].map(n => n.value); if (!selected.length) throw new Error('请先选择可导入的项目'); $('#commit-import').disabled = true; try { await api(state.importPath+'/commit','POST',{...state.request,preview_token:state.preview.preview_token,selected_tokens:selected}); $('#preview').hidden = true; state.preview = null; state.offset = 0; await library(); view('library'); } finally {$('#commit-import').disabled = false;} });
run(load)();
