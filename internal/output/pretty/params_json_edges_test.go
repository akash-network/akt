package pretty

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestModuleParameterJSONRenderersPreserveSemanticValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		render func(json.RawMessage) (string, error)
		raw    string
		want   []string
	}{
		{
			name: "staking", render: renderStakingParamsJSON,
			raw:  `{"params":{"unbonding_time":"1814400s","max_validators":100,"max_entries":7,"historical_entries":10000,"bond_denom":"uakt","min_commission_rate":"0.05"}}`,
			want: []string{"Staking Parameters", "21 days", "100", "10,000", "uakt", "5%"},
		},
		{
			name: "governance", render: renderGovParamsJSON,
			raw:  `{"params":{"voting_period":"172800s","min_deposit":[{"denom":"uakt","amount":"5000000"}],"max_deposit_period":"1209600s","quorum":"0.334","threshold":"0.5","veto_threshold":"0.334","expedited_voting_period":"86400s","expedited_threshold":"0.667","expedited_min_deposit":[{"denom":"uakt","amount":"10000000"}],"burn_vote_quorum":true,"burn_proposal_deposit_prevote":false,"burn_vote_veto":true}}`,
			want: []string{"Governance Parameters", "2 days", "5 AKT", "14 days", "33.4%", "50%", "Expedited", "1 day", "10 AKT", "Burn", "Yes", "No"},
		},
		{
			name: "mint", render: renderMintParamsJSON,
			raw:  `{"params":{"mint_denom":"uakt","inflation_rate_change":"0.13","inflation_max":"0.2","inflation_min":"0.07","goal_bonded":"0.67","blocks_per_year":"6311520"}}`,
			want: []string{"Minting Parameters", "uakt", "13%", "20%", "7%", "67%", "6,311,520"},
		},
		{
			name: "slashing", render: renderSlashingParamsJSON,
			raw:  `{"params":{"signed_blocks_window":"100","min_signed_per_window":"0.5","downtime_jail_duration":"600s","slash_fraction_double_sign":"0.05","slash_fraction_downtime":"0.0001"}}`,
			want: []string{"Slashing Parameters", "100", "50%", "10m", "5%", "0.01%"},
		},
		{
			name: "distribution", render: renderDistributionParamsJSON,
			raw:  `{"params":{"community_tax":"0.02","withdraw_addr_enabled":true}}`,
			want: []string{"Distribution Parameters", "2%", "Yes"},
		},
		{
			name: "auth", render: renderAuthParamsJSON,
			raw:  `{"params":{"max_memo_characters":"256","tx_sig_limit":"7","tx_size_cost_per_byte":"10","sig_verify_cost_ed25519":"590","sig_verify_cost_secp256k1":"1000"}}`,
			want: []string{"Auth Parameters", "256", "7", "10", "590", "1000"},
		},
		{
			name: "bank", render: renderBankParamsJSON,
			raw:  `{"params":{"send_enabled":[],"default_send_enabled":false}}`,
			want: []string{"Bank Parameters", "No"},
		},
		{
			name: "deployment", render: renderDeploymentParamsJSON,
			raw:  `{"MinDeposits":"[{\"denom\":\"uakt\",\"amount\":\"5000000\"}]"}`,
			want: []string{"Deployment Parameters", "5 AKT"},
		},
		{
			name: "market", render: renderMarketParamsJSON,
			raw:  `{"OrderMaxBids":"20","BidMinDeposits":"[{\"denom\":\"uakt\",\"amount\":\"3000\"}]"}`,
			want: []string{"Market Parameters", "20", "3 mAKT"},
		},
		{
			name: "transfer", render: renderTransferParamsJSON,
			raw:  `{"SendEnabled":"true","ReceiveEnabled":"false"}`,
			want: []string{"Transfer Parameters", "Yes", "No"},
		},
		{
			name: "ibc", render: renderIBCParamsJSON,
			raw:  `{"AllowedClients":"[\"07-tendermint\",\"09-localhost\"]"}`,
			want: []string{"IBC Parameters", "07-tendermint, 09-localhost"},
		},
		{
			name: "crisis", render: renderCrisisParamsJSON,
			raw:  `{"ConstantFee":"{\"denom\":\"uakt\",\"amount\":\"500\"}"}`,
			want: []string{"Crisis Parameters", "500 uAKT"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			output, err := tc.render(json.RawMessage(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			plain := ansi.Strip(output)
			for _, expected := range tc.want {
				if !strings.Contains(plain, expected) {
					t.Errorf("rendered parameters do not contain %q:\n%s", expected, plain)
				}
			}
		})
	}
}

func TestModuleParameterJSONRenderersRejectMalformedDocuments(t *testing.T) {
	t.Parallel()

	renderers := []func(json.RawMessage) (string, error){
		renderStakingParamsJSON,
		renderGovParamsJSON,
		renderMintParamsJSON,
		renderSlashingParamsJSON,
		renderDistributionParamsJSON,
		renderAuthParamsJSON,
		renderBankParamsJSON,
		renderDeploymentParamsJSON,
		renderMarketParamsJSON,
		renderTransferParamsJSON,
		renderIBCParamsJSON,
		renderCrisisParamsJSON,
	}
	for index, render := range renderers {
		if output, err := render(json.RawMessage(`{"params":`)); err == nil || output != "" {
			t.Errorf("renderer %d malformed result = %q, %v; want empty output and error", index, output, err)
		}
	}
}

func TestGenericParameterFormattingDegradesWithoutInventingValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "empty coin list", got: formatCoinsJSON(nil), want: "0"},
		{name: "null coin list", got: formatCoinsJSON(json.RawMessage("null")), want: "0"},
		{name: "invalid coin list", got: formatCoinsJSON(json.RawMessage(`{"amount":`)), want: `{"amount":`},
		{name: "invalid generic coins", got: formatGenericCoins("not-coins"), want: "not-coins"},
		{name: "invalid generic coin", got: formatGenericCoin("not-a-coin"), want: "not-a-coin"},
		{name: "unknown generic bool", got: formatGenericBool(`"sometimes"`), want: "sometimes"},
		{name: "empty string list", got: formatGenericStringList(`"[]"`), want: "(none)"},
		{name: "invalid string list", got: formatGenericStringList("not-a-list"), want: "not-a-list"},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}
