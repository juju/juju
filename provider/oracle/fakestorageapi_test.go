// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/response"
)

// FakeComposer implements common.Composer interface
type FakeComposer struct {
	compose string
}

func (f FakeComposer) ComposeName(name string) string {
	return f.compose
}

// FakeStorageVolume implements the common.StorageVolumeAPI
type FakeStorageVolume struct {
	StorageVolume    response.StorageVolume
	StorageVolumeErr error
	Create           response.StorageVolume
	CreateErr        error
	All              response.AllStorageVolumes
	AllErr           error
	DeleteErr        error
	Update           response.StorageVolume
	UpdateErr        error
}

func (f FakeStorageVolume) AllStorageVolumes([]api.Filter) (response.AllStorageVolumes, error) {
	return f.All, f.AllErr
}
func (f FakeStorageVolume) StorageVolumeDetails(string) (response.StorageVolume, error) {
	return f.StorageVolume, f.StorageVolumeErr
}
func (f FakeStorageVolume) CreateStorageVolume(api.StorageVolumeParams) (response.StorageVolume, error) {
	return f.Create, f.CreateErr
}
func (f FakeStorageVolume) DeleteStorageVolume(string) error {
	return f.DeleteErr
}
func (f FakeStorageVolume) UpdateStorageVolume(api.StorageVolumeParams, string) (response.StorageVolume, error) {
	return f.Update, f.UpdateErr
}

// FakeStorageAttachment implements the common.FakeStorageAttachmentAPI
type FakeStorageAttachment struct {
	Create               response.StorageAttachment
	CreateErr            error
	DeleteErr            error
	All                  response.AllStorageAttachments
	AllErr               error
	StorageAttachment    response.StorageAttachment
	StorageAttachmentErr error
}

func (f FakeStorageAttachment) CreateStorageAttachment(api.StorageAttachment) (response.StorageAttachment, error) {
	return f.Create, f.CreateErr
}
func (f FakeStorageAttachment) DeleteStorageAttachment(string) error {
	return f.DeleteErr
}
func (f FakeStorageAttachment) StorageAttachmentDetails(string) (response.StorageAttachment, error) {
	return f.StorageAttachment, f.StorageAttachmentErr
}
func (f FakeStorageAttachment) AllStorageAttachments([]api.Filter) (response.AllStorageAttachments, error) {
	return f.All, f.AllErr
}

// FakeStorageAPi used to mock the internal StorageAPI imeplementation
// This type implements the StorageAPI interface
type FakeStorageAPI struct {
	FakeComposer
	FakeStorageVolume
	FakeStorageAttachment
}

var (
	DefaultFakeStorageAPI = &FakeStorageAPI{
		FakeComposer: FakeComposer{
			compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
		},
		FakeStorageVolume:     FakeStorageVolume{},
		FakeStorageAttachment: FakeStorageAttachment{},
	}
)
