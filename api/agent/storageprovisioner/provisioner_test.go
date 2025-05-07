// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"context"
	"errors"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/storageprovisioner"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&provisionerSuite{})

type provisionerSuite struct {
	coretesting.BaseSuite
}

func (s *provisionerSuite) TestWatchApplications(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchApplications")
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	watcher, err := st.WatchApplications(context.Background())
	c.Assert(watcher, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *provisionerSuite) TestWatchVolumes(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchVolumes")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-123"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = st.WatchVolumes(context.Background(), names.NewMachineTag("123"))
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *provisionerSuite) TestWatchFilesystems(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchFilesystems")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-123"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = st.WatchFilesystems(context.Background(), names.NewMachineTag("123"))
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *provisionerSuite) TestWatchVolumeAttachments(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchVolumeAttachments")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-123"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.MachineStorageIdsWatchResults{})
		*(result.(*params.MachineStorageIdsWatchResults)) = params.MachineStorageIdsWatchResults{
			Results: []params.MachineStorageIdsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = st.WatchVolumeAttachments(context.Background(), names.NewMachineTag("123"))
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *provisionerSuite) TestWatchVolumeAttachmentPlans(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchVolumeAttachmentPlans")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-123"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.MachineStorageIdsWatchResults{})
		*(result.(*params.MachineStorageIdsWatchResults)) = params.MachineStorageIdsWatchResults{
			Results: []params.MachineStorageIdsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = st.WatchVolumeAttachmentPlans(context.Background(), names.NewMachineTag("123"))
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *provisionerSuite) TestWatchFilesystemAttachments(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchFilesystemAttachments")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-123"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.MachineStorageIdsWatchResults{})
		*(result.(*params.MachineStorageIdsWatchResults)) = params.MachineStorageIdsWatchResults{
			Results: []params.MachineStorageIdsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = st.WatchFilesystemAttachments(context.Background(), names.NewMachineTag("123"))
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 1)
}

func (s *provisionerSuite) TestWatchBlockDevices(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchBlockDevices")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{"machine-123"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = st.WatchBlockDevices(context.Background(), names.NewMachineTag("123"))
	c.Check(err, tc.ErrorMatches, "FAIL")
}

func (s *provisionerSuite) TestVolumes(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Volumes")
		c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{"volume-100"}}})
		c.Assert(result, tc.FitsTypeOf, &params.VolumeResults{})
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

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	volumes, err := st.Volumes(context.Background(), []names.VolumeTag{names.NewVolumeTag("100")})
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(volumes, tc.DeepEquals, []params.VolumeResult{{
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

func (s *provisionerSuite) TestFilesystems(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Filesystems")
		c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{"filesystem-100"}}})
		c.Assert(result, tc.FitsTypeOf, &params.FilesystemResults{})
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

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	filesystems, err := st.Filesystems(context.Background(), []names.FilesystemTag{names.NewFilesystemTag("100")})
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(filesystems, tc.DeepEquals, []params.FilesystemResult{{
		Result: params.Filesystem{
			FilesystemTag: "filesystem-100",
			Info: params.FilesystemInfo{
				FilesystemId: "filesystem-id",
				Size:         1024,
			},
		},
	}})
}

func (s *provisionerSuite) TestVolumeAttachments(c *tc.C) {
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
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "VolumeAttachments")
		c.Check(arg, tc.DeepEquals, params.MachineStorageIds{
			Ids: []params.MachineStorageId{{
				MachineTag: "machine-100", AttachmentTag: "volume-100",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.VolumeAttachmentResults{})
		*(result.(*params.VolumeAttachmentResults)) = params.VolumeAttachmentResults{
			Results: volumeAttachmentResults,
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	volumes, err := st.VolumeAttachments(context.Background(), []params.MachineStorageId{{
		MachineTag: "machine-100", AttachmentTag: "volume-100",
	}})
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(volumes, tc.DeepEquals, volumeAttachmentResults)
}

func (s *provisionerSuite) TestVolumeAttachmentPlans(c *tc.C) {
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
			BlockDevice: params.BlockDevice{
				DeviceName: "sda",
			},
		},
	}}

	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "VolumeAttachmentPlans")
		c.Check(arg, tc.DeepEquals, params.MachineStorageIds{
			Ids: []params.MachineStorageId{{
				MachineTag: "machine-100", AttachmentTag: "volume-100",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.VolumeAttachmentPlanResults{})
		*(result.(*params.VolumeAttachmentPlanResults)) = params.VolumeAttachmentPlanResults{
			Results: volumeAttachmentPlanResults,
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	volumes, err := st.VolumeAttachmentPlans(context.Background(), []params.MachineStorageId{{
		MachineTag: "machine-100", AttachmentTag: "volume-100",
	}})
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(volumes, tc.DeepEquals, volumeAttachmentPlanResults)
}

func (s *provisionerSuite) TestVolumeBlockDevices(c *tc.C) {
	blockDeviceResults := []params.BlockDeviceResult{{
		Result: params.BlockDevice{
			DeviceName: "xvdf1",
			HardwareId: "kjlaksjdlasjdklasd123123",
			Size:       1024,
		},
	}}

	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "VolumeBlockDevices")
		c.Check(arg, tc.DeepEquals, params.MachineStorageIds{
			Ids: []params.MachineStorageId{{
				MachineTag: "machine-100", AttachmentTag: "volume-100",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.BlockDeviceResults{})
		*(result.(*params.BlockDeviceResults)) = params.BlockDeviceResults{
			Results: blockDeviceResults,
		}
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	volumes, err := st.VolumeBlockDevices(context.Background(), []params.MachineStorageId{{
		MachineTag: "machine-100", AttachmentTag: "volume-100",
	}})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(volumes, tc.DeepEquals, blockDeviceResults)
}

func (s *provisionerSuite) TestFilesystemAttachments(c *tc.C) {
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
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "FilesystemAttachments")
		c.Check(arg, tc.DeepEquals, params.MachineStorageIds{
			Ids: []params.MachineStorageId{{
				MachineTag: "machine-100", AttachmentTag: "filesystem-100",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.FilesystemAttachmentResults{})
		*(result.(*params.FilesystemAttachmentResults)) = params.FilesystemAttachmentResults{
			Results: filesystemAttachmentResults,
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	filesystems, err := st.FilesystemAttachments(context.Background(), []params.MachineStorageId{{
		MachineTag: "machine-100", AttachmentTag: "filesystem-100",
	}})
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(filesystems, tc.DeepEquals, filesystemAttachmentResults)
}

func (s *provisionerSuite) TestVolumeParams(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "VolumeParams")
		c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{"volume-100"}}})
		c.Assert(result, tc.FitsTypeOf, &params.VolumeParamsResults{})
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

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	volumeParams, err := st.VolumeParams(context.Background(), []names.VolumeTag{names.NewVolumeTag("100")})
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(volumeParams, tc.DeepEquals, []params.VolumeParamsResult{{
		Result: params.VolumeParams{
			VolumeTag: "volume-100", Size: 1024, Provider: "loop",
		},
	}})
}

func (s *provisionerSuite) TestRemoveVolumeParams(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "RemoveVolumeParams")
		c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{"volume-100"}}})
		c.Assert(result, tc.FitsTypeOf, &params.RemoveVolumeParamsResults{})
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

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	volumeParams, err := st.RemoveVolumeParams(context.Background(), []names.VolumeTag{names.NewVolumeTag("100")})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(volumeParams, tc.DeepEquals, []params.RemoveVolumeParamsResult{{
		Result: params.RemoveVolumeParams{
			Provider: "foo",
			VolumeId: "bar",
			Destroy:  true,
		},
	}})
}

func (s *provisionerSuite) TestFilesystemParams(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "FilesystemParams")
		c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{"filesystem-100"}}})
		c.Assert(result, tc.FitsTypeOf, &params.FilesystemParamsResults{})
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

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	filesystemParams, err := st.FilesystemParams(context.Background(), []names.FilesystemTag{names.NewFilesystemTag("100")})
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(filesystemParams, tc.DeepEquals, []params.FilesystemParamsResult{{
		Result: params.FilesystemParams{
			FilesystemTag: "filesystem-100", Size: 1024, Provider: "loop",
		},
	}})
}

func (s *provisionerSuite) TestRemoveFilesystemParams(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "RemoveFilesystemParams")
		c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{"filesystem-100"}}})
		c.Assert(result, tc.FitsTypeOf, &params.RemoveFilesystemParamsResults{})
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

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	filesystemParams, err := st.RemoveFilesystemParams(context.Background(), []names.FilesystemTag{names.NewFilesystemTag("100")})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(filesystemParams, tc.DeepEquals, []params.RemoveFilesystemParamsResult{{
		Result: params.RemoveFilesystemParams{
			Provider:     "foo",
			FilesystemId: "bar",
			Destroy:      true,
		},
	}})
}

func (s *provisionerSuite) TestVolumeAttachmentParams(c *tc.C) {
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
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "VolumeAttachmentParams")
		c.Check(arg, tc.DeepEquals, params.MachineStorageIds{
			Ids: []params.MachineStorageId{{
				MachineTag: "machine-100", AttachmentTag: "volume-100",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.VolumeAttachmentParamsResults{})
		*(result.(*params.VolumeAttachmentParamsResults)) = params.VolumeAttachmentParamsResults{
			Results: paramsResults,
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	volumeParams, err := st.VolumeAttachmentParams(context.Background(), []params.MachineStorageId{{
		MachineTag: "machine-100", AttachmentTag: "volume-100",
	}})
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(volumeParams, tc.DeepEquals, paramsResults)
}

func (s *provisionerSuite) TestFilesystemAttachmentParams(c *tc.C) {
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
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "FilesystemAttachmentParams")
		c.Check(arg, tc.DeepEquals, params.MachineStorageIds{
			Ids: []params.MachineStorageId{{
				MachineTag: "machine-100", AttachmentTag: "filesystem-100",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.FilesystemAttachmentParamsResults{})
		*(result.(*params.FilesystemAttachmentParamsResults)) = params.FilesystemAttachmentParamsResults{
			Results: paramsResults,
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	filesystemParams, err := st.FilesystemAttachmentParams(context.Background(), []params.MachineStorageId{{
		MachineTag: "machine-100", AttachmentTag: "filesystem-100",
	}})
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(filesystemParams, tc.DeepEquals, paramsResults)
}

func (s *provisionerSuite) TestSetVolumeInfo(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetVolumeInfo")
		c.Check(arg, tc.DeepEquals, params.Volumes{
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
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	volumes := []params.Volume{{
		VolumeTag: "volume-100",
		Info: params.VolumeInfo{
			VolumeId: "123", HardwareId: "abc", Size: 1024, Persistent: true,
		},
	}}
	errorResults, err := st.SetVolumeInfo(context.Background(), volumes)
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(errorResults, tc.HasLen, 1)
	c.Assert(errorResults[0].Error, tc.IsNil)
}

func (s *provisionerSuite) TestCreateVolumeAttachmentPlan(c *tc.C) {
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
			BlockDevice: params.BlockDevice{
				DeviceName: "sda",
			},
		},
	}

	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "CreateVolumeAttachmentPlans")
		c.Check(arg, tc.DeepEquals, params.VolumeAttachmentPlans{
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
					BlockDevice: params.BlockDevice{
						DeviceName: "sda",
					},
				},
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	errorResults, err := st.CreateVolumeAttachmentPlans(context.Background(), attachmentPlan)
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(errorResults, tc.HasLen, 1)
	c.Assert(errorResults[0].Error, tc.IsNil)
}

func (s *provisionerSuite) TestSetVolumeAttachmentPlanBlockInfo(c *tc.C) {
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
			BlockDevice: params.BlockDevice{
				DeviceName: "sda",
			},
		},
	}

	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetVolumeAttachmentPlanBlockInfo")
		c.Check(arg, tc.DeepEquals, params.VolumeAttachmentPlans{
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
					BlockDevice: params.BlockDevice{
						DeviceName: "sda",
					},
				},
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	errorResults, err := st.SetVolumeAttachmentPlanBlockInfo(context.Background(), attachmentPlan)
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(errorResults, tc.HasLen, 1)
	c.Assert(errorResults[0].Error, tc.IsNil)
}

func (s *provisionerSuite) TestRemoveVolumeAttachmentPlan(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "RemoveVolumeAttachmentPlan")
		c.Check(arg, tc.DeepEquals, params.MachineStorageIds{
			Ids: []params.MachineStorageId{{
				MachineTag: "machine-100", AttachmentTag: "volume-100",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	errorResults, err := st.RemoveVolumeAttachmentPlan(context.Background(), []params.MachineStorageId{{
		MachineTag: "machine-100", AttachmentTag: "volume-100",
	}})
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(errorResults, tc.HasLen, 1)
	c.Assert(errorResults[0].Error, tc.IsNil)
}

func (s *provisionerSuite) TestSetFilesystemInfo(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetFilesystemInfo")
		c.Check(arg, tc.DeepEquals, params.Filesystems{
			Filesystems: []params.Filesystem{{
				FilesystemTag: "filesystem-100",
				Info: params.FilesystemInfo{
					FilesystemId: "123",
					Size:         1024,
				},
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	filesystems := []params.Filesystem{{
		FilesystemTag: "filesystem-100",
		Info: params.FilesystemInfo{
			FilesystemId: "123",
			Size:         1024,
		},
	}}
	errorResults, err := st.SetFilesystemInfo(context.Background(), filesystems)
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(errorResults, tc.HasLen, 1)
	c.Assert(errorResults[0].Error, tc.IsNil)
}

func (s *provisionerSuite) TestSetVolumeAttachmentInfo(c *tc.C) {
	volumeAttachments := []params.VolumeAttachment{{
		VolumeTag:  "volume-100",
		MachineTag: "machine-200",
		Info: params.VolumeAttachmentInfo{
			DeviceName: "xvdf1",
		},
	}}

	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetVolumeAttachmentInfo")
		c.Check(arg, tc.DeepEquals, params.VolumeAttachments{volumeAttachments})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	errorResults, err := st.SetVolumeAttachmentInfo(context.Background(), volumeAttachments)
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(errorResults, tc.HasLen, 1)
	c.Assert(errorResults[0].Error, tc.IsNil)
}

func (s *provisionerSuite) TestSetFilesystemAttachmentInfo(c *tc.C) {
	filesystemAttachments := []params.FilesystemAttachment{{
		FilesystemTag: "filesystem-100",
		MachineTag:    "machine-200",
		Info: params.FilesystemAttachmentInfo{
			MountPoint: "/srv",
		},
	}}

	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetFilesystemAttachmentInfo")
		c.Check(arg, tc.DeepEquals, params.FilesystemAttachments{filesystemAttachments})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	errorResults, err := st.SetFilesystemAttachmentInfo(context.Background(), filesystemAttachments)
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(errorResults, tc.HasLen, 1)
	c.Assert(errorResults[0].Error, tc.IsNil)
}

func (s *provisionerSuite) testOpWithTags(
	c *tc.C, opName string, apiCall func(*storageprovisioner.Client, []names.Tag) ([]params.ErrorResult, error),
) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, opName)
		c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "volume-100"}}})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	volumes := []names.Tag{names.NewVolumeTag("100")}
	errorResults, err := apiCall(st, volumes)
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(errorResults, tc.DeepEquals, []params.ErrorResult{{}})
}

func (s *provisionerSuite) TestRemove(c *tc.C) {
	s.testOpWithTags(c, "Remove", func(st *storageprovisioner.Client, tags []names.Tag) ([]params.ErrorResult, error) {
		return st.Remove(context.Background(), tags)
	})
}

func (s *provisionerSuite) TestEnsureDead(c *tc.C) {
	s.testOpWithTags(c, "EnsureDead", func(st *storageprovisioner.Client, tags []names.Tag) ([]params.ErrorResult, error) {
		return st.EnsureDead(context.Background(), tags)
	})
}

func (s *provisionerSuite) TestLife(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "StorageProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Life")
		c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "volume-100"}}})
		c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Life: life.Alive}},
		}
		callCount++
		return nil
	})

	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	volumes := []names.Tag{names.NewVolumeTag("100")}
	lifeResults, err := st.Life(context.Background(), volumes)
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
	c.Assert(lifeResults, tc.DeepEquals, []params.LifeResult{{Life: life.Alive}})
}

func (s *provisionerSuite) testClientError(c *tc.C, apiCall func(*storageprovisioner.Client) error) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("blargh")
	})
	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	err = apiCall(st)
	c.Check(err, tc.ErrorMatches, "blargh")
}

func (s *provisionerSuite) TestWatchVolumesClientError(c *tc.C) {
	s.testClientError(c, func(st *storageprovisioner.Client) error {
		_, err := st.WatchVolumes(context.Background(), names.NewMachineTag("123"))
		return err
	})
}

func (s *provisionerSuite) TestVolumesClientError(c *tc.C) {
	s.testClientError(c, func(st *storageprovisioner.Client) error {
		_, err := st.Volumes(context.Background(), nil)
		return err
	})
}

func (s *provisionerSuite) TestVolumeParamsClientError(c *tc.C) {
	s.testClientError(c, func(st *storageprovisioner.Client) error {
		_, err := st.VolumeParams(context.Background(), nil)
		return err
	})
}

func (s *provisionerSuite) TestRemoveVolumeParamsClientError(c *tc.C) {
	s.testClientError(c, func(st *storageprovisioner.Client) error {
		_, err := st.RemoveVolumeParams(context.Background(), nil)
		return err
	})
}

func (s *provisionerSuite) TestFilesystemParamsClientError(c *tc.C) {
	s.testClientError(c, func(st *storageprovisioner.Client) error {
		_, err := st.FilesystemParams(context.Background(), nil)
		return err
	})
}

func (s *provisionerSuite) TestRemoveFilesystemParamsClientError(c *tc.C) {
	s.testClientError(c, func(st *storageprovisioner.Client) error {
		_, err := st.RemoveFilesystemParams(context.Background(), nil)
		return err
	})
}

func (s *provisionerSuite) TestRemoveClientError(c *tc.C) {
	s.testClientError(c, func(st *storageprovisioner.Client) error {
		_, err := st.Remove(context.Background(), nil)
		return err
	})
}

func (s *provisionerSuite) TestRemoveAttachmentsClientError(c *tc.C) {
	s.testClientError(c, func(st *storageprovisioner.Client) error {
		_, err := st.RemoveAttachments(context.Background(), nil)
		return err
	})
}

func (s *provisionerSuite) TestSetVolumeInfoClientError(c *tc.C) {
	s.testClientError(c, func(st *storageprovisioner.Client) error {
		_, err := st.SetVolumeInfo(context.Background(), nil)
		return err
	})
}

func (s *provisionerSuite) TestEnsureDeadClientError(c *tc.C) {
	s.testClientError(c, func(st *storageprovisioner.Client) error {
		_, err := st.EnsureDead(context.Background(), nil)
		return err
	})
}

func (s *provisionerSuite) TestLifeClientError(c *tc.C) {
	s.testClientError(c, func(st *storageprovisioner.Client) error {
		_, err := st.Life(context.Background(), nil)
		return err
	})
}

func (s *provisionerSuite) TestAttachmentLifeClientError(c *tc.C) {
	s.testClientError(c, func(st *storageprovisioner.Client) error {
		_, err := st.AttachmentLife(context.Background(), nil)
		return err
	})
}

func (s *provisionerSuite) TestWatchVolumesServerError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	_, err = st.WatchVolumes(context.Background(), names.NewMachineTag("123"))
	c.Check(err, tc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestVolumesServerError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.VolumeResults)) = params.VolumeResults{
			Results: []params.VolumeResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	results, err := st.Volumes(context.Background(), []names.VolumeTag{names.NewVolumeTag("100")})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Error, tc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestVolumeParamsServerError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.VolumeParamsResults)) = params.VolumeParamsResults{
			Results: []params.VolumeParamsResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	results, err := st.VolumeParams(context.Background(), []names.VolumeTag{names.NewVolumeTag("100")})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Error, tc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestRemoveVolumeParamsServerError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.RemoveVolumeParamsResults)) = params.RemoveVolumeParamsResults{
			Results: []params.RemoveVolumeParamsResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	results, err := st.RemoveVolumeParams(context.Background(), []names.VolumeTag{names.NewVolumeTag("100")})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Error, tc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestFilesystemParamsServerError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.FilesystemParamsResults)) = params.FilesystemParamsResults{
			Results: []params.FilesystemParamsResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	results, err := st.FilesystemParams(context.Background(), []names.FilesystemTag{names.NewFilesystemTag("100")})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Error, tc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestRemoveFilesystemParamsServerError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.RemoveFilesystemParamsResults)) = params.RemoveFilesystemParamsResults{
			Results: []params.RemoveFilesystemParamsResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	results, err := st.RemoveFilesystemParams(context.Background(), []names.FilesystemTag{names.NewFilesystemTag("100")})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Error, tc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestSetVolumeInfoServerError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	results, err := st.SetVolumeInfo(context.Background(), []params.Volume{{
		VolumeTag: names.NewVolumeTag("100").String(),
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Error, tc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) testServerError(c *tc.C, apiCall func(*storageprovisioner.Client, []names.Tag) ([]params.ErrorResult, error)) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	tags := []names.Tag{
		names.NewVolumeTag("100"),
	}
	results, err := apiCall(st, tags)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Error, tc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestRemoveServerError(c *tc.C) {
	s.testServerError(c, func(st *storageprovisioner.Client, tags []names.Tag) ([]params.ErrorResult, error) {
		return st.Remove(context.Background(), tags)
	})
}

func (s *provisionerSuite) TestEnsureDeadServerError(c *tc.C) {
	s.testServerError(c, func(st *storageprovisioner.Client, tags []names.Tag) ([]params.ErrorResult, error) {
		return st.EnsureDead(context.Background(), tags)
	})
}

func (s *provisionerSuite) TestLifeServerError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st, err := storageprovisioner.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	tags := []names.Tag{
		names.NewVolumeTag("100"),
	}
	results, err := st.Life(context.Background(), tags)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Error, tc.ErrorMatches, "MSG")
}
