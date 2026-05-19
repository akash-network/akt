// Mock data + shared primitives for the akt TUI prototype

// ─────────────────────────────────────────────────────────────
// Deployments
// ─────────────────────────────────────────────────────────────
const DEPLOYMENTS = [
  { dseq: '17834201', name: 'llama-3-70b-inference', state: 'active',  cpu: '8.0', mem: '32Gi', gpu: '2× H100',  storage: '500Gi', provider: 'overclock.akash.pub',     uptime: '14d 06h', cost: '184.20 AKT/mo', region: 'us-west' },
  { dseq: '17840192', name: 'stable-diffusion-xl',   state: 'active',  cpu: '4.0', mem: '16Gi', gpu: '1× A100',  storage: '200Gi', provider: 'praetorapp.com',          uptime: '08d 22h', cost:  '92.10 AKT/mo', region: 'us-east' },
  { dseq: '17848903', name: 'pgvector-db',           state: 'active',  cpu: '2.0', mem: '8Gi',  gpu: '—',         storage: '1Ti',   provider: 'europlots.com',           uptime: '21d 14h', cost:  '14.80 AKT/mo', region: 'eu-central' },
  { dseq: '17851440', name: 'next-frontend',         state: 'active',  cpu: '1.0', mem: '2Gi',  gpu: '—',         storage: '20Gi',  provider: 'akash.computer',          uptime: '02d 09h', cost:   '4.20 AKT/mo', region: 'us-west' },
  { dseq: '17853220', name: 'vault-cluster-a',       state: 'pending', cpu: '4.0', mem: '16Gi', gpu: '—',         storage: '500Gi', provider: '—',                       uptime: '—',       cost: '—',              region: '—' },
  { dseq: '17828910', name: 'whisper-api',           state: 'closed',  cpu: '2.0', mem: '8Gi',  gpu: '1× L4',     storage: '50Gi',  provider: 'mainnet.akash.computer',  uptime: 'closed',  cost:  '0.00 AKT/mo',  region: 'us-east' },
  { dseq: '17812005', name: 'analytics-warehouse',   state: 'closed',  cpu: '8.0', mem: '64Gi', gpu: '—',         storage: '4Ti',   provider: 'akashprovider.io',        uptime: 'closed',  cost:  '0.00 AKT/mo',  region: 'eu-west' },
];

const LEASES = [
  { dseq: '17834201', provider: 'overclock.akash.pub', state: 'active', price: '0.0061 uakt/blk', escrow: '142.4 AKT', opened: '2026-04-30' },
  { dseq: '17840192', provider: 'praetorapp.com',      state: 'active', price: '0.0030 uakt/blk', escrow:  '88.0 AKT', opened: '2026-05-06' },
  { dseq: '17848903', provider: 'europlots.com',       state: 'active', price: '0.0004 uakt/blk', escrow:  '12.2 AKT', opened: '2026-04-23' },
  { dseq: '17851440', provider: 'akash.computer',      state: 'active', price: '0.0001 uakt/blk', escrow:   '3.8 AKT', opened: '2026-05-11' },
];

const PROVIDERS = [
  { host: 'overclock.akash.pub',     region: 'us-west',    gpu: 'H100, A100',    cpu: '512c', mem: '4.0Ti',  active: 184, score: 99.8, version: '0.8.2' },
  { host: 'praetorapp.com',          region: 'us-east',    gpu: 'A100, L4',      cpu: '256c', mem: '2.0Ti',  active:  92, score: 99.4, version: '0.8.2' },
  { host: 'europlots.com',           region: 'eu-central', gpu: '—',             cpu: '128c', mem: '1.0Ti',  active:  58, score: 98.1, version: '0.8.1' },
  { host: 'akash.computer',          region: 'us-west',    gpu: 'L4, T4',        cpu: '320c', mem: '2.5Ti',  active: 142, score: 99.2, version: '0.8.2' },
  { host: 'mainnet.akash.computer',  region: 'us-east',    gpu: 'A40',           cpu: '192c', mem: '1.5Ti',  active:  77, score: 97.6, version: '0.8.0' },
  { host: 'akashprovider.io',        region: 'eu-west',    gpu: 'A100',          cpu: '160c', mem: '1.2Ti',  active:  41, score: 96.9, version: '0.8.1' },
  { host: 'computefarm.cloud',       region: 'ap-south',   gpu: 'H100',          cpu: '256c', mem: '2.0Ti',  active:  63, score: 99.0, version: '0.8.2' },
  { host: 'sandbox.akash.pub',       region: 'us-west',    gpu: 'T4',            cpu:  '64c', mem: '512Gi',  active:  18, score: 95.4, version: '0.8.2' },
];

const VALIDATORS = [
  { rank:  1, moniker: 'DCloud',              power: '8.42M',  commission: '5%',  uptime: 100.00, vp: 7.81, signed: 4892 },
  { rank:  2, moniker: 'Akash Capital',       power: '6.91M',  commission: '5%',  uptime:  99.99, vp: 6.41, signed: 4891 },
  { rank:  3, moniker: 'Cosmostation',        power: '5.84M',  commission: '10%', uptime: 100.00, vp: 5.41, signed: 4892 },
  { rank:  4, moniker: 'Allnodes',            power: '4.62M',  commission: '5%',  uptime:  99.97, vp: 4.28, signed: 4890 },
  { rank:  5, moniker: 'Forbole',             power: '4.18M',  commission: '4%',  uptime:  99.98, vp: 3.87, signed: 4891 },
  { rank:  6, moniker: 'Stakefish',           power: '3.94M',  commission: '5%',  uptime: 100.00, vp: 3.65, signed: 4892 },
  { rank:  7, moniker: 'Polychain Labs',      power: '3.71M',  commission: '8%',  uptime:  99.95, vp: 3.44, signed: 4889 },
  { rank:  8, moniker: 'Imperator.co',        power: '3.50M',  commission: '5%',  uptime: 100.00, vp: 3.24, signed: 4892 },
  { rank:  9, moniker: 'Lavender.Five Nodes', power: '3.21M',  commission: '4%',  uptime:  99.99, vp: 2.97, signed: 4891 },
  { rank: 10, moniker: 'Notional',            power: '2.98M',  commission: '5%',  uptime:  99.97, vp: 2.76, signed: 4890 },
  { rank: 11, moniker: 'Chorus One',          power: '2.74M',  commission: '8%',  uptime:  99.94, vp: 2.54, signed: 4888 },
  { rank: 12, moniker: 'P-OPS Team',          power: '2.51M',  commission: '5%',  uptime: 100.00, vp: 2.33, signed: 4892 },
];

const PROPOSALS = [
  { id: 91, title: 'Update min deposit to 1,000 AKT',         status: 'voting',   yes: 78.2, no:  4.1, abstain: 12.7, veto: 5.0, ends: '2d 14h' },
  { id: 90, title: 'Enable GPU bid engine v3 mainnet',         status: 'passed',   yes: 92.4, no:  2.6, abstain:  4.0, veto: 1.0, ends: 'ended' },
  { id: 89, title: 'Provider audit fee rebate program',        status: 'passed',   yes: 81.0, no: 10.1, abstain:  7.4, veto: 1.5, ends: 'ended' },
  { id: 88, title: 'Inflation schedule revision FY26',         status: 'rejected', yes: 31.2, no: 58.4, abstain:  6.0, veto: 4.4, ends: 'ended' },
  { id: 87, title: 'Mainnet upgrade v0.40 (Lipari)',           status: 'passed',   yes: 96.1, no:  1.0, abstain:  2.4, veto: 0.5, ends: 'ended' },
];

const COMMANDS = [
  { id: 'goto.dashboard',    title: 'Go to Dashboard',          hint: 'h',     icon: '⌂' },
  { id: 'goto.deployments',  title: 'Go to Deployments',        hint: '1',     icon: '▦' },
  { id: 'goto.leases',       title: 'Go to Leases',             hint: '2',     icon: '◐' },
  { id: 'goto.providers',    title: 'Go to Providers',          hint: '3',     icon: '◇' },
  { id: 'goto.monitor',      title: 'Go to Monitor Hub',        hint: '4',     icon: '◉' },
  { id: 'goto.gov',          title: 'Go to Governance',         hint: '5',     icon: '§' },
  { id: 'goto.staking',      title: 'Go to Staking',            hint: '6',     icon: '⎈' },
  { id: 'deploy.new',        title: 'New Deployment (akt deploy)', hint: 'D',  icon: '+' },
  { id: 'deploy.from-sdl',   title: 'Deploy from SDL file…',    hint: '',      icon: '⊕' },
  { id: 'wallet.balance',    title: 'Show wallet balance',      hint: '',      icon: '$' },
  { id: 'wallet.switch',     title: 'Switch wallet…',           hint: '',      icon: '⇄' },
  { id: 'chain.switch',      title: 'Switch RPC endpoint…',     hint: '',      icon: '⇆' },
  { id: 'logs.tail',         title: 'Tail logs for deployment…',hint: 'l',     icon: '≡' },
  { id: 'shell.open',        title: 'Open shell in deployment…',hint: 's',     icon: '$_' },
  { id: 'help',              title: 'Show help',                hint: '?',     icon: '?' },
  { id: 'quit',              title: 'Quit akt',                 hint: 'q',     icon: '⏻' },
];

const SDL_SAMPLE = `---
version: "2.0"
services:
  llm:
    image: vllm/vllm-openai:v0.5.4
    expose:
      - port: 8000
        as: 80
        to: [{ global: true }]
    env:
      - MODEL=meta-llama/Llama-3-70B-Instruct
      - MAX_CONTEXT=8192
profiles:
  compute:
    llm:
      resources:
        cpu:     { units: 8 }
        memory:  { size: 32Gi }
        gpu:     { units: 2, attributes: { vendor: { nvidia: [{ model: h100 }] } } }
        storage: { size: 500Gi }
  placement:
    westcoast:
      pricing:
        llm: { denom: uakt, amount: 1000 }
deployment:
  llm:
    westcoast:
      profile: llm
      count: 1`;

// ─────────────────────────────────────────────────────────────
// Shared atoms (no JSX yet — pure data/strings exported)
// ─────────────────────────────────────────────────────────────

// Box-drawing helpers (used as text in places)
const BOX = { tl: '╭', tr: '╮', bl: '╰', br: '╯', h: '─', v: '│', tee: '┴', cross: '┼' };

// Color tokens (used in inline style via JS)
const C = {
  bg:        '#0d0d0f',
  panel:     '#141416',
  panelHi:   '#1a1a1d',
  border:    '#2c2c30',
  borderHi:  '#46464c',
  dim:       '#6b6b72',
  text:      '#e7e7ea',
  textBold:  '#ffffff',
  red:       '#ff414c',
  redDim:    'rgba(255,65,76,0.18)',
  redBg:     'rgba(255,65,76,0.08)',
  green:     '#1fd97e',
  yellow:    '#ffc966',
  blue:      '#65b3ff',
  purple:    '#c08cff',
  magenta:   '#ff7ccb',
};

Object.assign(window, {
  DEPLOYMENTS, LEASES, PROVIDERS, VALIDATORS, PROPOSALS, COMMANDS, SDL_SAMPLE,
  BOX, C,
});
