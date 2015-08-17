// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"

	"github.com/juju/juju/storage"
)

var errNoMountPoint = errors.New("filesystem mount point not specified")

// CommonProviders returns the storage providers used by all environments.
func CommonProviders() map[storage.ProviderType]storage.Provider {
	return map[storage.ProviderType]storage.Provider{
		LoopProviderType:   &loopProvider{logAndExec},
		RootfsProviderType: &rootfsProvider{logAndExec},
		TmpfsProviderType:  &tmpfsProvider{logAndExec},
	}
}

// ValidateConfig performs storage provider config validation, including
// any common validation.
func ValidateConfig(p storage.Provider, cfg *storage.Config) error {
	if p.Scope() == storage.ScopeMachine && cfg.IsPersistent() {
		return errors.Errorf("machine scoped storage provider %q does not support persistent storage", cfg.Name())
	}
	return p.ValidateConfig(cfg)
}
