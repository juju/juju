// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
	"github.com/juju/juju/provider/oracle"
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

var _ oracle.StorageAPI = (*FakeStorageAPI)(nil)

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

func (f FakeStorageAttachment) CreateStorageAttachment(api.StorageAttachmentParams) (response.StorageAttachment, error) {
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
	DefaultAllStorageVolumes = response.AllStorageVolumes{
		Result: []response.StorageVolume{
			response.StorageVolume{
				Account:           "/Compute-a432100/default",
				Bootable:          true,
				Description:       nil,
				Hypervisor:        nil,
				Imagelist:         "/Compute-a432100/sgiulitti@cloudbase.com/Ubuntu.16.04-LTS.amd64.20170307",
				Imagelist_entry:   1,
				Machineimage_name: "/Compute-a432100/sgiulitti@cloudbase.com/Ubuntu.16.04-LTS.amd64.20170307",
				Managed:           true,
				Name:              "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools_storage",
				Platform:          "linux",
				Properties: []common.StoragePool{
					"/oracle/public/storage/default",
				},
				Quota:            nil,
				Readonly:         false,
				Shared:           false,
				Size:             10,
				Snapshot:         nil,
				Snapshot_account: "",
				Snapshot_id:      "",
				Status:           "Online",
				Status_detail:    "",
				Status_timestamp: "2017-04-06T14:23:54Z",
				Storage_pool:     "/uscom-central-1/chi1-opc-c10r310-zfs-1-v1/storagepool/iscsi",
				Tags:             []string{},
				Uri:              "https://compute.uscom-central-1.oraclecloud.com/storage/volume/Compute-a432100/sgiulitti%40cloudbase.com/JujuTools_storage",
				Writecache:       false,
			},
		},
	}

	DefaultFakeStorageAPI = &FakeStorageAPI{
		FakeComposer: FakeComposer{
			compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
		},
		FakeStorageVolume: FakeStorageVolume{
			All:    DefaultAllStorageVolumes,
			AllErr: nil,
			StorageVolume: response.StorageVolume{
				Account:           "/Compute-a432100/default",
				Bootable:          true,
				Description:       nil,
				Hypervisor:        nil,
				Imagelist:         "/Compute-a432100/sgiulitti@cloudbase.com/Ubuntu.16.04-LTS.amd64.20170307",
				Imagelist_entry:   1,
				Machineimage_name: "/Compute-a432100/sgiulitti@cloudbase.com/Ubuntu.16.04-LTS.amd64.20170307",
				Managed:           true,
				Name:              "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools_storage",
				Platform:          "linux",
				Properties: []common.StoragePool{
					"/oracle/public/storage/default",
				},
				Quota:            nil,
				Readonly:         false,
				Shared:           false,
				Size:             10,
				Snapshot:         nil,
				Snapshot_account: "",
				Snapshot_id:      "",
				Status:           "Online",
				Status_detail:    "",
				Status_timestamp: "2017-04-06T14:23:54Z",
				Storage_pool:     "/uscom-central-1/chi1-opc-c10r310-zfs-1-v1/storagepool/iscsi",
				Tags:             []string{},
				Uri:              "https://compute.uscom-central-1.oraclecloud.com/storage/volume/Compute-a432100/sgiulitti%40cloudbase.com/JujuTools_storage",
				Writecache:       false,
			},
			StorageVolumeErr: nil,
			DeleteErr:        nil,
			Create: response.StorageVolume{
				Account:           "/Compute-a432100/default",
				Bootable:          true,
				Description:       nil,
				Hypervisor:        nil,
				Imagelist:         "/Compute-a432100/sgiulitti@cloudbase.com/Ubuntu.16.04-LTS.amd64.20170307",
				Imagelist_entry:   1,
				Machineimage_name: "/Compute-a432100/sgiulitti@cloudbase.com/Ubuntu.16.04-LTS.amd64.20170307",
				Managed:           true,
				Name:              "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools_storage",
				Platform:          "linux",
				Properties: []common.StoragePool{
					"/oracle/public/storage/default",
				},
				Quota:            nil,
				Readonly:         false,
				Shared:           false,
				Size:             10,
				Snapshot:         nil,
				Snapshot_account: "",
				Snapshot_id:      "",
				Status:           "Online",
				Status_detail:    "",
				Status_timestamp: "2017-04-06T14:23:54Z",
				Storage_pool:     "/uscom-central-1/chi1-opc-c10r310-zfs-1-v1/storagepool/iscsi",
				Tags:             []string{},
				Uri:              "https://compute.uscom-central-1.oraclecloud.com/storage/volume/Compute-a432100/sgiulitti%40cloudbase.com/JujuTools_storage",
				Writecache:       false,
			},
			CreateErr: nil,
			Update: response.StorageVolume{
				Account:           "/Compute-a432100/default",
				Bootable:          true,
				Description:       nil,
				Hypervisor:        nil,
				Imagelist:         "/Compute-a432100/sgiulitti@cloudbase.com/Ubuntu.16.04-LTS.amd64.20170307",
				Imagelist_entry:   1,
				Machineimage_name: "/Compute-a432100/sgiulitti@cloudbase.com/Ubuntu.16.04-LTS.amd64.20170307",
				Managed:           true,
				Name:              "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools_storage",
				Platform:          "linux",
				Properties: []common.StoragePool{
					"/oracle/public/storage/default",
				},
				Quota:            nil,
				Readonly:         false,
				Shared:           false,
				Size:             10,
				Snapshot:         nil,
				Snapshot_account: "",
				Snapshot_id:      "",
				Status:           "Online",
				Status_detail:    "",
				Status_timestamp: "2017-04-06T14:23:54Z",
				Storage_pool:     "/uscom-central-1/chi1-opc-c10r310-zfs-1-v1/storagepool/iscsi",
				Tags:             []string{},
				Uri:              "https://compute.uscom-central-1.oraclecloud.com/storage/volume/Compute-a432100/sgiulitti%40cloudbase.com/JujuTools_storage",
				Writecache:       false,
			},

			UpdateErr: nil,
		},
		FakeStorageAttachment: FakeStorageAttachment{
			All: response.AllStorageAttachments{
				Result: []response.StorageAttachment{
					response.StorageAttachment{
						Account:             nil,
						Hypervisor:          nil,
						Index:               1,
						Instance_name:       "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools/ebc4ce91-56bb-4120-ba78-13762597f837",
						Storage_volume_name: "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools_storage",
						Name:                "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools/ebc4ce91-56bb-4120-ba78-13762597f837/1f90e657-f852-45ad-afbf-9a94f640a7ae",
						Readonly:            false,
						State:               "attached",
						Uri:                 "https://compute.uscom-central-1.oraclecloud.com/storage/attachment/Compute-a432100/sgiulitti%40cloudbase.com/JujuTools/ebc4ce91-56bb-4120-ba78-13762597f837/1f90e657-f852-45ad-afbf-9a94f640a7ae",
					},
				},
			},
			AllErr: nil,
			StorageAttachment: response.StorageAttachment{
				Account:             nil,
				Hypervisor:          nil,
				Index:               1,
				Instance_name:       "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools/ebc4ce91-56bb-4120-ba78-13762597f837",
				Storage_volume_name: "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools_storage",
				Name:                "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools/ebc4ce91-56bb-4120-ba78-13762597f837/1f90e657-f852-45ad-afbf-9a94f640a7ae",
				Readonly:            false,
				State:               "attached",
				Uri:                 "https://compute.uscom-central-1.oraclecloud.com/storage/attachment/Compute-a432100/sgiulitti%40cloudbase.com/JujuTools/ebc4ce91-56bb-4120-ba78-13762597f837/1f90e657-f852-45ad-afbf-9a94f640a7ae",
			},
			StorageAttachmentErr: nil,
			Create: response.StorageAttachment{
				Account:             nil,
				Hypervisor:          nil,
				Index:               1,
				Instance_name:       "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools/ebc4ce91-56bb-4120-ba78-13762597f837",
				Storage_volume_name: "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools_storage",
				Name:                "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools/ebc4ce91-56bb-4120-ba78-13762597f837/1f90e657-f852-45ad-afbf-9a94f640a7ae",
				Readonly:            false,
				State:               "attached",
				Uri:                 "https://compute.uscom-central-1.oraclecloud.com/storage/attachment/Compute-a432100/sgiulitti%40cloudbase.com/JujuTools/ebc4ce91-56bb-4120-ba78-13762597f837/1f90e657-f852-45ad-afbf-9a94f640a7ae",
			},
			CreateErr: nil,
			DeleteErr: nil,
		},
	}
)
