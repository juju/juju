// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"errors"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/storageprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&provisionerSuite{})

type provisionerSuite struct {
	coretesting.BaseSuite
}

func (s *provisionerSuite) TestNewState(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	c.Assert(st, gc.NotNil)
	st = storageprovisioner.NewState(apiCaller, names.NewEnvironTag("87927ace-9e41-4fd5-8103-1a6fb5ff7eb4"))
	c.Assert(st, gc.NotNil)
	c.Assert(func() {
		storageprovisioner.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	}, gc.PanicMatches, "expected EnvironTag or MachineTag, got names.UnitTag")
}

func (s *provisionerSuite) TestWatchVolumes(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchVolumes")
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	_, err := st.WatchVolumes()
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *provisionerSuite) TestWatchFilesystems(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchFilesystems")
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	_, err := st.WatchFilesystems()
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *provisionerSuite) TestWatchVolumeAttachments(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchVolumeAttachments")
		c.Assert(result, gc.FitsTypeOf, &params.MachineStorageIdsWatchResults{})
		*(result.(*params.MachineStorageIdsWatchResults)) = params.MachineStorageIdsWatchResults{
			Results: []params.MachineStorageIdsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	_, err := st.WatchVolumeAttachments()
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *provisionerSuite) TestWatchFilesystemAttachments(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchFilesystemAttachments")
		c.Assert(result, gc.FitsTypeOf, &params.MachineStorageIdsWatchResults{})
		*(result.(*params.MachineStorageIdsWatchResults)) = params.MachineStorageIdsWatchResults{
			Results: []params.MachineStorageIdsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	_, err := st.WatchFilesystemAttachments()
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *provisionerSuite) TestWatchBlockDevices(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchBlockDevices")
		c.Assert(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{"machine-123"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	_, err := st.WatchBlockDevices(names.NewMachineTag("123"))
	c.Check(err, gc.ErrorMatches, "FAIL")
}

func (s *provisionerSuite) TestVolumes(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Volumes")
		c.Check(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{"volume-100"}}})
		c.Assert(result, gc.FitsTypeOf, &params.VolumeResults{})
		*(result.(*params.VolumeResults)) = params.VolumeResults{
			Results: []params.VolumeResult{{
				Result: params.Volume{
					VolumeTag: "volume-100",
					Info: params.VolumeInfo{
						VolumeId:   "volume-id",
						HardwareId: "abc",
						Size:       1024,
					},
				},
			}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	volumes, err := st.Volumes([]names.VolumeTag{names.NewVolumeTag("100")})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(volumes, jc.DeepEquals, []params.VolumeResult{{
		Result: params.Volume{
			VolumeTag: "volume-100",
			Info: params.VolumeInfo{
				VolumeId:   "volume-id",
				HardwareId: "abc",
				Size:       1024,
			},
		},
	}})
}

func (s *provisionerSuite) TestFilesystems(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Filesystems")
		c.Check(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{"filesystem-100"}}})
		c.Assert(result, gc.FitsTypeOf, &params.FilesystemResults{})
		*(result.(*params.FilesystemResults)) = params.FilesystemResults{
			Results: []params.FilesystemResult{{
				Result: params.Filesystem{
					FilesystemTag: "filesystem-100",
					Info: params.FilesystemInfo{
						FilesystemId: "filesystem-id",
						Size:         1024,
					},
				},
			}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	filesystems, err := st.Filesystems([]names.FilesystemTag{names.NewFilesystemTag("100")})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(filesystems, jc.DeepEquals, []params.FilesystemResult{{
		Result: params.Filesystem{
			FilesystemTag: "filesystem-100",
			Info: params.FilesystemInfo{
				FilesystemId: "filesystem-id",
				Size:         1024,
			},
		},
	}})
}

func (s *provisionerSuite) TestVolumeAttachments(c *gc.C) {
	volumeAttachmentResults := []params.VolumeAttachmentResult{{
		Result: params.VolumeAttachment{
			MachineTag: "machine-100",
			VolumeTag:  "volume-100",
			Info: params.VolumeAttachmentInfo{
				DeviceName: "xvdf1",
			},
		},
	}}

	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "VolumeAttachments")
		c.Check(arg, gc.DeepEquals, params.MachineStorageIds{
			Ids: []params.MachineStorageId{{
				MachineTag: "machine-100", AttachmentTag: "volume-100",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.VolumeAttachmentResults{})
		*(result.(*params.VolumeAttachmentResults)) = params.VolumeAttachmentResults{
			Results: volumeAttachmentResults,
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	volumes, err := st.VolumeAttachments([]params.MachineStorageId{{
		MachineTag: "machine-100", AttachmentTag: "volume-100",
	}})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(volumes, jc.DeepEquals, volumeAttachmentResults)
}

func (s *provisionerSuite) TestVolumeBlockDevices(c *gc.C) {
	blockDeviceResults := []params.BlockDeviceResult{{
		Result: storage.BlockDevice{
			DeviceName: "xvdf1",
			HardwareId: "kjlaksjdlasjdklasd123123",
			Size:       1024,
		},
	}}

	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "VolumeBlockDevices")
		c.Check(arg, gc.DeepEquals, params.MachineStorageIds{
			Ids: []params.MachineStorageId{{
				MachineTag: "machine-100", AttachmentTag: "volume-100",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.BlockDeviceResults{})
		*(result.(*params.BlockDeviceResults)) = params.BlockDeviceResults{
			Results: blockDeviceResults,
		}
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	volumes, err := st.VolumeBlockDevices([]params.MachineStorageId{{
		MachineTag: "machine-100", AttachmentTag: "volume-100",
	}})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(volumes, jc.DeepEquals, blockDeviceResults)
}

func (s *provisionerSuite) TestFilesystemAttachments(c *gc.C) {
	filesystemAttachmentResults := []params.FilesystemAttachmentResult{{
		Result: params.FilesystemAttachment{
			MachineTag:    "machine-100",
			FilesystemTag: "filesystem-100",
			Info: params.FilesystemAttachmentInfo{
				MountPoint: "/srv",
			},
		},
	}}

	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "FilesystemAttachments")
		c.Check(arg, gc.DeepEquals, params.MachineStorageIds{
			Ids: []params.MachineStorageId{{
				MachineTag: "machine-100", AttachmentTag: "filesystem-100",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.FilesystemAttachmentResults{})
		*(result.(*params.FilesystemAttachmentResults)) = params.FilesystemAttachmentResults{
			Results: filesystemAttachmentResults,
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	filesystems, err := st.FilesystemAttachments([]params.MachineStorageId{{
		MachineTag: "machine-100", AttachmentTag: "filesystem-100",
	}})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(filesystems, jc.DeepEquals, filesystemAttachmentResults)
}

func (s *provisionerSuite) TestVolumeParams(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "VolumeParams")
		c.Check(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{"volume-100"}}})
		c.Assert(result, gc.FitsTypeOf, &params.VolumeParamsResults{})
		*(result.(*params.VolumeParamsResults)) = params.VolumeParamsResults{
			Results: []params.VolumeParamsResult{{
				Result: params.VolumeParams{
					VolumeTag: "volume-100",
					Size:      1024,
					Provider:  "loop",
				},
			}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	volumeParams, err := st.VolumeParams([]names.VolumeTag{names.NewVolumeTag("100")})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(volumeParams, jc.DeepEquals, []params.VolumeParamsResult{{
		Result: params.VolumeParams{
			VolumeTag: "volume-100", Size: 1024, Provider: "loop",
		},
	}})
}

func (s *provisionerSuite) TestFilesystemParams(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "FilesystemParams")
		c.Check(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{"filesystem-100"}}})
		c.Assert(result, gc.FitsTypeOf, &params.FilesystemParamsResults{})
		*(result.(*params.FilesystemParamsResults)) = params.FilesystemParamsResults{
			Results: []params.FilesystemParamsResult{{
				Result: params.FilesystemParams{
					FilesystemTag: "filesystem-100",
					Size:          1024,
					Provider:      "loop",
				},
			}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	filesystemParams, err := st.FilesystemParams([]names.FilesystemTag{names.NewFilesystemTag("100")})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(filesystemParams, jc.DeepEquals, []params.FilesystemParamsResult{{
		Result: params.FilesystemParams{
			FilesystemTag: "filesystem-100", Size: 1024, Provider: "loop",
		},
	}})
}

func (s *provisionerSuite) TestVolumeAttachmentParams(c *gc.C) {
	paramsResults := []params.VolumeAttachmentParamsResult{{
		Result: params.VolumeAttachmentParams{
			MachineTag: "machine-100",
			VolumeTag:  "volume-100",
			InstanceId: "inst-ance",
			Provider:   "loop",
		},
	}}

	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "VolumeAttachmentParams")
		c.Check(arg, gc.DeepEquals, params.MachineStorageIds{
			Ids: []params.MachineStorageId{{
				MachineTag: "machine-100", AttachmentTag: "volume-100",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.VolumeAttachmentParamsResults{})
		*(result.(*params.VolumeAttachmentParamsResults)) = params.VolumeAttachmentParamsResults{
			Results: paramsResults,
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	volumeParams, err := st.VolumeAttachmentParams([]params.MachineStorageId{{
		MachineTag: "machine-100", AttachmentTag: "volume-100",
	}})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(volumeParams, jc.DeepEquals, paramsResults)
}

func (s *provisionerSuite) TestFilesystemAttachmentParams(c *gc.C) {
	paramsResults := []params.FilesystemAttachmentParamsResult{{
		Result: params.FilesystemAttachmentParams{
			MachineTag:    "machine-100",
			FilesystemTag: "filesystem-100",
			InstanceId:    "inst-ance",
			Provider:      "loop",
			MountPoint:    "/srv",
		},
	}}

	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "FilesystemAttachmentParams")
		c.Check(arg, gc.DeepEquals, params.MachineStorageIds{
			Ids: []params.MachineStorageId{{
				MachineTag: "machine-100", AttachmentTag: "filesystem-100",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.FilesystemAttachmentParamsResults{})
		*(result.(*params.FilesystemAttachmentParamsResults)) = params.FilesystemAttachmentParamsResults{
			Results: paramsResults,
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	filesystemParams, err := st.FilesystemAttachmentParams([]params.MachineStorageId{{
		MachineTag: "machine-100", AttachmentTag: "filesystem-100",
	}})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(filesystemParams, jc.DeepEquals, paramsResults)
}

func (s *provisionerSuite) TestSetVolumeInfo(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetVolumeInfo")
		c.Check(arg, gc.DeepEquals, params.Volumes{
			Volumes: []params.Volume{{
				VolumeTag: "volume-100",
				Info: params.VolumeInfo{
					VolumeId:   "123",
					HardwareId: "abc",
					Size:       1024,
					Persistent: true,
				},
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	volumes := []params.Volume{{
		VolumeTag: "volume-100",
		Info: params.VolumeInfo{
			VolumeId: "123", HardwareId: "abc", Size: 1024, Persistent: true,
		},
	}}
	errorResults, err := st.SetVolumeInfo(volumes)
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(errorResults, gc.HasLen, 1)
	c.Assert(errorResults[0].Error, gc.IsNil)
}

func (s *provisionerSuite) TestSetFilesystemInfo(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetFilesystemInfo")
		c.Check(arg, gc.DeepEquals, params.Filesystems{
			Filesystems: []params.Filesystem{{
				FilesystemTag: "filesystem-100",
				Info: params.FilesystemInfo{
					FilesystemId: "123",
					Size:         1024,
				},
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	filesystems := []params.Filesystem{{
		FilesystemTag: "filesystem-100",
		Info: params.FilesystemInfo{
			FilesystemId: "123",
			Size:         1024,
		},
	}}
	errorResults, err := st.SetFilesystemInfo(filesystems)
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(errorResults, gc.HasLen, 1)
	c.Assert(errorResults[0].Error, gc.IsNil)
}

func (s *provisionerSuite) TestSetVolumeAttachmentInfo(c *gc.C) {
	volumeAttachments := []params.VolumeAttachment{{
		VolumeTag:  "volume-100",
		MachineTag: "machine-200",
		Info: params.VolumeAttachmentInfo{
			DeviceName: "xvdf1",
		},
	}}

	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetVolumeAttachmentInfo")
		c.Check(arg, jc.DeepEquals, params.VolumeAttachments{volumeAttachments})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	errorResults, err := st.SetVolumeAttachmentInfo(volumeAttachments)
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(errorResults, gc.HasLen, 1)
	c.Assert(errorResults[0].Error, gc.IsNil)
}

func (s *provisionerSuite) TestSetFilesystemAttachmentInfo(c *gc.C) {
	filesystemAttachments := []params.FilesystemAttachment{{
		FilesystemTag: "filesystem-100",
		MachineTag:    "machine-200",
		Info: params.FilesystemAttachmentInfo{
			MountPoint: "/srv",
		},
	}}

	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetFilesystemAttachmentInfo")
		c.Check(arg, jc.DeepEquals, params.FilesystemAttachments{filesystemAttachments})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	errorResults, err := st.SetFilesystemAttachmentInfo(filesystemAttachments)
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(errorResults, gc.HasLen, 1)
	c.Assert(errorResults[0].Error, gc.IsNil)
}

func (s *provisionerSuite) testOpWithTags(
	c *gc.C, opName string, apiCall func(*storageprovisioner.State, []names.Tag) ([]params.ErrorResult, error),
) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, opName)
		c.Check(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "volume-100"}}})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	volumes := []names.Tag{names.NewVolumeTag("100")}
	errorResults, err := apiCall(st, volumes)
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(errorResults, jc.DeepEquals, []params.ErrorResult{{}})
}

func (s *provisionerSuite) TestRemove(c *gc.C) {
	s.testOpWithTags(c, "Remove", func(st *storageprovisioner.State, tags []names.Tag) ([]params.ErrorResult, error) {
		return st.Remove(tags)
	})
}

func (s *provisionerSuite) TestEnsureDead(c *gc.C) {
	s.testOpWithTags(c, "EnsureDead", func(st *storageprovisioner.State, tags []names.Tag) ([]params.ErrorResult, error) {
		return st.EnsureDead(tags)
	})
}

func (s *provisionerSuite) TestLife(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Life")
		c.Check(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "volume-100"}}})
		c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Life: params.Alive}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	volumes := []names.Tag{names.NewVolumeTag("100")}
	lifeResults, err := st.Life(volumes)
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(lifeResults, jc.DeepEquals, []params.LifeResult{{Life: params.Alive}})
}

func (s *provisionerSuite) testClientError(c *gc.C, apiCall func(*storageprovisioner.State) error) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("blargh")
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	err := apiCall(st)
	c.Check(err, gc.ErrorMatches, "blargh")
}

func (s *provisionerSuite) TestWatchVolumesClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.WatchVolumes()
		return err
	})
}

func (s *provisionerSuite) TestVolumesClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.Volumes(nil)
		return err
	})
}

func (s *provisionerSuite) TestVolumeParamsClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.VolumeParams(nil)
		return err
	})
}

func (s *provisionerSuite) TestRemoveClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.Remove(nil)
		return err
	})
}

func (s *provisionerSuite) TestRemoveAttachmentsClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.RemoveAttachments(nil)
		return err
	})
}

func (s *provisionerSuite) TestSetVolumeInfoClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.SetVolumeInfo(nil)
		return err
	})
}

func (s *provisionerSuite) TestEnsureDeadClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.EnsureDead(nil)
		return err
	})
}

func (s *provisionerSuite) TestLifeClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.Life(nil)
		return err
	})
}

func (s *provisionerSuite) TestAttachmentLifeClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.AttachmentLife(nil)
		return err
	})
}

func (s *provisionerSuite) TestWatchVolumesServerError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	_, err := st.WatchVolumes()
	c.Check(err, gc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestVolumesServerError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.VolumeResults)) = params.VolumeResults{
			Results: []params.VolumeResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	results, err := st.Volumes([]names.VolumeTag{names.NewVolumeTag("100")})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestVolumeParamsServerError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.VolumeParamsResults)) = params.VolumeParamsResults{
			Results: []params.VolumeParamsResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	results, err := st.VolumeParams([]names.VolumeTag{names.NewVolumeTag("100")})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestSetVolumeInfoServerError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	results, err := st.SetVolumeInfo([]params.Volume{{
		VolumeTag: names.NewVolumeTag("100").String(),
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) testServerError(c *gc.C, apiCall func(*storageprovisioner.State, []names.Tag) ([]params.ErrorResult, error)) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	tags := []names.Tag{
		names.NewVolumeTag("100"),
	}
	results, err := apiCall(st, tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestRemoveServerError(c *gc.C) {
	s.testServerError(c, func(st *storageprovisioner.State, tags []names.Tag) ([]params.ErrorResult, error) {
		return st.Remove(tags)
	})
}

func (s *provisionerSuite) TestEnsureDeadServerError(c *gc.C) {
	s.testServerError(c, func(st *storageprovisioner.State, tags []names.Tag) ([]params.ErrorResult, error) {
		return st.EnsureDead(tags)
	})
}

func (s *provisionerSuite) TestLifeServerError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	tags := []names.Tag{
		names.NewVolumeTag("100"),
	}
	results, err := st.Life(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestWatchForEnvironConfigChanges(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchForEnvironConfigChanges")
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResult{})
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			NotifyWatcherId: "abc",
		}
		return errors.New("FAIL")
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	_, err := st.WatchForEnvironConfigChanges()
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *provisionerSuite) TestEnvironConfig(c *gc.C) {
	inputCfg := coretesting.EnvironConfig(c)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "EnvironConfig")
		c.Assert(result, gc.FitsTypeOf, &params.EnvironConfigResult{})
		*(result.(*params.EnvironConfigResult)) = params.EnvironConfigResult{
			Config: inputCfg.AllAttrs(),
		}
		return nil
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	outputCfg, err := st.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(outputCfg.AllAttrs(), jc.DeepEquals, inputCfg.AllAttrs())
}
