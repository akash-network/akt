package config

type Config struct {
	Accounts []Account
	Profiles []Profile
	Networks []Network
}

type Account struct {
	Name    string
	Address string
	Type    string
}

type Profile struct {
	Name    string
	Account string
	Network string
}

type Network struct {
	Name      string
	ChainID   string
	Endpoints []Endpoint

	Fees      string
	GasPrices string
}

type Endpoint struct {
	Name string
	Type string
	URI  string
}

type LoadOptions struct {
	Path   string
	Global bool
}
