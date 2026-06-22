package admin

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>PaqetPremium</title>
<style>
:root{--bg:#0e1116;--panel:#161b22;--mut:#8b949e;--brand:#a78bfa;--ok:#34d399;--bad:#f87171;--line:#222a35}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:#e6edf3;font:14px/1.5 ui-sans-serif,system-ui,Segoe UI,Roboto,sans-serif}
.wrap{max-width:980px;margin:0 auto;padding:24px}
header{display:flex;align-items:baseline;gap:12px;margin-bottom:4px}
h1{font-size:20px;margin:0;color:var(--brand);letter-spacing:.3px}
.sub{color:var(--mut);font-size:13px}
.meta{color:var(--mut);font-size:12px;margin-bottom:18px}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:12px;margin-bottom:18px}
.card{background:var(--panel);border:1px solid var(--line);border-radius:12px;padding:14px}
.card .k{color:var(--mut);font-size:12px;text-transform:uppercase;letter-spacing:.5px}
.card .v{font-size:22px;font-weight:600;margin-top:6px}
.card .v small{font-size:12px;color:var(--mut);font-weight:400}
table{width:100%;border-collapse:collapse;background:var(--panel);border:1px solid var(--line);border-radius:12px;overflow:hidden}
th,td{padding:10px 12px;text-align:left;border-bottom:1px solid var(--line);font-size:13px}
th{color:var(--mut);font-weight:600;text-transform:uppercase;font-size:11px;letter-spacing:.5px}
tr:last-child td{border-bottom:0}
.badge{display:inline-block;padding:2px 8px;border-radius:999px;font-size:11px;font-weight:600}
.b-ok{background:rgba(52,211,153,.15);color:var(--ok)}
.b-bad{background:rgba(248,113,113,.15);color:var(--bad)}
.dot{display:inline-block;width:8px;height:8px;border-radius:50%;margin-right:6px;vertical-align:middle}
.d-ok{background:var(--ok)}
.foot{color:var(--mut);font-size:12px;margin-top:14px}
.err{color:var(--bad)}
h2{font-size:13px;color:var(--mut);text-transform:uppercase;letter-spacing:.5px;margin:18px 0 8px}
.cfg{background:var(--panel);border:1px solid var(--line);border-radius:12px;padding:14px;margin-top:8px}
.cfgbar{display:flex;align-items:center;gap:8px;margin-bottom:10px;flex-wrap:wrap}
button{background:var(--brand);color:#0e1116;border:0;border-radius:8px;padding:7px 12px;font-weight:600;cursor:pointer;font-size:13px}
button:hover{filter:brightness(1.08)}
button.ghost{background:transparent;color:var(--brand);border:1px solid var(--line)}
#cfgText{width:100%;min-height:320px;background:#0b0f14;color:#e6edf3;border:1px solid var(--line);border-radius:8px;padding:12px;font:12px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;resize:vertical}
.cfgmsg{font-size:12px;color:var(--mut)}
.cfgmsg.ok{color:var(--ok)}
.cfgmsg.err{color:var(--bad)}
.cfgnote{color:var(--mut);font-size:12px;margin-top:8px}
</style>
</head>
<body>
<div class="wrap">
  <header><h1>PaqetPremium</h1><span class="sub" id="role"></span></header>
  <div class="meta" id="meta">connecting...</div>
  <div class="grid" id="cards"></div>
  <div id="upstreams"></div>
  <h2>Configuration</h2>
  <div class="cfg">
    <div class="cfgbar">
      <button id="cfgLoad" class="ghost">Reload from server</button>
      <button id="cfgSave">Save &amp; Apply</button>
      <span id="cfgMsg" class="cfgmsg"></span>
    </div>
    <textarea id="cfgText" spellcheck="false" placeholder="loading config..."></textarea>
    <div class="cfgnote">Secrets are shown as ***REDACTED*** — leave them unchanged to keep the current value. Saving validates the config, writes it to disk, and reloads the running service.</div>
  </div>
  <div class="foot" id="foot"></div>
</div>
<script>
const params=new URLSearchParams(location.search);
const token=params.get('token')||'';
let prev=null,prevT=0;
function fmtBytes(n){const u=['B','KB','MB','GB','TB'];let i=0;n=Number(n||0);while(n>=1024&&i<u.length-1){n/=1024;i++;}return n.toFixed(i?1:0)+' '+u[i];}
function fmtRate(bps){return fmtBytes(bps)+'/s';}
function card(k,v,sub){return '<div class="card"><div class="k">'+k+'</div><div class="v">'+v+(sub?' <small>'+sub+'</small>':'')+'</div></div>';}
function esc(x){return String(x==null?'':x).replace(/[&<>"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]));}
async function tick(){
  try{
    const url='/api/v1/status'+(token?('?token='+encodeURIComponent(token)):'');
    const r=await fetch(url,{cache:'no-store'});
    if(!r.ok)throw new Error('HTTP '+r.status);
    render(await r.json());
  }catch(e){document.getElementById('meta').innerHTML='<span class="err">error: '+esc(e.message)+(token?'':' (a token may be required: add ?token=...)')+'</span>';}
}
function render(s){
  document.getElementById('role').textContent=(s.role||'')+(s.name?(' - '+s.name):'');
  document.getElementById('meta').textContent='v'+(s.version||'?')+(s.strategy?(' - strategy '+s.strategy):'')+(s.active_upstream?(' - active '+s.active_upstream):'')+(s.listen_port?(' - :'+s.listen_port):'');
  const st=s.stats||{};
  const now=Date.now();let inR=0,outR=0;
  if(prev&&prevT){const dt=(now-prevT)/1000;if(dt>0){inR=Math.max(0,(st.bytes_in-prev.bytes_in)/dt);outR=Math.max(0,(st.bytes_out-prev.bytes_out)/dt);}}
  prev=st;prevT=now;
  document.getElementById('cards').innerHTML=[
    card('Down',fmtBytes(st.bytes_in),fmtRate(inR)),
    card('Up',fmtBytes(st.bytes_out),fmtRate(outR)),
    card('Sessions',s.sessions||0),
    card('TCP active',st.tcp_active||0,'/ '+(st.tcp_accepted||0)+' total'),
    card('UDP pkts',st.udp_packets||0),
    card('Relay',(st.relay_tcp||0)+' tcp',(st.relay_udp||0)+' udp'),
    card('UDP dgram',st.udp_dgram_flows||0,(st.udp_dgram_out||0)+' out / '+(st.udp_dgram_in||0)+' in, '+(st.udp_dgram_dropped||0)+' drop'),
    card('Errors',st.errors||0)
  ].join('');
  let up=s.upstreams,html='';
  if(Array.isArray(up)&&up.length){
    html='<h2>Upstreams</h2><table><tr><th>Name</th><th>Address</th><th>Health</th><th>RTT</th><th>Sessions</th><th>Fails</th></tr>';
    for(const u of up){
      html+='<tr><td>'+(u.active?'<span class="dot d-ok"></span>':'')+esc(u.name)+'</td><td>'+esc(u.addr)+'</td>'+
        '<td><span class="badge '+(u.healthy?'b-ok':'b-bad')+'">'+(u.healthy?'healthy':'down')+'</span></td>'+
        '<td>'+(u.rtt_ms?u.rtt_ms.toFixed(1)+' ms':'-')+'</td><td>'+(u.sessions||0)+'</td><td>'+(u.failures||0)+'</td></tr>';
    }
    html+='</table>';
  }
  document.getElementById('upstreams').innerHTML=html;
  document.getElementById('foot').textContent='updated '+new Date().toLocaleTimeString();
}
function cfgURL(){return '/api/v1/config'+(token?('?token='+encodeURIComponent(token)):'');}
async function cfgLoad(){
  const m=document.getElementById('cfgMsg');m.textContent='loading...';m.className='cfgmsg';
  try{
    const r=await fetch(cfgURL(),{cache:'no-store'});
    const t=await r.text();
    if(!r.ok)throw new Error(t||('HTTP '+r.status));
    document.getElementById('cfgText').value=t;
    m.textContent='loaded';m.className='cfgmsg';
  }catch(e){m.textContent='error: '+esc(e.message);m.className='cfgmsg err';}
}
async function cfgSave(){
  const m=document.getElementById('cfgMsg');m.textContent='saving...';m.className='cfgmsg';
  try{
    const r=await fetch(cfgURL(),{method:'POST',headers:{'Content-Type':'application/x-yaml'},body:document.getElementById('cfgText').value});
    const t=await r.text();
    if(!r.ok)throw new Error(t||('HTTP '+r.status));
    m.textContent='saved & applied';m.className='cfgmsg ok';
  }catch(e){m.textContent='error: '+esc(e.message);m.className='cfgmsg err';}
}
document.getElementById('cfgLoad').addEventListener('click',cfgLoad);
document.getElementById('cfgSave').addEventListener('click',cfgSave);
cfgLoad();
tick();setInterval(tick,2000);
</script>
</body>
</html>`
