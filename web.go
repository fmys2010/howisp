package main

import (
	_ "embed"
	"net/http"
	"strings"
)

// worldMapSVG 是嵌进页面的世界地图：Natural Earth 1:110m 国界（公有领域），
// 等距圆柱投影、去掉南极洲，坐标取整后约 50KB。每个国家带 data-cc（两位国家码）
// 和 data-x/data-y（标注点），出口还没查到经纬度时就钉在所在国的标注点上。
//
// 不用在线地图瓦片：管理界面不该依赖外部 CDN，被墙或离线时地图照样能看。
//
//go:embed worldmap.svg
var worldMapSVG string

// indexPage 是嵌好地图的完整页面，启动时拼一次。
var indexPage = strings.Replace(indexHTML, "<!--WORLDMAP-->", worldMapSVG, 1)

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexPage))
}

const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>howisp</title>
<style>
:root{
  --bg:#0c1016; --panel:#131921; --panel2:#171e28; --line:#222b37; --line2:#2c3746;
  --text:#e2e8f0; --dim:#8b97a8; --faint:#5b6677;
  --accent:#4c9fe6; --accent-w:rgba(76,159,230,.14);
  --ok:#3fb27f; --ok-w:rgba(63,178,127,.14);
  --warn:#d6a23e; --warn-w:rgba(214,162,62,.14);
  --bad:#e0605b; --bad-w:rgba(224,96,91,.14);
  --land:#1a2230; --land-line:#273142; --land-hot:#1f3a57;
  --mono:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;
  --sans:-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Hiragino Sans GB","Microsoft YaHei",sans-serif;
}
*{box-sizing:border-box}
[hidden]{display:none!important}
body{margin:0;background:var(--bg);color:var(--text);font:13px/1.5 var(--sans)}
.mono{font-family:var(--mono);font-variant-numeric:tabular-nums}
header{position:sticky;top:0;z-index:20;display:flex;align-items:center;gap:14px;
  padding:10px 20px;border-bottom:1px solid var(--line);background:rgba(19,25,33,.92);
  backdrop-filter:blur(8px)}
.brand{display:flex;align-items:center;gap:9px}
.brand svg{width:20px;height:20px;color:var(--accent);stroke-width:2}
h1{font-size:15px;font-weight:650;margin:0;letter-spacing:.2px}
.pill{font-size:11px;color:var(--dim);border:1px solid var(--line);border-radius:999px;padding:1px 9px}
.spacer{flex:1}
button{font:inherit;color:var(--text);background:var(--panel2);border:1px solid var(--line2);
  border-radius:6px;padding:5px 11px;cursor:pointer;display:inline-flex;
  align-items:center;gap:6px;white-space:nowrap;transition:border-color .15s,background .15s,color .15s}
button:hover:not(:disabled){border-color:var(--accent)}
button:disabled{opacity:.45;cursor:default}
button.primary{background:var(--accent);border-color:var(--accent);color:#08111b;font-weight:600}
button.primary:hover:not(:disabled){background:#62adec}
button.icon{padding:5px;background:transparent;border-color:transparent;color:var(--dim)}
button.icon:hover:not(:disabled){color:var(--accent);border-color:var(--line2);background:var(--panel2)}
button.icon.danger:hover:not(:disabled){color:var(--bad);border-color:rgba(224,96,91,.4)}
svg{width:15px;height:15px;stroke:currentColor;fill:none;stroke-width:1.8;
  stroke-linecap:round;stroke-linejoin:round;flex:none}
.links{display:flex;gap:14px}
.links a{color:var(--dim);text-decoration:none;font-size:12px}
.links a:hover{color:var(--accent)}
main{padding:18px 20px 48px;max-width:1240px;margin:0 auto}

/* ---- 地图 ---- */
.mapcard{border:1px solid var(--line);border-radius:10px;background:var(--panel);overflow:hidden;margin-bottom:14px}
.maphead{display:flex;align-items:center;gap:14px;flex-wrap:wrap;padding:11px 16px;border-bottom:1px solid var(--line)}
.hostline{display:flex;align-items:center;gap:8px;min-width:0;flex-wrap:wrap;color:var(--dim);font-size:12px}
.hostline b{color:var(--text);font-weight:600}
.hostline .sep{color:var(--faint)}
.legend{display:flex;gap:14px;color:var(--dim);font-size:11px;margin-left:auto}
.legend span{display:inline-flex;align-items:center;gap:5px}
.legend i{width:8px;height:8px;border-radius:50%;display:inline-block}
.mapwrap{position:relative;background:radial-gradient(ellipse at 50% 40%,#111926 0%,#0d131c 70%)}
.mapscroll{overflow-x:auto}
#worldmap{display:block;width:100%;height:auto;max-height:440px;margin:0 auto}
#worldmap .land path{fill:var(--land);stroke:var(--land-line);stroke-width:1;vector-effect:non-scaling-stroke}
#worldmap .land path.hot{fill:var(--land-hot);stroke:#2f5579}
#arcs path{fill:none;stroke-width:1.4;vector-effect:non-scaling-stroke;opacity:.75}
#arcs path.up{stroke:var(--ok)}
#arcs path.starting{stroke:var(--warn);stroke-dasharray:5 5;animation:flow 1s linear infinite}
#arcs path.failed{stroke:var(--bad);stroke-dasharray:2 6;opacity:.5}
@keyframes flow{to{stroke-dashoffset:-10}}
.pin{cursor:pointer}
.pin .core{stroke:#0c1016;vector-effect:non-scaling-stroke;stroke-width:1.5}
.pin.up .core{fill:var(--ok)}
.pin.starting .core{fill:var(--warn)}
.pin.failed .core{fill:var(--bad)}
.pin .halo{fill:currentColor;opacity:.18}
.pin.up{color:var(--ok)} .pin.starting{color:var(--warn)} .pin.failed{color:var(--bad)}
.pin.starting .halo{animation:pulse 1.4s ease-in-out infinite}
.pin.hl .halo,.pin:hover .halo{opacity:.45}
.hostpin .core{fill:var(--accent);stroke:#0c1016;stroke-width:1.5;vector-effect:non-scaling-stroke}
.hostpin .ring{fill:none;stroke:var(--accent);stroke-width:1.5;vector-effect:non-scaling-stroke;opacity:.7}
@keyframes pulse{0%,100%{opacity:.12}50%{opacity:.45}}
.maptip{position:absolute;z-index:5;pointer-events:none;min-width:190px;max-width:280px;
  background:rgba(12,16,22,.96);border:1px solid var(--line2);border-radius:8px;padding:9px 11px;
  font-size:12px;box-shadow:0 8px 24px rgba(0,0,0,.35);transform:translate(-50%,calc(-100% - 14px))}
.maptip .t{font-weight:600;margin-bottom:3px}
.maptip .r{color:var(--dim)}
.mapempty{position:absolute;inset:0;display:flex;align-items:center;justify-content:center;
  color:var(--dim);font-size:13px;pointer-events:none}

/* ---- 概况 ---- */
.stats{display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:10px;margin-bottom:18px}
.stat{border:1px solid var(--line);border-radius:10px;background:var(--panel);padding:11px 14px}
.stat .k{color:var(--dim);font-size:11px}
.stat .v{font-size:20px;font-weight:650;margin-top:2px;font-family:var(--mono);font-variant-numeric:tabular-nums}
.stat .v small{font-size:12px;color:var(--faint);font-weight:500;margin-left:3px}

/* ---- 出口列表 ---- */
.bar{display:flex;align-items:center;gap:10px;margin-bottom:10px}
.bar h2{font-size:14px;margin:0;font-weight:650}
.count{color:var(--dim);font-size:12px}
.exit{border:1px solid var(--line);border-radius:10px;margin-bottom:9px;background:var(--panel);
  overflow:hidden;transition:border-color .2s,box-shadow .2s}
.exit:hover,.exit.hl{border-color:var(--line2)}
.exit.flash{border-color:var(--accent);box-shadow:0 0 0 3px var(--accent-w)}
.exit>.row{display:grid;align-items:center;gap:10px 18px;padding:12px 14px;
  grid-template-columns:minmax(210px,1.15fr) minmax(220px,1fr) minmax(0,1.25fr) auto;
  grid-template-areas:"id q proxy acts"}
.ex-id{grid-area:id;display:flex;gap:11px;align-items:flex-start;min-width:0}
.ex-q{grid-area:q;min-width:0}
.ex-proxy{grid-area:proxy;min-width:0}
.acts{grid-area:acts;display:flex;gap:2px;justify-self:end}
.dot{width:9px;height:9px;border-radius:50%;background:var(--faint);margin-top:6px;flex:none}
.dot.up{background:var(--ok);box-shadow:0 0 0 3px var(--ok-w)}
.dot.starting{background:var(--warn);animation:blink 1.2s ease-in-out infinite}
.dot.failed{background:var(--bad)}
@keyframes blink{0%,100%{opacity:1}50%{opacity:.3}}
.ip{font-family:var(--mono);font-size:14px;font-weight:600;font-variant-numeric:tabular-nums;
  display:flex;align-items:center;gap:7px;flex-wrap:wrap}
.loc{color:var(--dim);font-size:12px;margin-top:2px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.tag{display:inline-flex;align-items:center;font-size:10.5px;line-height:1;padding:3px 6px;
  border-radius:4px;border:1px solid var(--line2);color:var(--dim);font-family:var(--sans);font-weight:500}
.tag.src{border-color:rgba(214,162,62,.45);color:var(--warn)}
.tag.warp{border-color:rgba(244,129,32,.5);color:#f48120;background:rgba(244,129,32,.1)}
.tag.good{border-color:rgba(63,178,127,.45);color:var(--ok);background:var(--ok-w)}
.tag.meh{border-color:rgba(214,162,62,.45);color:var(--warn);background:var(--warn-w)}
.qline{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.risk{display:inline-flex;align-items:center;gap:7px}
.risk .n{font-family:var(--mono);font-weight:650;font-size:14px;min-width:22px;text-align:right}
.risk .meter{width:64px;height:5px;border-radius:3px;background:var(--line);overflow:hidden}
.risk .meter i{display:block;height:100%;border-radius:3px}
.risk .lbl{font-size:11px}
.lv-low .n,.lv-low .lbl{color:var(--ok)} .lv-low .meter i{background:var(--ok)}
.lv-mid .n,.lv-mid .lbl{color:var(--warn)} .lv-mid .meter i{background:var(--warn)}
.lv-high .n,.lv-high .lbl{color:var(--bad)} .lv-high .meter i{background:var(--bad)}
.lv-na .n,.lv-na .lbl{color:var(--faint)}
.asn{color:var(--dim);font-size:11.5px;margin-top:4px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.qwait{color:var(--faint);font-size:12px}
.proxy{display:flex;align-items:center;gap:4px;min-width:0;background:var(--bg);
  border:1px solid var(--line);border-radius:7px;padding:3px 4px 3px 3px}
.proxy .portbtn{background:var(--panel2);border-color:var(--line2);font-family:var(--mono);
  font-size:12px;padding:3px 8px;color:var(--text)}
.proxy .portbtn svg{width:12px;height:12px;stroke-width:2;color:var(--dim)}
.proxy .addr{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;
  color:var(--dim);font-size:12px;font-family:var(--mono)}
.errline{padding:0 14px 11px 34px;color:var(--bad);font-size:11.5px;
  overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.errline.note{color:var(--warn)}
.empty{border:1px dashed var(--line2);border-radius:10px;padding:44px 20px;
  text-align:center;color:var(--dim)}
.empty button{margin-top:14px}

/* ---- 作业 ---- */
.jobs{margin-bottom:4px}
.job{border:1px solid var(--line);border-radius:10px;background:var(--panel);padding:11px 14px;margin-bottom:12px}
.job .top{display:flex;align-items:center;gap:10px;margin-bottom:8px}
.job .top strong{font-weight:600;font-size:13px}
.steps{display:flex;flex-wrap:wrap;gap:6px}
.step{display:flex;align-items:center;gap:5px;font-size:11.5px;color:var(--dim);
  border:1px solid var(--line);border-radius:5px;padding:2px 8px;background:var(--bg)}
.step.ok{color:var(--ok);border-color:rgba(63,178,127,.35)}
.step.failed{color:var(--bad);border-color:rgba(224,96,91,.35)}
.step.running{color:var(--warn);border-color:rgba(214,162,62,.35)}
.spin{animation:rot 1s linear infinite;transform-origin:center}
@keyframes rot{to{transform:rotate(360deg)}}

/* ---- 弹窗 ---- */
.modal{position:fixed;inset:0;background:rgba(5,8,12,.72);display:none;
  align-items:center;justify-content:center;z-index:50;padding:20px}
.modal.open{display:flex}
.sheet{background:var(--bg);border:1px solid var(--line2);border-radius:12px;
  width:min(700px,100%);max-height:86vh;display:flex;flex-direction:column;
  box-shadow:0 24px 60px rgba(0,0,0,.45)}
.sheet .head{display:flex;align-items:center;gap:10px;padding:12px 16px;
  border-bottom:1px solid var(--line);background:var(--panel);border-radius:12px 12px 0 0}
.sheet .head h2{font-size:14px;margin:0;font-weight:650}
.sheet .body{overflow:auto;padding:16px}
.sheet .foot{display:flex;align-items:center;gap:10px;padding:12px 16px;
  border-top:1px solid var(--line);background:var(--panel);border-radius:0 0 12px 12px}
.fl{display:block;color:var(--dim);font-size:11.5px;margin-bottom:7px}
label.f{display:block;margin-bottom:18px}
label.f>span{display:block;color:var(--dim);font-size:11.5px;margin-bottom:7px}
.regions{display:grid;grid-template-columns:repeat(auto-fill,minmax(150px,1fr));
  gap:7px;max-height:236px;overflow:auto}
.rg{border:1px solid var(--line);background:var(--panel);border-radius:7px;padding:8px 10px;
  cursor:pointer;text-align:left;display:block;width:100%}
.rg:hover{border-color:var(--accent)}
.rg.sel{border-color:var(--accent);background:var(--accent-w)}
.rg b{font-weight:600;font-size:12.5px;display:block;overflow:hidden;
  text-overflow:ellipsis;white-space:nowrap}
.rg em{display:block;font-style:normal;color:var(--dim);font-size:11px;margin-top:2px}
.seg{display:flex;width:fit-content;border:1px solid var(--line2);border-radius:7px;
  overflow:hidden;background:var(--panel)}
.seg button{border:0;border-radius:0;background:transparent;padding:6px 16px}
.seg button+button{border-left:1px solid var(--line2)}
.seg button.sel{background:var(--accent-w);color:var(--accent);font-weight:600}
.stepper{display:flex;align-items:center;width:fit-content;
  border:1px solid var(--line2);border-radius:7px;overflow:hidden;background:var(--panel)}
.stepper button{border:0;border-radius:0;background:transparent;padding:6px 12px}
select,input[type=search],input[type=text],input[type=password]{font:inherit;background:var(--panel);
  border:1px solid var(--line2);color:var(--text);border-radius:7px;padding:7px 10px;width:100%}
select:focus,input:focus{outline:none;border-color:var(--accent)}
.stepper input[type=text]{width:58px;text-align:center;border:0;border-radius:0;
  border-left:1px solid var(--line2);border-right:1px solid var(--line2);
  padding:6px 0;font-family:var(--mono)}
.hint{color:var(--dim);font-size:11.5px;margin-top:7px}
.hint.bad{color:var(--bad)}
.setrow{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-top:18px}
label.chk{display:flex;align-items:center;gap:8px;color:var(--text);font-size:13px;cursor:pointer;margin:0}
label.chk input{margin:0}
.share{padding:10px 12px;background:var(--panel);border:1px solid var(--line);
  border-radius:7px;word-break:break-all;font-size:12px;line-height:1.7;margin-bottom:8px;font-family:var(--mono)}
.ef{display:block}
.ef>span{display:block;color:var(--dim);font-size:11.5px;margin-bottom:5px}
.credrow{display:flex;align-items:flex-end;gap:12px;flex-wrap:wrap;margin:6px 0 10px}
.credrow .ef input{width:200px;font-family:var(--mono)}
.linkbox{display:flex;align-items:center;gap:6px;background:var(--panel);border:1px solid var(--line2);
  border-radius:7px;padding:4px 4px 4px 11px}
.linkbox .addr{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;
  color:var(--dim);font-size:12px;font-family:var(--mono)}
button.danger{background:transparent;color:var(--bad);border-color:rgba(224,96,91,.45);padding:4px 10px}
button.danger:hover:not(:disabled){border-color:var(--bad);background:var(--bad-w)}
.lost{position:fixed;inset:0;z-index:90;display:flex;align-items:center;justify-content:center;
  background:rgba(5,8,12,.88);padding:20px}
.lost .card{max-width:420px;background:var(--panel);border:1px solid var(--line2);border-radius:12px;padding:22px 24px}
.lost h2{font-size:15px;margin:0 0 8px}
.lost p{margin:0;color:var(--dim);line-height:1.7}
.lost code{font-family:var(--mono);color:var(--text);background:var(--bg);border:1px solid var(--line);
  border-radius:4px;padding:1px 5px;white-space:nowrap}
.toast{position:fixed;left:50%;bottom:24px;transform:translateX(-50%);
  background:var(--panel2);border:1px solid var(--line2);border-radius:8px;
  padding:9px 15px;font-size:12.5px;z-index:80;opacity:0;pointer-events:none;
  transition:opacity .18s;box-shadow:0 8px 24px rgba(0,0,0,.35)}
.toast.show{opacity:1}
.toast.bad{border-color:rgba(224,96,91,.5);color:var(--bad)}
.updsec{margin-top:20px;padding-top:16px;border-top:1px solid var(--line)}
.updrow{display:flex;align-items:center;gap:10px}
.updver b{font-weight:600}
.updver span{color:var(--dim);margin-left:8px}
.updnotes{margin-top:10px;padding:10px 12px;background:var(--panel);border:1px solid var(--line);
  border-radius:7px;font-size:12px;line-height:1.6;color:var(--dim);white-space:pre-wrap;
  max-height:180px;overflow:auto}

@media(max-width:1000px){
  .exit>.row{grid-template-columns:minmax(0,1fr) minmax(0,1fr) auto;
    grid-template-areas:"id q acts" "proxy proxy proxy"}
}
@media(max-width:720px){
  .links{display:none}
  header{padding:10px 16px}
  main{padding:14px 16px 40px}
  .stats{grid-template-columns:repeat(3,minmax(0,1fr))}
  .stats .stat:nth-child(n+4){display:none}
  .exit>.row{grid-template-columns:minmax(0,1fr) auto;
    grid-template-areas:"id acts" "q q" "proxy proxy"}
  .legend{display:none}
  .pill{display:none}
  #worldmap{min-width:560px}
  .bar{flex-wrap:wrap}
  .setrow{grid-template-columns:1fr}
}
</style>
</head>
<body>
<header>
  <div class="brand">
    <svg viewBox="0 0 24 24"><circle cx="5" cy="12" r="2"/><path d="M7 12h4"/><path d="M11 12 19 5"/><path d="M11 12h8"/><path d="m11 12 8 7"/></svg>
    <h1>howisp</h1>
  </div>
  <span class="pill" id="nodecount" title="VPN Gate 节点数（可用 / 全部）"></span>
  <span class="spacer"></span>
  <button id="refreshBtn" title="重新抓取 VPN Gate 节点列表">
    <svg viewBox="0 0 24 24"><path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/></svg>
    刷新节点
  </button>
  <button class="icon" id="settingsBtn" title="设置">
    <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
  </button>
  <nav class="links">
    <a href="https://github.com/fmys2010/howisp" target="_blank" rel="noopener">GitHub</a>
  </nav>
</header>

<main>
  <div class="jobs" id="jobs"></div>

  <section class="mapcard">
    <div class="maphead">
      <div class="hostline" id="hostline">母机 <b>…</b></div>
      <div class="legend">
        <span><i style="background:var(--accent)"></i>母机</span>
        <span><i style="background:var(--ok)"></i>已连通</span>
        <span><i style="background:var(--warn)"></i>连接中</span>
        <span><i style="background:var(--bad)"></i>失败</span>
      </div>
    </div>
    <div class="mapwrap" id="mapwrap">
      <div class="mapscroll" id="mapscroll"><!--WORLDMAP--></div>
      <div class="maptip" id="maptip" hidden></div>
      <div class="mapempty" id="mapempty" hidden>还没有出口</div>
    </div>
  </section>

  <section class="stats" id="stats"></section>

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
      <div style="margin-bottom:18px">
        <span class="fl">出口来源</span>
        <div class="seg" id="pvseg">
          <button data-pv="">VPN Gate</button>
          <button data-pv="psiphon">Psiphon</button>
          <button data-pv="warp">Cloudflare WARP</button>
        </div>
        <div class="hint" id="pvhint"></div>
      </div>
      <label class="f" id="rgbox">
        <span>地区</span>
        <input type="search" id="rgfilter" placeholder="筛选地区">
        <div class="regions" id="regions" style="margin-top:7px"></div>
      </label>
      <label class="f" id="countbox">
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
      <h2>代理访问凭据</h2>
      <span class="count" id="crtitle"></span>
      <span class="spacer"></span>
      <button class="icon" data-close="credbox" title="关闭">
        <svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
      </button>
    </div>
    <div class="body">
      <div class="hint" style="margin:0 0 9px">这个端口同时支持 SOCKS5 和 HTTP 代理，两种地址都能用<span id="crnote"></span>：</div>
      <div class="share" id="crurl"></div>
      <div class="share" id="crurlhttp"></div>
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
      <span class="fl">登录链接</span>
      <div class="linkbox">
        <span class="addr" id="setLink">读取中…</span>
        <button class="icon" id="linkCopy" title="复制完整的登录链接">
          <svg viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
        </button>
        <button class="danger" id="linkReset" title="换一个密钥，旧链接和所有设备的登录一起作废">重置</button>
      </div>
      <div class="hint" style="margin-bottom:18px">在别的设备上打开一次这个链接就能登录，记住 30 天，常用会自动续期。链接就是钥匙，别发给别人。</div>

      <label class="chk"><input type="checkbox" id="setResi"> 只用家宽节点</label>
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

<div class="lost" id="lost" hidden>
  <div class="card">
    <h2>登录已失效</h2>
    <p>登录链接可能被重置了，或者这台设备太久没来过。在服务器上运行 <code>f info</code>
      （Docker 用 <code>docker exec fanout fanout -link</code>）看登录链接，再打开一次。</p>
  </div>
</div>

<div class="toast" id="toast"></div>

<script>
const $ = s => document.querySelector(s);
const ICON = {
  copy:'<svg viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>',
  stop:'<svg viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>',
  redo:'<svg viewBox="0 0 24 24"><path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/></svg>',
  gauge:'<svg viewBox="0 0 24 24"><path d="M12 14l4-4"/><path d="M3.3 17a9 9 0 1 1 17.4 0"/></svg>',
  ok:'<svg viewBox="0 0 24 24"><path d="M20 6 9 17l-5-5"/></svg>',
  bad:'<svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>',
  run:'<svg viewBox="0 0 24 24" class="spin"><path d="M21 12a9 9 0 1 1-6.2-8.5"/></svg>',
  wait:'<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/></svg>',
  x:'<svg viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>',
  lock:'<svg viewBox="0 0 24 24"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>'
};

// 请求一律走相对路径，换端口、套反代都不用改
async function api(path, opts){
  const r = await fetch(path.replace(/^\//, ''), opts);
  // 没登录时服务端对一切请求都回 404。界面能打开说明之前是登录着的，
  // 现在 404 就是 cookie 失效了（密钥在别处被重置、或太久没来），说清楚该怎么办
  if(r.status === 404){
    $('#lost').hidden = false;
    throw new Error('登录已失效');
  }
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

// ---- IP 质量（IPPure） ----
// 风险分 0-100，越高越可疑。分档是我们自己定的，只用来上色。
function riskLevel(score){
  if(score == null || score < 0) return ['na', '未知'];
  if(score <= 30) return ['low', '低风险'];
  if(score <= 60) return ['mid', '中风险'];
  return ['high', '高风险'];
}
function riskHTML(i){
  const s = i ? i.fraud_score : -1;
  const [lv, label] = riskLevel(s);
  const pct = s >= 0 ? Math.max(4, Math.min(100, s)) : 0;
  return '<span class="risk lv-' + lv + '" title="IPPure 风险系数，0-100，越高越可疑">'
    + '<span class="n">' + (s >= 0 ? s : '–') + '</span>'
    + '<span class="meter"><i style="width:' + pct + '%"></i></span>'
    + '<span class="lbl">' + label + '</span></span>';
}
function qualityTags(i){
  return '<span class="tag ' + (i.residential ? 'good' : '') + '" title="'
      + (i.residential ? '运营商家庭宽带' : '数据中心 / 机房 IP') + '">' + (i.residential ? '家宽' : '机房') + '</span>'
    + '<span class="tag ' + (i.broadcast ? 'meh' : 'good') + '" title="'
      + (i.broadcast ? '广播 IP：注册地与实际所在地不一致' : '原生 IP：注册地与所在地一致') + '">'
      + (i.broadcast ? '广播' : '原生') + '</span>';
}
function placeText(e){
  const i = e.ipinfo;
  const city = i ? [i.city, i.region].filter(Boolean).filter((v, k, a) => a.indexOf(v) === k).join(', ') : '';
  return (e.country || e.region || '—') + (city ? ' · ' + city : '');
}
function asnText(i){
  if(!i) return '';
  return (i.asn ? 'AS' + i.asn : '') + (i.asn && i.as_org ? ' · ' : '') + (i.as_org || '');
}

// ---- 地图 ----
// 和 worldmap.svg 的投影保持一致：等距圆柱，宽 2000，上边是北纬 84 度
const MAP_W = 2000, MAP_TOP = 84, MAP_K = MAP_W / 360;
const svgMap = $('#worldmap');
function project(lat, lon){ return [(lon + 180) * MAP_K, (MAP_TOP - lat) * MAP_K]; }
const ANCHOR = {};
svgMap.querySelectorAll('path[data-cc]').forEach(p => {
  ANCHOR[p.dataset.cc] = [Number(p.dataset.x), Number(p.dataset.y)];
});
// 1:110m 底图里没有这些小地方的轮廓，补上它们的经纬度
const EXTRA = {SG:[1.35,103.82], HK:[22.32,114.17], MO:[22.19,113.55], MT:[35.9,14.45],
  BH:[26.07,50.55], MU:[-20.3,57.58], AD:[42.51,1.52], LI:[47.16,9.55], MC:[43.74,7.42], SM:[43.94,12.46]};
function anchorOf(cc){
  cc = (cc || '').toUpperCase();
  if(ANCHOR[cc]) return ANCHOR[cc];
  if(EXTRA[cc]) return project(EXTRA[cc][0], EXTRA[cc][1]);
  return null;
}
function exitPoint(e){
  const i = e.ipinfo;
  if(i && i.has_geo) return project(i.lat, i.lon);
  return anchorOf((i && i.country_code) || e.region);
}
// 地图按容器宽度缩放，钉子的大小要按屏幕像素算，不然手机上小得看不见
function mapUnit(){
  const w = svgMap.getBoundingClientRect().width || MAP_W;
  return MAP_W / w;
}

function renderMap(){
  const u = mapUnit();
  const exits = view.exits;
  const host = view.host_info && view.host_info.has_geo ? project(view.host_info.lat, view.host_info.lon) : null;

  // 有出口的国家点亮
  const hot = new Set(exits.map(e => ((e.ipinfo && e.ipinfo.country_code) || e.region || '').toUpperCase()));
  svgMap.querySelectorAll('path[data-cc]').forEach(p => p.classList.toggle('hot', hot.has(p.dataset.cc)));

  // 同一个地方的出口（比如同城的几条 Psiphon）围成一圈摆开，别叠成一个点
  const groups = new Map();
  for(const e of exits){
    const pt = exitPoint(e);
    if(!pt) continue;
    const key = Math.round(pt[0] / (14 * u)) + ',' + Math.round(pt[1] / (14 * u));
    if(!groups.has(key)) groups.set(key, []);
    groups.get(key).push({e, pt});
  }
  const placed = [];
  for(const g of groups.values()){
    const n = g.length;
    g.forEach((it, k) => {
      let [x, y] = it.pt;
      if(n > 1){
        const a = 2 * Math.PI * k / n - Math.PI / 2, r = 11 * u;
        x += Math.cos(a) * r; y += Math.sin(a) * r;
      }
      placed.push({e: it.e, x, y});
    });
  }

  let arcs = '';
  if(host){
    for(const p of placed){
      const [hx, hy] = host, dx = p.x - hx, dy = p.y - hy;
      const d = Math.hypot(dx, dy);
      if(d < 4 * u) continue;
      // 控制点取中点往"上"拱，弧度随距离变大
      const mx = (hx + p.x) / 2, my = (hy + p.y) / 2;
      let nx = -dy / d, ny = dx / d;
      if(ny > 0){ nx = -nx; ny = -ny; }
      const bend = Math.min(d * 0.22, 160);
      arcs += '<path class="' + p.e.status + '" d="M' + hx.toFixed(1) + ' ' + hy.toFixed(1)
        + 'Q' + (mx + nx * bend).toFixed(1) + ' ' + (my + ny * bend).toFixed(1)
        + ' ' + p.x.toFixed(1) + ' ' + p.y.toFixed(1) + '"/>';
    }
  }
  $('#arcs').innerHTML = arcs;

  let pins = placed.map(p => '<g class="pin ' + p.e.status + '" data-pin="' + p.e.slot
    + '" transform="translate(' + p.x.toFixed(1) + ' ' + p.y.toFixed(1) + ')">'
    + '<circle class="halo" r="' + (11 * u).toFixed(1) + '"/>'
    + '<circle class="core" r="' + (5 * u).toFixed(1) + '"/></g>').join('');
  if(host){
    pins = '<g class="hostpin" data-host="1" transform="translate(' + host[0].toFixed(1) + ' ' + host[1].toFixed(1) + ')">'
      + '<circle class="ring" r="' + (10 * u).toFixed(1) + '"/>'
      + '<circle class="core" r="' + (4.5 * u).toFixed(1) + '"/></g>' + pins;
  }
  $('#pins').innerHTML = pins;
  $('#mapempty').hidden = exits.length > 0;

  // 窄屏上地图横向可滚，第一次画出钉子时把视野挪到钉子中间，别停在美洲
  const sc = $('#mapscroll');
  if(!mapCentered && sc.scrollWidth > sc.clientWidth + 4 && (placed.length || host)){
    const xs = placed.map(p => p.x).concat(host ? [host[0]] : []);
    const mid = (Math.min(...xs) + Math.max(...xs)) / 2 / u;
    sc.scrollLeft = Math.max(0, mid - sc.clientWidth / 2);
    mapCentered = true;
  }
}
let mapCentered = false;

function renderHost(){
  const h = view.host_info, ip = view.public_ip;
  let html = '<span>母机</span><b class="mono">' + esc(ip || (h && h.ip) || '…') + '</b>';
  if(h){
    const city = [h.city, h.country].filter(Boolean).join(', ');
    if(city) html += '<span class="sep">·</span><span>' + esc(city) + '</span>';
    if(h.as_org) html += '<span class="sep">·</span><span>' + esc(h.as_org) + '</span>';
    html += '<span class="sep">·</span>' + riskHTML(h);
  }
  $('#hostline').innerHTML = html;
}

// 悬停钉子出提示，点一下跳到对应那一行
const tip = $('#maptip');
function showTip(target, html){
  const wrap = $('#mapwrap').getBoundingClientRect();
  const r = target.getBoundingClientRect();
  tip.innerHTML = html;
  tip.hidden = false;
  let x = r.left + r.width / 2 - wrap.left, y = r.top - wrap.top;
  x = Math.max(100, Math.min(wrap.width - 100, x));
  tip.style.left = x + 'px';
  tip.style.top = Math.max(y, 60) + 'px';
}
svgMap.addEventListener('pointerover', ev => {
  const pin = ev.target.closest('[data-pin]');
  if(pin){
    const e = view.exits.find(x => String(x.slot) === pin.dataset.pin);
    if(!e) return;
    const i = e.ipinfo;
    showTip(pin, '<div class="t mono">' + esc(e.exit_ip || STATUS[e.status] || e.status) + '</div>'
      + '<div class="r">' + esc(placeText(e)) + '</div>'
      + (i ? '<div class="r">' + esc(asnText(i)) + '</div><div style="margin-top:5px">' + riskHTML(i) + '</div>' : '')
      + '<div class="r mono" style="margin-top:4px">:' + e.port + (e.provider === 'psiphon' ? ' · Psiphon' : e.provider === 'warp' ? ' · WARP' : '') + '</div>');
    highlight(e.slot, true);
    return;
  }
  const hp = ev.target.closest('[data-host]');
  if(hp){
    const h = view.host_info;
    showTip(hp, '<div class="t">母机 <span class="mono">' + esc(view.public_ip || (h && h.ip) || '') + '</span></div>'
      + (h ? '<div class="r">' + esc([h.city, h.country].filter(Boolean).join(', ')) + '</div><div class="r">'
      + esc(asnText(h)) + '</div>' : ''));
  }
});
svgMap.addEventListener('pointerout', ev => {
  if(ev.target.closest('[data-pin],[data-host]')){ tip.hidden = true; highlight(null); }
});
svgMap.addEventListener('click', ev => {
  const pin = ev.target.closest('[data-pin]');
  if(!pin) return;
  const row = document.getElementById('exit-' + pin.dataset.pin);
  if(!row) return;
  row.scrollIntoView({behavior:'smooth', block:'center'});
  row.classList.add('flash');
  setTimeout(() => row.classList.remove('flash'), 1400);
});
function highlight(slot, on){
  document.querySelectorAll('.pin.hl, .exit.hl').forEach(el => el.classList.remove('hl'));
  if(slot == null || !on) return;
  const pin = svgMap.querySelector('[data-pin="' + slot + '"]');
  if(pin) pin.classList.add('hl');
  const row = document.getElementById('exit-' + slot);
  if(row) row.classList.add('hl');
}
$('#list').addEventListener('pointerover', ev => {
  const row = ev.target.closest('.exit');
  if(row) highlight(row.dataset.slot, true);
});
$('#list').addEventListener('pointerleave', () => highlight(null));
let resizeTimer;
window.addEventListener('resize', () => { clearTimeout(resizeTimer); resizeTimer = setTimeout(renderMap, 150); });

// ---- 概况 ----
function renderStats(){
  const ex = view.exits;
  const up = ex.filter(e => e.status === 'up').length;
  const withInfo = ex.filter(e => e.ipinfo);
  const resi = withInfo.filter(e => e.ipinfo.residential).length;
  const native = withInfo.filter(e => !e.ipinfo.broadcast).length;
  const scored = withInfo.filter(e => e.ipinfo.fraud_score >= 0);
  const avg = scored.length ? Math.round(scored.reduce((a, e) => a + e.ipinfo.fraud_score, 0) / scored.length) : -1;
  const [lv] = riskLevel(avg);
  const of = n => '<small>/ ' + n + '</small>';
  const tile = (k, v) => '<div class="stat"><div class="k">' + k + '</div><div class="v">' + v + '</div></div>';
  $('#stats').innerHTML = tile('出口', ex.length)
    + tile('已连通', up + of(ex.length))
    + tile('平均风险', '<span class="risk lv-' + lv + '"><span class="n" style="font-size:20px;text-align:left">'
      + (avg >= 0 ? avg : '–') + '</span></span>')
    + tile('家宽', resi + of(withInfo.length))
    + tile('原生 IP', native + of(withInfo.length));
}

// ---- 出口列表 ----
function renderExits(){
  const list = $('#list');
  const n = view.exits.length;
  $('#ecount').textContent = n ? n + ' 个' : '';
  $('#stopall').disabled = !n;

  if(!n){
    list.innerHTML = '<div class="empty">还没有出口，一个出口就是一个代理端口'
      + '<div><button class="primary" id="newexit2">'
      + '<svg viewBox="0 0 24 24"><path d="M12 5v14"/><path d="M5 12h14"/></svg>'
      + '新建出口</button></div></div>';
    return;
  }

  list.innerHTML = view.exits.map(e => {
    const label = e.exit_ip || (e.status === 'starting' ? '连接中…' : '—');
    // starting 时后端会把"N 秒后重试（最后错误）"写在 err 里，也值得看到
    const err = (e.status === 'failed' || e.status === 'starting') && e.err
      ? '<div class="errline' + (e.status === 'starting' ? ' note' : '') + '" title="' + esc(e.err) + '">' + esc(e.err) + '</div>' : '';
    const psi = e.provider === 'psiphon', warp = e.provider === 'warp';
    const src = '<span class="tag' + (psi ? ' src' : warp ? ' warp' : '') + '" title="'
      + (psi ? 'Psiphon 出口：服务器由 Psiphon 自动挑，IP 可能随重连变化，只转 TCP'
        : warp ? 'Cloudflare WARP（官方客户端代理模式）：出口是 Cloudflare 的 IP，只转 TCP'
        : 'VPN Gate 志愿者节点：' + esc(e.host))
      + '">' + (psi ? 'Psiphon' : warp ? 'WARP' : 'VPN Gate') + '</span>';
    const url = socksURL(credHost(e), e.port, e.socks_user, e.socks_pass);
    const i = e.ipinfo;
    let q;
    if(i){
      q = '<div class="qline">' + riskHTML(i) + qualityTags(i) + '</div>'
        + '<div class="asn" title="' + esc(asnText(i)) + '">' + esc(asnText(i)) + '</div>';
    } else if(e.status === 'up'){
      q = '<div class="qwait">IP 质量查询中…</div>';
    } else {
      q = '<div class="qwait">连通后查询 IP 质量</div>';
    }
    return '<div class="exit" id="exit-' + e.slot + '" data-slot="' + e.slot + '">'
      + '<div class="row">'
      +   '<div class="ex-id"><span class="dot ' + e.status + '" title="' + (STATUS[e.status] || e.status) + '"></span>'
      +     '<div style="min-width:0"><div class="ip">' + esc(label) + src + '</div>'
      +     '<div class="loc" title="' + esc(placeText(e)) + '">' + esc(placeText(e)) + '</div></div></div>'
      +   '<div class="ex-q">' + q + '</div>'
      +   '<div class="ex-proxy"><div class="proxy">'
      +     '<button class="portbtn" data-cred="' + e.slot + '" title="代理访问凭据（改口令）">' + ICON.lock + ':' + e.port + '</button>'
      +     '<span class="addr" title="' + esc(url) + '">' + esc(url) + '</span>'
      +     '<button class="icon" data-copy="' + esc(url) + '" title="复制 SOCKS5 地址（同一端口也支持 HTTP 代理）">' + ICON.copy + '</button>'
      +   '</div></div>'
      +   '<div class="acts">'
      +     '<button class="icon" data-iq="' + e.slot + '" title="重新查询 IP 质量"' + (e.status === 'up' ? '' : ' disabled') + '>' + ICON.gauge + '</button>'
      +     '<button class="icon" data-swap="' + e.slot + '" title="' + (psi ? '换一个出口 IP（让 Psiphon 重新挑服务器）' : warp ? '换一个出口 IP（重新注册 WARP 设备）' : '换一个节点') + '">' + ICON.redo + '</button>'
      +     '<button class="icon danger" data-stop="' + e.slot + '" title="停止这个出口">' + ICON.stop + '</button>'
      +   '</div>'
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
    renderHost();
    renderMap();
    renderStats();
    renderExits();
  }catch(e){}
  try{
    const d = await api('/api/nodes');
    const total = d.total != null ? d.total : (d.nodes || []).length;
    const avail = d.available != null ? d.available : total;
    $('#nodecount').textContent = 'VPN Gate 节点 ' + avail
      + (avail === total ? '' : ' / ' + total);
  }catch(e){}
  try{ renderJobs(await api('/api/jobs') || []); }catch(e){}
}

// ---- 新建向导 ----
// provider 为空是 VPN Gate，'psiphon' 是 Psiphon。两边的地区清单和"可用数"含义不同：
// VPN Gate 是空闲节点数，Psiphon 不挑节点、同地区开几条都行，可用数就是剩余槽位。
let regions = [], region = '', provider = '';
const PVHINT = {
  '': 'VPN Gate 志愿者节点，一条出口钉一个节点，支持 UDP。',
  psiphon: 'Psiphon 机房出口：服务器由 Psiphon 在地区内自动挑，IP 可能随重连变化；只转 TCP，域名在远端解析。首次使用会下载 psiphon-tunnel-core（仅 x86_64）。',
  warp: 'Cloudflare WARP：官方客户端的代理模式，出口是 Cloudflare 的 IP，地区跟着本机走、不能选；只转 TCP。最多一个。首次使用会从 Cloudflare 官方软件源安装 cloudflare-warp。'
};
const hasWarp = () => !!(view && view.exits && view.exits.some(e => e.provider === 'warp'));

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
  const psi = provider === 'psiphon';
  document.querySelectorAll('#pvseg [data-pv]').forEach(b =>
    b.classList.toggle('sel', b.dataset.pv === provider));
  $('#pvhint').textContent = PVHINT[provider] || '';
  // WARP 不选地区也不选数量：只有一个
  const warp = provider === 'warp';
  $('#rgbox').hidden = warp;
  $('#countbox').hidden = warp;
  if(warp){
    $('#go').disabled = hasWarp();
    $('#wzhint').textContent = hasWarp() ? '已经有一条 WARP 出口，先停掉再开' : '';
    return;
  }
  $('#wzhint').textContent = '';
  $('#regions').innerHTML = ['<button class="rg' + (region === '' ? ' sel' : '')
      + '" data-rg=""><b>不限地区</b><em>' + (psi ? '由 Psiphon 挑最快的' : '速度优先') + '</em></button>',
    '<button class="rg' + (region === '*' ? ' sel' : '')
      + '" data-rg="*"><b>每个国家</b><em>' + regions.length + ' 个国家各来几个</em></button>']
    .concat(list.map(r => '<button class="rg' + (region === r.code ? ' sel' : '')
      + '" data-rg="' + esc(r.code) + '"><b>' + esc(r.name || r.code) + '</b>'
      + '<em>' + (psi ? esc(r.code) + ' · 按需连接'
        : r.available + ' 个空闲 · ' + r.best_speed_mbps.toFixed(0) + ' Mbps') + '</em></button>'))
    .join('');
  updateAvail();
}

// Psiphon 每个地区的 available 都是剩余槽位，不能按地区相加
function roomOf(){ return regions.length ? regions[0].available : 0; }

function availOf(code){
  if(provider === 'psiphon') return roomOf();
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
  if(provider === 'psiphon'){
    hint.textContent = avail ? '还能开 ' + avail + ' 条' : '槽位已满，先停几个出口';
    hint.className = 'hint' + (want > avail ? ' bad' : '');
    if(want > avail && avail) hint.textContent = '只剩 ' + avail + ' 个槽位，将全部使用';
    $('#go').disabled = !avail;
    return;
  }
  hint.textContent = avail ? '可用 ' + avail + ' 个节点' : '这个地区没有空闲节点';
  hint.className = 'hint' + (want > avail ? ' bad' : '');
  if(want > avail && avail) hint.textContent = '只剩 ' + avail + ' 个，将全部使用';
  $('#go').disabled = !avail;
}

async function loadWizard(){
  if(provider === 'warp'){ regions = []; renderRegions(); return; }
  try{
    regions = await api('/api/regions' + (provider ? '?provider=' + provider : '')) || [];
    renderRegions();
  }catch(e){ toast('读取地区失败: ' + e.message, true); }
}

document.addEventListener('click', e => {
  if(e.target.closest('#newexit') || e.target.closest('#newexit2')){
    openModal('wizard');
    loadWizard();
  }
  const pv = e.target.closest('[data-pv]');
  if(pv && pv.dataset.pv !== provider){
    // 换来源后地区清单完全不同，选中的地区作废
    provider = pv.dataset.pv;
    region = '';
    regions = [];
    renderRegions();
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
  const btn = e.currentTarget;
  const warp = provider === 'warp';
  const want = warp ? 1 : Math.min(Number($('#count').value) || 1, availOf(region) || 1);
  btn.disabled = true;
  try{
    if(warp) await api('/api/provision?count=1&provider=warp', {method:'POST'});
    else await api('/api/provision?count=' + want
      + (provider ? '&provider=' + provider : '')
      + (region === '*' ? '&every=1' : '&region=' + encodeURIComponent(region)), {method:'POST'});
    closeModal('wizard');
    poll();
  }catch(err){ toast(err.message, true); }
  btn.disabled = false;
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
  const iq = e.target.closest('[data-iq]');
  if(iq){
    iq.disabled = true;
    try{
      await api('/api/ipinfo/refresh?slot=' + iq.dataset.iq, {method:'POST'});
      toast('正在重新查询 IP 质量');
    }catch(err){ toast(err.message, true); }
    setTimeout(poll, 3000);
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
  const btn = e.currentTarget;
  if(!confirm('停止全部 ' + view.exits.length + ' 个出口？')) return;
  btn.disabled = true;
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

// 同一个端口同时支持 SOCKS5 与 HTTP 代理，两种地址都给出来
function socksURL(host, port, user, pass){
  if(!user) return 'socks5://' + host + ':' + port;
  return 'socks5://' + user + ':' + pass + '@' + host + ':' + port;
}
function httpProxyURL(host, port, user, pass){
  if(!user) return 'http://' + host + ':' + port;
  return 'http://' + user + ':' + pass + '@' + host + ':' + port;
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
  $('#crnote').textContent = e.provider === 'psiphon' ? '（Psiphon 出口只转 TCP，SOCKS5 的 UDP 不可用）'
    : e.provider === 'warp' ? '（WARP 出口只转 TCP，SOCKS5 的 UDP 不可用）' : '';
  $('#crtitle').textContent = (e.country || e.region) + ' · :' + e.port;
  $('#cruser').value = e.socks_user || '';
  $('#crpass').value = e.socks_pass || '';
  refreshCredURL();
  openModal('credbox');
}

function refreshCredURL(){
  if(!curCred) return;
  const host = curCred.host, port = curCred.port;
  const user = $('#cruser').value.trim(), pass = $('#crpass').value.trim();
  $('#crurl').textContent = socksURL(host, port, user, pass);
  $('#crurlhttp').textContent = httpProxyURL(host, port, user, pass);
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
  const btn = e.currentTarget; btn.disabled = true;
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

// ---- 设置：登录链接 / 改端口 / 改本地监听 / 开关 ----
let curSettings = null, curKey = '';

// 登录链接用当前地址拼：用户从哪个地址打开的面板，在别的设备上也该用这个地址
function loginURL(key){ return location.protocol + '//' + location.host + '/?k=' + encodeURIComponent(key); }
// 面板上只露头尾，免得被旁边的人看去；复制出来的是完整链接
function showLink(key){
  curKey = key || '';
  $('#setLink').textContent = curKey
    ? location.protocol + '//' + location.host + '/?k=' + curKey.slice(0, 4) + '…' + curKey.slice(-4)
    : '读取失败';
}
$('#linkCopy').onclick = () => { if(curKey) copy(loginURL(curKey)); };
$('#linkReset').onclick = async e => {
  if(!confirm('重置登录链接？旧链接作废，其他设备都要用新链接重新登录（这个浏览器不受影响）。')) return;
  const btn = e.currentTarget;
  btn.disabled = true;
  try{
    const r = await api('/api/key/reset', {method:'POST'});
    showLink(r.login_key);
    toast('已重置，新链接已生成');
  }catch(err){ toast(err.message, true); }
  btn.disabled = false;
};

$('#settingsBtn').onclick = async () => {
  $('#setLink').textContent = '读取中…';
  openModal('settings');
  try{
    const s = await api('/api/settings');
    curSettings = s;
    showLink(s.login_key);
    $('#setPort').value = s.port || '';
    $('#setListen').value = s.listen_addr || '0.0.0.0';
    $('#setResi').checked = s.residential_only !== false;
    $('#updCur').textContent = s.version || '-';
    $('#updLatest').textContent = '';
    $('#updNotes').hidden = true;
    $('#updApply').hidden = true;
    $('#updCheck').disabled = false;
    $('#updCheck').textContent = '检查更新';
  }catch(err){ $('#setLink').textContent = '读取失败: ' + err.message; }
};

// 检查更新：问后端 GitHub 最新版，有新版就亮出更新按钮和更新内容
$('#updCheck').onclick = async e => {
  const btn = e.currentTarget;
  btn.disabled = true;
  btn.textContent = '检查中…';
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
  btn.disabled = false;
  btn.textContent = '检查更新';
};

// 一键更新：后端下载替换二进制并重启服务，进程重启期间界面会短暂断连
$('#updApply').onclick = async e => {
  const btn = e.currentTarget;
  const ver = $('#updApplyVer').textContent;
  if(!confirm('更新到 ' + ver + '？服务会重启，界面会短暂断开。')) return;
  btn.disabled = true;
  btn.textContent = '更新中…';
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
      btn.disabled = false;
      btn.innerHTML = '更新到 <span id="updApplyVer">' + esc(ver) + '</span>';
    }
  }catch(err){
    toast(err.message, true);
    btn.disabled = false;
    btn.innerHTML = '更新到 <span id="updApplyVer">' + esc(ver) + '</span>';
  }
};

// 端口/监听地址变了要提示用户之后从新地址进。cookie 不分端口，换了端口照样是登录状态
function nextURL(port, listen){
  const host = (listen && listen !== '0.0.0.0') ? listen : location.hostname;
  return location.protocol + '//' + host + ':' + port + '/';
}

$('#setSave').onclick = async e => {
  const btn = e.currentTarget;
  btn.disabled = true;
  const body = {};
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
      const url = nextURL(port, body.listen_addr);
      $('#setPortHint').innerHTML = '监听已切换，请从新地址打开：<a href="' + esc(url) + '">' + esc(url) + '</a>';
      toast('监听已切换，用新地址重新打开');
      // 端口变了当前连接会断，不自动跳转，让用户看清新地址
    } else {
      toast('已保存');
      closeModal('settings');
      poll();
    }
  }catch(err){ toast(err.message, true); }
  btn.disabled = false;
};

poll();
setInterval(poll, 3000);
</script>
</body>
</html>`
