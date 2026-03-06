// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import "github.com/juju/juju/internal/errors"

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
