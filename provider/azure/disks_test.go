// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"encoding/xml"
	"fmt"
	"net/http"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/gwacl"

	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

var mediaLinkPrefix = fmt.Sprintf(
	"https://account-name.blob.core.windows.net/vhds/%s/",
	testing.EnvironmentTag.Id(),
)

type storageProviderSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&storageSuite{})

func (*storageProviderSuite) TestValidateConfigUnknownConfig(c *gc.C) {
	p := azureStorageProvider{}
	cfg, err := storage.NewConfig("foo", storageProviderType, map[string]interface{}{
		"unknown": "config",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, jc.ErrorIsNil) // unknown attrs ignored
}

func (s *storageProviderSuite) TestSupports(c *gc.C) {
	p := azureStorageProvider{}
	c.Assert(p.Supports(storage.StorageKindBlock), jc.IsTrue)
	c.Assert(p.Supports(storage.StorageKindFilesystem), jc.IsFalse)
}

var _ = gc.Suite(&azureVolumeSuite{})

type azureVolumeSuite struct {
	testing.BaseSuite
}

func (s *azureVolumeSuite) volumeSource(c *gc.C, cfg *storage.Config) storage.VolumeSource {
	envCfg := makeEnviron(c).Config()
	p := azureStorageProvider{}
	vs, err := p.VolumeSource(envCfg, cfg)
	c.Assert(err, jc.ErrorIsNil)
	return vs
}

func (s *azureVolumeSuite) TestCreateVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)

	machine := names.NewMachineTag("123")
	volume0 := names.NewVolumeTag("0")
	volume1 := names.NewVolumeTag("1")

	env := makeEnviron(c)
	prefix := env.getEnvPrefix()
	serviceName := "service"
	service := makeDeployment(env, prefix+serviceName)

	roleName := service.Deployments[0].RoleList[0].RoleName
	inst, err := env.getInstance(service, roleName)
	c.Assert(err, jc.ErrorIsNil)

	params := []storage.VolumeParams{{
		Tag:      volume0,
		Size:     10 * 1000,
		Provider: storageProviderType,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Machine:    machine,
				InstanceId: inst.Id(),
			},
		},
	}, {
		Tag:      volume1,
		Size:     20 * 1000,
		Provider: storageProviderType,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Machine:    machine,
				InstanceId: inst.Id(),
			},
		},
	}}

	getRoleResponse0, err := xml.Marshal(&gwacl.PersistentVMRole{})
	c.Assert(err, jc.ErrorIsNil)

	// Second time, respond saying LUN 0 is in use; this should
	// cause LUN 1 to be assigned.
	dataVirtualHardDisks := []gwacl.DataVirtualHardDisk{
		{LUN: 0},
	}
	getRoleResponse1, err := xml.Marshal(&gwacl.PersistentVMRole{
		DataVirtualHardDisks: &dataVirtualHardDisks,
	})
	c.Assert(err, jc.ErrorIsNil)

	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(getRoleResponse0, http.StatusOK, nil),
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil), // AddDataDisk
		gwacl.NewDispatcherResponse(getRoleResponse1, http.StatusOK, nil),
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil), // AddDataDisk
	})

	results, err := vs.CreateVolumes(params)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.HasLen, 2)
	c.Assert(results[0], jc.DeepEquals, storage.CreateVolumesResult{
		Volume: &storage.Volume{
			Tag: volume0,
			VolumeInfo: storage.VolumeInfo{
				VolumeId: "volume-0.vhd",
				Size:     10 * 1024, // rounded up
			},
		},
		VolumeAttachment: &storage.VolumeAttachment{
			Volume:  volume0,
			Machine: machine,
			VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
				BusAddress: "scsi@5:0.0.0",
			},
		}},
	)
	c.Assert(results[1], jc.DeepEquals, storage.CreateVolumesResult{
		Volume: &storage.Volume{
			Tag: volume1,
			VolumeInfo: storage.VolumeInfo{
				VolumeId: "volume-1.vhd",
				Size:     20 * 1024, // rounded up
			},
		},
		VolumeAttachment: &storage.VolumeAttachment{
			Volume:  volume1,
			Machine: machine,
			VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
				BusAddress: "scsi@5:0.0.1",
			},
		}},
	)
}

func (s *azureVolumeSuite) TestCreateVolumesInvalidVolumeParams(c *gc.C) {
	vs := s.volumeSource(c, nil)

	args := storage.VolumeParams{
		Tag:      names.NewVolumeTag("0"),
		Size:     1023 * 1024,
		Provider: storageProviderType,
	}
	err := vs.ValidateVolumeParams(args)
	c.Assert(err, jc.ErrorIsNil)

	args.Size++ // One more MiB and we're out

	results, err := vs.CreateVolumes([]storage.VolumeParams{args})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results[0].Error, gc.ErrorMatches, "1024 GiB exceeds the maximum of 1023 GiB")
}

func (s *azureVolumeSuite) TestCreateVolumesNoLuns(c *gc.C) {
	vs := s.volumeSource(c, nil)

	machine := names.NewMachineTag("123")
	volume := names.NewVolumeTag("0")

	env := makeEnviron(c)
	prefix := env.getEnvPrefix()
	serviceName := "service"
	service := makeDeployment(env, prefix+serviceName)
	roleName := service.Deployments[0].RoleList[0].RoleName
	inst, err := env.getInstance(service, roleName)
	c.Assert(err, jc.ErrorIsNil)

	params := []storage.VolumeParams{{
		Tag:      volume,
		Size:     10 * 1000,
		Provider: storageProviderType,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Machine:    machine,
				InstanceId: inst.Id(),
			},
		},
	}}

	dataVirtualHardDisks := make([]gwacl.DataVirtualHardDisk, 32)
	for i := range dataVirtualHardDisks {
		dataVirtualHardDisks[i].LUN = i
	}
	getRoleResponse, err := xml.Marshal(&gwacl.PersistentVMRole{
		DataVirtualHardDisks: &dataVirtualHardDisks,
	})
	c.Assert(err, jc.ErrorIsNil)

	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(getRoleResponse, http.StatusOK, nil),
	})

	results, err := vs.CreateVolumes(params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.ErrorMatches, "choosing LUN: all LUNs are in use")
}

func (s *azureVolumeSuite) TestCreateVolumesLegacyInstance(c *gc.C) {
	vs := s.volumeSource(c, nil)

	machine := names.NewMachineTag("123")
	volume := names.NewVolumeTag("0")

	env := makeEnviron(c)
	prefix := env.getEnvPrefix()
	serviceName := "service"
	service := makeLegacyDeployment(env, prefix+serviceName)
	inst, err := env.getInstance(service, "")
	c.Assert(err, jc.ErrorIsNil)

	params := []storage.VolumeParams{{
		Tag:      volume,
		Size:     10 * 1000,
		Provider: storageProviderType,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Machine:    machine,
				InstanceId: inst.Id(),
			},
		},
	}}

	results, err := vs.CreateVolumes(params)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.ErrorMatches, "attaching disks to legacy instances not supported")
}

func (s *azureVolumeSuite) TestDestroyVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	results, err := vs.DestroyVolumes([]string{"volume-0.vhd", "volume-1.vhd"})
	c.Assert(err, gc.ErrorMatches, "DestroyVolumes not supported")
	c.Assert(results, gc.HasLen, 0)
}

func (s *azureVolumeSuite) TestDescribeVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)

	type disks struct {
		Disks []gwacl.Disk `xml:"Disk"`
	}

	listDisksResponse, err := xml.Marshal(&disks{Disks: []gwacl.Disk{{
		MediaLink:       mediaLinkPrefix + "volume-1.vhd",
		LogicalSizeInGB: 22,
	}, {
		MediaLink:       mediaLinkPrefix + "volume-0.vhd",
		LogicalSizeInGB: 11,
	}, {
		MediaLink:       "someOtherJunk.vhd",
		LogicalSizeInGB: 33,
	}}})
	c.Assert(err, jc.ErrorIsNil)

	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(listDisksResponse, http.StatusOK, nil),
	})

	volumes, err := vs.DescribeVolumes([]string{"volume-0.vhd", "volume-1.vhd"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes, gc.HasLen, 2)
	c.Assert(volumes, jc.DeepEquals, []storage.DescribeVolumesResult{{
		VolumeInfo: &storage.VolumeInfo{
			Size:     11 * 1024,
			VolumeId: "volume-0.vhd",
		},
	}, {
		VolumeInfo: &storage.VolumeInfo{
			Size:     22 * 1024,
			VolumeId: "volume-1.vhd",
		},
	}})
}

func (s *azureVolumeSuite) TestDescribeVolumesNotFound(c *gc.C) {
	vs := s.volumeSource(c, nil)

	type disks struct {
		Disks []gwacl.Disk `xml:"Disk"`
	}

	listDisksResponse, err := xml.Marshal(&disks{Disks: []gwacl.Disk{{
		MediaLink:       mediaLinkPrefix + "volume-0.vhd",
		LogicalSizeInGB: 11,
	}}})
	c.Assert(err, jc.ErrorIsNil)

	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(listDisksResponse, http.StatusOK, nil),
	})

	volumes, err := vs.DescribeVolumes([]string{"volume-0.vhd", "volume-1.vhd"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes, gc.HasLen, 2)
	c.Assert(volumes[0].Error, gc.IsNil)
	c.Assert(volumes[1].Error, gc.ErrorMatches, "volume volume-1.vhd not found")
}

func (s *azureVolumeSuite) TestListVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)

	type disks struct {
		Disks []gwacl.Disk `xml:"Disk"`
	}

	listDisksResponse, err := xml.Marshal(&disks{Disks: []gwacl.Disk{{
		MediaLink:       mediaLinkPrefix + "volume-1.vhd",
		LogicalSizeInGB: 22,
	}, {
		MediaLink:       mediaLinkPrefix + "volume-0.vhd",
		LogicalSizeInGB: 11,
	}, {
		MediaLink:       "someOtherJunk.vhd",
		LogicalSizeInGB: 33,
	}}})
	c.Assert(err, jc.ErrorIsNil)

	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(listDisksResponse, http.StatusOK, nil),
	})

	volIds, err := vs.ListVolumes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volIds, jc.SameContents, []string{"volume-0.vhd", "volume-1.vhd"})
}

func (s *azureVolumeSuite) TestAttachVolumesAlreadyAttached(c *gc.C) {
	vs := s.volumeSource(c, nil)

	machine0 := names.NewMachineTag("0")
	machine1 := names.NewMachineTag("1")
	volume0 := names.NewVolumeTag("0")
	volume1 := names.NewVolumeTag("1")
	volume2 := names.NewVolumeTag("2")

	env := makeEnviron(c)
	prefix := env.getEnvPrefix()
	service := makeDeployment(env, prefix+"service")
	roleName0 := service.Deployments[0].RoleList[0].RoleName
	roleName1 := service.Deployments[0].RoleList[1].RoleName
	inst0, err := env.getInstance(service, roleName0)
	c.Assert(err, jc.ErrorIsNil)
	inst1, err := env.getInstance(service, roleName1)
	c.Assert(err, jc.ErrorIsNil)

	// First VM.
	dataVirtualHardDisks0 := []gwacl.DataVirtualHardDisk{{
		MediaLink:           mediaLinkPrefix + "volume-0.vhd",
		LUN:                 0,
		LogicalDiskSizeInGB: 1,
	}, {
		MediaLink:           mediaLinkPrefix + "volume-1.vhd",
		LUN:                 1,
		LogicalDiskSizeInGB: 2,
	}}
	getRoleResponse0, err := xml.Marshal(&gwacl.PersistentVMRole{
		DataVirtualHardDisks: &dataVirtualHardDisks0,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Second VM.
	dataVirtualHardDisks1 := []gwacl.DataVirtualHardDisk{{
		MediaLink:           mediaLinkPrefix + "volume-2.vhd",
		LUN:                 0,
		LogicalDiskSizeInGB: 3,
	}}
	getRoleResponse1, err := xml.Marshal(&gwacl.PersistentVMRole{
		DataVirtualHardDisks: &dataVirtualHardDisks1,
	})
	c.Assert(err, jc.ErrorIsNil)

	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(getRoleResponse0, http.StatusOK, nil),
		gwacl.NewDispatcherResponse(getRoleResponse1, http.StatusOK, nil),
	})

	results, err := vs.AttachVolumes([]storage.VolumeAttachmentParams{{
		Volume:   volume0,
		VolumeId: "volume-0.vhd",
		AttachmentParams: storage.AttachmentParams{
			Machine:    machine0,
			InstanceId: inst0.Id(),
		},
	}, {
		Volume:   volume1,
		VolumeId: "volume-1.vhd",
		AttachmentParams: storage.AttachmentParams{
			Machine:    machine0,
			InstanceId: inst0.Id(),
		},
	}, {
		Volume:   volume2,
		VolumeId: "volume-2.vhd",
		AttachmentParams: storage.AttachmentParams{
			Machine:    machine1,
			InstanceId: inst1.Id(),
		},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []storage.AttachVolumesResult{{
		VolumeAttachment: &storage.VolumeAttachment{
			Volume:  volume0,
			Machine: machine0,
			VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
				BusAddress: "scsi@5:0.0.0",
			},
		},
	}, {
		VolumeAttachment: &storage.VolumeAttachment{
			Volume:  volume1,
			Machine: machine0,
			VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
				BusAddress: "scsi@5:0.0.1",
			},
		},
	}, {
		VolumeAttachment: &storage.VolumeAttachment{
			Volume:  volume2,
			Machine: machine1,
			VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
				BusAddress: "scsi@5:0.0.0",
			},
		},
	}})
}

func (s *azureVolumeSuite) TestAttachVolumesNotAttached(c *gc.C) {
	vs := s.volumeSource(c, nil)

	machine := names.NewMachineTag("0")
	volume := names.NewVolumeTag("0")

	env := makeEnviron(c)
	prefix := env.getEnvPrefix()
	service := makeDeployment(env, prefix+"service")
	roleName := service.Deployments[0].RoleList[0].RoleName
	inst, err := env.getInstance(service, roleName)
	c.Assert(err, jc.ErrorIsNil)

	getRoleResponse, err := xml.Marshal(&gwacl.PersistentVMRole{})
	c.Assert(err, jc.ErrorIsNil)

	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(getRoleResponse, http.StatusOK, nil),
	})

	results, err := vs.AttachVolumes([]storage.VolumeAttachmentParams{{
		Volume:   volume,
		VolumeId: "volume-0.vhd",
		AttachmentParams: storage.AttachmentParams{
			Machine:    machine,
			InstanceId: inst.Id(),
		},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.ErrorMatches, "attaching volumes not supported")
}

func (s *azureVolumeSuite) TestAttachVolumesGetRoleError(c *gc.C) {
	vs := s.volumeSource(c, nil)

	machine := names.NewMachineTag("0")
	volume := names.NewVolumeTag("0")

	env := makeEnviron(c)
	prefix := env.getEnvPrefix()
	service := makeLegacyDeployment(env, prefix+"service")
	inst, err := env.getInstance(service, "")
	c.Assert(err, jc.ErrorIsNil)

	results, err := vs.AttachVolumes([]storage.VolumeAttachmentParams{{
		Volume:   volume,
		VolumeId: "volume-0.vhd",
		AttachmentParams: storage.AttachmentParams{
			Machine:    machine,
			InstanceId: inst.Id(),
		},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.ErrorMatches, "attaching disks to legacy instances not supported")
}

func (s *azureVolumeSuite) TestDetachVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	_, err := vs.DetachVolumes([]storage.VolumeAttachmentParams{{}})
	c.Assert(err, gc.ErrorMatches, "detaching volumes not supported")
}
