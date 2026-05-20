// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	core "k8s.io/api/core/v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/provider/kubernetes/storage"
	"github.com/juju/juju/internal/testing"
)

type storageSuite struct {
	testing.BaseSuite
}

func TestStorageSuite(t *stdtesting.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) TestParseStorageConfig(c *tc.C) {
	cfg, err := storage.ParseStorageConfig(map[string]any{
		"storage-class":       "juju-ebs",
		"storage-provisioner": "ebs",
		"parameters.type":     "gp2",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.StorageClass, tc.Equals, "juju-ebs")
	c.Assert(cfg.StorageProvisioner, tc.Equals, "ebs")
	c.Assert(cfg.Parameters, tc.DeepEquals, map[string]string{"type": "gp2"})
}

func (s *storageSuite) TestGetStorageMode(c *tc.C) {
	type testCase struct {
		attrs map[string]any
		mode  core.PersistentVolumeAccessMode
		err   string
	}

	for i, t := range []testCase{
		{
			attrs: map[string]any{
				"storage-mode": "RWO",
			},
			mode: core.ReadWriteOnce,
		},
		{
			attrs: map[string]any{
				"storage-mode": "ReadWriteOnce",
			},
			mode: core.ReadWriteOnce,
		},
		{
			attrs: map[string]any{
				"storage-mode": "RWX",
			},
			mode: core.ReadWriteMany,
		},
		{
			attrs: map[string]any{
				"storage-mode": "ReadWriteMany",
			},
			mode: core.ReadWriteMany,
		},
		{
			attrs: map[string]any{
				"storage-mode": "ROX",
			},
			mode: core.ReadOnlyMany,
		},
		{
			attrs: map[string]any{
				"storage-mode": "ReadOnlyMany",
			},
			mode: core.ReadOnlyMany,
		},
		{
			attrs: map[string]any{
				"storage-mode": "bad-mode",
			},
			err: `storage mode "bad-mode" not supported`,
		},
	} {
		c.Logf("testing get storage mode %d", i)
		mode, err := storage.ParseStorageMode(t.attrs)
		if t.err == "" {
			c.Check(err, tc.ErrorIsNil)
			c.Check(*mode, tc.DeepEquals, t.mode)
		} else {
			c.Check(err, tc.ErrorMatches, t.err)
		}
	}
}

func (s *storageSuite) TestPushUniqueVolume(c *tc.C) {
	podSpec := &core.PodSpec{}

	vol1 := core.Volume{
		Name: "vol1",
		VolumeSource: core.VolumeSource{
			EmptyDir: &core.EmptyDirVolumeSource{},
		},
	}
	vol2 := core.Volume{
		Name: "vol2",
		VolumeSource: core.VolumeSource{
			HostPath: &core.HostPathVolumeSource{
				Path: "/var/log/gitlab",
			},
		},
	}
	aDifferentVol2 := core.Volume{
		Name: "vol2",
		VolumeSource: core.VolumeSource{
			HostPath: &core.HostPathVolumeSource{
				Path: "/var/log/foo",
			},
		},
	}
	err := storage.PushUniqueVolume(podSpec, vol1, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(podSpec.Volumes, tc.DeepEquals, []core.Volume{
		vol1,
	})

	err = storage.PushUniqueVolume(podSpec, vol1, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(podSpec.Volumes, tc.DeepEquals, []core.Volume{
		vol1,
	})

	err = storage.PushUniqueVolume(podSpec, vol2, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(podSpec.Volumes, tc.DeepEquals, []core.Volume{
		vol1, vol2,
	})

	err = storage.PushUniqueVolume(podSpec, aDifferentVol2, false)
	c.Assert(err, tc.ErrorMatches, `duplicated volume "vol2" not valid`)
	c.Assert(podSpec.Volumes, tc.DeepEquals, []core.Volume{
		vol1, vol2,
	})

	err = storage.PushUniqueVolume(podSpec, aDifferentVol2, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(podSpec.Volumes, tc.DeepEquals, []core.Volume{
		vol1, aDifferentVol2,
	})
}

// TestFilesystemInfoEmptyDirApplicationStorage tests that emptyDir volumes
// with the correct app name prefix are properly parsed as juju managed storage and the storage name
// is extracted from the volume mount name by removing the app name prefix.
func (s *storageSuite) TestFilesystemInfoEmptyDirApplicationStorage(c *tc.C) {
	now := time.Now()

	info, err := storage.FilesystemInfo(
		c.Context(),
		nil,
		"",
		core.Volume{
			Name: "gitlab-config",
			VolumeSource: core.VolumeSource{
				EmptyDir: &core.EmptyDirVolumeSource{},
			},
		},
		core.VolumeMount{
			Name:      "gitlab-config",
			MountPath: "/config",
		},
		"gitlab",
		now,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, &caas.FilesystemInfo{
		StorageName:               "config",
		Size:                      0,
		PersistentVolumeClaimName: "gitlab-config",
		MountPoint:                "/config",
		ReadOnly:                  false,
		Status: status.StatusInfo{
			Status: status.Attached,
			Since:  &now,
		},
		Volume: caas.VolumeInfo{
			PersistentVolumeName: "gitlab-config",
			Size:                 0,
			Persistent:           false,
			Status: status.StatusInfo{
				Status: status.Attached,
				Since:  &now,
			},
		},
	})
}

// TestFilesystemInfoEmptyDirNonApplicationStorage tests that emptyDir volumes
// that are not attached with the expected "<app>-<storage>" pattern are
// treated by FilesystemInfo as non-Juju-managed storage, yielding an empty storage name.
func (s *storageSuite) TestFilesystemInfoEmptyDirNonApplicationStorage(c *tc.C) {
	now := time.Now()

	info, err := storage.FilesystemInfo(
		c.Context(),
		nil,
		"",
		core.Volume{
			Name: "vol",
			VolumeSource: core.VolumeSource{
				EmptyDir: &core.EmptyDirVolumeSource{},
			},
		},
		core.VolumeMount{
			Name:      "vol",
			MountPath: "/vol",
		},
		"gitlab",
		now,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, &caas.FilesystemInfo{
		StorageName:               "",
		Size:                      0,
		PersistentVolumeClaimName: "vol",
		MountPoint:                "/vol",
		ReadOnly:                  false,
		Status: status.StatusInfo{
			Status: status.Attached,
			Since:  &now,
		},
		Volume: caas.VolumeInfo{
			PersistentVolumeName: "vol",
			Size:                 0,
			Persistent:           false,
			Status: status.StatusInfo{
				Status: status.Attached,
				Since:  &now,
			},
		},
	})
}
