package main

import "net/http"

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>fanout</title>
<style>
:root{
  --bg:#12151a; --panel:#181c23; --line:#262c36; --text:#dde3ec;
  --dim:#8b95a5; --accent:#4a9eda; --ok:#3fa66b; --warn:#c9903a; --bad:#c25450;
}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);
  font:13px/1.5 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
header{display:flex;align-items:center;gap:16px;padding:10px 16px;
  border-bottom:1px solid var(--line);background:var(--panel)}
h1{font-size:13px;font-weight:600;margin:0;letter-spacing:0}
.spacer{flex:1}
button{font:inherit;color:var(--text);background:#222833;border:1px solid var(--line);
  border-radius:4px;padding:4px 10px;cursor:pointer;display:inline-flex;
  align-items:center;gap:5px;white-space:nowrap}
button:hover:not(:disabled){border-color:var(--accent)}
button:disabled{opacity:.45;cursor:default}
button.primary{background:var(--accent);border-color:var(--accent);color:#0b0e12;font-weight:600}
button.icon{padding:3px 6px;background:transparent;border-color:transparent;color:var(--dim)}
button.icon:hover:not(:disabled){color:var(--accent);border-color:var(--line)}
button.icon.danger:hover:not(:disabled){color:var(--bad);border-color:rgba(194,84,80,.35)}
svg{width:14px;height:14px;stroke:currentColor;fill:none;stroke-width:1.8;
  stroke-linecap:round;stroke-linejoin:round;flex:none}
main{padding:14px 16px 40px;max-width:1180px;margin:0 auto}
.bar{display:flex;align-items:center;gap:10px;margin-bottom:12px}
.bar h2{font-size:12px;margin:0;font-weight:600;color:var(--dim)}
.exit{border:1px solid var(--line);border-radius:6px;margin-bottom:8px;
  background:var(--panel);overflow:hidden}
.exit>.row{display:grid;gap:6px 12px;align-items:center;padding:9px 12px;
  grid-template-columns:14px minmax(132px,auto) 1fr auto auto;
  grid-template-areas:"dot ip meta socks acts"}
.exit .dot{grid-area:dot}
.exit .ip{grid-area:ip}
.exit .meta{grid-area:meta}
.exit .socks{grid-area:socks}
.exit .acts{grid-area:acts}
.dot{width:8px;height:8px;border-radius:50%;background:var(--dim);justify-self:center}
.dot.up{background:var(--ok)}
.dot.starting{background:var(--warn);animation:pulse 1.2s ease-in-out infinite}
.dot.failed{background:var(--bad)}
@keyframes pulse{0%,100%{opacity:1}50%{opacity:.3}}
.ip{font-weight:600;font-variant-numeric:tabular-nums}
.meta{color:var(--dim);font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.socks{display:flex;align-items:center;gap:2px;min-width:0;
  color:var(--dim);font-size:12px;font-variant-numeric:tabular-nums}
.socks .addr{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.socks button{background:transparent;border-color:transparent;color:var(--dim);
  font-size:12px;padding:2px 6px;font-variant-numeric:tabular-nums}
.socks button:hover:not(:disabled){color:var(--accent);border-color:var(--line)}
.socks button .lock{width:11px;height:11px;stroke-width:2}
.acts{display:flex;gap:2px;justify-self:end}
.errline{padding:0 12px 9px 38px;color:var(--bad);font-size:11px;
  overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.empty{border:1px dashed var(--line);border-radius:6px;padding:40px 20px;
  text-align:center;color:var(--dim)}
.empty button{margin-top:14px}
.jobs{margin-bottom:12px}
.job{border:1px solid var(--line);border-radius:6px;background:var(--panel);
  padding:10px 12px;margin-bottom:8px}
.job .top{display:flex;align-items:center;gap:10px;margin-bottom:8px}
.job .top strong{font-weight:600;font-size:12px}
.steps{display:flex;flex-wrap:wrap;gap:6px}
.step{display:flex;align-items:center;gap:5px;font-size:11px;color:var(--dim);
  border:1px solid var(--line);border-radius:3px;padding:2px 7px;background:#0e1116}
.step.ok{color:var(--ok);border-color:rgba(63,166,107,.35)}
.step.failed{color:var(--bad);border-color:rgba(194,84,80,.35)}
.step.running{color:var(--warn);border-color:rgba(201,144,58,.35)}
.spin{animation:rot 1s linear infinite;transform-origin:center}
@keyframes rot{to{transform:rotate(360deg)}}
.links{display:flex;gap:14px;margin-right:4px}
.links a{color:var(--dim);text-decoration:none;font-size:12px}
.links a:hover{color:var(--accent)}
@media(max-width:820px){.links{display:none}
  main{padding:12px 12px 40px}
  .exit>.row{grid-template-columns:14px 1fr auto;
    grid-template-areas:"dot ip acts" ". meta meta" ". socks socks"}
  .bar{flex-wrap:wrap}}
.modal{position:fixed;inset:0;background:rgba(8,10,14,.72);display:none;
  align-items:center;justify-content:center;z-index:50;padding:20px}
.modal.open{display:flex}
.sheet{background:var(--bg);border:1px solid var(--line);border-radius:6px;
  width:min(680px,100%);max-height:86vh;display:flex;flex-direction:column}
.sheet .head{display:flex;align-items:center;gap:10px;padding:10px 14px;
  border-bottom:1px solid var(--line);background:var(--panel);border-radius:6px 6px 0 0}
.sheet .head h2{font-size:12px;margin:0;font-weight:600}
.sheet .body{overflow:auto;padding:14px}
.sheet .foot{display:flex;align-items:center;gap:10px;padding:10px 14px;
  border-top:1px solid var(--line);background:var(--panel);border-radius:0 0 6px 6px}
.count{color:var(--dim);font-size:11px}
label.f{display:block;margin-bottom:16px}
label.f[hidden]{display:none}
label.f>span{display:block;color:var(--dim);font-size:11px;margin-bottom:6px}
.regions{display:grid;grid-template-columns:repeat(auto-fill,minmax(148px,1fr));
  gap:6px;max-height:224px;overflow:auto}
.rg{border:1px solid var(--line);background:#0e1116;border-radius:4px;padding:7px 9px;
  cursor:pointer;text-align:left;display:block;width:100%}
.rg:hover{border-color:var(--accent)}
.rg.sel{border-color:var(--accent);background:rgba(74,158,218,.1)}
.rg b{font-weight:600;font-size:12px;display:block;overflow:hidden;
  text-overflow:ellipsis;white-space:nowrap}
.rg em{display:block;font-style:normal;color:var(--dim);font-size:11px;margin-top:2px}
.stepper{display:flex;align-items:center;gap:0;width:fit-content;
  border:1px solid var(--line);border-radius:4px;overflow:hidden;background:#0e1116}
.stepper button{border:0;border-radius:0;background:transparent;padding:5px 11px}
select,input[type=search],input[type=text]{font:inherit;background:#0e1116;
  border:1px solid var(--line);color:var(--text);border-radius:4px;
  padding:5px 8px;width:100%}
select:focus,input[type=search]:focus,input[type=text]:focus{outline:none;border-color:var(--accent)}
.stepper input[type=text]{width:56px;text-align:center;font:inherit;background:transparent;
  border:0;border-left:1px solid var(--line);border-right:1px solid var(--line);
  color:var(--text);padding:5px 0;font-variant-numeric:tabular-nums}
.stepper input:focus{outline:none}
.hint{color:var(--dim);font-size:11px;margin-top:6px}
.setrow{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-top:16px}
.setrow input,.setrow select{width:100%}
label.chk{display:flex;align-items:center;gap:7px;color:var(--text);font-size:12px;
  cursor:pointer;margin:0}
label.chk input{margin:0}
.hint.bad{color:var(--bad)}
.share{padding:10px;background:#0e1116;border:1px solid var(--line);
  border-radius:4px;word-break:break-all;font-size:12px;line-height:1.7;margin-bottom:8px}
.ef{display:block}
.ef>span{display:block;color:var(--dim);font-size:11px;margin-bottom:4px}
.ef input{width:150px}
.credrow{display:flex;align-items:flex-end;gap:12px;flex-wrap:wrap;margin-bottom:10px}
.credrow .ef input{width:190px}
.toast{position:fixed;left:50%;bottom:24px;transform:translateX(-50%);
  background:var(--panel);border:1px solid var(--line);border-radius:4px;
  padding:8px 14px;font-size:12px;z-index:80;opacity:0;pointer-events:none;
  transition:opacity .18s}
.toast.show{opacity:1}
.toast.bad{border-color:rgba(194,84,80,.5);color:var(--bad)}
.updsec{margin-top:18px;padding-top:14px;border-top:1px solid var(--line)}
.updrow{display:flex;align-items:center;gap:10px}
.updver{font-size:12px;color:var(--text)}
.updver b{font-weight:600}
.updver span{color:var(--dim);margin-left:8px}
.updnotes{margin-top:10px;padding:10px;background:#0e1116;border:1px solid var(--line);
  border-radius:4px;font-size:12px;line-height:1.6;color:var(--dim);white-space:pre-wrap;
  max-height:180px;overflow:auto}
</style>
</head>
<body>
<header>
  <h1>fanout</h1>
  <span class="count" id="nodecount"></span>
  <span class="spacer"></span>
  <button id="refreshBtn" title="重新抓取 VPN Gate 节点列表">
    <svg viewBox="0 0 24 24"><path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/></svg>
    刷新节点
  </button>
  <button class="icon" id="settingsBtn" title="设置">
    <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
  </button>
  <nav class="links">
    <a href="https://t.me/+ft-zI76oovgwNmRh" target="_blank" rel="noopener">交流群</a>
    <a href="https://youtube.com/@joeyblog" target="_blank" rel="noopener">油管</a>
    <a href="https://joeyblog.net" target="_blank" rel="noopener">博客</a>
    <a href="https://github.com/byJoey/fanout" target="_blank" rel="noopener">GitHub</a>
  </nav>
</header>

<main>
  <div class="jobs" id="jobs"></div>

  <div class="bar">
    <h2>出口</h2>
    <span class="count" id="ecount"></span>
    <span class="spacer"></span>
    <button id="stopall" title="停止所有出口">
      <svg viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>
      全部停止
    </button>
    <button class="primary" id="newexit">
      <svg viewBox="0 0 24 24"><path d="M12 5v14"/><path d="M5 12h14"/></svg>
      新建出口
    </button>
  </div>

  <div id="list"></div>
</main>

<div class="modal" id="wizard">
  <div class="sheet">
    <div class="head">
      <h2>新建出口</h2>
      <span class="spacer"></span>
      <button class="icon" data-close="wizard" title="关闭">
        <svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
      </button>
    </div>
    <div class="body">
      <label class="f">
        <span>地区</span>
        <input type="search" id="rgfilter" placeholder="筛选地区">
        <div class="regions" id="regions" style="margin-top:6px"></div>
      </label>
      <label class="f">
        <span id="countlabel">数量</span>
        <div class="stepper">
          <button id="minus" title="减少">
            <svg viewBox="0 0 24 24"><path d="M5 12h14"/></svg>
          </button>
          <input id="count" type="text" inputmode="numeric" value="3">
          <button id="plus" title="增加">
            <svg viewBox="0 0 24 24"><path d="M12 5v14"/><path d="M5 12h14"/></svg>
          </button>
        </div>
        <div class="hint" id="availhint"></div>
      </label>
    </div>
    <div class="foot">
      <span class="count" id="wzhint"></span>
      <span class="spacer"></span>
      <button data-close="wizard">取消</button>
      <button class="primary" id="go">开始</button>
    </div>
  </div>
</div>

<div class="modal" id="credbox">
  <div class="sheet">
    <div class="head">
      <h2>SOCKS5 访问凭据</h2>
      <span class="count" id="crtitle"></span>
      <span class="spacer"></span>
      <button class="icon" data-close="credbox" title="关闭">
        <svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
      </button>
    </div>
    <div class="body">
      <div class="share" id="crurl"></div>
      <div class="credrow">
        <label class="ef"><span>用户名</span>
          <input id="cruser" type="text" spellcheck="false"></label>
        <label class="ef"><span>口令</span>
          <input id="crpass" type="text" spellcheck="false"></label>
        <button id="crrand" title="随机生成一套">
          <svg viewBox="0 0 24 24"><path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/></svg>
          随机
        </button>
      </div>
      <div class="hint">改完立即生效，已连上的会话不断；用旧凭据的客户端要改配置。</div>
    </div>
    <div class="foot">
      <button id="crcopy">
        <svg viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
        复制地址
      </button>
      <span class="spacer"></span>
      <button data-close="credbox">取消</button>
      <button class="primary" id="crsave">保存</button>
    </div>
  </div>
</div>

<div class="modal" id="settings">
  <div class="sheet">
    <div class="head">
      <h2>设置</h2>
      <span class="spacer"></span>
      <button class="icon" data-close="settings" title="关闭">
        <svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
      </button>
    </div>
    <div class="body">
      <label class="f"><span>访问口令</span>
        <input id="setPw" type="password" spellcheck="false" autocomplete="new-password" placeholder="留空则不改"></label>
      <div class="hint">改完只影响新登录，当前这个浏览器不会被踢下线。</div>

      <label class="f" style="margin-top:16px"><span>访问路径</span>
        <input id="setPath" type="text" spellcheck="false" placeholder="留空则去掉路径前缀"></label>
      <div class="hint" id="setPathHint">界面挂在这个路径下，扫端口的探不到。只能用字母数字和 - _。</div>

      <label class="chk" style="margin-top:16px"><input type="checkbox" id="setResi"> 只用家宽节点</label>
      <div class="hint" id="setResiHint">vpngate 里混着一批它自己的机房机器，出口一眼看得出是数据中心。勾着就只挑志愿者家宽。</div>

      <div class="setrow">
        <label class="f" style="margin:0"><span>监听端口</span>
          <input id="setPort" type="text" inputmode="numeric" spellcheck="false"></label>
        <label class="f" style="margin:0"><span>本地监听地址</span>
          <select id="setListen">
            <option value="0.0.0.0">所有网卡（0.0.0.0）</option>
            <option value="127.0.0.1">仅本机（127.0.0.1）</option>
          </select></label>
      </div>
      <div class="hint bad" id="setPortHint">改端口或监听地址会切换监听，保存后要用新地址重新打开界面。</div>

      <div class="updsec">
        <div class="updrow">
          <div class="updver">版本 <b id="updCur">-</b><span id="updLatest"></span></div>
          <span class="spacer"></span>
          <button id="updCheck">检查更新</button>
          <button class="primary" id="updApply" hidden>更新到 <span id="updApplyVer"></span></button>
        </div>
        <div class="updnotes" id="updNotes" hidden></div>
      </div>

    </div>
    <div class="foot">
      <span class="spacer"></span>
      <button data-close="settings">取消</button>
      <button class="primary" id="setSave">保存</button>
    </div>
  </div>
</div>

<div class="toast" id="toast"></div>

<script>
const $ = s => document.querySelector(s);
const ICON = {
  copy:'<svg viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>',
  stop:'<svg viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>',
  redo:'<svg viewBox="0 0 24 24"><path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/></svg>',
  ok:'<svg viewBox="0 0 24 24"><path d="M20 6 9 17l-5-5"/></svg>',
  bad:'<svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>',
  run:'<svg viewBox="0 0 24 24" class="spin"><path d="M21 12a9 9 0 1 1-6.2-8.5"/></svg>',
  wait:'<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/></svg>',
  x:'<svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>',
  lock:'<svg viewBox="0 0 24 24" class="lock"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>'
};

// 界面挂在随机前缀下，请求一律走相对路径
async function api(path, opts){
  const r = await fetch(path.replace(/^\//, ''), opts);
  const d = await r.json().catch(()=>({}));
  if(!r.ok) throw new Error(d.error || ('HTTP '+r.status));
  return d;
}
function esc(s){ return String(s == null ? '' : s).replace(/[&<>"']/g, c =>
  ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }

let toastTimer;
function toast(msg, bad){
  const el = $('#toast');
  el.textContent = msg;
  el.className = 'toast show' + (bad ? ' bad' : '');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.className = 'toast'; }, 2400);
}
async function copy(text){
  // navigator.clipboard 只在 HTTPS/localhost 下存在，而面板通常是 http://IP 访问，
  // 所以必须留一条 execCommand 兜底路径，否则复制在正常使用场景里必然失败。
  if(navigator.clipboard && window.isSecureContext){
    try{ await navigator.clipboard.writeText(text); toast('已复制'); return; }
    catch(e){}
  }
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.setAttribute('readonly', '');
  // 放在视口内但不可见：置于视口外会让 iOS 在聚焦时滚动页面
  ta.style.cssText = 'position:fixed;top:0;left:0;width:1px;height:1px;opacity:0;padding:0;border:0';
  document.body.appendChild(ta);
  const prev = document.activeElement;
  ta.focus();
  ta.setSelectionRange(0, ta.value.length);
  let ok = false;
  try{ ok = document.execCommand('copy'); }catch(e){}
  ta.remove();
  if(prev && prev.focus) prev.focus();
  toast(ok ? '已复制' : '复制失败，请手动选中', !ok);
}

let view = {exits:[], public_ip:''};

const STATUS = {up:'已连通', starting:'连接中', failed:'失败', stopped:'已停止'};

function renderExits(){
  const list = $('#list');
  const n = view.exits.length;
  $('#ecount').textContent = n ? n + ' 个' : '';
  $('#stopall').disabled = !n;

  if(!n){
    list.innerHTML = '<div class="empty">还没有出口'
      + '<div><button class="primary" id="newexit2">'
      + '<svg viewBox="0 0 24 24"><path d="M12 5v14"/><path d="M5 12h14"/></svg>'
      + '新建出口</button></div></div>';
    return;
  }

  list.innerHTML = view.exits.map(e => {
    const label = e.exit_ip || (e.status === 'starting' ? '连接中…' : '—');
    const err = e.status === 'failed' && e.err
      ? '<div class="errline" title="' + esc(e.err) + '">' + esc(e.err) + '</div>' : '';
    // country 后端已经给成"国旗 中文"，再拼国家码就重复了
    const place = esc(e.country || e.region || '—');
    const url = socksURL(credHost(e), e.port, e.socks_user, e.socks_pass);
    return '<div class="exit">'
      + '<div class="row">'
      +   '<span class="dot ' + e.status + '" title="' + (STATUS[e.status] || e.status) + '"></span>'
      +   '<span class="ip">' + esc(label) + '</span>'
      +   '<span class="meta">' + place + ' · ' + esc(e.host) + '</span>'
      +   '<span class="socks">'
      +     '<button data-cred="' + e.slot + '" title="SOCKS5 访问凭据（改口令）">' + ICON.lock + ':' + e.port + '</button>'
      +     '<span class="addr" title="' + esc(url) + '">' + esc(url) + '</span>'
      +     '<button class="icon" data-copy="' + esc(url) + '" title="复制 SOCKS5 地址">' + ICON.copy + '</button>'
      +   '</span>'
      +   '<span class="acts">'
      +     '<button class="icon" data-swap="' + e.slot + '" title="换一个节点">' + ICON.redo + '</button>'
      +     '<button class="icon" data-stop="' + e.slot + '" title="停止这个出口">' + ICON.stop + '</button>'
      +   '</span>'
      + '</div>' + err + '</div>';
  }).join('');
}

function renderJobs(jobs){
  const box = $('#jobs');
  box.innerHTML = jobs.map(j => {
    const steps = j.steps.map(s => {
      const ic = {ok:ICON.ok, failed:ICON.bad, running:ICON.run}[s.status] || ICON.wait;
      const t = s.detail ? s.label + ' — ' + s.detail : s.label;
      return '<span class="step ' + s.status + '" title="' + esc(t) + '">' + ic
        + esc(s.status === 'ok' && s.detail ? s.detail : s.label) + '</span>';
    }).join('');
    const close = j.status === 'running' ? ''
      : '<button class="icon" data-job="' + esc(j.id) + '" title="关闭">' + ICON.x + '</button>';
    return '<div class="job"><div class="top"><strong>' + esc(j.summary) + '</strong>'
      + '<span class="count">' + j.done + '/' + j.total + '</span>'
      + '<span class="spacer"></span>' + close + '</div>'
      + '<div class="steps">' + steps + '</div></div>';
  }).join('');
}

async function poll(){
  try{
    view = await api('/api/exits');
    renderExits();
  }catch(e){}
  try{
    const d = await api('/api/nodes');
    const total = d.total != null ? d.total : (d.nodes || []).length;
    const avail = d.available != null ? d.available : total;
    $('#nodecount').textContent = '节点 ' + avail
      + (avail === total ? '' : ' / ' + total);
  }catch(e){}
  try{ renderJobs(await api('/api/jobs') || []); }catch(e){}
}

// ---- 新建向导 ----
let regions = [], region = '';

function openModal(id){ $('#' + id).classList.add('open'); }
function closeModal(id){ $('#' + id).classList.remove('open'); }

document.addEventListener('click', e => {
  const c = e.target.closest('[data-close]');
  if(c) closeModal(c.dataset.close);
});
document.addEventListener('keydown', e => {
  if(e.key === 'Escape') document.querySelectorAll('.modal.open')
    .forEach(m => m.classList.remove('open'));
});
document.querySelectorAll('.modal').forEach(m => {
  m.onclick = e => { if(e.target === m) m.classList.remove('open'); };
});

function renderRegions(){
  const kw = $('#rgfilter').value.trim().toLowerCase();
  const list = regions.filter(r => !kw
    || r.code.toLowerCase().includes(kw) || r.name.toLowerCase().includes(kw));
  $('#regions').innerHTML = ['<button class="rg' + (region === '' ? ' sel' : '')
      + '" data-rg=""><b>不限地区</b><em>速度优先</em></button>',
    '<button class="rg' + (region === '*' ? ' sel' : '')
      + '" data-rg="*"><b>每个国家</b><em>' + regions.length + ' 个国家各来几个</em></button>']
    .concat(list.map(r => '<button class="rg' + (region === r.code ? ' sel' : '')
      + '" data-rg="' + esc(r.code) + '"><b>' + esc(r.name || r.code) + '</b>'
      + '<em>' + r.available + ' 个空闲 · ' + r.best_speed_mbps.toFixed(0) + ' Mbps</em></button>'))
    .join('');
  updateAvail();
}

function availOf(code){
  if(code === '') return regions.reduce((a, r) => a + r.available, 0);
  if(code === '*') return regions.reduce((a, r) => a + r.available, 0);
  const r = regions.find(x => x.code === code);
  return r ? r.available : 0;
}

function updateAvail(){
  const want = Number($('#count').value) || 0;
  const hint = $('#availhint');
  // 选了"每个国家"时，数量的意思是每国几个，提示要给出总条数
  $('#countlabel').textContent = region === '*' ? '每个国家几个' : '数量';
  if(region === '*'){
    const n = regions.length;
    const total = Math.min(n * want, availOf('*'));
    hint.className = 'hint';
    hint.textContent = n
      ? n + ' 个国家 × ' + want + '，一共 ' + total + ' 条出口'
      : '还没有可用节点';
    $('#go').disabled = !n || !want;
    return;
  }
  const avail = availOf(region);
  hint.textContent = avail ? '可用 ' + avail + ' 个节点' : '这个地区没有空闲节点';
  hint.className = 'hint' + (want > avail ? ' bad' : '');
  if(want > avail && avail) hint.textContent = '只剩 ' + avail + ' 个，将全部使用';
  $('#go').disabled = !avail;
}

async function loadWizard(){
  try{
    regions = await api('/api/regions') || [];
    renderRegions();
  }catch(e){ toast('读取地区失败: ' + e.message, true); }
}

document.addEventListener('click', e => {
  if(e.target.closest('#newexit') || e.target.closest('#newexit2')){
    openModal('wizard');
    loadWizard();
  }
  const rg = e.target.closest('[data-rg]');
  if(rg){
    region = rg.dataset.rg;
    // "每个国家"是批量，默认每国 1 个，免得一点就开出几十条
    if(region === '*' && Number($('#count').value) > 3) $('#count').value = '1';
    renderRegions();
  }
});

$('#rgfilter').oninput = renderRegions;
$('#minus').onclick = () => { step(-1); };
$('#plus').onclick = () => { step(1); };
function step(d){
  const el = $('#count');
  el.value = Math.min(20, Math.max(1, (Number(el.value) || 1) + d));
  updateAvail();
}
$('#count').oninput = updateAvail;

$('#go').onclick = async e => {
  const want = Math.min(Number($('#count').value) || 1, availOf(region) || 1);
  e.target.disabled = true;
  try{
    await api('/api/provision?count=' + want
      + (region === '*' ? '&every=1' : '&region=' + encodeURIComponent(region)), {method:'POST'});
    closeModal('wizard');
    poll();
  }catch(err){ toast(err.message, true); }
  e.target.disabled = false;
};

// ---- 出口操作 ----
document.addEventListener('click', async e => {
  const stop = e.target.closest('[data-stop]');
  if(stop){
    stop.disabled = true;
    try{ await api('/api/stop?slot=' + stop.dataset.stop, {method:'POST'}); }
    catch(err){ toast(err.message, true); }
    poll();
    return;
  }
  const swap = e.target.closest('[data-swap]');
  if(swap){
    swap.disabled = true;
    try{
      await api('/api/swap?slot=' + swap.dataset.swap, {method:'POST'});
      toast('正在换节点');
    }catch(err){ toast(err.message, true); }
    poll();
    return;
  }
  const cred = e.target.closest('[data-cred]');
  if(cred){ openCred(Number(cred.dataset.cred)); return; }
  const job = e.target.closest('[data-job]');
  if(job){
    try{ await api('/api/jobs/dismiss?id=' + job.dataset.job, {method:'POST'}); }catch(err){}
    poll();
    return;
  }
});

$('#stopall').onclick = async e => {
  if(!confirm('停止全部 ' + view.exits.length + ' 个出口？')) return;
  e.target.disabled = true;
  for(const x of view.exits){
    try{ await api('/api/stop?slot=' + x.slot, {method:'POST'}); }catch(err){}
  }
  poll();
};

$('#refreshBtn').onclick = async e => {
  const btn = e.currentTarget;
  btn.disabled = true;
  try{
    const r = await api('/api/refresh', {method:'POST'});
    toast('已刷新，' + r.count + ' 个节点');
  }catch(err){ toast(err.message, true); }
  btn.disabled = false;
  poll();
};

document.addEventListener('click', e => {
  const c = e.target.closest('[data-copy]');
  if(c) copy(c.dataset.copy);
});

// ---- SOCKS5 凭据 ----
let curCred = null;

function socksURL(host, port, user, pass){
  if(!user) return 'socks5://' + host + ':' + port;
  return 'socks5://' + user + ':' + pass + '@' + host + ':' + port;
}

// SOCKS5 端口监听在母机（跑 fanout 的这台服务器）上，客户端要连的是母机的
// 公网 IPv4，流量再从出口 IP 出去。出口 IP 是"出去以后"的地址，不能当连接地址。
// public_ip 是后端探测到的母机公网地址；探测不到才退回访问面板用的主机名。
function credHost(e){
  return view.public_ip || location.hostname || e.host;
}

function openCred(slot){
  const e = view.exits.find(x => x.slot === slot);
  if(!e){ toast('这个出口不在了', true); return; }
  curCred = {slot: slot, port: e.port, host: credHost(e)};
  $('#crtitle').textContent = (e.country || e.region) + ' · :' + e.port;
  $('#cruser').value = e.socks_user || '';
  $('#crpass').value = e.socks_pass || '';
  refreshCredURL();
  openModal('credbox');
}

function refreshCredURL(){
  if(!curCred) return;
  $('#crurl').textContent = socksURL(curCred.host, curCred.port,
    $('#cruser').value.trim(), $('#crpass').value.trim());
}
$('#cruser').oninput = refreshCredURL;
$('#crpass').oninput = refreshCredURL;

$('#crrand').onclick = () => {
  // 客户端和服务端都要能识别，只用无歧义、无需转义的字符
  const abc = 'abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789';
  const gen = n => Array.from(crypto.getRandomValues(new Uint8Array(n)))
    .map(v => abc[v % abc.length]).join('');
  $('#cruser').value = 'fo' + gen(6);
  $('#crpass').value = gen(14);
  refreshCredURL();
};

$('#crcopy').onclick = () => { copy($('#crurl').textContent); };

$('#crsave').onclick = async e => {
  if(!curCred) return;
  const btn = e.target; btn.disabled = true;
  const q = new URLSearchParams({
    slot: curCred.slot,
    user: $('#cruser').value.trim(),
    pass: $('#crpass').value.trim(),
  });
  try{
    const r = await api('/api/cred?' + q, {method:'POST'});
    $('#cruser').value = r.user;
    $('#crpass').value = r.pass;
    refreshCredURL();
    toast('已保存，立即生效');
    poll();
  }catch(err){ toast(err.message, true); }
  btn.disabled = false;
};

// ---- 设置：改密码 / 改路径 / 改端口 / 改本地监听 ----
let curSettings = null;

$('#settingsBtn').onclick = async () => {
  $('#setPw').value = '';
  $('#setPath').value = '';
  $('#setPathHint').textContent = '读取中…';
  openModal('settings');
  try{
    const s = await api('/api/settings');
    curSettings = s;
    $('#setPath').value = (s.base_path || '').replace(/^\//, '');
    $('#setPort').value = s.port || '';
    $('#setListen').value = s.listen_addr || '0.0.0.0';
    $('#setResi').checked = s.residential_only !== false;
    $('#updCur').textContent = s.version || '-';
    $('#updLatest').textContent = '';
    $('#updNotes').hidden = true;
    $('#updApply').hidden = true;
    $('#updCheck').disabled = false;
    $('#updCheck').textContent = '检查更新';
    $('#setPathHint').textContent = '界面挂在这个路径下，扫端口的探不到。只能用字母数字和 - _。';
  }catch(err){ $('#setPathHint').textContent = '读取失败: ' + err.message; }
};

// 检查更新：问后端 GitHub 最新版，有新版就亮出更新按钮和更新内容
$('#updCheck').onclick = async e => {
  e.target.disabled = true;
  e.target.textContent = '检查中…';
  try{
    const u = await api('/api/update/check');
    $('#updCur').textContent = u.current || '-';
    if(u.has_update){
      $('#updLatest').textContent = '有新版本 ' + u.latest;
      $('#updApplyVer').textContent = u.latest;
      $('#updApply').hidden = false;
      $('#updNotes').textContent = u.notes || '（这个版本没写更新说明）';
      $('#updNotes').hidden = false;
    } else {
      $('#updLatest').textContent = '已是最新';
      $('#updApply').hidden = true;
      $('#updNotes').hidden = true;
    }
  }catch(err){ toast(err.message, true); }
  e.target.disabled = false;
  e.target.textContent = '检查更新';
};

// 一键更新：后端下载替换二进制并重启服务，进程重启期间界面会短暂断连
$('#updApply').onclick = async e => {
  if(!confirm('更新到 ' + $('#updApplyVer').textContent + '？服务会重启，界面会短暂断开。')) return;
  e.target.disabled = true;
  e.target.textContent = '更新中…';
  try{
    const r = await api('/api/update/apply', {method:'POST'});
    if(r.restarting){
      $('#updNotes').textContent = '已下载新版本，服务正在重启，几秒后刷新页面即可。';
      $('#updNotes').hidden = false;
      toast('更新中，服务重启后刷新页面');
      // 给服务重启留点时间再自动刷新
      setTimeout(() => location.reload(), 6000);
    } else {
      toast(r.message || '已是最新版');
      e.target.disabled = false;
      e.target.textContent = '更新到 ' + $('#updApplyVer').textContent;
    }
  }catch(err){
    toast(err.message, true);
    e.target.disabled = false;
    e.target.textContent = '更新到 ' + $('#updApplyVer').textContent;
  }
};

// 端口/监听地址变了要提示用户之后从新地址进；密码/路径可原地生效
function nextURL(port, listen, path){
  const host = (listen && listen !== '0.0.0.0') ? listen : location.hostname;
  return location.protocol + '//' + host + ':' + port + (path ? '/' + path : '') + '/';
}

$('#setSave').onclick = async e => {
  e.target.disabled = true;
  const body = {};
  const pw = $('#setPw').value.trim();
  if(pw) body.password = pw;
  body.base_path = $('#setPath').value.trim();
  const port = parseInt($('#setPort').value.trim(), 10);
  if(port) body.port = port;
  body.listen_addr = $('#setListen').value;
  body.residential_only = $('#setResi').checked;

  const portChanged = curSettings && (port !== curSettings.port
    || body.listen_addr !== (curSettings.listen_addr || '0.0.0.0'));

  try{
    await api('/api/settings', {
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body: JSON.stringify(body),
    });
    if(portChanged){
      const url = nextURL(port, body.listen_addr, body.base_path);
      $('#setPortHint').innerHTML = '监听已切换，请从新地址打开：<a href="' + esc(url) + '">' + esc(url) + '</a>';
      toast('监听已切换，用新地址重新打开');
      // 端口变了当前连接会断，不自动跳转，让用户看清新地址
    } else {
      toast('已保存');
      // 路径可能变了，重新加载到新路径下
      const np = body.base_path;
      const cur = (curSettings && curSettings.base_path || '').replace(/^\//, '');
      if(np !== cur){
        location.href = location.protocol + '//' + location.host
          + (np ? '/' + np : '') + '/';
        return;
      }
      closeModal('settings');
      poll();
    }
  }catch(err){ toast(err.message, true); }
  e.target.disabled = false;
};

poll();
setInterval(poll, 3000);
</script>
</body>
</html>`
