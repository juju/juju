// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/resource/store"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/containerimageresourcestore"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/errors"
)

// State provides methods for interacting
// with the container image resource store.
type State interface {
	// RemoveContainerImageMetadata removes a container image resources metadata
	// from the container image metadata resource store.
	RemoveContainerImageMetadata(
		ctx context.Context,
		storageKey string,
	) error
	// PutContainerImageMetadata puts a container image resources metadata into
	// the container image metadata resource store.
	// If an image is already stored under the storage key, it returns:
	// - [containerimageresourcestoreerrors.ContainerImageMetadataAlreadyStored]
	PutContainerImageMetadata(
		ctx context.Context,
		storageKey string,
		registryPath, userName, password string,
	) (store.ID, error)
	// GetContainerImageMetadata gets a container image resources metadata from
	// the container image metadata resource store.
	GetContainerImageMetadata(
		ctx context.Context,
		storageKey string,
	) (containerimageresourcestore.ContainerImageMetadata, error)
}

// NewService creates a new instance of Service using the provided State.
func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}

// Service is a ResourceStore for container image resource types.
type Service struct {
	logger logger.Logger
	st     State
}

// Get returns an io.ReadCloser for a resource in the resource store.
func (s *Service) Get(
	ctx context.Context,
	storageKey string,
) (r io.ReadCloser, size int64, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	metadata, err := s.st.GetContainerImageMetadata(ctx, storageKey)
	if err != nil {
		return nil, 0, errors.Errorf("getting container image metadata from state: %w", err)
	}

	details := docker.DockerImageDetails{
		RegistryPath: metadata.RegistryPath,
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: metadata.Username,
				Password: metadata.Password,
			},
		},
	}

	return marshalToReader(details)
}

// Put stores data from io.Reader in the resource store at the path specified in
// the resource.
// If an image is already stored under the storage key, it returns:
// - [containerimageresourcestoreerrors.ContainerImageMetadataAlreadyStored]
func (s *Service) Put(
	ctx context.Context,
	storageKey string,
	r io.Reader,
	_ int64,
	_ store.Fingerprint,
) (_ store.ID, _ int64, _ store.Fingerprint, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	respBuf := new(bytes.Buffer)
	bytesRead, err := respBuf.ReadFrom(r)
	if err != nil {
		return store.ID{}, 0, store.Fingerprint{}, errors.Errorf("reading container image resource: %w", err)
	} else if bytesRead == 0 {
		return store.ID{}, 0, store.Fingerprint{}, errors.Errorf("reading container image resource: zero bytes read")
	}

	dockerDetails, err := docker.UnmarshalDockerResource(respBuf.Bytes())
	if err != nil {
		return store.ID{}, 0, store.Fingerprint{}, errors.Errorf("unmarshalling container image metadata: %w", err)
	}

	mUUID, err := s.st.PutContainerImageMetadata(ctx, storageKey, dockerDetails.RegistryPath, dockerDetails.Username, dockerDetails.Password)
	if err != nil {
		return store.ID{}, 0, store.Fingerprint{}, errors.Errorf("putting container image metadata in state: %w", err)
	}

	size, fingerprint, err := getSizeAndFingerprint(dockerDetails)
	if err != nil {
		return store.ID{}, 0, store.Fingerprint{}, errors.Errorf("generating size and fingerprint for stored resource: %w", err)
	}
	return mUUID, size, fingerprint, nil
}

// Remove removes a resource from storage.
func (s *Service) Remove(
	ctx context.Context,
	storageKey string,
) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.RemoveContainerImageMetadata(ctx, storageKey); err != nil {
		return errors.Errorf("removing container image metadata from state: %w", err)
	}
	return nil
}

// getSizeAndFingerprint gets the size and fingerprint that a DockerImageDetails
// struct will have when exported as a blob from the container image metadata
// service.
func getSizeAndFingerprint(dockerDetails docker.DockerImageDetails) (int64, store.Fingerprint, error) {
	data, err := json.Marshal(dockerDetails)
	if err != nil {
		return 0, store.Fingerprint{}, errors.Errorf("marshelling container image metadata: %w", err)
	}

	fingerprint, err := charmresource.GenerateFingerprint(bytes.NewReader(data))
	if err != nil {
		return 0, store.Fingerprint{}, errors.Errorf("generating fingerprint: %w", err)
	}
	return int64(len(data)), store.NewFingerprint(fingerprint.Fingerprint), nil
}

// marshalToReader marshals a DockerImageDetails struct into a blob. The size and
// fingerprint of the marshalled blob must match the size and fingerprint
// returned from the Put for these details.
func marshalToReader(dockerDetails docker.DockerImageDetails) (io.ReadCloser, int64, error) {
	data, err := json.Marshal(dockerDetails)
	if err != nil {
		return nil, 0, errors.Errorf("marshelling container image metadata: %w", err)
	}
	infoReader := bytes.NewReader(data)
	return io.NopCloser(infoReader), int64(infoReader.Len()), nil
}
