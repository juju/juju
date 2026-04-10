// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/provider/kubernetes/storage"
	"github.com/juju/juju/testing"
)

type storageSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) TestParseStorageConfig(c *gc.C) {
	cfg, err := storage.ParseStorageConfig(map[string]interface{}{
		"storage-class":       "juju-ebs",
		"storage-provisioner": "ebs",
		"parameters.type":     "gp2",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.StorageClass, gc.Equals, "juju-ebs")
	c.Assert(cfg.StorageProvisioner, gc.Equals, "ebs")
	c.Assert(cfg.Parameters, jc.DeepEquals, map[string]string{"type": "gp2"})
}

func (s *storageSuite) TestGetStorageMode(c *gc.C) {
	type testCase struct {
		attrs map[string]interface{}
		mode  core.PersistentVolumeAccessMode
		err   string
	}

	for i, t := range []testCase{
		{
			attrs: map[string]interface{}{
				"storage-mode": "RWO",
			},
			mode: core.ReadWriteOnce,
		},
		{
			attrs: map[string]interface{}{
				"storage-mode": "ReadWriteOnce",
			},
			mode: core.ReadWriteOnce,
		},
		{
			attrs: map[string]interface{}{
				"storage-mode": "RWX",
			},
			mode: core.ReadWriteMany,
		},
		{
			attrs: map[string]interface{}{
				"storage-mode": "ReadWriteMany",
			},
			mode: core.ReadWriteMany,
		},
		{
			attrs: map[string]interface{}{
				"storage-mode": "ROX",
			},
			mode: core.ReadOnlyMany,
		},
		{
			attrs: map[string]interface{}{
				"storage-mode": "ReadOnlyMany",
			},
			mode: core.ReadOnlyMany,
		},
		{
			attrs: map[string]interface{}{
				"storage-mode": "bad-mode",
			},
			err: `storage mode "bad-mode" not supported`,
		},
	} {
		c.Logf("testing get storage mode %d", i)
		mode, err := storage.ParseStorageMode(t.attrs)
		if t.err == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(*mode, jc.DeepEquals, t.mode)
		} else {
			c.Check(err, gc.ErrorMatches, t.err)
		}
	}
}

func (s *storageSuite) TestPushUniqueVolume(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(podSpec.Volumes, jc.DeepEquals, []core.Volume{
		vol1,
	})

	err = storage.PushUniqueVolume(podSpec, vol1, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(podSpec.Volumes, jc.DeepEquals, []core.Volume{
		vol1,
	})

	err = storage.PushUniqueVolume(podSpec, vol2, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(podSpec.Volumes, jc.DeepEquals, []core.Volume{
		vol1, vol2,
	})

	err = storage.PushUniqueVolume(podSpec, aDifferentVol2, false)
	c.Assert(err, gc.ErrorMatches, `duplicated volume "vol2" not valid`)
	c.Assert(podSpec.Volumes, jc.DeepEquals, []core.Volume{
		vol1, vol2,
	})

	err = storage.PushUniqueVolume(podSpec, aDifferentVol2, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(podSpec.Volumes, jc.DeepEquals, []core.Volume{
		vol1, aDifferentVol2,
	})
}

// TestFilesystemInfoEmptyDirApplicationStorage tests that emptyDir volumes
// with the correct app name prefix are properly parsed as juju managed storage and the storage name
// is extracted from the volume mount name by removing the app name prefix.
func (s *storageSuite) TestFilesystemInfoEmptyDirApplicationStorage(c *gc.C) {
	now := time.Now()

	info, err := storage.FilesystemInfo(
		context.Background(),
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &caas.FilesystemInfo{
		StorageName:  "config",
		Size:         0,
		FilesystemId: "gitlab-config",
		MountPoint:   "/config",
		ReadOnly:     false,
		Status: status.StatusInfo{
			Status: status.Attached,
			Since:  &now,
		},
		Volume: caas.VolumeInfo{
			VolumeId:   "gitlab-config",
			Size:       0,
			Persistent: false,
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
func (s *storageSuite) TestFilesystemInfoEmptyDirNonApplicationStorage(c *gc.C) {
	now := time.Now()

	info, err := storage.FilesystemInfo(
		context.Background(),
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &caas.FilesystemInfo{
		StorageName:  "",
		Size:         0,
		FilesystemId: "vol",
		MountPoint:   "/vol",
		ReadOnly:     false,
		Status: status.StatusInfo{
			Status: status.Attached,
			Since:  &now,
		},
		Volume: caas.VolumeInfo{
			VolumeId:   "vol",
			Size:       0,
			Persistent: false,
			Status: status.StatusInfo{
				Status: status.Attached,
				Since:  &now,
			},
		},
	})
}
