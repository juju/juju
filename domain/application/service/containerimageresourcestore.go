// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/juju/juju/domain/application/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/errors"
)

func newContainerImageResourceStore(st ContainerImageMetadataState) *containerImageResourceStore {
	return &containerImageResourceStore{st: st}
}

// containerImageResourceStore is a ResourceStore for container image resource types.
type containerImageResourceStore struct {
	st ContainerImageMetadataState
}

// Get returns an io.ReadCloser for a resource in the resource store.
func (s containerImageResourceStore) Get(
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
func (s containerImageResourceStore) Put(
	ctx context.Context,
	storageKey string,
	r io.Reader,
	size int64,
	fingerprint charmresource.Fingerprint,
) (resource.ResourceStorageUUID, error) {
	respBuf := new(bytes.Buffer)
	bytesRead, err := respBuf.ReadFrom(r)
	if err != nil {
		return "", errors.Errorf("reading container image resource: %w", err)
	} else if bytesRead == 0 {
		return "", errors.Errorf("reading container image resource: zero bytes read")
	}

	dockerDetails, err := docker.UnmarshalDockerResource(respBuf.Bytes())
	if err != nil {
		return "", errors.Errorf("unmarshalling container image metadata: %w", err)
	}

	mUUID, err := s.st.PutContainerImageMetadata(ctx, storageKey, dockerDetails.RegistryPath, dockerDetails.Username, dockerDetails.Password)
	if err != nil {
		return "", errors.Errorf("putting container image metadata in state: %w", err)
	}

	return mUUID, nil
}

// Remove removes a resource from storage.
func (s containerImageResourceStore) Remove(
	ctx context.Context,
	storageKey string,
) error {
	err := s.st.RemoveContainerImageMetadata(ctx, storageKey)
	if err != nil {
		return errors.Errorf("removing container image metadata from state: %w", err)
	}
	return nil
}
