// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
)

var _ storage.Provider = (*StorageProvider)(nil)

// StorageProvider is an implementation of storage.Provider, suitable for testing.
// Each method's default behaviour may be overridden by setting the corresponding
// Func field.
type StorageProvider struct {
	testing.Stub

	// StorageScope defines the scope of storage managed by this provider.
	StorageScope storage.Scope

	// IsDynamic defines whether or not the provider reports that it supports
	// dynamic provisioning.
	IsDynamic bool

	// VolumeSourceFunc will be called by VolumeSource, if non-nil;
	// otherwise VolumeSource will return a NotSupported error.
	VolumeSourceFunc func(*config.Config, *storage.Config) (storage.VolumeSource, error)

	// FilesystemSourceFunc will be called by FilesystemSource, if non-nil;
	// otherwise FilesystemSource will return a NotSupported error.
	FilesystemSourceFunc func(*config.Config, *storage.Config) (storage.FilesystemSource, error)

	// ValidateConfigFunc will be called by ValidateConfig, if non-nil;
	// otherwise ValidateConfig returns nil.
	ValidateConfigFunc func(*storage.Config) error

	// SupportsFunc will be called by Supports, if non-nil; otherwise,
	// Supports returns true.
	SupportsFunc func(kind storage.StorageKind) bool
}

// VolumeSource is defined on storage.Provider.
func (p *StorageProvider) VolumeSource(environConfig *config.Config, providerConfig *storage.Config) (storage.VolumeSource, error) {
	p.MethodCall(p, "VolumeSource", environConfig, providerConfig)
	if p.VolumeSourceFunc != nil {
		return p.VolumeSourceFunc(environConfig, providerConfig)
	}
	return nil, errors.NotSupportedf("volumes")
}

// FilesystemSource is defined on storage.Provider.
func (p *StorageProvider) FilesystemSource(environConfig *config.Config, providerConfig *storage.Config) (storage.FilesystemSource, error) {
	p.MethodCall(p, "FilesystemSource", environConfig, providerConfig)
	if p.FilesystemSourceFunc != nil {
		return p.FilesystemSourceFunc(environConfig, providerConfig)
	}
	return nil, errors.NotSupportedf("filesystems")
}

// ValidateConfig is defined on storage.Provider.
func (p *StorageProvider) ValidateConfig(providerConfig *storage.Config) error {
	p.MethodCall(p, "ValidateConfig", providerConfig)
	if p.ValidateConfigFunc != nil {
		return p.ValidateConfigFunc(providerConfig)
	}
	return nil
}

// Supports is defined on storage.Provider.
func (p *StorageProvider) Supports(kind storage.StorageKind) bool {
	p.MethodCall(p, "Supports", kind)
	if p.SupportsFunc != nil {
		return p.SupportsFunc(kind)
	}
	return true
}

// Scope is defined on storage.Provider.
func (p *StorageProvider) Scope() storage.Scope {
	p.MethodCall(p, "Scope")
	return p.StorageScope
}

// Dynamic is defined on storage.Provider.
func (p *StorageProvider) Dynamic() bool {
	p.MethodCall(p, "Dynamic")
	return p.IsDynamic
}
