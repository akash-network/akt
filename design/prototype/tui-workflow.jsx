// Deployment workflow — 7 animated steps

const { useState: useS3, useEffect: useE3, useMemo: useM3 } = React;

const STEPS = [
  { id: 'parse',   name: 'Parse SDL',     desc: 'validate manifest',   dur: 1200 },
  { id: 'create',  name: 'Create',        desc: 'MsgCreateDeployment', dur: 2400 },
  { id: 'bids',    name: 'Bids',          desc: '60-block auction',    dur: 4000 },
  { id: 'select',  name: 'Select',        desc: 'rank by price + audit', dur: 1800 },
  { id: 'lease',   name: 'Lease',         desc: 'MsgCreateLease',      dur: 2200 },
  { id: 'send',    name: 'Manifest',      desc: 'mTLS upload',         dur: 1600 },
  { id: 'wait',    name: 'Active',        desc: 'pull image · run',    dur: 3200 },
  { id: 'show',    name: 'Endpoints',     desc: 'forwarded URIs',      dur: 800  },
];

function ViewWorkflow({ ctx }) {
  const [step, setStep]     = useS3(0);
  const [phase, setPhase]   = useS3('running'); // running | done
  const [log, setLog]       = useS3([]);
  const [paused, setPaused] = useS3(false);
  const [bids, setBids]     = useS3([]);

  // tick the step progression
  useE3(() => {
    if (paused || phase === 'done') return;
    if (step >= STEPS.length) { setPhase('done'); return; }
    const s = STEPS[step];

    // emit initial lines for this step
    setLog(l => [...l, { c: C.dim, t: `─ ${s.name.toLowerCase()} ─────────────────────` }, ...stepIntro(s.id)]);

    let tick = 0;
    const interval = 200;
    const target = Math.ceil(s.dur / interval);
    const id = setInterval(() => {
      tick++;
      // stream a chunk of fake output
      const lines = stepTick(s.id, tick, target);
      if (lines.length) setLog(l => [...l, ...lines]);
      if (s.id === 'bids') {
        // accumulate bids over time
        setBids(b => {
          if (b.length < 12 && Math.random() > 0.4) return [...b, mkBid(b.length)];
          return b;
        });
      }
      if (tick >= target) {
        clearInterval(id);
        setLog(l => [...l, { c: C.green, t: `  ✓ ${s.name} complete` }, { c: C.dim, t: '' }]);
        setStep(v => v + 1);
      }
    }, interval);
    return () => clearInterval(id);
  }, [step, paused]);

  ctx.useKey((e) => {
    if (e.key === ' ' || e.key === 'p') { setPaused(p => !p); return true; }
    if (e.key === 'r' && phase === 'done') { setStep(0); setPhase('running'); setLog([]); setBids([]); return true; }
    return false;
  });

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: 14, gap: 10 }}>
      <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
        <Badge tone="accent">akt deploy</Badge>
        <span style={{ color: C.text, fontSize: 13 }}>./deploy.yaml</span>
        <span style={{ color: C.dim, fontSize: 12 }}>llama-3-70b-inference</span>
        <div style={{ flex: 1 }}/>
        {phase === 'done'
          ? <Badge tone="active">deployment ready</Badge>
          : paused ? <Badge tone="pending">paused</Badge>
          : <><Spinner/><span style={{ color: C.red, fontSize: 12 }}>{STEPS[step]?.name || ''}</span></>}
      </div>

      {/* step strip */}
      <Panel padding="12px 14px">
        <div style={{ display: 'flex', alignItems: 'stretch', gap: 0 }}>
          {STEPS.map((s, i) => {
            const done    = i < step;
            const current = i === step && phase === 'running';
            return (
              <React.Fragment key={s.id}>
                <div style={{ flex: 1, textAlign: 'center', padding: '4px 4px', minWidth: 0 }}>
                  <div style={{
                    width: 26, height: 26,
                    borderRadius: '50%',
                    border: `1.5px solid ${done ? C.green : current ? C.red : C.border}`,
                    background: done ? C.green : current ? 'transparent' : C.panel,
                    color: done ? '#000' : current ? C.red : C.dim,
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                    fontSize: 12, fontWeight: 700,
                    margin: '0 auto',
                  }}>
                    {done ? '✓' : current ? <Spinner color={C.red}/> : i + 1}
                  </div>
                  <div style={{
                    marginTop: 6, fontSize: 11.5,
                    color: done ? C.green : current ? C.textBold : C.dim,
                    fontWeight: current ? 700 : 500,
                    whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
                  }}>{s.name}</div>
                  <div style={{
                    fontSize: 10, color: C.dim, marginTop: 2,
                    whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
                  }}>{s.desc}</div>
                </div>
                {i < STEPS.length - 1 && (
                  <div style={{ alignSelf: 'center', width: 14, height: 1.5, background: done ? C.green : C.border, marginTop: -22 }} />
                )}
              </React.Fragment>
            );
          })}
        </div>
      </Panel>

      <div style={{ display: 'grid', gridTemplateColumns: '1.4fr 1fr', gap: 12, flex: 1, minHeight: 0 }}>
        <Panel title="Stream" active style={{ display: 'flex', flexDirection: 'column' }} bodyStyle={{ padding: 0, flex: 1, display: 'flex', flexDirection: 'column' }}>
          <LogStream log={log} />
        </Panel>
        <Panel title={STEPS[step]?.id === 'bids' || step >= 3 ? 'Bids received' : 'Pending'} style={{ display: 'flex', flexDirection: 'column' }} bodyStyle={{ padding: 0, flex: 1 }}>
          <BidPanel bids={bids} selectedIdx={step > 3 ? 1 : null} />
        </Panel>
      </div>

      {phase === 'done' ? (
        <Panel title="Endpoints" active>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6, fontSize: 12.5 }}>
            <Row k="status"  v={<Badge tone="active">live</Badge>} />
            <Row k="dseq"    v="17854992" />
            <Row k="provider" v="praetorapp.com" />
            <Row k="endpoint" v={<span style={{ color: C.textBold }}>https://provider-4992.praetorapp.com</span>} />
            <Row k="lease cost" v={<span>0.0030 uakt/blk · ~92 AKT/mo</span>} />
          </div>
          <div style={{ marginTop: 10, padding: 10, background: C.panelHi, borderRadius: 3, fontSize: 12 }}>
            <span style={{ color: C.green }}>$</span> <span style={{ color: C.text }}>curl https://provider-4992.praetorapp.com/v1/completions \</span><br/>
            <span style={{ color: C.dim, marginLeft: 18 }}>     -d </span><span style={{ color: C.yellow }}>'{`{"prompt":"hello"}`}'</span>
          </div>
          <FootHint items={[['r', 'redeploy', true], ['l', 'tail logs'], ['esc', 'dashboard']]} />
        </Panel>
      ) : (
        <FootHint items={[['space', paused ? 'resume' : 'pause'], ['c', 'cancel'], ['esc', 'back']]} />
      )}
    </div>
  );
}

function LogStream({ log }) {
  const ref = React.useRef(null);
  useE3(() => { if (ref.current) ref.current.scrollTop = ref.current.scrollHeight; }, [log.length]);
  return (
    <div ref={ref} style={{ overflow: 'auto', padding: 10, fontSize: 11.5, lineHeight: 1.45, height: '100%', minHeight: 0 }}>
      {log.map((l, i) => (
        <div key={i} style={{ color: l.c, whiteSpace: 'pre-wrap' }}>{l.t}</div>
      ))}
      <Cursor />
    </div>
  );
}

function BidPanel({ bids, selectedIdx }) {
  if (!bids.length) {
    return (
      <div style={{ padding: 16, color: C.dim, fontSize: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
        <Spinner color={C.dim}/> waiting for bids…
      </div>
    );
  }
  return (
    <div style={{ overflow: 'auto', height: '100%' }}>
      {bids.map((b, i) => {
        const sel = selectedIdx === i;
        return (
          <div key={i} style={{
            display: 'grid', gridTemplateColumns: '1fr 60px 50px 24px',
            gap: 10, alignItems: 'center',
            padding: '6px 12px',
            background: sel ? C.redBg : 'transparent',
            borderLeft: `2px solid ${sel ? C.red : 'transparent'}`,
            fontSize: 12,
          }}>
            <span style={{ color: sel ? C.textBold : C.text, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{b.host}</span>
            <span style={{ color: C.dim, textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{b.bid}</span>
            <span style={{ color: b.audit > 99 ? C.green : C.yellow, textAlign: 'right', fontSize: 11 }}>{b.audit.toFixed(1)}</span>
            <span style={{ color: sel ? C.red : C.dim }}>{sel ? '◉' : '○'}</span>
          </div>
        );
      })}
    </div>
  );
}

function mkBid(i) {
  const p = PROVIDERS[i % PROVIDERS.length];
  return { host: p.host, bid: (0.0008 + Math.random() * 0.012).toFixed(4), audit: p.score };
}

function stepIntro(id) {
  switch (id) {
    case 'parse':  return [{ c: C.text, t: '  reading deploy.yaml' }, { c: C.text, t: '  validating schema (v2.0)' }];
    case 'create': return [{ c: C.text, t: '  gas estimate 84,210 → fee 0.084 AKT' }];
    case 'bids':   return [{ c: C.text, t: '  auction opens at block 16,234,892' }, { c: C.dim, t: '  awaiting first bid…' }];
    case 'select': return [{ c: C.text, t: '  sorting bids: price → audit → region' }];
    case 'lease':  return [{ c: C.text, t: '  signing MsgCreateLease with akash1zk2…vq4r' }];
    case 'send':   return [{ c: C.text, t: '  establishing mTLS to provider' }];
    case 'wait':   return [{ c: C.text, t: '  provider pulling vllm/vllm-openai:v0.5.4' }];
    case 'show':   return [];
    default: return [];
  }
}

function stepTick(id, tick, target) {
  if (tick === Math.floor(target / 2)) {
    switch (id) {
      case 'parse':   return [{ c: C.green, t: '    services: 1 · profiles: 1 · placement: 1' }];
      case 'create':  return [{ c: C.text,  t: '    tx hash 0x8f4a…d12c'  }, { c: C.text, t: '    dseq 17854992 oseq 1 gseq 1' }];
      case 'select':  return [{ c: C.text,  t: '    chosen: praetorapp.com · 0.0030 uakt/blk' }];
      case 'lease':   return [{ c: C.text,  t: '    tx hash 0xa9c1…7b88'   }];
      case 'send':    return [{ c: C.text,  t: '    PUT /deployment/17854992/manifest 200' }];
      case 'wait':    return [{ c: C.text,  t: '    image layer 4/8 (1.2 GiB pulled)' }];
      default: return [];
    }
  }
  if (id === 'bids' && tick % 4 === 0) {
    const p = PROVIDERS[(tick / 4) % PROVIDERS.length];
    return [{ c: C.text, t: `    bid ${tick/4}/12 · ${p.host} · ${(0.0008 + Math.random() * 0.012).toFixed(4)} uakt/blk` }];
  }
  return [];
}

Object.assign(window, { ViewWorkflow });
