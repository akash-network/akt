package console

import "encoding/json"

// FlexString unmarshals from either a JSON string or a JSON number, since the
// Console API is inconsistent about numeric identifiers (dseq, amounts).
// It always marshals as a JSON string.
type FlexString string

// String returns the underlying string value.
func (s FlexString) String() string { return string(s) }

// UnmarshalJSON implements json.Unmarshaler.
func (s *FlexString) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = FlexString(v)
		return nil
	}

	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*s = FlexString(n.String())

	return nil
}

// --- Account / wallet -------------------------------------------------------

// User describes the authenticated Console user (GET /v1/user/me).
// ID is the internal UUID required by ListWallets.
type User struct {
	ID            string `json:"id"`
	UserID        string `json:"userId"`
	Username      string `json:"username"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"emailVerified"`
}

// Balances reports managed-wallet balances in µACT integer units
// (GET /v1/balances). ACT is pegged 1:1 to USD, so USD = value / 1e6.
type Balances struct {
	Balance     int64 `json:"balance"`
	Deployments int64 `json:"deployments"`
	Total       int64 `json:"total"`
}

// BalanceUSD returns the free balance in USD.
func (b Balances) BalanceUSD() float64 { return float64(b.Balance) / 1e6 }

// DeploymentsUSD returns the funds locked in deployment escrow in USD.
func (b Balances) DeploymentsUSD() float64 { return float64(b.Deployments) / 1e6 }

// TotalUSD returns the total balance in USD.
func (b Balances) TotalUSD() float64 { return float64(b.Total) / 1e6 }

// Wallet describes a managed wallet (GET /v1/wallets?userId=).
type Wallet struct {
	Address      string  `json:"address"`
	CreditAmount float64 `json:"creditAmount"`
	IsTrialing   bool    `json:"isTrialing"`
	Denom        string  `json:"denom"`
}

// WalletSettings holds account-level wallet settings
// (GET/PUT /v1/wallet-settings).
type WalletSettings struct {
	AutoReloadEnabled bool `json:"autoReloadEnabled"`
}

// UsagePoint is one day of spend history (GET /v1/usage/history).
type UsagePoint struct {
	Date              string  `json:"date"`
	ActiveDeployments int     `json:"activeDeployments"`
	DailyUsdcSpent    float64 `json:"dailyUsdcSpent"`
	TotalUsdcSpent    float64 `json:"totalUsdcSpent"`
}

// --- Deployments ------------------------------------------------------------

// DeploymentID identifies a deployment on chain.
type DeploymentID struct {
	Owner string     `json:"owner"`
	DSeq  FlexString `json:"dseq"`
}

// Deployment is the chain-side deployment record embedded in Console
// responses.
type Deployment struct {
	ID        DeploymentID    `json:"id"`
	State     string          `json:"state"`
	CreatedAt json.RawMessage `json:"created_at,omitempty"`
}

// LeaseID identifies a lease (and doubles as a bid ID).
type LeaseID struct {
	Owner    string     `json:"owner"`
	DSeq     FlexString `json:"dseq"`
	GSeq     uint32     `json:"gseq"`
	OSeq     uint32     `json:"oseq"`
	Provider string     `json:"provider"`
}

// Price is a denominated amount as reported by the chain.
type Price struct {
	Denom  string     `json:"denom"`
	Amount FlexString `json:"amount"`
}

// LeaseStatus carries provider-reported runtime status. The deep structures
// are provider-defined, so they are kept raw.
type LeaseStatus struct {
	Services       json.RawMessage `json:"services,omitempty"`
	ForwardedPorts json.RawMessage `json:"forwarded_ports,omitempty"`
	IPs            json.RawMessage `json:"ips,omitempty"`
}

// Lease describes a lease attached to a deployment.
type Lease struct {
	ID     LeaseID      `json:"id"`
	State  string       `json:"state"`
	Price  *Price       `json:"price,omitempty"`
	Status *LeaseStatus `json:"status,omitempty"`
}

// DeploymentListItem is one entry of GET /v1/deployments.
type DeploymentListItem struct {
	Deployment Deployment `json:"deployment"`
	Leases     []Lease    `json:"leases"`
}

// Pagination describes list paging metadata.
type Pagination struct {
	Total   int  `json:"total"`
	Skip    int  `json:"skip"`
	Limit   int  `json:"limit"`
	HasMore bool `json:"hasMore"`
}

// DeploymentList is the payload of GET /v1/deployments.
type DeploymentList struct {
	Deployments []DeploymentListItem `json:"deployments"`
	Pagination  Pagination           `json:"pagination"`
}

// DeploymentDetail is the payload of GET /v1/deployments/{dseq} (and of the
// update/lease-creation responses). EscrowAccount is kept raw; its state
// carries funds/transferred details.
type DeploymentDetail struct {
	Deployment    Deployment      `json:"deployment"`
	Leases        []Lease         `json:"leases"`
	EscrowAccount json.RawMessage `json:"escrow_account,omitempty"`
}

// SignTx reports the broadcast result of a managed-wallet transaction.
type SignTx struct {
	Code            int    `json:"code"`
	TransactionHash string `json:"transactionHash"`
	RawLog          string `json:"rawLog"`
}

// CreateDeploymentResult is the payload of POST /v1/deployments.
type CreateDeploymentResult struct {
	DSeq     FlexString `json:"dseq"`
	Manifest string     `json:"manifest"`
	SignTx   *SignTx    `json:"signTx,omitempty"`
}

// Bid is a provider bid on an open order (GET /v1/bids?dseq=).
type Bid struct {
	ID    LeaseID `json:"id"`
	State string  `json:"state"`
	Price *Price  `json:"price,omitempty"`
}

// LeaseRequest identifies a specific bid to accept (POST /v1/leases).
type LeaseRequest struct {
	DSeq     string `json:"dseq"`
	GSeq     uint32 `json:"gseq"`
	OSeq     uint32 `json:"oseq"`
	Provider string `json:"provider"`
}

// DeploymentSettings holds per-deployment automation settings
// (GET/PATCH /v2/deployment-settings/{dseq}).
type DeploymentSettings struct {
	DSeq                 FlexString `json:"dseq"`
	AutoTopUpEnabled     bool       `json:"autoTopUpEnabled"`
	EstimatedTopUpAmount float64    `json:"estimatedTopUpAmount"`
	TopUpFrequencyMs     int64      `json:"topUpFrequencyMs"`
}

// --- API keys / auth --------------------------------------------------------

// APIKey describes an existing API key (GET /v1/api-keys). The secret is
// never returned after creation.
type APIKey struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ExpiresAt  string `json:"expiresAt,omitempty"`
	CreatedAt  string `json:"createdAt,omitempty"`
	LastUsedAt string `json:"lastUsedAt,omitempty"`
	KeyFormat  string `json:"keyFormat,omitempty"`
}

// CreatedAPIKey is the payload of POST /v1/api-keys. APIKey holds the secret,
// which is shown exactly once at creation time.
type CreatedAPIKey struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	APIKey string `json:"apiKey"`
}

// --- Marketplace / catalog (public) -----------------------------------------

// Provider is a provider summary (GET /v1/providers). GPUModels and IPRegion
// are kept raw: their shapes vary between deployments of the API.
type Provider struct {
	Owner     string          `json:"owner"`
	Name      string          `json:"name,omitempty"`
	HostURI   string          `json:"hostUri,omitempty"`
	IsOnline  bool            `json:"isOnline,omitempty"`
	IsAudited bool            `json:"isAudited,omitempty"`
	Uptime1D  float64         `json:"uptime1d,omitempty"`
	Uptime7D  float64         `json:"uptime7d,omitempty"`
	Uptime30D float64         `json:"uptime30d,omitempty"`
	GPUModels json.RawMessage `json:"gpuModels,omitempty"`
	IPRegion  json.RawMessage `json:"ipRegion,omitempty"`
}

// ProviderDetail is the payload of GET /v1/providers/{address}: the typed
// summary fields plus the full raw document (stats etc.) in Raw.
type ProviderDetail struct {
	Provider

	// Raw is the complete response body for callers that need fields not
	// modeled on Provider.
	Raw json.RawMessage `json:"-"`
}

// ProviderRegion is one entry of GET /v1/provider-regions.
type ProviderRegion struct {
	Key         string   `json:"key"`
	Description string   `json:"description"`
	Providers   []string `json:"providers"`
}

// Auditor is one entry of GET /v1/auditors.
type Auditor struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Website string `json:"website,omitempty"`
}

// GPUPriceRange is the observed price spread for a GPU model, USD per month.
type GPUPriceRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
	Avg float64 `json:"avg"`
}

// GPUModel is one entry of the GPU price catalog.
type GPUModel struct {
	Vendor string        `json:"vendor"`
	Model  string        `json:"model"`
	RAM    string        `json:"ram"`
	Price  GPUPriceRange `json:"price"`
}

// GPUAvailability summarizes network-wide GPU counts.
type GPUAvailability struct {
	Total     int `json:"total"`
	Available int `json:"available"`
}

// GPUPrices is the payload of GET /v1/gpu-prices (not data-enveloped).
type GPUPrices struct {
	Availability GPUAvailability `json:"availability"`
	Models       []GPUModel      `json:"models"`
}

// Template is the payload of GET /v1/templates/{id}. Extra fields beyond the
// common ones are intentionally not modeled.
type Template struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
	Deploy  string `json:"deploy,omitempty"`
	Readme  string `json:"readme,omitempty"`
}

// Attribute is a provider attribute requirement key/value pair.
type Attribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// SignedBy expresses auditor requirements for bid screening. Empty lists are
// omitted on the wire: the API schema types anyOf/allOf as (non-nullable)
// string arrays, so serializing nil slices as JSON null violates the contract.
type SignedBy struct {
	AnyOf []string `json:"anyOf,omitempty"`
	AllOf []string `json:"allOf,omitempty"`
}

// BidScreeningRequirements narrows the provider set for bid screening. An
// empty Attributes list is omitted on the wire (the API schema types it as a
// non-nullable array, so JSON null is rejected).
type BidScreeningRequirements struct {
	SignedBy   SignedBy    `json:"signedBy"`
	Attributes []Attribute `json:"attributes,omitempty"`
}

// BidScreeningRequest is the body of POST /v1/bid-screening (not
// data-enveloped). Resources and ReclamationWindow are passed through raw so
// callers can supply whatever shape the SDL produced.
//
// The API requires Resources and Timezone; ScreenBids rejects requests
// missing either before hitting the network. Timezone must be an IANA zone
// name (e.g. "America/Chicago").
type BidScreeningRequest struct {
	Requirements      BidScreeningRequirements `json:"requirements"`
	Resources         json.RawMessage          `json:"resources,omitempty"`
	Timezone          string                   `json:"timezone,omitempty"`
	ReclamationWindow json.RawMessage          `json:"reclamationWindow,omitempty"`
}

// ScreenedProvider is one provider returned by bid screening. Location is
// kept raw (object or string depending on API version).
type ScreenedProvider struct {
	Owner        string          `json:"owner"`
	HostURI      string          `json:"hostUri,omitempty"`
	IsAudited    bool            `json:"isAudited,omitempty"`
	Location     json.RawMessage `json:"location,omitempty"`
	Organization string          `json:"organization,omitempty"`
}
