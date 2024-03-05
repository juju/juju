// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"
	"io"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// StateBackend describes an API for accessing/mutating information in state.
type StateBackend interface {
	UpdateUploadedCharm(info state.CharmInfo) (UploadedCharm, error)
	PrepareCharmUpload(curl string) (UploadedCharm, error)
	ModelUUID() string
}

// UploadedCharm represents a charm whose upload status can be queried.
type UploadedCharm interface {
	IsUploaded() bool
}

// ModelBackend describes an API for accessing model-specific details.
type ModelBackend interface {
	Config() (*config.Config, error)
}

// Storage describes an API for storing and deleting blobs.
type Storage interface {
	// Put stores data from reader at path, namespaced to the model.
	Put(context.Context, string, io.Reader, int64) error
	// Remove removes data at path, namespaced to the model.
	Remove(context.Context, string) error
}
