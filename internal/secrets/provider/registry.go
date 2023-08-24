// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.secrets.provider")

type secretBackendRegistry struct {
	backends map[string]SecretBackendProvider
}

var globalBackendRegistry = &secretBackendRegistry{
	backends: map[string]SecretBackendProvider{},
}

// Register registers the named secret backend provider.
func (r *secretBackendRegistry) Register(p SecretBackendProvider) error {
	backendType := p.Type()
	if r.backends[backendType] != nil {
		return errors.Errorf("duplicate backend name %q", backendType)
	}
	logger.Tracef("registering secret provider %q", backendType)
	r.backends[backendType] = p
	return nil
}

// Provider returns the named secret backend provider.
func (r *secretBackendRegistry) Provider(backendType string) (SecretBackendProvider, error) {
	p, ok := r.backends[backendType]
	if !ok {
		return nil, errors.NewNotFound(
			nil, fmt.Sprintf("no registered provider for %q", backendType),
		)
	}
	return p, nil
}

// Register registers the named secret backend provider.
func Register(p SecretBackendProvider) {
	if err := globalBackendRegistry.Register(p); err != nil {
		panic(fmt.Errorf("juju: %v", err))
	}
}

// Provider returns the named secret backend provider.
func Provider(backendType string) (SecretBackendProvider, error) {
	return globalBackendRegistry.Provider(backendType)
}
