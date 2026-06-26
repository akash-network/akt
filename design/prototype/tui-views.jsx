// Primary views for the akt TUI

const { useState: useS, useEffect: useE, useMemo: useM } = React;

// ─────────────────────────────────────────────────────────────
// Dashboard — landing
// ─────────────────────────────────────────────────────────────
function ViewDashboard({ ctx }) {
  const blockData = useM(() => Array.from({ length: 60 }, () => 4 + Math.random() * 4), []);
  const priceData = useM(() => {
    const out = [];
    let v = 3.2;
    for (let i = 0; i < 60; i++) { v += (Math.random() - 0.48) * 0.08; out.push(v); }
    return out;
  }, []);
  const active = DEPLOYMENTS.filter(d => d.state === 'active');
  const totalSpend = active.reduce((s, d) => s + parseFloat(d.cost), 0).toFixed(2);

  return (
    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 12, padding: 14, height: '100%', overflow: 'auto', alignContent: 'start' }}>
      {/* Welcome banner */}
      <Panel style={{ gridColumn: '1 / -1' }} padding="14px 18px">
        <div style={{ display: 'flex', alignItems: 'center', gap: 18 }}>
          <pre style={{ margin: 0, color: C.red, fontSize: 11, lineHeight: 1.1 }}>
{`  ▄▀█ █▄▀ ▀█▀
  █▀█ █ █  █ `}
          </pre>
          <div style={{ flex: 1 }}>
            <div style={{ fontSize: 14, color: C.textBold, fontWeight: 700 }}>welcome back, akash1zk2…vq4r</div>
            <div style={{ fontSize: 12, color: C.dim, marginTop: 2 }}>
              connected to <span style={{ color: C.text }}>mainnet</span> · rpc <span style={{ color: C.text }}>rpc.akash.forbole.com:443</span> · last sync <span style={{ color: C.text }}>2s ago</span>
            </div>
          </div>
          <div style={{ display: 'flex', gap: 8 }}>
            <Badge tone="active">● synced</Badge>
            <Badge tone="info">v1.2.0</Badge>
          </div>
        </div>
      </Panel>

      {/* Account */}
      <Panel title="Wallet">
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6, fontSize: 12 }}>
          <Row k="address"  v="akash1zk2…vq4r" />
          <Row k="liquid"   v={<span style={{ color: C.textBold }}>1,284.92 AKT</span>} />
          <Row k="staked"   v="3,400.00 AKT" />
          <Row k="rewards"  v={<span style={{ color: C.green }}>+12.40 AKT</span>} />
          <Row k="escrow"   v="246.40 AKT" />
          <div style={{ height: 4 }} />
          <div style={{ color: C.dim, fontSize: 11 }}>price (24h)</div>
          <Sparkline data={priceData} width={32} color={C.green} />
          <div style={{ fontSize: 12 }}>$3.41 <span style={{ color: C.green }}>▲ 4.2%</span></div>
        </div>
      </Panel>

      {/* Active deployments */}
      <Panel title={`Active · ${active.length}`}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6, fontSize: 12 }}>
          {active.slice(0, 4).map(d => (
            <div key={d.dseq} style={{ display: 'flex', justifyContent: 'space-between', gap: 8, minWidth: 0 }}>
              <span style={{ color: C.text, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0, flex: '1 1 0' }}>{d.name}</span>
              <span style={{ color: C.dim, whiteSpace: 'nowrap', flexShrink: 0 }}>{d.cost}</span>
            </div>
          ))}
          <div style={{ height: 4 }} />
          <div style={{ display: 'flex', justifyContent: 'space-between', borderTop: `1px dashed ${C.border}`, paddingTop: 6 }}>
            <span style={{ color: C.dim }}>monthly burn</span>
            <span style={{ color: C.textBold, fontWeight: 600 }}>{totalSpend} AKT</span>
          </div>
          <div style={{ marginTop: 6, color: C.dim, fontSize: 11 }}>
            press <KeyPill k="1" /> for full list · <KeyPill k="D" /> new deployment
          </div>
        </div>
      </Panel>

      {/* Network */}
      <Panel title="Network">
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6, fontSize: 12 }}>
          <Row k="height"        v={<span style={{ color: C.textBold }}>16,234,891</span>} />
          <Row k="block time"    v="6.04s" />
          <Row k="active prov."  v="184" />
          <Row k="bonded"        v="64.2M AKT" />
          <Row k="inflation"     v="13.84%" />
          <div style={{ height: 4 }} />
          <div style={{ color: C.dim, fontSize: 11 }}>block times (last 60)</div>
          <Sparkline data={blockData} width={32} color={C.red} />
          <div style={{ fontSize: 11, color: C.dim }}>avg <span style={{ color: C.text }}>6.04s</span> · max <span style={{ color: C.text }}>7.8s</span></div>
        </div>
      </Panel>

      {/* Recent activity */}
      <Panel title="Recent activity" style={{ gridColumn: '1 / 3' }}>
        <div style={{ fontSize: 12, display: 'flex', flexDirection: 'column' }}>
          {[
            { t: '14:02:11', kind: 'tx',  txt: 'MsgSendManifest · dseq 17834201 · provider overclock.akash.pub', tone: 'active' },
            { t: '14:01:48', kind: 'tx',  txt: 'MsgCreateLease · dseq 17834201 · bid 2/12', tone: 'active' },
            { t: '13:58:22', kind: 'evt', txt: 'bid received · provider praetorapp.com · 0.0030 uakt/blk', tone: 'info' },
            { t: '13:55:09', kind: 'tx',  txt: 'MsgCreateDeployment · dseq 17834201', tone: 'active' },
            { t: '11:41:00', kind: 'gov', txt: 'proposal #91 entered voting period', tone: 'voting' },
            { t: '09:12:34', kind: 'tx',  txt: 'MsgCloseDeployment · dseq 17828910', tone: 'closed' },
          ].map((r, i) => (
            <div key={i} style={{ display: 'grid', gridTemplateColumns: '70px 50px 1fr', gap: 10, padding: '3px 0', borderBottom: i < 5 ? `1px dashed ${C.border}` : 'none' }}>
              <span style={{ color: C.dim }}>{r.t}</span>
              <Badge tone={r.tone} style={{ justifySelf: 'start' }}>{r.kind}</Badge>
              <span style={{ color: C.text }}>{r.txt}</span>
            </div>
          ))}
        </div>
      </Panel>

      {/* Shortcuts */}
      <Panel title="Shortcuts">
        <div style={{ fontSize: 12, display: 'flex', flexDirection: 'column', gap: 6 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}><KeyPill k="1-6" /> <span style={{ color: C.dim }}>primary nav</span></div>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}><KeyPill k="↵" /> <span style={{ color: C.dim }}>drill down</span></div>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}><KeyPill k="esc" /> <span style={{ color: C.dim }}>pop back</span></div>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}><KeyPill k=":" /> <span style={{ color: C.dim }}>command palette</span></div>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}><KeyPill k="?" /> <span style={{ color: C.dim }}>help overlay</span></div>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}><KeyPill k="D" accent /> <span style={{ color: C.dim }}>new deployment</span></div>
        </div>
      </Panel>
    </div>
  );
}

function Row({ k, v }) {
  return (
    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'baseline', minWidth: 0 }}>
      <span style={{ color: C.dim, flexShrink: 0, whiteSpace: 'nowrap' }}>{k}</span>
      <span style={{ color: C.text, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', minWidth: 0, textAlign: 'right' }}>{v}</span>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Deployments list
// ─────────────────────────────────────────────────────────────
function ViewDeployments({ ctx }) {
  const items = DEPLOYMENTS;
  const [idx, setIdx, onKey] = useListSelect(items, (item) => ctx.push({ view: 'deployment-detail', dseq: item.dseq }));
  ctx.useKey((e) => {
    if (onKey(e)) return true;
    if (e.key === 'd') { ctx.openConfirm({ kind: 'close', item: items[idx] }); return true; }
    if (e.key === 'l') { ctx.push({ view: 'log-viewer', dseq: items[idx].dseq }); return true; }
    if (e.key === 's') { ctx.push({ view: 'shell', dseq: items[idx].dseq }); return true; }
    if (e.key === '/') { ctx.openPalette({ scope: 'deployments' }); return true; }
    return false;
  });

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: 14, gap: 10 }}>
      <Panel title="Deployments" active style={{ flex: 1, display: 'flex', flexDirection: 'column' }} bodyStyle={{ padding: 0 }}>
        <Table
          cols={[
            { k: 'dseq',     label: 'dseq',     w: '110px', align: 'left' },
            { k: 'name',     label: 'name',     w: '1fr' },
            { k: 'state',    label: 'state',    w: '92px', render: v => stateBadge(v) },
            { k: 'cpu',      label: 'cpu',      w: '56px', align: 'right' },
            { k: 'mem',      label: 'mem',      w: '64px', align: 'right' },
            { k: 'gpu',      label: 'gpu',      w: '100px' },
            { k: 'provider', label: 'provider', w: '1fr' },
            { k: 'uptime',   label: 'uptime',   w: '90px', align: 'right' },
            { k: 'cost',     label: 'cost',     w: '140px', align: 'right' },
          ]}
          rows={items}
          selectedIdx={idx}
          onSelect={setIdx}
        />
      </Panel>
      <FootHint items={[
        ['↑↓', 'move'], ['↵', 'open'], ['l', 'logs'], ['s', 'shell'], ['d', 'close'], ['/', 'search'], ['D', 'new', true],
      ]}/>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Generic table
// ─────────────────────────────────────────────────────────────
function Table({ cols, rows, selectedIdx, onSelect }) {
  const gridCols = cols.map(c => c.w || '1fr').join(' ');
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* header */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: gridCols,
        gap: 14,
        padding: '8px 14px',
        borderBottom: `1px solid ${C.border}`,
        fontSize: 11,
        textTransform: 'uppercase',
        letterSpacing: 0.6,
        color: C.dim,
      }}>
        {cols.map(c => (
          <div key={c.k} style={{ textAlign: c.align || 'left' }}>{c.label}</div>
        ))}
      </div>
      {/* rows */}
      <div style={{ flex: 1, overflow: 'auto' }}>
        {rows.map((r, i) => {
          const sel = i === selectedIdx;
          return (
            <div
              key={i}
              onClick={() => onSelect && onSelect(i)}
              style={{
                display: 'grid',
                gridTemplateColumns: gridCols,
                gap: 14,
                padding: '6px 14px',
                fontSize: 12.5,
                cursor: 'pointer',
                color: sel ? C.textBold : C.text,
                background: sel ? C.redBg : 'transparent',
                borderLeft: `2px solid ${sel ? C.red : 'transparent'}`,
                paddingLeft: 12,
              }}
            >
              {cols.map(c => (
                <div key={c.k} style={{
                  textAlign: c.align || 'left',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                  fontVariantNumeric: 'tabular-nums',
                }}>
                  {c.render ? c.render(r[c.k], r) : r[c.k]}
                </div>
              ))}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Footer key hints
// ─────────────────────────────────────────────────────────────
function FootHint({ items }) {
  return (
    <div style={{ display: 'flex', gap: 14, flexWrap: 'wrap', alignItems: 'center', padding: '0 4px' }}>
      {items.map(([k, l, accent], i) => (
        <KeyPill key={i} k={k} label={l} accent={accent} />
      ))}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Deployment detail
// ─────────────────────────────────────────────────────────────
function ViewDeploymentDetail({ ctx, dseq }) {
  const d = DEPLOYMENTS.find(x => x.dseq === dseq) || DEPLOYMENTS[0];
  const [tab, setTab] = useS('overview');
  const tabs = ['overview', 'lease', 'escrow', 'endpoints'];

  ctx.useKey((e) => {
    if (e.key === 'Tab')        { setTab(t => tabs[(tabs.indexOf(t) + 1) % tabs.length]); return true; }
    if (e.key === 'l')          { ctx.push({ view: 'log-viewer', dseq }); return true; }
    if (e.key === 's')          { ctx.push({ view: 'shell', dseq }); return true; }
    if (e.key === 'd')          { ctx.openConfirm({ kind: 'close', item: d }); return true; }
    if (/^[1-4]$/.test(e.key))  { setTab(tabs[parseInt(e.key) - 1]); return true; }
    return false;
  });

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: 14, gap: 10 }}>
      {/* header strip */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 14, padding: '0 4px' }}>
        <div style={{ fontSize: 18, color: C.textBold, fontWeight: 700 }}>{d.name}</div>
        <div style={{ color: C.dim, fontSize: 12 }}>dseq <span style={{ color: C.text }}>{d.dseq}</span></div>
        {stateBadge(d.state)}
        <div style={{ flex: 1 }} />
        <div style={{ fontSize: 11, color: C.dim }}>owner</div>
        <div style={{ fontSize: 12, color: C.text }}>akash1zk2…vq4r</div>
      </div>

      {/* sub-tabs */}
      <div style={{ display: 'flex', gap: 0, borderBottom: `1px solid ${C.border}` }}>
        {tabs.map((t, i) => (
          <div key={t} onClick={() => setTab(t)} style={{
            padding: '6px 14px',
            fontSize: 12,
            cursor: 'pointer',
            color: tab === t ? C.textBold : C.dim,
            background: tab === t ? C.panel : 'transparent',
            border: `1px solid ${tab === t ? C.border : 'transparent'}`,
            borderBottom: tab === t ? `1px solid ${C.panel}` : `1px solid ${C.border}`,
            marginBottom: -1,
            borderRadius: '4px 4px 0 0',
            textTransform: 'capitalize',
            whiteSpace: 'nowrap',
          }}>
            <span style={{ color: C.red, marginRight: 6, fontWeight: 700 }}>{i + 1}</span>
            {t}
          </div>
        ))}
      </div>

      <div style={{ flex: 1, overflow: 'auto' }}>
        {tab === 'overview' && <DepOverview d={d} />}
        {tab === 'lease' && <DepLease d={d} />}
        {tab === 'escrow' && <DepEscrow d={d} />}
        {tab === 'endpoints' && <DepEndpoints d={d} />}
      </div>

      <FootHint items={[
        ['tab', 'cycle'], ['1-4', 'jump'], ['l', 'logs'], ['s', 'shell'], ['d', 'close', true], ['esc', 'back'],
      ]}/>
    </div>
  );
}

function DepOverview({ d }) {
  return (
    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
      <Panel title="Resources">
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8, fontSize: 12.5 }}>
          <Row k="cpu"     v={`${d.cpu} cores`} />
          <Row k="memory"  v={d.mem} />
          <Row k="gpu"     v={d.gpu} />
          <Row k="storage" v={d.storage} />
        </div>
      </Panel>
      <Panel title="Placement">
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8, fontSize: 12.5 }}>
          <Row k="provider" v={d.provider} />
          <Row k="region"   v={d.region} />
          <Row k="uptime"   v={d.uptime} />
          <Row k="cost"     v={d.cost} />
        </div>
      </Panel>
      <Panel title="SDL manifest" style={{ gridColumn: '1 / -1' }}>
        <pre style={{ margin: 0, fontSize: 11.5, color: C.text, lineHeight: 1.45, maxHeight: 220, overflow: 'auto' }}>
{SDL_SAMPLE.split('\n').map((l, i) => (
  <span key={i}>
    <span style={{ color: C.dim, userSelect: 'none' }}>{String(i+1).padStart(2, ' ')}  </span>
    {colorYaml(l)}
    {'\n'}
  </span>
))}
        </pre>
      </Panel>
    </div>
  );
}

function colorYaml(line) {
  // very lightweight: keys → red, strings → green-ish
  const m = line.match(/^(\s*-?\s*)([a-zA-Z0-9_-]+)(:)(.*)$/);
  if (m) {
    return (
      <>
        <span>{m[1]}</span>
        <span style={{ color: C.red }}>{m[2]}</span>
        <span style={{ color: C.dim }}>{m[3]}</span>
        <span style={{ color: C.text }}>{m[4]}</span>
      </>
    );
  }
  return <span>{line}</span>;
}

function DepLease({ d }) {
  const lease = LEASES.find(l => l.dseq === d.dseq) || LEASES[0];
  return (
    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
      <Panel title="Lease">
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8, fontSize: 12.5 }}>
          <Row k="provider"  v={lease.provider} />
          <Row k="state"     v={stateBadge(lease.state)} />
          <Row k="price"     v={lease.price} />
          <Row k="opened"    v={lease.opened} />
          <Row k="oseq/gseq" v="1 / 1" />
        </div>
      </Panel>
      <Panel title="Bid history">
        <div style={{ fontSize: 12 }}>
          {[
            { p: 'overclock.akash.pub',    bid: '0.0061', winner: true },
            { p: 'praetorapp.com',         bid: '0.0072', winner: false },
            { p: 'akash.computer',         bid: '0.0089', winner: false },
            { p: 'computefarm.cloud',      bid: '0.0091', winner: false },
            { p: 'sandbox.akash.pub',      bid: '0.0124', winner: false },
          ].map((b, i) => (
            <div key={i} style={{ display: 'grid', gridTemplateColumns: '1fr 90px 80px', padding: '4px 0', borderBottom: i < 4 ? `1px dashed ${C.border}` : 'none', alignItems: 'center' }}>
              <span style={{ color: b.winner ? C.textBold : C.text }}>{b.p}</span>
              <span style={{ color: C.dim, textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{b.bid} uakt</span>
              {b.winner ? <Badge tone="accent" style={{ justifySelf: 'end' }}>winner</Badge> : <span style={{ color: C.dim, textAlign: 'right', fontSize: 11 }}>—</span>}
            </div>
          ))}
        </div>
      </Panel>
    </div>
  );
}

function DepEscrow({ d }) {
  return (
    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
      <Panel title="Escrow balance">
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8, fontSize: 12.5 }}>
          <Row k="deposit"   v="200.00 AKT" />
          <Row k="consumed"  v="57.60 AKT" />
          <Row k="remaining" v={<span style={{ color: C.textBold }}>142.40 AKT</span>} />
          <Row k="burn rate" v="4.12 AKT / day" />
          <Row k="runs out"  v={<span style={{ color: C.yellow }}>~34 days</span>} />
        </div>
        <div style={{ marginTop: 10 }}>
          <Progress value={71} width={36} color={C.green} />
        </div>
      </Panel>
      <Panel title="Recent fee events">
        <div style={{ fontSize: 12 }}>
          {[
            ['14:02:11', 'lease settled',       '−0.34 AKT'],
            ['12:00:02', 'lease settled',       '−0.34 AKT'],
            ['10:00:01', 'lease settled',       '−0.34 AKT'],
            ['08:00:00', 'lease settled',       '−0.34 AKT'],
            ['06:00:00', 'lease settled',       '−0.34 AKT'],
            ['00:00:00', 'top-up',              '+50.00 AKT'],
          ].map((r, i) => (
            <div key={i} style={{ display: 'grid', gridTemplateColumns: '70px 1fr 90px', padding: '3px 0', fontSize: 12, borderBottom: i < 5 ? `1px dashed ${C.border}` : 'none' }}>
              <span style={{ color: C.dim }}>{r[0]}</span>
              <span style={{ color: C.text }}>{r[1]}</span>
              <span style={{ color: r[2].startsWith('+') ? C.green : C.dim, textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{r[2]}</span>
            </div>
          ))}
        </div>
      </Panel>
    </div>
  );
}

function DepEndpoints({ d }) {
  return (
    <Panel title="Forwarded endpoints">
      <div style={{ fontSize: 12.5, display: 'flex', flexDirection: 'column', gap: 8 }}>
        {[
          { svc: 'llm',  port: '8000 → 80',  url: `https://provider-${d.dseq.slice(-4)}.overclock.akash.pub`, https: true },
          { svc: 'llm',  port: '8000 → 443', url: `https://${d.dseq.slice(-4)}.llama.overclock.akash.pub`,    https: true },
          { svc: 'grpc', port: '50051',      url: `tcp://provider-${d.dseq.slice(-4)}.overclock.akash.pub:32417`, https: false },
        ].map((e, i) => (
          <div key={i} style={{ display: 'grid', gridTemplateColumns: '60px 120px 1fr 60px', gap: 14, padding: '8px 0', borderBottom: i < 2 ? `1px dashed ${C.border}` : 'none', alignItems: 'center' }}>
            <Badge tone="info">{e.svc}</Badge>
            <span style={{ color: C.dim }}>{e.port}</span>
            <span style={{ color: C.textBold }}>{e.url}</span>
            <span style={{ color: e.https ? C.green : C.yellow, fontSize: 11, textAlign: 'right' }}>{e.https ? '● tls' : '○ tcp'}</span>
          </div>
        ))}
      </div>
      <div style={{ marginTop: 14, padding: 10, background: C.panelHi, borderRadius: 3, fontSize: 12, color: C.dim }}>
        <span style={{ color: C.green }}>$</span> curl -X POST <span style={{ color: C.text }}>https://provider-{d.dseq.slice(-4)}.overclock.akash.pub/v1/completions</span> \<br/>
        <span style={{ marginLeft: 18 }}>  -H </span><span style={{ color: C.yellow }}>"Authorization: Bearer $AKT_TOKEN"</span>
      </div>
    </Panel>
  );
}

// ─────────────────────────────────────────────────────────────
// Leases
// ─────────────────────────────────────────────────────────────
function ViewLeases({ ctx }) {
  const items = LEASES;
  const [idx, setIdx, onKey] = useListSelect(items, (item) => ctx.push({ view: 'deployment-detail', dseq: item.dseq }));
  ctx.useKey((e) => onKey(e));

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: 14, gap: 10 }}>
      <Panel title="Active leases" active style={{ flex: 1 }} bodyStyle={{ padding: 0 }}>
        <Table
          cols={[
            { k: 'dseq',     label: 'dseq',     w: '110px' },
            { k: 'provider', label: 'provider', w: '1fr' },
            { k: 'state',    label: 'state',    w: '90px', render: v => stateBadge(v) },
            { k: 'price',    label: 'price',    w: '180px' },
            { k: 'escrow',   label: 'escrow',   w: '120px', align: 'right' },
            { k: 'opened',   label: 'opened',   w: '120px' },
          ]}
          rows={items}
          selectedIdx={idx}
          onSelect={setIdx}
        />
      </Panel>
      <FootHint items={[['↑↓', 'move'], ['↵', 'detail'], ['esc', 'back']]} />
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Providers
// ─────────────────────────────────────────────────────────────
function ViewProviders({ ctx }) {
  const items = PROVIDERS;
  const [idx, setIdx, onKey] = useListSelect(items, (p) => ctx.push({ view: 'provider-detail', host: p.host }));
  ctx.useKey((e) => onKey(e));
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: 14, gap: 10 }}>
      <Panel title="Provider directory" active style={{ flex: 1 }} bodyStyle={{ padding: 0 }}>
        <Table
          cols={[
            { k: 'host',       label: 'host',     w: '1fr' },
            { k: 'region',     label: 'region',   w: '110px' },
            { k: 'gpu',        label: 'gpu',      w: '160px' },
            { k: 'cpu',        label: 'cpu',      w: '70px', align: 'right' },
            { k: 'mem',        label: 'mem',      w: '80px', align: 'right' },
            { k: 'active',     label: 'leases',   w: '70px', align: 'right' },
            { k: 'score',      label: 'audit',    w: '70px', align: 'right', render: v => <span style={{ color: v > 99 ? C.green : v > 97 ? C.yellow : C.red }}>{v.toFixed(1)}</span> },
            { k: 'version',    label: 'version',  w: '80px' },
          ]}
          rows={items}
          selectedIdx={idx}
          onSelect={setIdx}
        />
      </Panel>
      <FootHint items={[['↑↓', 'move'], ['↵', 'detail'], ['esc', 'back']]} />
    </div>
  );
}

function ViewProviderDetail({ ctx, host }) {
  const p = PROVIDERS.find(x => x.host === host) || PROVIDERS[0];
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: 14, gap: 10 }}>
      <div style={{ display: 'flex', gap: 14, alignItems: 'center' }}>
        <div style={{ fontSize: 18, color: C.textBold, fontWeight: 700 }}>{p.host}</div>
        <Badge tone="active">audit {p.score}</Badge>
        <Badge tone="info">v{p.version}</Badge>
        <div style={{ flex: 1 }}/>
        <div style={{ color: C.dim, fontSize: 12 }}>region <span style={{ color: C.text }}>{p.region}</span></div>
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, flex: 1 }}>
        <Panel title="Capacity">
          <div style={{ fontSize: 12.5, display: 'flex', flexDirection: 'column', gap: 8 }}>
            <Row k="cpu (total)"  v={p.cpu} />
            <Row k="memory"       v={p.mem} />
            <Row k="gpu"          v={p.gpu} />
            <Row k="active leases" v={p.active} />
            <div style={{ marginTop: 8, color: C.dim, fontSize: 11 }}>cpu utilization</div>
            <Progress value={62} width={36} color={C.green} />
            <div style={{ color: C.dim, fontSize: 11 }}>gpu utilization</div>
            <Progress value={84} width={36} color={C.yellow} />
          </div>
        </Panel>
        <Panel title="Attributes">
          <div style={{ fontSize: 12.5, display: 'flex', flexDirection: 'column', gap: 6 }}>
            <Row k="host-uri"        v={`https://${p.host}:8443`} />
            <Row k="organization"    v="Overclock Labs" />
            <Row k="email"           v="ops@overclock.com" />
            <Row k="tier"            v="community-1" />
            <Row k="ip-lookup"       v="enabled" />
            <Row k="capabilities/gpu/vendor/nvidia/model/h100" v="true" />
            <Row k="capabilities/storage/2/class" v="beta3" />
          </div>
        </Panel>
        <Panel title="Audit history" style={{ gridColumn: '1 / -1' }}>
          <div style={{ fontSize: 12 }}>
            {[
              { t: '2026-05-12', h: 16234812, by: 'akashnet.audit', result: 'pass'    },
              { t: '2026-05-05', h: 16124002, by: 'akashnet.audit', result: 'pass'    },
              { t: '2026-04-28', h: 16012744, by: 'akashnet.audit', result: 'pass'    },
              { t: '2026-04-21', h: 15901301, by: 'akashnet.audit', result: 'warning' },
              { t: '2026-04-14', h: 15789140, by: 'akashnet.audit', result: 'pass'    },
            ].map((r, i) => (
              <div key={i} style={{ display: 'grid', gridTemplateColumns: '100px 130px 1fr 80px', gap: 12, padding: '4px 0', borderBottom: i < 4 ? `1px dashed ${C.border}` : 'none' }}>
                <span style={{ color: C.dim }}>{r.t}</span>
                <span style={{ color: C.text, fontVariantNumeric: 'tabular-nums' }}>blk {r.h.toLocaleString()}</span>
                <span style={{ color: C.text }}>{r.by}</span>
                <Badge tone={r.result === 'pass' ? 'active' : 'pending'} style={{ justifySelf: 'end' }}>{r.result}</Badge>
              </div>
            ))}
          </div>
        </Panel>
      </div>
      <FootHint items={[['↵', 'open in browser'], ['esc', 'back']]} />
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Governance
// ─────────────────────────────────────────────────────────────
function ViewGovernance({ ctx }) {
  const items = PROPOSALS;
  const [idx, setIdx, onKey] = useListSelect(items, (p) => ctx.push({ view: 'proposal-detail', id: p.id }));
  ctx.useKey((e) => {
    if (onKey(e)) return true;
    if (e.key === 'v') { ctx.openConfirm({ kind: 'vote', item: items[idx] }); return true; }
    return false;
  });
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: 14, gap: 10 }}>
      <Panel title="Governance proposals" active style={{ flex: 1 }} bodyStyle={{ padding: 0 }}>
        <Table
          cols={[
            { k: 'id',     label: '#',        w: '50px' },
            { k: 'title',  label: 'title',    w: '1fr' },
            { k: 'status', label: 'status',   w: '110px', render: v => stateBadge(v) },
            { k: 'yes',    label: 'yes',      w: '60px',  align: 'right', render: v => <span style={{ color: C.green }}>{v}%</span> },
            { k: 'no',     label: 'no',       w: '60px',  align: 'right', render: v => <span style={{ color: C.red }}>{v}%</span> },
            { k: 'abstain',label: 'abstain',  w: '70px',  align: 'right', render: v => <span style={{ color: C.dim }}>{v}%</span> },
            { k: 'veto',   label: 'veto',     w: '60px',  align: 'right', render: v => <span style={{ color: C.yellow }}>{v}%</span> },
            { k: 'ends',   label: 'ends',     w: '90px',  align: 'right' },
          ]}
          rows={items}
          selectedIdx={idx}
          onSelect={setIdx}
        />
      </Panel>
      <FootHint items={[['↑↓', 'move'], ['↵', 'detail'], ['v', 'vote', true], ['esc', 'back']]} />
    </div>
  );
}

function ViewProposalDetail({ ctx, id }) {
  const p = PROPOSALS.find(x => x.id === id) || PROPOSALS[0];
  ctx.useKey((e) => {
    if (e.key === 'v') { ctx.openConfirm({ kind: 'vote', item: p }); return true; }
    return false;
  });
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: 14, gap: 10 }}>
      <div style={{ display: 'flex', gap: 14, alignItems: 'center' }}>
        <div style={{ fontSize: 18, color: C.textBold, fontWeight: 700 }}>#{p.id} · {p.title}</div>
        {stateBadge(p.status)}
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: '1.4fr 1fr', gap: 12, flex: 1 }}>
        <Panel title="Tally">
          <VoteBar yes={p.yes} no={p.no} abstain={p.abstain} veto={p.veto} />
          <div style={{ marginTop: 16, display: 'flex', flexDirection: 'column', gap: 8, fontSize: 12.5 }}>
            <VoteRow color={C.green}  label="Yes"     v={p.yes} />
            <VoteRow color={C.red}    label="No"      v={p.no} />
            <VoteRow color={C.dim}    label="Abstain" v={p.abstain} />
            <VoteRow color={C.yellow} label="Veto"    v={p.veto} />
          </div>
        </Panel>
        <Panel title="Timeline">
          <div style={{ fontSize: 12.5, display: 'flex', flexDirection: 'column', gap: 10 }}>
            {[
              ['submitted',  '2026-05-04 11:02', true],
              ['deposit',    '2026-05-04 13:18', true],
              ['voting',     '2026-05-04 13:18', true],
              ['ends',       '2026-05-16 13:18', false],
              ['executed',   p.status === 'passed' ? '2026-05-16 13:20' : '—', p.status === 'passed'],
            ].map(([k, v, done], i) => (
              <div key={i} style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
                <span style={{ color: done ? C.green : C.dim }}>{done ? '●' : '○'}</span>
                <span style={{ color: C.dim, width: 90 }}>{k}</span>
                <span style={{ color: C.text }}>{v}</span>
              </div>
            ))}
          </div>
        </Panel>
        <Panel title="Description" style={{ gridColumn: '1 / -1' }}>
          <div style={{ fontSize: 12.5, color: C.text, lineHeight: 1.55, maxHeight: 140, overflow: 'auto' }}>
            This proposal raises the minimum deposit required to submit a governance proposal from 512 AKT to 1,000 AKT. The goal is to reduce spam proposals and align the deposit with current AKT market price.<br/><br/>
            <span style={{ color: C.dim }}>Submitted by</span> <span style={{ color: C.text }}>akash1abc…wxyz</span>
            <span style={{ color: C.dim }}> · </span><span style={{ color: C.text }}>akash-governance-wg</span>
          </div>
        </Panel>
      </div>
      <FootHint items={[['v', 'vote', true], ['esc', 'back']]} />
    </div>
  );
}

function VoteBar({ yes, no, abstain, veto }) {
  return (
    <div style={{ display: 'flex', height: 18, borderRadius: 3, overflow: 'hidden', width: '100%' }}>
      <div style={{ background: C.green,  width: `${yes}%`     }} />
      <div style={{ background: C.red,    width: `${no}%`      }} />
      <div style={{ background: '#444',   width: `${abstain}%` }} />
      <div style={{ background: C.yellow, width: `${veto}%`    }} />
    </div>
  );
}

function VoteRow({ color, label, v }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
      <span style={{ width: 10, height: 10, background: color, display: 'inline-block', borderRadius: 2 }} />
      <span style={{ color: C.dim, width: 70 }}>{label}</span>
      <span style={{ color: C.text, fontVariantNumeric: 'tabular-nums' }}>{v}%</span>
      <div style={{ flex: 1 }}>
        <Progress value={v} width={28} color={color} />
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Staking
// ─────────────────────────────────────────────────────────────
function ViewStaking({ ctx }) {
  const items = VALIDATORS;
  const [idx, setIdx, onKey] = useListSelect(items, (v) => ctx.push({ view: 'validator-detail', moniker: v.moniker }));
  ctx.useKey((e) => {
    if (onKey(e)) return true;
    if (e.key === 'd') { ctx.openConfirm({ kind: 'delegate', item: items[idx] }); return true; }
    if (e.key === 'u') { ctx.openConfirm({ kind: 'unbond',   item: items[idx] }); return true; }
    if (e.key === 'r') { ctx.openConfirm({ kind: 'redeleg',  item: items[idx] }); return true; }
    return false;
  });
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: 14, gap: 10 }}>
      <Panel title="Validators" active style={{ flex: 1 }} bodyStyle={{ padding: 0 }}>
        <Table
          cols={[
            { k: 'rank',       label: '#',          w: '48px',  align: 'right' },
            { k: 'moniker',    label: 'moniker',    w: '1fr' },
            { k: 'power',      label: 'power',      w: '90px',  align: 'right' },
            { k: 'vp',         label: 'vp%',        w: '70px',  align: 'right', render: v => <span style={{ color: C.text }}>{v.toFixed(2)}%</span> },
            { k: 'commission', label: 'commission', w: '100px', align: 'right' },
            { k: 'uptime',     label: 'uptime',     w: '90px',  align: 'right', render: v => <span style={{ color: v >= 99.95 ? C.green : C.yellow }}>{v.toFixed(2)}%</span> },
            { k: 'signed',     label: 'signed',     w: '90px',  align: 'right' },
          ]}
          rows={items}
          selectedIdx={idx}
          onSelect={setIdx}
        />
      </Panel>
      <FootHint items={[['↑↓', 'move'], ['↵', 'detail'], ['d', 'delegate', true], ['u', 'unbond'], ['r', 'redelegate'], ['esc', 'back']]} />
    </div>
  );
}

function ViewValidatorDetail({ ctx, moniker }) {
  const v = VALIDATORS.find(x => x.moniker === moniker) || VALIDATORS[0];
  ctx.useKey((e) => {
    if (e.key === 'd') { ctx.openConfirm({ kind: 'delegate', item: v }); return true; }
    if (e.key === 'u') { ctx.openConfirm({ kind: 'unbond',   item: v }); return true; }
    if (e.key === 'r') { ctx.openConfirm({ kind: 'redeleg',  item: v }); return true; }
    return false;
  });
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: 14, gap: 10 }}>
      <div style={{ display: 'flex', gap: 14, alignItems: 'center' }}>
        <div style={{ fontSize: 18, color: C.textBold, fontWeight: 700 }}>{v.moniker}</div>
        <Badge tone="info">rank #{v.rank}</Badge>
        <Badge tone="active">vp {v.vp.toFixed(2)}%</Badge>
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, flex: 1 }}>
        <Panel title="Power">
          <div style={{ fontSize: 12.5, display: 'flex', flexDirection: 'column', gap: 8 }}>
            <Row k="voting power" v={v.power} />
            <Row k="rank"         v={`#${v.rank}`} />
            <Row k="commission"   v={v.commission} />
            <Row k="max change"   v="1%/day" />
            <Row k="self bond"    v="142.0K AKT (1.7%)" />
          </div>
        </Panel>
        <Panel title="Performance">
          <div style={{ fontSize: 12.5, display: 'flex', flexDirection: 'column', gap: 8 }}>
            <Row k="uptime"       v={<span style={{ color: v.uptime >= 99.95 ? C.green : C.yellow }}>{v.uptime.toFixed(2)}%</span>} />
            <Row k="signed"       v={`${v.signed} / 4892`} />
            <Row k="missed"       v={`${4892 - v.signed}`} />
            <Row k="jailed"       v="never" />
            <Row k="slashes"      v="0" />
          </div>
        </Panel>
        <Panel title="Your delegation" style={{ gridColumn: '1 / -1' }}>
          <div style={{ fontSize: 12.5, display: 'flex', gap: 24 }}>
            <div><div style={{ color: C.dim }}>delegated</div><div style={{ color: C.textBold, fontSize: 18, fontWeight: 700 }}>820.40 AKT</div></div>
            <div><div style={{ color: C.dim }}>rewards</div><div style={{ color: C.green, fontSize: 18, fontWeight: 700 }}>+8.42 AKT</div></div>
            <div><div style={{ color: C.dim }}>APR</div><div style={{ color: C.text, fontSize: 18, fontWeight: 700 }}>14.81%</div></div>
            <div><div style={{ color: C.dim }}>unbonding</div><div style={{ color: C.text, fontSize: 18, fontWeight: 700 }}>—</div></div>
          </div>
        </Panel>
      </div>
      <FootHint items={[['d', 'delegate', true], ['u', 'unbond'], ['r', 'redelegate'], ['esc', 'back']]} />
    </div>
  );
}

Object.assign(window, {
  ViewDashboard, ViewDeployments, ViewDeploymentDetail,
  ViewLeases, ViewProviders, ViewProviderDetail,
  ViewGovernance, ViewProposalDetail, ViewStaking, ViewValidatorDetail,
  Row, FootHint, Table,
});
