// Monitor Hub — Network, Provider Fleet, Oracle/BME
// Plus tertiary views: Log Viewer, Shell, Workflow

const { useState: useS2, useEffect: useE2, useMemo: useM2, useRef: useR2 } = React;

// ─────────────────────────────────────────────────────────────
// Monitor Hub container
// ─────────────────────────────────────────────────────────────
function ViewMonitorHub({ ctx }) {
  const [tab, setTab] = useS2('network');     // network | fleet | oracle
  const [sub, setSub] = useS2('overview');    // for network: overview | validators | params
  const tabs = ['network', 'fleet', 'oracle'];
  const networkSubs = ['overview', 'validators', 'params'];

  ctx.useKey((e) => {
    if (e.key === 'Tab')       { setTab(t => tabs[(tabs.indexOf(t) + 1) % tabs.length]); setSub('overview'); return true; }
    if (tab === 'network' && /^[1-3]$/.test(e.key)) { setSub(networkSubs[parseInt(e.key) - 1]); return true; }
    if (tab === 'fleet'  && e.key === 'Enter')      { ctx.push({ view: 'node-detail' }); return true; }
    return false;
  });

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: 14, gap: 10 }}>
      {/* hub tabs */}
      <div style={{ display: 'flex', gap: 0, borderBottom: `1px solid ${C.border}` }}>
        {[
          ['network', 'Network',       'consensus · validators · params'],
          ['fleet',   'Provider Fleet','version dist · health scan'],
          ['oracle',  'Oracle / BME',  'prices · vault · ledger'],
        ].map(([id, label, hint]) => {
          const sel = tab === id;
          return (
            <div key={id} onClick={() => { setTab(id); setSub('overview'); }} style={{
              padding: '8px 16px', cursor: 'pointer',
              borderTop:    `2px solid ${sel ? C.red : 'transparent'}`,
              borderBottom: sel ? 'none' : `1px solid ${C.border}`,
              borderLeft:   `1px solid ${sel ? C.border : 'transparent'}`,
              borderRight:  `1px solid ${sel ? C.border : 'transparent'}`,
              background:   sel ? C.panel : 'transparent',
              marginBottom: -1,
              fontSize: 12,
              whiteSpace: 'nowrap',
            }}>
              <div style={{ color: sel ? C.textBold : C.text, fontWeight: sel ? 700 : 500 }}>{label}</div>
              <div style={{ color: C.dim, fontSize: 10.5, marginTop: 1 }}>{hint}</div>
            </div>
          );
        })}
        <div style={{ flex: 1, borderBottom: `1px solid ${C.border}` }} />
        <div style={{ borderBottom: `1px solid ${C.border}`, padding: '12px 6px 6px', fontSize: 11, color: C.dim }}>
          <KeyPill k="Tab" label="cycle" />
        </div>
      </div>

      <div style={{ flex: 1, overflow: 'auto' }}>
        {tab === 'network' && <MonNetwork sub={sub} setSub={setSub} />}
        {tab === 'fleet' && <MonFleet ctx={ctx} />}
        {tab === 'oracle' && <MonOracle />}
      </div>
    </div>
  );
}

function MonNetwork({ sub, setSub }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', gap: 10 }}>
      <div style={{ display: 'flex', gap: 4 }}>
        {[['overview','Overview'],['validators','Validators'],['params','Gov Params']].map(([id, label], i) => (
          <div key={id} onClick={() => setSub(id)} style={{
            padding: '4px 12px',
            fontSize: 12,
            cursor: 'pointer',
            borderRadius: 3,
            background: sub === id ? C.redBg : 'transparent',
            color: sub === id ? C.red : C.dim,
            border: `1px solid ${sub === id ? C.red : C.border}`,
            whiteSpace: 'nowrap',
          }}>
            <span style={{ fontWeight: 700, marginRight: 6 }}>{i+1}</span>{label}
          </div>
        ))}
      </div>
      {sub === 'overview'   && <NetOverview />}
      {sub === 'validators' && <NetValidators />}
      {sub === 'params'     && <NetParams />}
    </div>
  );
}

function NetOverview() {
  const [tick, setTick] = useS2(0);
  useE2(() => { const id = setInterval(() => setTick(t => t + 1), 1500); return () => clearInterval(id); }, []);
  const blockTimes = useM2(() => Array.from({ length: 80 }, (_, i) => 5 + Math.sin(i * 0.3 + tick * 0.4) * 0.8 + Math.random() * 0.6), [tick]);
  const txs = useM2(() => Array.from({ length: 80 }, (_, i) => 30 + Math.sin(i * 0.2) * 12 + Math.random() * 14), [tick]);

  return (
    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr 1fr', gap: 12 }}>
      <StatCard label="height"    value={(16234891 + tick).toLocaleString()} hint={<Spinner color={C.green}/>}/>
      <StatCard label="block time" value="6.04s" sub={<><span style={{ color: C.green }}>▼ 0.2s</span> vs 1h</>} />
      <StatCard label="tx / sec"   value="42.1"  sub={<><span style={{ color: C.green }}>▲ 8.3%</span> 24h</>} />
      <StatCard label="bonded"     value="64.21M AKT" sub={<span style={{ color: C.dim }}>{`(${(64.21/100*100).toFixed(1)}% of supply)`}</span>} />

      <Panel title="Block time (last 80)" style={{ gridColumn: '1 / 3' }}>
        <Sparkline data={blockTimes} width={70} color={C.red} />
        <div style={{ marginTop: 6, fontSize: 11, color: C.dim, display: 'flex', justifyContent: 'space-between' }}>
          <span>min <span style={{ color: C.text }}>5.1s</span></span>
          <span>p50 <span style={{ color: C.text }}>6.0s</span></span>
          <span>p99 <span style={{ color: C.text }}>7.8s</span></span>
          <span>max <span style={{ color: C.text }}>8.2s</span></span>
        </div>
      </Panel>

      <Panel title="Transactions / block" style={{ gridColumn: '3 / 5' }}>
        <Sparkline data={txs} width={70} color={C.green} />
        <div style={{ marginTop: 6, fontSize: 11, color: C.dim, display: 'flex', justifyContent: 'space-between' }}>
          <span>MsgCreateDeployment <span style={{ color: C.text }}>34%</span></span>
          <span>MsgCreateLease <span style={{ color: C.text }}>18%</span></span>
          <span>MsgSend <span style={{ color: C.text }}>22%</span></span>
          <span>other <span style={{ color: C.text }}>26%</span></span>
        </div>
      </Panel>

      <Panel title="Top voting power" style={{ gridColumn: '1 / 3' }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6, fontSize: 12 }}>
          {VALIDATORS.slice(0, 5).map(v => (
            <div key={v.moniker} style={{ display: 'grid', gridTemplateColumns: '20px 1fr 60px 130px', gap: 10, alignItems: 'center' }}>
              <span style={{ color: C.dim, fontVariantNumeric: 'tabular-nums' }}>{v.rank}</span>
              <span style={{ color: C.text, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{v.moniker}</span>
              <span style={{ color: C.text, textAlign: 'right' }}>{v.vp.toFixed(2)}%</span>
              <Progress value={v.vp * 10} width={20} color={C.red} />
            </div>
          ))}
        </div>
      </Panel>

      <Panel title="Consensus state" style={{ gridColumn: '3 / 5' }}>
        <div style={{ fontSize: 12.5, fontFamily: 'inherit' }}>
          {[
            ['proposer',     'DCloud',                C.text],
            ['height',       String(16234891 + tick), C.text],
            ['round',        '0',                     C.text],
            ['step',         'precommit',             C.green],
            ['prevotes',     '136 / 150',             C.text],
            ['precommits',   '128 / 150',             C.yellow],
            ['gossip peers', '46',                    C.text],
          ].map(([k, v, color], i) => (
            <div key={i} style={{ display: 'grid', gridTemplateColumns: '110px 1fr', padding: '3px 0', borderBottom: i < 6 ? `1px dashed ${C.border}` : 'none' }}>
              <span style={{ color: C.dim }}>{k}</span>
              <span style={{ color }}>{v}</span>
            </div>
          ))}
        </div>
      </Panel>
    </div>
  );
}

function NetValidators() {
  return (
    <Panel title="Signing history (last 100 blocks)" active style={{ flex: 1 }}>
      <div style={{ fontFamily: 'inherit' }}>
        {VALIDATORS.slice(0, 10).map(v => (
          <div key={v.moniker} style={{ display: 'grid', gridTemplateColumns: '24px 200px 1fr 50px', gap: 12, alignItems: 'center', padding: '4px 0', borderBottom: `1px dashed ${C.border}` }}>
            <span style={{ color: C.dim, fontVariantNumeric: 'tabular-nums', fontSize: 12 }}>{v.rank}</span>
            <span style={{ color: C.text, fontSize: 12.5 }}>{v.moniker}</span>
            <div style={{ letterSpacing: '-1px', fontSize: 14, lineHeight: 1 }}>
              {Array.from({ length: 100 }).map((_, i) => {
                const miss = (v.moniker === 'Chorus One' && (i === 12 || i === 47 || i === 81)) || (v.moniker === 'Polychain Labs' && (i === 33 || i === 92));
                return <span key={i} style={{ color: miss ? C.red : C.green }}>{miss ? '✗' : '█'}</span>;
              })}
            </div>
            <span style={{ color: v.uptime >= 99.95 ? C.green : C.yellow, fontSize: 12, textAlign: 'right' }}>{v.uptime.toFixed(2)}%</span>
          </div>
        ))}
      </div>
    </Panel>
  );
}

function NetParams() {
  const params = [
    ['gov',     'min_deposit',          '512000000 uakt'],
    ['gov',     'voting_period',        '12d'],
    ['gov',     'quorum',               '0.334'],
    ['gov',     'threshold',            '0.500'],
    ['gov',     'veto_threshold',       '0.334'],
    ['staking', 'unbonding_time',       '21d'],
    ['staking', 'max_validators',       '100'],
    ['staking', 'bond_denom',           'uakt'],
    ['mint',    'inflation_rate',       '0.1384'],
    ['mint',    'inflation_max',        '0.5400'],
    ['mint',    'inflation_min',        '0.1300'],
    ['mint',    'goal_bonded',          '0.6700'],
    ['slashing','signed_blocks_window', '10000'],
    ['slashing','min_signed_per_window','0.500'],
    ['slashing','slash_fraction_dt',    '0.0100'],
    ['deploy',  'min_deposit',          '5000000 uakt'],
    ['deploy',  'max_groups',           '20'],
    ['market',  'bid_min_deposit',      '50000000 uakt'],
  ];
  return (
    <Panel title="Module parameters" active style={{ flex: 1 }}>
      <div style={{ fontSize: 12.5 }}>
        <div style={{ display: 'grid', gridTemplateColumns: '110px 1fr 1fr', gap: 12, padding: '6px 0', borderBottom: `1px solid ${C.border}`, fontSize: 11, textTransform: 'uppercase', color: C.dim, letterSpacing: 0.5 }}>
          <span>module</span><span>key</span><span>value</span>
        </div>
        {params.map(([m, k, v], i) => (
          <div key={i} style={{ display: 'grid', gridTemplateColumns: '110px 1fr 1fr', gap: 12, padding: '4px 0', borderBottom: i < params.length - 1 ? `1px dashed ${C.border}` : 'none' }}>
            <Badge tone="dim" style={{ justifySelf: 'start' }}>{m}</Badge>
            <span style={{ color: C.text }}>{k}</span>
            <span style={{ color: C.textBold, fontVariantNumeric: 'tabular-nums' }}>{v}</span>
          </div>
        ))}
      </div>
    </Panel>
  );
}

function StatCard({ label, value, sub, hint }) {
  return (
    <Panel padding="10px 14px">
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <span style={{ fontSize: 11, color: C.dim, textTransform: 'uppercase', letterSpacing: 0.6 }}>{label}</span>
        {hint}
      </div>
      <div style={{ fontSize: 22, color: C.textBold, fontWeight: 700, marginTop: 4, fontVariantNumeric: 'tabular-nums' }}>{value}</div>
      {sub && <div style={{ fontSize: 11, color: C.dim, marginTop: 2 }}>{sub}</div>}
    </Panel>
  );
}

function MonFleet({ ctx }) {
  const fleet = useM2(() => PROVIDERS.map(p => ({
    ...p,
    cpuUtil: 40 + Math.random() * 50,
    gpuUtil: 50 + Math.random() * 45,
    status: Math.random() > 0.93 ? 'degraded' : 'healthy',
  })), []);
  const [idx, setIdx, onKey] = useListSelect(fleet, p => ctx.push({ view: 'node-detail', host: p.host }));
  ctx.useKey ? ctx.useKey(onKey) : null;

  return (
    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, gridAutoRows: 'min-content' }}>
      <Panel title="Version distribution" style={{ gridColumn: '1 / -1' }}>
        <div style={{ display: 'flex', gap: 16, alignItems: 'center', fontSize: 12 }}>
          {[
            ['0.8.2', 4, C.green],
            ['0.8.1', 2, C.yellow],
            ['0.8.0', 1, C.red],
          ].map(([ver, n, col], i) => (
            <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span style={{ width: 10, height: 10, background: col, borderRadius: 2 }} />
              <span style={{ color: C.text }}>v{ver}</span>
              <span style={{ color: C.dim }}>{n} providers</span>
            </div>
          ))}
          <div style={{ flex: 1 }}/>
          <div style={{ color: C.dim }}>last scan <span style={{ color: C.text }}>32s ago</span></div>
        </div>
      </Panel>
      <Panel title="Fleet health" active style={{ gridColumn: '1 / -1' }} bodyStyle={{ padding: 0 }}>
        <Table
          cols={[
            { k: 'host',    label: 'host',    w: '1fr' },
            { k: 'region',  label: 'region',  w: '110px' },
            { k: 'version', label: 'version', w: '80px' },
            { k: 'status',  label: 'status',  w: '110px', render: v => <Badge tone={v === 'healthy' ? 'active' : 'pending'}>{v}</Badge> },
            { k: 'cpuUtil', label: 'cpu',     w: '120px', render: v => <Progress value={v} width={12} color={v > 85 ? C.yellow : C.green} /> },
            { k: 'gpuUtil', label: 'gpu',     w: '120px', render: v => <Progress value={v} width={12} color={v > 85 ? C.yellow : C.green} /> },
            { k: 'active',  label: 'leases',  w: '70px', align: 'right' },
          ]}
          rows={fleet}
          selectedIdx={idx}
          onSelect={setIdx}
        />
      </Panel>
    </div>
  );
}

function MonOracle() {
  const prices = [
    { sym: 'AKT/USD',  px: '3.412', d: '+4.2%', src: '12 sources', up: true },
    { sym: 'AKT/USDT', px: '3.408', d: '+4.1%', src: '8 sources',  up: true },
    { sym: 'ATOM/USD', px: '7.211', d: '−1.4%', src: '14 sources', up: false },
    { sym: 'OSMO/USD', px: '0.891', d: '+0.6%', src: '10 sources', up: true },
  ];
  return (
    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
      <Panel title="Oracle price feed" style={{ gridColumn: '1 / -1' }}>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr 1fr', gap: 14 }}>
          {prices.map(p => (
            <div key={p.sym} style={{ padding: 10, background: C.panelHi, borderRadius: 3, border: `1px solid ${C.border}` }}>
              <div style={{ fontSize: 11, color: C.dim, textTransform: 'uppercase', letterSpacing: 0.5 }}>{p.sym}</div>
              <div style={{ fontSize: 20, color: C.textBold, fontWeight: 700, marginTop: 2, fontVariantNumeric: 'tabular-nums' }}>${p.px}</div>
              <div style={{ fontSize: 11, marginTop: 2 }}>
                <span style={{ color: p.up ? C.green : C.red }}>{p.d}</span>
                <span style={{ color: C.dim, marginLeft: 8 }}>{p.src}</span>
              </div>
            </div>
          ))}
        </div>
      </Panel>
      <Panel title="BME vault">
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8, fontSize: 12.5 }}>
          <Row k="locked TVL"    v="14,920,440 AKT" />
          <Row k="active leases" v="528" />
          <Row k="burn (24h)"    v="4,210 AKT" />
          <Row k="emission (24h)" v={<span style={{ color: C.green }}>+12,840 AKT</span>} />
          <Row k="net flow"      v={<span style={{ color: C.green }}>+8,630 AKT</span>} />
        </div>
      </Panel>
      <Panel title="Ledger commits">
        <div style={{ fontFamily: 'inherit', fontSize: 11.5, color: C.text }}>
          {[
            ['16234891', '0x8f4a…d12c', 'price_commit',  '12 src'],
            ['16234887', '0xa9c1…7b88', 'price_commit',  '12 src'],
            ['16234881', '0x4d92…3e0a', 'bme_settle',    '—'],
            ['16234876', '0xc714…99fa', 'price_commit',  '11 src'],
            ['16234872', '0x18de…0470', 'oracle_rotate', '—'],
          ].map((r, i) => (
            <div key={i} style={{ display: 'grid', gridTemplateColumns: '80px 110px 1fr 60px', gap: 8, padding: '3px 0', borderBottom: i < 4 ? `1px dashed ${C.border}` : 'none' }}>
              <span style={{ color: C.dim, fontVariantNumeric: 'tabular-nums' }}>{r[0]}</span>
              <span style={{ color: C.text }}>{r[1]}</span>
              <span style={{ color: C.green }}>{r[2]}</span>
              <span style={{ color: C.dim }}>{r[3]}</span>
            </div>
          ))}
        </div>
      </Panel>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Log Viewer — streaming logs
// ─────────────────────────────────────────────────────────────
const LOG_LINES = [
  ['info', 'starting vllm worker', 'main'],
  ['info', 'loading model meta-llama/Llama-3-70B-Instruct', 'main'],
  ['info', 'tensor parallel size = 2, gpu memory = 80GiB ×2', 'engine'],
  ['warn', 'cuda kernel cache miss for shape (1, 8192, 8192)', 'engine'],
  ['info', 'model loaded in 142.3s', 'engine'],
  ['info', 'warmup batch size=8 seqlen=2048 latency=312ms', 'engine'],
  ['info', 'api server listening on 0.0.0.0:8000', 'http'],
  ['info', 'POST /v1/completions 200 — 412ms — 64 tok', 'http'],
  ['info', 'POST /v1/completions 200 — 388ms — 58 tok', 'http'],
  ['info', 'GET /health 200 — 1ms', 'http'],
  ['info', 'POST /v1/completions 200 — 502ms — 128 tok', 'http'],
  ['warn', 'queue depth = 4, latency degrading', 'engine'],
  ['info', 'POST /v1/completions 200 — 612ms — 96 tok', 'http'],
  ['info', 'GET /health 200 — 1ms', 'http'],
  ['info', 'POST /v1/embeddings 200 — 22ms — 8 vec', 'http'],
  ['err',  'connection reset by peer at 10.244.1.83', 'http'],
  ['info', 'POST /v1/completions 200 — 401ms — 71 tok', 'http'],
];

function ViewLogViewer({ ctx, dseq }) {
  const d = DEPLOYMENTS.find(x => x.dseq === dseq) || DEPLOYMENTS[0];
  const [lines, setLines] = useS2(() => LOG_LINES.slice(0, 8).map((l, i) => ({ ...mkLine(l), i })));
  const [paused, setPaused] = useS2(false);
  const scrollRef = useR2(null);

  useE2(() => {
    if (paused) return;
    const id = setInterval(() => {
      setLines(ls => {
        const next = [...ls, { ...mkLine(LOG_LINES[Math.floor(Math.random() * LOG_LINES.length)]), i: ls.length }].slice(-200);
        return next;
      });
    }, 700);
    return () => clearInterval(id);
  }, [paused]);

  useE2(() => {
    if (scrollRef.current && !paused) scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
  }, [lines, paused]);

  ctx.useKey((e) => {
    if (e.key === ' ' || e.key === 'p') { setPaused(p => !p); return true; }
    if (e.key === 'c') { setLines([]); return true; }
    return false;
  });

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: 14, gap: 10 }}>
      <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
        <Badge tone="info">logs</Badge>
        <span style={{ color: C.text, fontSize: 13 }}>{d.name}</span>
        <span style={{ color: C.dim, fontSize: 12 }}>dseq {d.dseq}</span>
        <span style={{ color: C.dim, fontSize: 12 }}>service llm</span>
        <div style={{ flex: 1 }}/>
        {paused ? <Badge tone="pending">paused</Badge> : <><Spinner color={C.green}/><span style={{ color: C.green, fontSize: 12 }}>streaming</span></>}
      </div>
      <Panel active style={{ flex: 1, display: 'flex', flexDirection: 'column' }} bodyStyle={{ padding: 0, height: '100%' }}>
        <div ref={scrollRef} style={{ flex: 1, overflow: 'auto', padding: 10, fontSize: 12, height: '100%', fontFamily: 'inherit' }}>
          {lines.map((l, i) => (
            <div key={i} style={{ display: 'grid', gridTemplateColumns: '78px 50px 60px 1fr', gap: 8, lineHeight: 1.4 }}>
              <span style={{ color: C.dim }}>{l.ts}</span>
              <span style={{ color: l.lvl === 'err' ? C.red : l.lvl === 'warn' ? C.yellow : C.green, fontWeight: 600, textTransform: 'uppercase' }}>{l.lvl}</span>
              <span style={{ color: C.purple }}>{l.scope}</span>
              <span style={{ color: C.text }}>{l.msg}</span>
            </div>
          ))}
          <div style={{ height: 6 }} />
          <Cursor />
        </div>
      </Panel>
      <FootHint items={[['space', paused ? 'resume' : 'pause'], ['c', 'clear'], ['/', 'filter'], ['esc', 'back']]} />
    </div>
  );
}

function mkLine([lvl, msg, scope]) {
  const d = new Date();
  const ts = `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
  return { lvl, msg, scope, ts };
}
function pad(n) { return String(n).padStart(2, '0'); }

// ─────────────────────────────────────────────────────────────
// Shell — interactive terminal in deployment
// ─────────────────────────────────────────────────────────────
function ViewShell({ ctx, dseq }) {
  const d = DEPLOYMENTS.find(x => x.dseq === dseq) || DEPLOYMENTS[0];
  const [history, setHistory] = useS2([
    { kind: 'out', text: `Connected to ${d.name}.akash:llm via provider-${d.dseq.slice(-4)}.overclock.akash.pub` },
    { kind: 'out', text: `Last login: Tue May 14 11:02:08 from 10.0.0.42` },
    { kind: 'out', text: '' },
    { kind: 'cmd', text: 'nvidia-smi --query-gpu=name,utilization.gpu,memory.used --format=csv' },
    { kind: 'out', text: 'name, utilization.gpu [%], memory.used [MiB]' },
    { kind: 'out', text: 'NVIDIA H100 80GB HBM3, 82 %, 71840 MiB' },
    { kind: 'out', text: 'NVIDIA H100 80GB HBM3, 78 %, 70210 MiB' },
    { kind: 'cmd', text: 'ps -eo pid,pcpu,pmem,comm --sort=-pcpu | head -6' },
    { kind: 'out', text: '  PID %CPU %MEM COMMAND' },
    { kind: 'out', text: '  142 320.4 41.2 python -m vllm.entrypoints.openai.api_server' },
    { kind: 'out', text: '   89   2.1  1.2 sshd' },
    { kind: 'out', text: '    1   0.0  0.1 /sbin/init' },
  ]);
  const [input, setInput] = useS2('');
  const scrollRef = useR2(null);

  useE2(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
  }, [history]);

  const submit = () => {
    if (!input.trim()) return;
    const out = simulateShell(input.trim());
    setHistory(h => [...h, { kind: 'cmd', text: input }, ...out.map(t => ({ kind: 'out', text: t }))]);
    setInput('');
  };

  ctx.useKey((e) => {
    if (e.key === 'Enter') { submit(); return true; }
    if (e.key === 'Backspace') { setInput(s => s.slice(0, -1)); return true; }
    if (e.key.length === 1 && !e.ctrlKey && !e.metaKey) { setInput(s => s + e.key); return true; }
    return false;
  });

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: 14, gap: 10 }}>
      <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
        <Badge tone="accent">shell</Badge>
        <span style={{ color: C.text, fontSize: 13 }}>{d.name}</span>
        <span style={{ color: C.dim, fontSize: 12 }}>service llm</span>
        <div style={{ flex: 1 }}/>
        <Badge tone="active">connected</Badge>
      </div>
      <Panel active style={{ flex: 1 }} bodyStyle={{ padding: 0, height: '100%' }}>
        <div ref={scrollRef} style={{ overflow: 'auto', padding: 12, fontSize: 12.5, height: '100%' }}>
          {history.map((h, i) => h.kind === 'cmd' ? (
            <div key={i} style={{ display: 'flex', gap: 6 }}>
              <span style={{ color: C.green }}>akash@{d.dseq.slice(-4)}</span>
              <span style={{ color: C.dim }}>:</span>
              <span style={{ color: C.blue }}>~</span>
              <span style={{ color: C.dim }}>$</span>
              <span style={{ color: C.text }}>{h.text}</span>
            </div>
          ) : (
            <div key={i} style={{ color: C.text, whiteSpace: 'pre' }}>{h.text}</div>
          ))}
          <div style={{ display: 'flex', gap: 6 }}>
            <span style={{ color: C.green }}>akash@{d.dseq.slice(-4)}</span>
            <span style={{ color: C.dim }}>:</span>
            <span style={{ color: C.blue }}>~</span>
            <span style={{ color: C.dim }}>$</span>
            <span style={{ color: C.text }}>{input}</span>
            <Cursor />
          </div>
        </div>
      </Panel>
      <FootHint items={[['↵', 'run'], ['try:', 'ls / df -h / curl ...'], ['esc', 'back']]} />
    </div>
  );
}

function simulateShell(cmd) {
  if (cmd === 'ls' || cmd === 'ls -la') return [
    'drwxr-xr-x  models', '-rw-r--r--  Dockerfile', '-rw-r--r--  requirements.txt', 'drwxr-xr-x  vllm',
  ];
  if (cmd.startsWith('df')) return ['Filesystem      Size  Used Avail Use% Mounted on', 'overlay         500G  142G  358G  29% /'];
  if (cmd.startsWith('curl')) return ['{"id":"cmpl-9x4Z","object":"text_completion","choices":[{"text":"Hello!"}]}'];
  if (cmd === 'whoami') return ['root'];
  if (cmd === 'exit') return ['(exit suppressed in prototype)'];
  return [`sh: ${cmd}: not found`];
}

// ─────────────────────────────────────────────────────────────
// Node detail (from provider fleet)
// ─────────────────────────────────────────────────────────────
function ViewNodeDetail({ ctx, host }) {
  const p = PROVIDERS.find(x => x.host === host) || PROVIDERS[0];
  const gpus = useM2(() => [
    { idx: 0, model: 'H100-80GB', util: 84, mem: '71840 / 81920 MiB', temp: 71, power: '648 W' },
    { idx: 1, model: 'H100-80GB', util: 78, mem: '70210 / 81920 MiB', temp: 69, power: '612 W' },
    { idx: 2, model: 'A100-40GB', util: 42, mem: '18402 / 40960 MiB', temp: 62, power: '281 W' },
    { idx: 3, model: 'A100-40GB', util: 12, mem:  '6244 / 40960 MiB', temp: 58, power: '184 W' },
    { idx: 4, model: 'A100-40GB', util:  0, mem:    '142 / 40960 MiB', temp: 41, power: ' 64 W' },
  ], []);
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: 14, gap: 10 }}>
      <div style={{ display: 'flex', gap: 14, alignItems: 'center' }}>
        <div style={{ fontSize: 18, color: C.textBold, fontWeight: 700 }}>{p.host}</div>
        <Badge tone="active">healthy</Badge>
        <Badge tone="info">v{p.version}</Badge>
      </div>
      <Panel title="GPU inventory" active style={{ flex: 1 }} bodyStyle={{ padding: 0 }}>
        <Table
          cols={[
            { k: 'idx',   label: 'idx',   w: '50px',  align: 'right' },
            { k: 'model', label: 'model', w: '160px' },
            { k: 'util',  label: 'util',  w: '180px', render: v => <Progress value={v} width={20} color={v > 85 ? C.yellow : v > 0 ? C.green : C.dim}/> },
            { k: 'mem',   label: 'memory',w: '1fr' },
            { k: 'temp',  label: 'temp',  w: '80px', align: 'right', render: v => <span style={{ color: v > 75 ? C.red : v > 65 ? C.yellow : C.green }}>{v}°C</span> },
            { k: 'power', label: 'power', w: '90px', align: 'right' },
          ]}
          rows={gpus}
          selectedIdx={0}
          onSelect={() => {}}
        />
      </Panel>
      <FootHint items={[['esc', 'back']]} />
    </div>
  );
}

Object.assign(window, {
  ViewMonitorHub, ViewLogViewer, ViewShell, ViewNodeDetail,
});
