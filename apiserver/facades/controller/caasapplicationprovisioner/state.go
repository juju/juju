// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/state"
)

// CAASApplicationProvisionerState provides the subset of model state
// required by the CAAS operator provisioner facade.
type CAASApplicationProvisionerState interface {
	ApplyOperation(state.ModelOperation) error
	Model() (Model, error)
	Resources(objectstore.ObjectStore) Resources
	IsController() bool
}

type Model interface {
	Containers(providerIds ...string) ([]state.CloudContainer, error)
}

type Resources interface {
	OpenResource(applicationID string, name string) (resource.Resource, io.ReadCloser, error)
}

type stateShim struct {
	*state.State
}

func (s stateShim) Model() (Model, error) {
	model, err := s.State.Model()
	if err != nil {
		return nil, err
	}
	return model.CAASModel()
}

func (s stateShim) Resources(_ objectstore.ObjectStore) Resources {
	return &resourcesShim{}
}

type resourcesShim struct{}

func (s resourcesShim) OpenResource(applicationID string, name string) (resource.Resource, io.ReadCloser, error) {
	return resource.Resource{}, nil, errors.NotImplementedf("OpenResource")
}

// StorageBackend provides the subset of backend storage
// functionality required by the CAAS app provisioner facade.
type StorageBackend interface {
	StorageInstance(names.StorageTag) (state.StorageInstance, error)
	Filesystem(names.FilesystemTag) (state.Filesystem, error)
	StorageInstanceFilesystem(names.StorageTag) (state.Filesystem, error)
	UnitStorageAttachments(unit names.UnitTag) ([]state.StorageAttachment, error)
	SetFilesystemInfo(names.FilesystemTag, state.FilesystemInfo) error
	SetFilesystemAttachmentInfo(names.Tag, names.FilesystemTag, state.FilesystemAttachmentInfo) error
	Volume(tag names.VolumeTag) (state.Volume, error)
	StorageInstanceVolume(tag names.StorageTag) (state.Volume, error)
	SetVolumeInfo(names.VolumeTag, state.VolumeInfo) error
	SetVolumeAttachmentInfo(names.Tag, names.VolumeTag, state.VolumeAttachmentInfo) error

	// These are for cleanup up orphaned filesystems when pods are recreated.
	// TODO(caas) - record unit id on the filesystem so we can query by unit
	AllFilesystems() ([]state.Filesystem, error)
	DestroyStorageInstance(tag names.StorageTag, destroyAttachments bool, force bool, maxWait time.Duration) (err error)
	DestroyFilesystem(tag names.FilesystemTag, force bool) (err error)
}
