package cache

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"

	"pkg.akt.dev/akt/internal/monitor/rpc"
)

const (
	// Refresh intervals based on provider state
	OnlineCheckInterval     = 1 * time.Minute
	RecentOfflineInterval   = 5 * time.Minute
	LongTermOfflineInterval = 6 * time.Hour

	// Threshold for long-term offline
	LongTermOfflineThreshold = 5
)

// Bucket names
var (
	bucketProviders = []byte("providers")
	bucketMeta      = []byte("meta")
	keyLastSync     = []byte("last_chain_sync")
)

// CachedProvider represents a provider with cached status information.
type CachedProvider struct {
	HostURI             string            `json:"host_uri"`
	Name                string            `json:"name"`
	Country             string            `json:"country"`
	Attributes          map[string]string `json:"attributes"`
	IsOnline            bool              `json:"is_online"`
	Version             string            `json:"version"`
	CPUAvailable        uint64            `json:"cpu_available"`
	CPUTotal            uint64            `json:"cpu_total"`
	MemAvailable        uint64            `json:"mem_available"`
	MemTotal            uint64            `json:"mem_total"`
	GPUAvailable        uint64            `json:"gpu_available"`
	GPUTotal            uint64            `json:"gpu_total"`
	GPUModels           []string          `json:"gpu_models,omitempty"`
	LastSeenOnline      time.Time         `json:"last_seen_online"`
	LastChecked         time.Time         `json:"last_checked"`
	ConsecutiveFailures int               `json:"consecutive_failures"`
}

// ProviderStore defines the interface for provider caching operations.
type ProviderStore interface {
	HasProviders() bool
	GetProvider(owner string) (*CachedProvider, bool)
	GetAllProviders() map[string]*CachedProvider
	GetOnlineProviders() []*CachedProvider
	MarkProviderOnline(owner, version string, cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal uint64, gpuModels []string)
	MarkProviderOffline(owner string)
	SyncWithChain(onChainProviders []rpc.OnChainProvider) []string
	GetProvidersDueForCheck() []string
	GetProvidersByPriority() []string
	ProviderCount() int
	OnlineCount() int
	Save() error
}

// Ensure ProviderCache implements ProviderStore.
var _ ProviderStore = (*ProviderCache)(nil)

// ProviderCache manages provider data inside a bbolt database.
type ProviderCache struct {
	db *bolt.DB
}

// Open opens or creates the provider cache in the given bbolt database.
// It ensures the required buckets exist.
func Open(db *bolt.DB) (*ProviderCache, error) {
	err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketProviders); err != nil {
			return fmt.Errorf("create providers bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists(bucketMeta); err != nil {
			return fmt.Errorf("create meta bucket: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &ProviderCache{db: db}, nil
}

// HasProviders returns true if the cache has any providers.
func (c *ProviderCache) HasProviders() bool {
	var has bool
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketProviders)
		if b == nil {
			return nil
		}
		has = b.Stats().KeyN > 0
		return nil
	})
	return has
}

// GetProvider returns a provider by owner address.
func (c *ProviderCache) GetProvider(owner string) (*CachedProvider, bool) {
	var p CachedProvider
	var found bool
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketProviders)
		if b == nil {
			return nil
		}
		data := b.Get([]byte(owner))
		if data == nil {
			return nil
		}
		if err := json.Unmarshal(data, &p); err != nil {
			return nil //nolint:nilerr // returning the error would abort the whole bbolt scan over one unreadable row; skipping the row is the intended behaviour
		}
		found = true
		return nil
	})
	if !found {
		return nil, false
	}
	return &p, true
}

// GetAllProviders returns all cached providers.
func (c *ProviderCache) GetAllProviders() map[string]*CachedProvider {
	result := make(map[string]*CachedProvider)
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketProviders)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			var p CachedProvider
			if err := json.Unmarshal(v, &p); err != nil {
				return nil //nolint:nilerr // returning the error would abort the whole bbolt scan over one unreadable row; skipping the row is the intended behaviour
			}
			result[string(k)] = &p
			return nil
		})
	})
	return result
}

// GetOnlineProviders returns all providers that are currently online.
func (c *ProviderCache) GetOnlineProviders() []*CachedProvider {
	var online []*CachedProvider
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketProviders)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			var p CachedProvider
			if err := json.Unmarshal(v, &p); err != nil {
				return nil //nolint:nilerr // returning the error would abort the whole bbolt scan over one unreadable row; skipping the row is the intended behaviour
			}
			if p.IsOnline {
				cp := p
				online = append(online, &cp)
			}
			return nil
		})
	})
	return online
}

// MarkProviderOnline marks a provider as online with updated stats.
func (c *ProviderCache) MarkProviderOnline(owner, version string, cpuAvail, cpuTotal, memAvail, memTotal, gpuAvail, gpuTotal uint64, gpuModels []string) {
	_ = c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketProviders)
		if b == nil {
			return nil
		}
		data := b.Get([]byte(owner))
		if data == nil {
			return nil
		}
		var p CachedProvider
		if err := json.Unmarshal(data, &p); err != nil {
			return nil //nolint:nilerr // returning the error would abort the whole bbolt scan over one unreadable row; skipping the row is the intended behaviour
		}
		p.IsOnline = true
		p.Version = version
		p.CPUAvailable = cpuAvail
		p.CPUTotal = cpuTotal
		p.MemAvailable = memAvail
		p.MemTotal = memTotal
		p.GPUAvailable = gpuAvail
		p.GPUTotal = gpuTotal
		p.GPUModels = gpuModels
		p.LastSeenOnline = time.Now()
		p.LastChecked = time.Now()
		p.ConsecutiveFailures = 0
		return putProvider(b, owner, &p)
	})
}

// MarkProviderOffline marks a provider as offline.
func (c *ProviderCache) MarkProviderOffline(owner string) {
	_ = c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketProviders)
		if b == nil {
			return nil
		}
		data := b.Get([]byte(owner))
		if data == nil {
			return nil
		}
		var p CachedProvider
		if err := json.Unmarshal(data, &p); err != nil {
			return nil //nolint:nilerr // returning the error would abort the whole bbolt scan over one unreadable row; skipping the row is the intended behaviour
		}
		p.IsOnline = false
		p.LastChecked = time.Now()
		p.ConsecutiveFailures++
		return putProvider(b, owner, &p)
	})
}

// SyncWithChain syncs the cache with on-chain providers.
// Returns a list of new provider owners that weren't in the cache.
func (c *ProviderCache) SyncWithChain(onChainProviders []rpc.OnChainProvider) []string {
	var newProviders []string
	_ = c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketProviders)
		if b == nil {
			return nil
		}
		meta := tx.Bucket(bucketMeta)

		for _, ocp := range onChainProviders {
			existing := b.Get([]byte(ocp.Owner))
			if existing == nil {
				// New provider
				name := ocp.Attributes["organization"]
				if name == "" {
					name = extractHostname(ocp.HostURI)
				}
				p := &CachedProvider{
					HostURI:    ocp.HostURI,
					Name:       name,
					Country:    ocp.Attributes["country"],
					Attributes: ocp.Attributes,
					IsOnline:   ocp.IsOnline,
				}
				if ocp.IsOnline {
					p.LastSeenOnline = time.Now()
				}
				_ = putProvider(b, ocp.Owner, p)
				newProviders = append(newProviders, ocp.Owner)
			} else {
				// Update hostURI and attributes in case they changed
				var p CachedProvider
				if err := json.Unmarshal(existing, &p); err != nil {
					continue
				}
				p.HostURI = ocp.HostURI
				p.Attributes = ocp.Attributes
				if name := ocp.Attributes["organization"]; name != "" {
					p.Name = name
				}
				if country := ocp.Attributes["country"]; country != "" {
					p.Country = country
				}
				_ = putProvider(b, ocp.Owner, &p)
			}
		}

		// Record sync time
		if meta != nil {
			ts, _ := time.Now().MarshalBinary()
			_ = meta.Put(keyLastSync, ts)
		}
		return nil
	})
	return newProviders
}

// GetProvidersDueForCheck returns providers that need to be checked based on smart scheduling.
func (c *ProviderCache) GetProvidersDueForCheck() []string {
	now := time.Now()
	var due []string
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketProviders)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			var p CachedProvider
			if err := json.Unmarshal(v, &p); err != nil {
				return nil //nolint:nilerr // returning the error would abort the whole bbolt scan over one unreadable row; skipping the row is the intended behaviour
			}
			var interval time.Duration
			if p.IsOnline {
				interval = OnlineCheckInterval
			} else if p.ConsecutiveFailures >= LongTermOfflineThreshold {
				interval = LongTermOfflineInterval
			} else {
				interval = RecentOfflineInterval
			}
			if now.Sub(p.LastChecked) >= interval {
				due = append(due, string(k))
			}
			return nil
		})
	})
	return due
}

// GetProvidersByPriority returns providers sorted by check priority:
// unchecked (0) > online (1) > recently offline (2) > long-term offline (3)
func (c *ProviderCache) GetProvidersByPriority() []string {
	type providerPriority struct {
		owner     string
		priority  int
		lastCheck time.Time
	}

	var providers []providerPriority
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketProviders)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			var p CachedProvider
			if err := json.Unmarshal(v, &p); err != nil {
				return nil //nolint:nilerr // returning the error would abort the whole bbolt scan over one unreadable row; skipping the row is the intended behaviour
			}
			priority := calculatePriority(&p)
			providers = append(providers, providerPriority{string(k), priority, p.LastChecked})
			return nil
		})
	})

	sort.Slice(providers, func(i, j int) bool {
		if providers[i].priority != providers[j].priority {
			return providers[i].priority < providers[j].priority
		}
		return providers[i].lastCheck.Before(providers[j].lastCheck)
	})

	result := make([]string, len(providers))
	for i, p := range providers {
		result[i] = p.owner
	}
	return result
}

// ProviderCount returns the total number of providers in cache.
func (c *ProviderCache) ProviderCount() int {
	var count int
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketProviders)
		if b == nil {
			return nil
		}
		count = b.Stats().KeyN
		return nil
	})
	return count
}

// OnlineCount returns the number of online providers.
func (c *ProviderCache) OnlineCount() int {
	var count int
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketProviders)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			var p CachedProvider
			if err := json.Unmarshal(v, &p); err != nil {
				return nil //nolint:nilerr // returning the error would abort the whole bbolt scan over one unreadable row; skipping the row is the intended behaviour
			}
			if p.IsOnline {
				count++
			}
			return nil
		})
	})
	return count
}

// Save is a no-op for bbolt — data is already persisted on each write.
func (c *ProviderCache) Save() error {
	return nil
}

func putProvider(b *bolt.Bucket, owner string, p *CachedProvider) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return b.Put([]byte(owner), data)
}

func calculatePriority(p *CachedProvider) int {
	if p.LastChecked.IsZero() {
		return 0
	}
	if p.IsOnline {
		return 1
	}
	if p.ConsecutiveFailures < LongTermOfflineThreshold {
		return 2
	}
	return 3
}

func extractHostname(hostURI string) string {
	host := strings.TrimPrefix(hostURI, "https://")
	host = strings.TrimPrefix(host, "http://")
	if idx := strings.IndexAny(host, ":/"); idx != -1 {
		host = host[:idx]
	}
	return host
}
