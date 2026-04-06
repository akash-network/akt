# Test Environment

Isolated environment for running `akt` without affecting your real config. Uses [direnv](https://direnv.net) to scope `AKT_HOME` to `.cache/run/test/.akt`.

## Setup

```bash
cd _run/test
direnv allow
make akt          # build the binary
```

## Working Commands

### Version

```bash
akt version
```

### Context Management

```bash
akt context list
akt context create mainnet --network mainnet --set-current
akt context create dev --network testnet --keyring dev --default-account alice
akt context use mainnet
akt context show
akt context edit mainnet --default-account bob --gas auto
akt context rename dev staging
akt context delete staging --yes
akt context log
akt context log --type tx --limit 10 --since 1h
```

### Network Management

```bash
akt context network list
akt context network create mainnet --template mainnet
akt context network create local --chain-id localnet-1 --rpc http://localhost:26657
akt context network show mainnet
akt context network edit mainnet --gas-prices 0.04uakt
akt context network delete local
```

### Key Management

Requires an active context (`akt context use <name>`).

```bash
akt context keys add alice
akt context keys add alice --recover        # from mnemonic
akt context keys list
akt context keys show alice
akt context keys show alice --address       # bech32 only
akt context keys export alice > alice.key
akt context keys import bob alice.key
akt context keys rename alice alice2
akt context keys delete alice2
akt context keys mnemonic
akt context keys parse akash1...
```

### Monitor (`akt monitor`)

No keyring or active context required -- just an RPC endpoint.

```bash
akt monitor                                      # hub, defaults to Network dashboard
akt monitor https://rpc.akashnet.net:443         # positional endpoint
akt monitor network                              # network dashboard directly
akt monitor provider                             # provider dashboard directly
akt monitor oracle                               # oracle/BME dashboard (does not 
akt monitor bme                                  # oracle/BME dashboard (alias)
akt monitor --rpc https://rpc.akashnet.net:443   # flag endpoint
akt monitor --insecure                           # skip TLS verification
akt monitor --clean-cache                        # clear cache
```

Hub: `Tab`/`Shift-Tab` cycle dashboards (Network, Provider, Oracle/BME). Network sub-tabs: `1` Overview, `2` Validators, `3` Governance. `q` quit.

### TUI Mode

```bash
akt                                          # launches interactive TUI
```

### Transactions & Queries

Requires an active context with a valid RPC endpoint and (for tx) a funded account.

```bash
# Queries (positional filter argument)
akt query bank balances akash1...
akt query deployment                         # list for default account
akt query deployment 12345                   # by dseq
akt query market lease --state active
akt query provider akash1prov...
akt query staking validators

# Transactions
akt tx bank send alice akash1dest... 1000000uakt
akt tx deployment create deploy.yaml
akt tx deployment close --dseq 12345
```

## Reset

Delete the scoped home to start fresh:

```bash
make clean
```
