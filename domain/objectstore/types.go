// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
)

// Metadata represents the metadata for an object.
type Metadata struct {
	// UUID is the uuid for the metadata.
	UUID string
	// Hash is the hash of the object.
	Hash string
	// Path is the path to the object.
	Path string
	// Size is the size of the object.
	Size int64
}

// S3Credentials represents the credentials for the s3 object store.
type S3Credentials struct {
	// Endpoint is the endpoint for the object store.
	Endpoint string
	// AccessKey is the access key for the object store.
	AccessKey string
	// SecretKey is the secret key for the object store.
	SecretKey string
	// SessionToken is the session token for the object store.
	SessionToken string
}

// Validate validates the S3Credentials.
func (s S3Credentials) Validate() error {
	if s.Endpoint == "" {
		return errors.New("endpoint is required")
	}
	if s.AccessKey == "" {
		return errors.New("access key is required")
	}
	if s.SecretKey == "" {
		return errors.New("secret key is required")
	}
	return nil
}

// DrainingInfo represents the information about the draining process, including
// the phase of the draining process, and the uuids of the backends that are
// being drained from and to. This information can be used to correlate with
// logs and other information about the draining process.
type DrainingInfo struct {
	// UUID is the uuid for the draining info.
	UUID string
	// Phase is the phase of the draining process.
	Phase string
	// FromBackendUUID is the uuid of the backend that is being drained from.
	FromBackendUUID string
	// ToBackendUUID is the uuid of the backend that is being drained to.
	ToBackendUUID string
}

// BackendInfo represents the information about an object store backend,
// including the uuid and the type of the object store.
type BackendInfo struct {
	// UUID is the uuid for the backend.
	UUID string
	// LifeID is the life id for the backend, which indicates if the backend is
	// active (life_id = 0) or not (life_id >= 1).
	LifeID life.Life
	// ObjectStoreType is the type of the object store.
	ObjectStoreType string

	// Endpoint, AccessKey, SecretKey, and Region are only used for S3 backend.
	Endpoint *string

	// AccessKey is not returned for security reasons, but it is expected to be
	// set in the state when the backend is S3, and it will be used to create
	// the S3 client for the draining process.
	AccessKey *string

	// SecretKey is not returned for security reasons, but it is expected to be
	// set in the state when the backend is S3, and it will be used to create
	// the S3 client for the draining process.
	SecretKey *string

	// SessionToken is not returned for security reasons, but it is expected to
	// be set in the state when the backend is S3, and it will be used to create
	// the S3 client for the draining process.
	SessionToken *string
}
