// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

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

func (s *provisionerSuite) TestWatchApplications(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchApplications")
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	watcher, err := st.WatchApplications()
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *provisionerSuite) TestWatchVolumes(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchVolumes")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-123"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.WatchVolumes(names.NewMachineTag("123"))
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
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-123"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.WatchFilesystems(names.NewMachineTag("123"))
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
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-123"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.MachineStorageIdsWatchResults{})
		*(result.(*params.MachineStorageIdsWatchResults)) = params.MachineStorageIdsWatchResults{
			Results: []params.MachineStorageIdsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.WatchVolumeAttachments(names.NewMachineTag("123"))
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *provisionerSuite) TestWatchVolumeAttachmentPlans(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchVolumeAttachmentPlans")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-123"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.MachineStorageIdsWatchResults{})
		*(result.(*params.MachineStorageIdsWatchResults)) = params.MachineStorageIdsWatchResults{
			Results: []params.MachineStorageIdsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.WatchVolumeAttachmentPlans(names.NewMachineTag("123"))
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
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-123"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.MachineStorageIdsWatchResults{})
		*(result.(*params.MachineStorageIdsWatchResults)) = params.MachineStorageIdsWatchResults{
			Results: []params.MachineStorageIdsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.WatchFilesystemAttachments(names.NewMachineTag("123"))
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.WatchBlockDevices(names.NewMachineTag("123"))
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	volumes, err := st.VolumeAttachments([]params.MachineStorageId{{
		MachineTag: "machine-100", AttachmentTag: "volume-100",
	}})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(volumes, jc.DeepEquals, volumeAttachmentResults)
}

func (s *provisionerSuite) TestVolumeAttachmentPlans(c *gc.C) {
	volumeAttachmentPlanResults := []params.VolumeAttachmentPlanResult{{
		Result: params.VolumeAttachmentPlan{
			MachineTag: "machine-100",
			VolumeTag:  "volume-100",
			PlanInfo: params.VolumeAttachmentPlanInfo{
				DeviceType: storage.DeviceTypeISCSI,
				DeviceAttributes: map[string]string{
					"iqn":         "bogusIQN",
					"address":     "192.168.1.1",
					"port":        "9999",
					"chap-user":   "example",
					"chap-secret": "supersecretpassword",
				},
			},
			BlockDevice: storage.BlockDevice{
				DeviceName: "sda",
			},
		},
	}}

	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "VolumeAttachmentPlans")
		c.Check(arg, gc.DeepEquals, params.MachineStorageIds{
			Ids: []params.MachineStorageId{{
				MachineTag: "machine-100", AttachmentTag: "volume-100",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.VolumeAttachmentPlanResults{})
		*(result.(*params.VolumeAttachmentPlanResults)) = params.VolumeAttachmentPlanResults{
			Results: volumeAttachmentPlanResults,
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	volumes, err := st.VolumeAttachmentPlans([]params.MachineStorageId{{
		MachineTag: "machine-100", AttachmentTag: "volume-100",
	}})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(volumes, jc.DeepEquals, volumeAttachmentPlanResults)
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	volumeParams, err := st.VolumeParams([]names.VolumeTag{names.NewVolumeTag("100")})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(volumeParams, jc.DeepEquals, []params.VolumeParamsResult{{
		Result: params.VolumeParams{
			VolumeTag: "volume-100", Size: 1024, Provider: "loop",
		},
	}})
}

func (s *provisionerSuite) TestRemoveVolumeParams(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RemoveVolumeParams")
		c.Check(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{"volume-100"}}})
		c.Assert(result, gc.FitsTypeOf, &params.RemoveVolumeParamsResults{})
		*(result.(*params.RemoveVolumeParamsResults)) = params.RemoveVolumeParamsResults{
			Results: []params.RemoveVolumeParamsResult{{
				Result: params.RemoveVolumeParams{
					Provider: "foo",
					VolumeId: "bar",
					Destroy:  true,
				},
			}},
		}
		return nil
	})

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	volumeParams, err := st.RemoveVolumeParams([]names.VolumeTag{names.NewVolumeTag("100")})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(volumeParams, jc.DeepEquals, []params.RemoveVolumeParamsResult{{
		Result: params.RemoveVolumeParams{
			Provider: "foo",
			VolumeId: "bar",
			Destroy:  true,
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	filesystemParams, err := st.FilesystemParams([]names.FilesystemTag{names.NewFilesystemTag("100")})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(filesystemParams, jc.DeepEquals, []params.FilesystemParamsResult{{
		Result: params.FilesystemParams{
			FilesystemTag: "filesystem-100", Size: 1024, Provider: "loop",
		},
	}})
}

func (s *provisionerSuite) TestRemoveFilesystemParams(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RemoveFilesystemParams")
		c.Check(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{"filesystem-100"}}})
		c.Assert(result, gc.FitsTypeOf, &params.RemoveFilesystemParamsResults{})
		*(result.(*params.RemoveFilesystemParamsResults)) = params.RemoveFilesystemParamsResults{
			Results: []params.RemoveFilesystemParamsResult{{
				Result: params.RemoveFilesystemParams{
					Provider:     "foo",
					FilesystemId: "bar",
					Destroy:      true,
				},
			}},
		}
		return nil
	})

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	filesystemParams, err := st.RemoveFilesystemParams([]names.FilesystemTag{names.NewFilesystemTag("100")})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(filesystemParams, jc.DeepEquals, []params.RemoveFilesystemParamsResult{{
		Result: params.RemoveFilesystemParams{
			Provider:     "foo",
			FilesystemId: "bar",
			Destroy:      true,
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
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

func (s *provisionerSuite) TestCreateVolumeAttachmentPlan(c *gc.C) {
	var callCount int

	attachmentPlan := []params.VolumeAttachmentPlan{
		{
			MachineTag: "machine-100",
			VolumeTag:  "volume-100",
			PlanInfo: params.VolumeAttachmentPlanInfo{
				DeviceType: storage.DeviceTypeISCSI,
				DeviceAttributes: map[string]string{
					"iqn":         "bogusIQN",
					"address":     "192.168.1.1",
					"port":        "9999",
					"chap-user":   "example",
					"chap-secret": "supersecretpassword",
				},
			},
			BlockDevice: storage.BlockDevice{
				DeviceName: "sda",
			},
		},
	}

	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "CreateVolumeAttachmentPlans")
		c.Check(arg, gc.DeepEquals, params.VolumeAttachmentPlans{
			VolumeAttachmentPlans: []params.VolumeAttachmentPlan{
				{
					MachineTag: "machine-100",
					VolumeTag:  "volume-100",
					PlanInfo: params.VolumeAttachmentPlanInfo{
						DeviceType: storage.DeviceTypeISCSI,
						DeviceAttributes: map[string]string{
							"iqn":         "bogusIQN",
							"address":     "192.168.1.1",
							"port":        "9999",
							"chap-user":   "example",
							"chap-secret": "supersecretpassword",
						},
					},
					BlockDevice: storage.BlockDevice{
						DeviceName: "sda",
					},
				},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	errorResults, err := st.CreateVolumeAttachmentPlans(attachmentPlan)
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(errorResults, gc.HasLen, 1)
	c.Assert(errorResults[0].Error, gc.IsNil)
}

func (s *provisionerSuite) TestSetVolumeAttachmentPlanBlockInfo(c *gc.C) {
	var callCount int

	attachmentPlan := []params.VolumeAttachmentPlan{
		{
			MachineTag: "machine-100",
			VolumeTag:  "volume-100",
			PlanInfo: params.VolumeAttachmentPlanInfo{
				DeviceType: storage.DeviceTypeISCSI,
				DeviceAttributes: map[string]string{
					"iqn":         "bogusIQN",
					"address":     "192.168.1.1",
					"port":        "9999",
					"chap-user":   "example",
					"chap-secret": "supersecretpassword",
				},
			},
			BlockDevice: storage.BlockDevice{
				DeviceName: "sda",
			},
		},
	}

	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetVolumeAttachmentPlanBlockInfo")
		c.Check(arg, gc.DeepEquals, params.VolumeAttachmentPlans{
			VolumeAttachmentPlans: []params.VolumeAttachmentPlan{
				{
					MachineTag: "machine-100",
					VolumeTag:  "volume-100",
					PlanInfo: params.VolumeAttachmentPlanInfo{
						DeviceType: storage.DeviceTypeISCSI,
						DeviceAttributes: map[string]string{
							"iqn":         "bogusIQN",
							"address":     "192.168.1.1",
							"port":        "9999",
							"chap-user":   "example",
							"chap-secret": "supersecretpassword",
						},
					},
					BlockDevice: storage.BlockDevice{
						DeviceName: "sda",
					},
				},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	errorResults, err := st.SetVolumeAttachmentPlanBlockInfo(attachmentPlan)
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(errorResults, gc.HasLen, 1)
	c.Assert(errorResults[0].Error, gc.IsNil)
}

func (s *provisionerSuite) TestRemoveVolumeAttachmentPlan(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RemoveVolumeAttachmentPlan")
		c.Check(arg, gc.DeepEquals, params.MachineStorageIds{
			Ids: []params.MachineStorageId{{
				MachineTag: "machine-100", AttachmentTag: "volume-100",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	errorResults, err := st.RemoveVolumeAttachmentPlan([]params.MachineStorageId{{
		MachineTag: "machine-100", AttachmentTag: "volume-100",
	}})
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
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

	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
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
	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	err = apiCall(st)
	c.Check(err, gc.ErrorMatches, "blargh")
}

func (s *provisionerSuite) TestWatchVolumesClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.WatchVolumes(names.NewMachineTag("123"))
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

func (s *provisionerSuite) TestRemoveVolumeParamsClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.RemoveVolumeParams(nil)
		return err
	})
}

func (s *provisionerSuite) TestFilesystemParamsClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.FilesystemParams(nil)
		return err
	})
}

func (s *provisionerSuite) TestRemoveFilesystemParamsClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.RemoveFilesystemParams(nil)
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
	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.WatchVolumes(names.NewMachineTag("123"))
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
	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
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
	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	results, err := st.VolumeParams([]names.VolumeTag{names.NewVolumeTag("100")})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestRemoveVolumeParamsServerError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.RemoveVolumeParamsResults)) = params.RemoveVolumeParamsResults{
			Results: []params.RemoveVolumeParamsResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	results, err := st.RemoveVolumeParams([]names.VolumeTag{names.NewVolumeTag("100")})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestFilesystemParamsServerError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.FilesystemParamsResults)) = params.FilesystemParamsResults{
			Results: []params.FilesystemParamsResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	results, err := st.FilesystemParams([]names.FilesystemTag{names.NewFilesystemTag("100")})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestRemoveFilesystemParamsServerError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.RemoveFilesystemParamsResults)) = params.RemoveFilesystemParamsResults{
			Results: []params.RemoveFilesystemParamsResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	results, err := st.RemoveFilesystemParams([]names.FilesystemTag{names.NewFilesystemTag("100")})
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
	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
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
	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
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
	st, err := storageprovisioner.NewState(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	tags := []names.Tag{
		names.NewVolumeTag("100"),
	}
	results, err := st.Life(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "MSG")
}
