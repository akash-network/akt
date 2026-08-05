package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

// prepareCommandHelp normalizes examples after the complete command tree has
// been assembled. Dependency-provided commands commonly embed an Example
// block in Long instead of setting cobra.Command.Example, which makes help
// formatting inconsistent and leaves the field unavailable to other clients.
func prepareCommandHelp(root *cobra.Command) {
	walkCommandTree(root, func(cmd *cobra.Command) {
		if strings.TrimSpace(cmd.Example) != "" {
			return
		}

		long, example, ok := strings.Cut(cmd.Long, "\nExample:\n")
		if !ok {
			return
		}

		cmd.Long = strings.TrimSpace(long)
		cmd.Example = strings.TrimSpace(example)
	})

	examples := map[string]string{
		"completion bash":       `  akt completion bash > /etc/bash_completion.d/akt`,
		"completion fish":       `  akt completion fish > ~/.config/fish/completions/akt.fish`,
		"completion powershell": `  akt completion powershell > akt.ps1`,
		"completion zsh":        `  akt completion zsh > "${fpath[1]}/_akt"`,
		"help":                  `  akt help query bank balances`,

		"query audit get": `  # Replace the placeholders with full Akash account addresses.
  akt query audit get <provider-address> <auditor-address>`,
		"query audit list":              `  akt query audit list --limit 10`,
		"query auth account":            `  akt query auth account akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl`,
		"query auth accounts":           `  akt query auth accounts --limit 10`,
		"query auth module-accounts":    `  akt query auth module-accounts`,
		"query auth params":             `  akt query auth params`,
		"query authz grants":            `  akt query authz grants <granter-address> <grantee-address>`,
		"query authz grants-by-grantee": `  akt query authz grants-by-grantee <grantee-address>`,
		"query authz grants-by-granter": `  akt query authz grants-by-granter <granter-address>`,
		"query block-results":           `  akt query block-results 1`,
		"query bme ledger": `  akt query bme ledger --limit 10

  # Conversions that have been requested but have not settled yet
  akt query bme ledger --owner <your-address> --status ledger_record_status_pending`,
		"query bme params":          `  akt query bme params`,
		"query bme status":          `  akt query bme status`,
		"query bme vault-state":     `  akt query bme vault-state`,
		"query deployment group":    `  akt query deployment group`,
		"query deployment params":   `  akt query deployment params`,
		"query distribution params": `  akt query distribution params`,
		"query escrow accounts": `  # Arguments narrow the result; both may be omitted.
  akt query escrow accounts open deployment`,
		"query escrow payments": `  # Arguments narrow the result; both may be omitted.
  akt query escrow payments open deployment`,
		"query market bid":             `  akt query market bid`,
		"query market lease":           `  akt query market lease`,
		"query market order":           `  akt query market order`,
		"query market params":          `  akt query market params`,
		"query mint annual-provisions": `  akt query mint annual-provisions`,
		"query mint inflation":         `  akt query mint inflation`,
		"query mint params":            `  akt query mint params`,
		"query module-name-to-address": `  akt query module-name-to-address gov`,
		"query oracle aggregated-price": `  # The oracle keys prices by base denom; akt, AKT and uakt all resolve to "akt".
  akt query oracle aggregated-price akt`,
		"query oracle params":   `  akt query oracle params`,
		"query oracle prices":   `  akt query oracle prices --limit 10`,
		"query params subspace": `  akt query params subspace staking MaxValidators`,
		"query provider get":    `  akt query provider get akash1uwqjtgjhjctjc45ugy7ev5prprhehc7w5xcfwl`,
		"query provider list":   `  akt query provider list --limit 10`,
		"query slashing params": `  akt query slashing params`,
		"query slashing signing-info": `  # Replace the placeholder with a full consensus address.
  akt query slashing signing-info <validator-consensus-address>`,
		"query slashing signing-infos": `  akt query slashing signing-infos --limit 10`,
		"query tx": `  # Replace the placeholder with a 64-character transaction hash.
  akt query tx <tx-hash>`,
		"query wasm build-address": `  # Replace the hashes and address with values for the contract.
  akt query wasm build-address <code-hash> <creator-address> 00 '{}'`,
		"query wasm code":                      `  akt query wasm code 1 contract.wasm`,
		"query wasm code-info":                 `  akt query wasm code-info 1`,
		"query wasm contract":                  `  akt query wasm contract <contract-address>`,
		"query wasm contract-history":          `  akt query wasm contract-history <contract-address>`,
		"query wasm contract-state all":        `  akt query wasm contract-state all <contract-address>`,
		"query wasm contract-state raw":        `  akt query wasm contract-state raw <contract-address> 00`,
		"query wasm contract-state smart":      `  akt query wasm contract-state smart <contract-address> '{"config":{}}'`,
		"query wasm list-code":                 `  akt query wasm list-code --limit 10`,
		"query wasm list-contract-by-code":     `  akt query wasm list-contract-by-code 1 --limit 10`,
		"query wasm list-contracts-by-creator": `  akt query wasm list-contracts-by-creator <creator-address> --limit 10`,
		"query wasm params":                    `  akt query wasm params`,
		"query wasm pinned":                    `  akt query wasm pinned --limit 10`,

		"tx audit attr create": `  # Attribute keys and values follow the provider address in pairs.
  akt tx audit attr create <provider-address> region us-west --from auditor --generate-only`,
		"tx audit attr delete": `  # List the attribute keys to remove after the provider address.
  akt tx audit attr delete <provider-address> region --from auditor --generate-only`,
		"tx authz grant contract": `  akt tx authz grant contract <grantee-address> execution <contract-address> \
    --allow-all-messages --from mykey --generate-only`,
		"tx authz grant store-code": `  # Replace <code-hash> with the contract checksum.
  akt tx authz grant store-code <grantee-address> <code-hash>:everybody \
    --from mykey --generate-only`,
		"tx bank send": `  # The sender may be a key name or address.
  akt tx bank send mykey <recipient-address> 1uakt --generate-only`,
		"tx broadcast": `  akt tx broadcast signed-tx.json`,
		"tx crisis invariant-broken": `  akt tx crisis invariant-broken bank total-supply \
    --from mykey --generate-only`,
		"tx decode":            `  akt tx decode <base64-encoded-transaction>`,
		"tx deployment create": `  akt tx deployment create deploy.yaml --from mykey --generate-only`,
		"tx encode":            `  akt tx encode unsigned-tx.json`,
		"tx evidence":          `  akt tx evidence --help`,
		"tx feegrant grant": `  akt tx feegrant grant <grantee-address> --spend-limit 1000000uakt \
    --from mykey --generate-only`,
		"tx gov draft-proposal": `  akt tx gov draft-proposal`,
		"tx gov submit-proposal add-code-upload-params-addresses": `  akt tx gov submit-proposal add-code-upload-params-addresses <address> \
    --authority <authority-address> --title "Allow uploader" \
    --summary "Allow one address to upload code" --from mykey --generate-only`,
		"tx gov submit-proposal clear-contract-admin": `  akt tx gov submit-proposal clear-contract-admin <contract-address> \
    --authority <authority-address> --title "Clear contract admin" \
    --summary "Make the contract immutable" --from mykey --generate-only`,
		"tx gov submit-proposal execute-contract": `  akt tx gov submit-proposal execute-contract <contract-address> '{"run":{}}' \
    --authority <authority-address> --title "Execute contract" \
    --summary "Execute a governance message" --from mykey --generate-only`,
		"tx gov submit-proposal instantiate-contract": `  akt tx gov submit-proposal instantiate-contract 1 '{}' \
    --label example --authority <authority-address> --title "Instantiate contract" \
    --summary "Create a contract instance" --from mykey --generate-only`,
		"tx gov submit-proposal instantiate-contract-2": `  akt tx gov submit-proposal instantiate-contract-2 1 '{}' 00 \
    --label example --authority <authority-address> --title "Instantiate contract" \
    --summary "Create a predictable contract instance" --from mykey --generate-only`,
		"tx gov submit-proposal migrate-contract": `  akt tx gov submit-proposal migrate-contract <contract-address> 2 '{}' \
    --authority <authority-address> --title "Migrate contract" \
    --summary "Move the contract to code 2" --from mykey --generate-only`,
		"tx gov submit-proposal pin-codes": `  akt tx gov submit-proposal pin-codes 1,2 \
    --authority <authority-address> --title "Pin contract code" \
    --summary "Keep selected code in cache" --from mykey --generate-only`,
		"tx gov submit-proposal remove-code-upload-params-addresses": `  akt tx gov submit-proposal remove-code-upload-params-addresses <address> \
    --authority <authority-address> --title "Remove uploader" \
    --summary "Remove one code uploader" --from mykey --generate-only`,
		"tx gov submit-proposal set-contract-admin": `  akt tx gov submit-proposal set-contract-admin <contract-address> <new-admin-address> \
    --authority <authority-address> --title "Set contract admin" \
    --summary "Transfer contract administration" --from mykey --generate-only`,
		"tx gov submit-proposal store-instantiate": `  akt tx gov submit-proposal store-instantiate contract.wasm '{}' \
    --label example --authority <authority-address> --title "Store and instantiate" \
    --summary "Upload and instantiate contract code" --from mykey --generate-only`,
		"tx gov submit-proposal store-migrate": `  akt tx gov submit-proposal store-migrate contract.wasm <contract-address> '{}' \
    --authority <authority-address> --title "Store and migrate" \
    --summary "Upload code and migrate a contract" --from mykey --generate-only`,
		"tx gov submit-proposal sudo-contract": `  akt tx gov submit-proposal sudo-contract <contract-address> '{"run":{}}' \
    --authority <authority-address> --title "Sudo contract" \
    --summary "Run a privileged contract message" --from mykey --generate-only`,
		"tx gov submit-proposal unpin-codes": `  akt tx gov submit-proposal unpin-codes 1,2 \
    --authority <authority-address> --title "Unpin contract code" \
    --summary "Remove selected code from cache" --from mykey --generate-only`,
		"tx gov submit-proposal wasm-store": `  akt tx gov submit-proposal wasm-store contract.wasm \
    --authority <authority-address> --title "Store contract code" \
    --summary "Upload contract code" --from mykey --generate-only`,
		"tx ibc channelv2": `  akt tx ibc channelv2 --help`,
		"tx ibc client recover-client": `  akt tx ibc client recover-client 07-tendermint-0 07-tendermint-1 \
    --title "Recover IBC client" --summary "Replace a frozen client" \
    --from mykey --generate-only`,
		"tx ibc client schedule-ibc-upgrade": `  akt tx ibc client schedule-ibc-upgrade v2 20000000 upgraded-client-state.json \
    --title "Schedule IBC upgrade" --summary "Upgrade the IBC client" \
    --from mykey --generate-only`,
		"tx market bid close": `  akt tx market bid close --dseq 12345 \
    --from provider --generate-only`,
		"tx market bid create": `  akt tx market bid create --dseq 12345 --price 1uakt \
    --from provider --generate-only`,
		"tx oracle feed": `  akt tx oracle feed uakt usd 0.42 2026-07-31T12:00:00Z \
    --from oracle --generate-only`,
		"tx provider create": `  akt tx provider create provider.yaml --from provider --generate-only`,
		"tx provider update": `  akt tx provider update provider.yaml --from provider --generate-only`,
		"tx sign":            `  akt tx sign unsigned-tx.json --from mykey`,
		"tx sign-batch":      `  akt tx sign-batch unsigned-1.json unsigned-2.json --from mykey`,
		"tx slashing unjail": `  akt tx slashing unjail --from validator --generate-only`,
		"tx staking edit-validator": `  akt tx staking edit-validator --new-moniker "my-validator" \
    --from validator --generate-only`,
		"tx upgrade cancel-software-upgrade": `  akt tx upgrade cancel-software-upgrade --title "Cancel upgrade" \
    --summary "Cancel the scheduled upgrade" --from mykey --generate-only`,
		"tx upgrade software-upgrade": `  akt tx upgrade software-upgrade v2 --upgrade-height 20000000 \
    --title "Upgrade to v2" --summary "Schedule the v2 upgrade" \
    --from mykey --generate-only`,
		"tx validate-signatures": `  akt tx validate-signatures signed-tx.json`,
		"tx vesting create-periodic-vesting-account": `  akt tx vesting create-periodic-vesting-account <recipient-address> periods.json \
    --from mykey --generate-only`,
		"tx vesting create-permanent-locked-account": `  akt tx vesting create-permanent-locked-account <recipient-address> 1000000uakt \
    --from mykey --generate-only`,
		"tx vesting create-vesting-account": `  akt tx vesting create-vesting-account <recipient-address> 1000000uakt 1800000000 \
    --from mykey --generate-only`,
		"tx wasm clear-contract-admin": `  akt tx wasm clear-contract-admin <contract-address> \
    --from mykey --generate-only`,
		"tx wasm execute": `  akt tx wasm execute <contract-address> '{"run":{}}' \
    --from mykey --generate-only`,
		"tx wasm instantiate2": `  akt tx wasm instantiate2 1 '{}' 00 --label example \
    --from mykey --generate-only`,
		"tx wasm migrate": `  akt tx wasm migrate <contract-address> 2 '{}' \
    --from mykey --generate-only`,
		"tx wasm set-contract-admin": `  akt tx wasm set-contract-admin <contract-address> <new-admin-address> \
    --from mykey --generate-only`,
		"tx wasm set-contract-label": `  akt tx wasm set-contract-label <contract-address> example \
    --from mykey --generate-only`,
		"tx wasm store": `  akt tx wasm store contract.wasm --from mykey --generate-only`,
		"tx wasm update-instantiate-config": `  akt tx wasm update-instantiate-config 1 --instantiate-everybody true \
    --from mykey --generate-only`,

		// Narrow corrections for examples owned by upstream command packages.
		"query ibc channelv2 unreceived-packets": `  akt query ibc channelv2 unreceived-packets <client-id> --sequences=1,2,3`,
		"query ibc client config":                `  akt query ibc client config 08-wasm-0`,
		"query ibc client consensus-state": `  # Supply a revision height, or ask for the latest state.
  akt query ibc client consensus-state 07-tendermint-0 1-100
  akt query ibc client consensus-state 07-tendermint-0 --latest-height`,
		"query ibc client header":   `  akt query ibc client header`,
		"query ibc connection path": `  akt query ibc connection path 07-tendermint-0`,
		"tx ibc client update-client-config": `  akt tx ibc client update-client-config 08-wasm-0 <relayer-address> \
    --from mykey --generate-only`,
	}

	walkCommandTree(root, func(cmd *cobra.Command) {
		path := strings.TrimPrefix(cmd.CommandPath(), root.Name()+" ")
		if example, ok := examples[path]; ok {
			cmd.Example = example
		}

		if path == "query ibc client consensus-state" {
			cmd.Long = strings.ReplaceAll(cmd.Long, "'--latest' flag", "'--latest-height' flag")
		}
	})
}

func walkCommandTree(cmd *cobra.Command, visit func(*cobra.Command)) {
	visit(cmd)
	for _, child := range cmd.Commands() {
		if child.Hidden {
			continue
		}
		walkCommandTree(child, visit)
	}
}
