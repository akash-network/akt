package context

// NetworkTemplates returns pre-configured network definitions for known Akash networks.
// Returns a fresh map on each call to prevent mutation of shared state.
func NetworkTemplates() map[string]Network {
	return map[string]Network{
		"mainnet": {
			Name:    "mainnet",
			ChainID: "akashnet-2",
			Endpoints: Endpoints{
				RPC: []string{
					"https://rpc.akt.dev:443/rpc",
					"https://rpc.akashnet.net:443",
					"https://rpc-akash.ecostake.com:443",
				},
				API: []string{
					"https://api.akashnet.net:443",
					"https://akash-api.polkachu.com:443",
				},
				GRPC: []string{
					"grpc.akashnet.net:443",
				},
			},
			GasPrices:     "0.025uakt",
			GasAdjustment: "1.5",
		},
		"testnet": {
			Name:    "testnet",
			ChainID: "testnet-02",
			Endpoints: Endpoints{
				RPC: []string{
					"https://rpc.testnet-02.aksh.pw:443",
				},
				API: []string{
					"https://api.testnet-02.aksh.pw:443",
				},
				GRPC: []string{
					"grpc.testnet-02.aksh.pw:443",
				},
			},
			GasPrices:     "0.025uakt",
			GasAdjustment: "1.5",
		},
		"sandbox": {
			Name:    "sandbox",
			ChainID: "sandbox-01",
			Endpoints: Endpoints{
				RPC: []string{
					"https://rpc.sandbox-01.aksh.pw:443",
				},
				API: []string{
					"https://api.sandbox-01.aksh.pw:443",
				},
				GRPC: []string{
					"grpc.sandbox-01.aksh.pw:443",
				},
			},
			GasPrices:     "0.025uakt",
			GasAdjustment: "1.5",
		},
	}
}

// DefaultKeyring returns the default keyring configuration.
func DefaultKeyring() Keyring {
	return Keyring{
		Name:    "default",
		Backend: "os",
	}
}

// DefaultConfig returns a new empty config with version set.
func DefaultConfig() Config {
	return Config{
		Version:  ConfigVersion,
		Networks: []Network{},
		Keyrings: []Keyring{DefaultKeyring()},
		Contexts: []Context{},
		Defaults: Defaults{
			Output:        "pretty",
			BroadcastMode: "sync",
		},
		TUI: TUISettings{
			Theme:       "dark",
			Keybindings: "vim",
		},
	}
}
