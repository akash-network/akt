// Overlays: Command Palette, Confirmation Dialog, Help

const { useState: useS4, useEffect: useE4, useMemo: useM4, useRef: useR4 } = React;

// ─────────────────────────────────────────────────────────────
// Command palette
// ─────────────────────────────────────────────────────────────
function CommandPalette({ onClose, onRun }) {
  const [q, setQ]     = useS4('');
  const [idx, setIdx] = useS4(0);
  const filtered = useM4(() => {
    if (!q) return COMMANDS;
    const t = q.toLowerCase();
    return COMMANDS.filter(c => c.title.toLowerCase().includes(t) || c.id.toLowerCase().includes(t));
  }, [q]);

  useE4(() => {
    const handler = (e) => {
      if (e.key === 'Escape') { e.stopPropagation(); onClose(); return; }
      if (e.key === 'ArrowDown' || (e.ctrlKey && e.key === 'j')) { e.preventDefault(); setIdx(i => Math.min(filtered.length - 1, i + 1)); return; }
      if (e.key === 'ArrowUp'   || (e.ctrlKey && e.key === 'k')) { e.preventDefault(); setIdx(i => Math.max(0, i - 1)); return; }
      if (e.key === 'Enter') { e.preventDefault(); if (filtered[idx]) onRun(filtered[idx]); return; }
      if (e.key === 'Backspace') { e.preventDefault(); setQ(s => s.slice(0, -1)); setIdx(0); return; }
      if (e.key.length === 1 && !e.metaKey && !e.ctrlKey) { e.preventDefault(); setQ(s => s + e.key); setIdx(0); return; }
    };
    window.addEventListener('keydown', handler, true);
    return () => window.removeEventListener('keydown', handler, true);
  }, [filtered, idx, onClose, onRun]);

  return (
    <Backdrop>
      <div style={{ width: 540, background: C.panel, border: `1px solid ${C.red}`, borderRadius: 6, boxShadow: '0 18px 60px rgba(0,0,0,0.6)', overflow: 'hidden' }}>
        <div style={{ padding: '12px 14px', borderBottom: `1px solid ${C.border}`, display: 'flex', alignItems: 'center', gap: 10 }}>
          <span style={{ color: C.red, fontSize: 14, fontWeight: 700 }}>:</span>
          <span style={{ color: C.textBold, fontSize: 14, flex: 1 }}>{q}<Cursor /></span>
          <Badge tone="accent">command</Badge>
        </div>
        <div style={{ maxHeight: 360, overflow: 'auto' }}>
          {filtered.length === 0 && <div style={{ padding: 14, color: C.dim, fontSize: 12 }}>no commands match</div>}
          {filtered.map((c, i) => {
            const sel = i === idx;
            return (
              <div key={c.id} onMouseEnter={() => setIdx(i)} onClick={() => onRun(c)} style={{
                display: 'grid', gridTemplateColumns: '32px 1fr 50px',
                alignItems: 'center', gap: 12,
                padding: '8px 14px',
                cursor: 'pointer',
                background: sel ? C.redBg : 'transparent',
                borderLeft: `2px solid ${sel ? C.red : 'transparent'}`,
                color: sel ? C.textBold : C.text,
                fontSize: 13,
              }}>
                <span style={{ color: sel ? C.red : C.dim, fontSize: 14 }}>{c.icon}</span>
                <span>{c.title}</span>
                <span style={{ textAlign: 'right' }}>{c.hint && <KeyPill k={c.hint} accent={sel} />}</span>
              </div>
            );
          })}
        </div>
        <div style={{ padding: '8px 14px', borderTop: `1px solid ${C.border}`, display: 'flex', gap: 14, fontSize: 11 }}>
          <KeyPill k="↑↓" label="navigate" />
          <KeyPill k="↵"  label="run" />
          <KeyPill k="esc" label="close" />
        </div>
      </div>
    </Backdrop>
  );
}

// ─────────────────────────────────────────────────────────────
// Confirmation dialog (close / vote / delegate / etc)
// ─────────────────────────────────────────────────────────────
function ConfirmDialog({ kind, item, onClose, onConfirm }) {
  const [choice, setChoice] = useS4(kind === 'vote' ? 'yes' : null);
  const [amount, setAmount] = useS4('100.00');
  const cfg = confirmConfig(kind, item);

  useE4(() => {
    const handler = (e) => {
      if (e.key === 'Escape') { e.stopPropagation(); onClose(); return; }
      if (e.key === 'Enter')  { e.stopPropagation(); onConfirm({ choice, amount }); return; }
      if (e.key === 'Tab')    { e.preventDefault(); }
      if (kind === 'vote') {
        if (e.key === 'y') setChoice('yes');
        if (e.key === 'n') setChoice('no');
        if (e.key === 'a') setChoice('abstain');
        if (e.key === 'v') setChoice('veto');
      }
      if ((kind === 'delegate' || kind === 'unbond') && /[0-9.]/.test(e.key) && e.key.length === 1) {
        setAmount(a => a + e.key);
      }
      if ((kind === 'delegate' || kind === 'unbond') && e.key === 'Backspace') {
        setAmount(a => a.slice(0, -1));
      }
    };
    window.addEventListener('keydown', handler, true);
    return () => window.removeEventListener('keydown', handler, true);
  }, [kind, choice, amount, onClose, onConfirm]);

  return (
    <Backdrop>
      <div style={{ width: 480, background: C.panel, border: `1px solid ${cfg.danger ? C.red : C.borderHi}`, borderRadius: 6, boxShadow: '0 18px 60px rgba(0,0,0,0.6)' }}>
        <div style={{ padding: '12px 16px', borderBottom: `1px solid ${C.border}`, display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ color: cfg.danger ? C.red : C.text, fontSize: 13, fontWeight: 700 }}>{cfg.title}</span>
          <div style={{ flex: 1 }} />
          {cfg.danger && <Badge tone="error">destructive</Badge>}
        </div>
        <div style={{ padding: 16, fontSize: 12.5, color: C.text, lineHeight: 1.55 }}>
          {cfg.body}
          {kind === 'vote' && (
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr 1fr', gap: 8, marginTop: 12 }}>
              {['yes', 'no', 'abstain', 'veto'].map(opt => (
                <div key={opt} onClick={() => setChoice(opt)} style={{
                  padding: '8px 10px',
                  textAlign: 'center',
                  border: `1px solid ${choice === opt ? C.red : C.border}`,
                  background: choice === opt ? C.redBg : 'transparent',
                  borderRadius: 3,
                  cursor: 'pointer',
                  color: choice === opt ? C.textBold : C.text,
                  fontSize: 12,
                  textTransform: 'capitalize',
                }}>
                  <span style={{ color: C.red, marginRight: 6, fontWeight: 700 }}>{opt[0]}</span>{opt}
                </div>
              ))}
            </div>
          )}
          {(kind === 'delegate' || kind === 'unbond') && (
            <div style={{ marginTop: 12 }}>
              <div style={{ color: C.dim, fontSize: 11, marginBottom: 4 }}>amount</div>
              <div style={{ border: `1px solid ${C.red}`, background: C.bg, padding: '8px 10px', borderRadius: 3, fontSize: 14, color: C.textBold }}>
                {amount} <span style={{ color: C.dim }}>AKT</span><Cursor />
              </div>
            </div>
          )}
        </div>
        <div style={{ padding: '10px 16px', borderTop: `1px solid ${C.border}`, background: C.panelHi, display: 'flex', flexDirection: 'column', gap: 6, fontSize: 12 }}>
          <Row k="fee preview" v={<span style={{ color: C.text }}>0.0142 AKT (gas 14,200 × 0.001)</span>} />
          <Row k="from"        v="akash1zk2…vq4r" />
        </div>
        <div style={{ padding: '10px 16px', display: 'flex', gap: 14, justifyContent: 'flex-end', fontSize: 12 }}>
          <KeyPill k="esc" label="cancel" />
          <KeyPill k="↵"   label={cfg.confirmLabel} accent />
        </div>
      </div>
    </Backdrop>
  );
}

function confirmConfig(kind, item) {
  switch (kind) {
    case 'close':    return { title: `Close deployment ${item?.dseq}?`, danger: true,
      body: <>This will terminate <span style={{ color: C.textBold }}>{item?.name}</span> and release escrow. Remaining <span style={{ color: C.green }}>142.40 AKT</span> will return to your wallet.</>, confirmLabel: 'close' };
    case 'vote':     return { title: `Vote on proposal #${item?.id}`, danger: false,
      body: <>{item?.title} <div style={{ color: C.dim, marginTop: 4 }}>Voting period ends in <span style={{ color: C.text }}>{item?.ends}</span>.</div></>, confirmLabel: 'submit vote' };
    case 'delegate': return { title: `Delegate to ${item?.moniker}`, danger: false,
      body: <>You'll start earning rewards next block. Unbonding takes <span style={{ color: C.yellow }}>21 days</span>.</>, confirmLabel: 'delegate' };
    case 'unbond':   return { title: `Unbond from ${item?.moniker}?`, danger: true,
      body: <>Funds will be locked for <span style={{ color: C.yellow }}>21 days</span> before becoming spendable.</>, confirmLabel: 'unbond' };
    case 'redeleg':  return { title: `Redelegate from ${item?.moniker}`, danger: false,
      body: <>Move stake to another validator without waiting for unbonding. <span style={{ color: C.dim }}>(7-day cooldown applies)</span></>, confirmLabel: 'redelegate' };
    default: return { title: 'Confirm', body: '', confirmLabel: 'confirm' };
  }
}

// ─────────────────────────────────────────────────────────────
// Help overlay
// ─────────────────────────────────────────────────────────────
function HelpOverlay({ ctx, onClose }) {
  useE4(() => {
    const handler = (e) => { if (e.key === 'Escape' || e.key === '?') { e.stopPropagation(); onClose(); }};
    window.addEventListener('keydown', handler, true);
    return () => window.removeEventListener('keydown', handler, true);
  }, [onClose]);

  const sections = [
    { title: 'Navigation', items: [
      ['1', 'Deployments'], ['2', 'Leases'], ['3', 'Providers'],
      ['4', 'Monitor Hub'], ['5', 'Governance'], ['6', 'Staking'],
      ['h', 'home / dashboard'], ['↵', 'drill into selection'], ['esc', 'pop back'],
      ['Tab / Shift-Tab', 'cycle sub-tabs'],
    ]},
    { title: 'Lists', items: [
      ['j / ↓', 'next'], ['k / ↑', 'prev'], ['g / G', 'first / last'], ['/', 'fuzzy search'],
    ]},
    { title: 'Actions', items: [
      ['D', 'new deployment'], ['l', 'tail logs'], ['s', 'open shell'],
      ['d', 'destructive action (close/unbond)'], ['v', 'vote'], ['r', 'redelegate'],
    ]},
    { title: 'Overlays', items: [
      [': / Ctrl+P', 'command palette'], ['?', 'this help'], ['q', 'quit akt'],
    ]},
  ];

  return (
    <Backdrop>
      <div style={{ width: 680, background: C.panel, border: `1px solid ${C.borderHi}`, borderRadius: 6, boxShadow: '0 18px 60px rgba(0,0,0,0.6)' }}>
        <div style={{ padding: '12px 16px', borderBottom: `1px solid ${C.border}`, display: 'flex', alignItems: 'center', gap: 10 }}>
          <Badge tone="accent">?</Badge>
          <span style={{ color: C.textBold, fontSize: 14, fontWeight: 700 }}>Keybindings</span>
          <span style={{ color: C.dim, fontSize: 12 }}>context-sensitive · current view: {ctx.currentLabel}</span>
        </div>
        <div style={{ padding: 16, display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 20 }}>
          {sections.map(s => (
            <div key={s.title}>
              <div style={{ fontSize: 11, color: C.red, fontWeight: 700, textTransform: 'uppercase', letterSpacing: 0.6, marginBottom: 8 }}>{s.title}</div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6, fontSize: 12.5 }}>
                {s.items.map(([k, l], i) => (
                  <div key={i} style={{ display: 'grid', gridTemplateColumns: '120px 1fr', gap: 8 }}>
                    <KeyPill k={k} />
                    <span style={{ color: C.text }}>{l}</span>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
        <div style={{ padding: '10px 16px', borderTop: `1px solid ${C.border}`, fontSize: 11, color: C.dim, display: 'flex', justifyContent: 'space-between' }}>
          <span>akt v1.2.0 · <span style={{ color: C.text }}>akash.network</span></span>
          <span>press <KeyPill k="esc" label="" /> to close</span>
        </div>
      </div>
    </Backdrop>
  );
}

// ─────────────────────────────────────────────────────────────
// Toast — transient flash for action results
// ─────────────────────────────────────────────────────────────
function Toast({ toast, onDone }) {
  useE4(() => {
    if (!toast) return;
    const id = setTimeout(onDone, 2400);
    return () => clearTimeout(id);
  }, [toast, onDone]);
  if (!toast) return null;
  const tones = {
    ok:    { fg: C.green,  bg: 'rgba(31,217,126,0.10)', border: C.green,  icon: '✓' },
    info:  { fg: C.blue,   bg: 'rgba(101,179,255,0.10)',border: C.blue,   icon: 'ℹ' },
    err:   { fg: C.red,    bg: 'rgba(255,65,76,0.10)',  border: C.red,    icon: '✗' },
  };
  const t = tones[toast.tone] || tones.info;
  return (
    <div style={{
      position: 'absolute', bottom: 48, right: 18, zIndex: 50,
      padding: '10px 14px',
      background: C.panel,
      border: `1px solid ${t.border}`,
      borderLeft: `3px solid ${t.border}`,
      borderRadius: 4,
      display: 'flex', gap: 10, alignItems: 'center',
      minWidth: 260, maxWidth: 420,
      boxShadow: '0 12px 32px rgba(0,0,0,0.4)',
      fontSize: 12.5,
    }}>
      <span style={{ color: t.fg, fontSize: 14 }}>{t.icon}</span>
      <span style={{ color: C.text }}>{toast.msg}</span>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────
// Backdrop wrapper
// ─────────────────────────────────────────────────────────────
function Backdrop({ children }) {
  return (
    <div style={{
      position: 'absolute', inset: 0, zIndex: 100,
      background: 'rgba(0,0,0,0.55)',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      animation: 'fadeIn 120ms ease-out',
    }}>
      {children}
    </div>
  );
}

Object.assign(window, { CommandPalette, ConfirmDialog, HelpOverlay, Toast });
