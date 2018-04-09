// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"github.com/juju/errors"

	"github.com/juju/juju/storage"
)

type storageProvider struct{}

var _ storage.Provider = (*storageProvider)(nil)

func (s *storageProvider) VolumeSource(cfg *storage.Config) (storage.VolumeSource, error) {
	return nil, errors.NotImplementedf("VolumeSource")
}

func (s *storageProvider) FilesystemSource(cfg *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystemsource")
}

func (s *storageProvider) Supports(kind storage.StorageKind) bool {
	return kind == storage.StorageKindBlock
}

func (s *storageProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

func (s *storageProvider) Dynamic() bool {
	return true
}

func (s *storageProvider) Releasable() bool {
	// TODO (gsamfira): add support
	return false
}

func (s *storageProvider) DefaultPools() []*storage.Config {
	return nil
}

func (s *storageProvider) ValidateConfig(cfg *storage.Config) error {
	return errors.NotImplementedf("ValidateConfig")
}
