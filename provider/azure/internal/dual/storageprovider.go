package dual

import (
	"sync"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
)

// StorageProvider is a storage.Provider that switches between one of two
// storage.Providers depending on the configuration. Once a provider has
// delegated to one provider based on configuration, all future operations
// will be delegated to that same provider.
//
// One provider is considered the primary, and one the secondary. In case no
// provider has yet been selected based on configuration, the primary provider
// will take precedence for methods that do not receive configuration.
type StorageProvider struct {
	primary, secondary storage.Provider
	isPrimary          func(*config.Config) bool

	mu     sync.Mutex
	active storage.Provider
}

// NewStorageProvider returns a new StorageProvider with the given primary and
// secondary StorageProviders, and a function that determines whether or not
// the given configuration is relevant to the primary provider.
func NewStorageProvider(
	primary, secondary storage.Provider,
	isPrimary func(*config.Config) bool,
) *StorageProvider {
	return &StorageProvider{
		primary:   primary,
		secondary: secondary,
		isPrimary: isPrimary,
	}
}

// VolumeSource is part of the storage.Provider interface.
func (p *StorageProvider) VolumeSource(
	environConfig *config.Config, providerConfig *storage.Config,
) (storage.VolumeSource, error) {
	return p.ensureActive(environConfig).VolumeSource(environConfig, providerConfig)
}

// FilesystemSource is part of the storage.Provider interface.
func (p *StorageProvider) FilesystemSource(
	environConfig *config.Config, providerConfig *storage.Config,
) (storage.FilesystemSource, error) {
	return p.ensureActive(environConfig).FilesystemSource(environConfig, providerConfig)
}

// Supports is part of the storage.Provider interface.
func (p *StorageProvider) Supports(kind storage.StorageKind) bool {
	return p.Active().Supports(kind)
}

// Scope is part of the storage.Provider interface.
func (p *StorageProvider) Scope() storage.Scope {
	return p.Active().Scope()
}

// Dynamic is part of the storage.Provider interface.
func (p *StorageProvider) Dynamic() bool {
	return p.Active().Dynamic()
}

// ValidateConfig is part of the storage.Provider interface.
func (p *StorageProvider) ValidateConfig(cfg *storage.Config) error {
	return p.Active().ValidateConfig(cfg)
}

// Active returns the currently active storage.Provider.
func (p *StorageProvider) Active() storage.Provider {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active != nil {
		return p.active
	}
	return p.primary
}

// ensureActive returns the currently active storage.Provider, setting it if
// it is not yet set based on the given configuration.
func (p *StorageProvider) ensureActive(cfg *config.Config) storage.Provider {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active == nil {
		if p.isPrimary(cfg) {
			p.active = p.primary
		} else {
			p.active = p.secondary
		}
	}
	return p.active
}
