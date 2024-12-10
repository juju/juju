// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/juju/juju/core/resource/store"
	"github.com/juju/juju/domain/containerimageresourcestore"
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

func NewService(st State) *Service {
	return &Service{st: st}
}

// Service is a ResourceStore for container image resource types.
type Service struct {
	st State
}

// Get returns an io.ReadCloser for a resource in the resource store.
func (s Service) Get(
	ctx context.Context,
	storageKey string,
) (r io.ReadCloser, size int64, err error) {
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
	data, err := json.Marshal(details)
	if err != nil {
		return nil, 0, errors.Errorf("marshelling container image metadata: %w", err)
	}
	infoReader := bytes.NewReader(data)
	length := infoReader.Len()
	return io.NopCloser(infoReader), int64(length), nil
}

// Put stores data from io.Reader in the resource store at the
// path specified in the resource.
func (s Service) Put(
	ctx context.Context,
	storageKey string,
	r io.Reader,
	_ int64,
	_ store.Fingerprint,
) (store.ID, error) {
	respBuf := new(bytes.Buffer)
	bytesRead, err := respBuf.ReadFrom(r)
	if err != nil {
		return store.ID{}, errors.Errorf("reading container image resource: %w", err)
	} else if bytesRead == 0 {
		return store.ID{}, errors.Errorf("reading container image resource: zero bytes read")
	}

	dockerDetails, err := docker.UnmarshalDockerResource(respBuf.Bytes())
	if err != nil {
		return store.ID{}, errors.Errorf("unmarshalling container image metadata: %w", err)
	}

	mUUID, err := s.st.PutContainerImageMetadata(ctx, storageKey, dockerDetails.RegistryPath, dockerDetails.Username, dockerDetails.Password)
	if err != nil {
		return store.ID{}, errors.Errorf("putting container image metadata in state: %w", err)
	}

	return mUUID, nil
}

// Remove removes a resource from storage.
func (s Service) Remove(
	ctx context.Context,
	storageKey string,
) error {
	err := s.st.RemoveContainerImageMetadata(ctx, storageKey)
	if err != nil {
		return errors.Errorf("removing container image metadata from state: %w", err)
	}
	return nil
}
