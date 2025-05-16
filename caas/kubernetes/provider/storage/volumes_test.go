// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	core "k8s.io/api/core/v1"

	"github.com/juju/juju/caas/kubernetes/provider/storage"
	"github.com/juju/juju/internal/testing"
)

type storageSuite struct {
	testing.BaseSuite
}

func TestStorageSuite(t *stdtesting.T) { tc.Run(t, &storageSuite{}) }
func (s *storageSuite) TestParseStorageConfig(c *tc.C) {
	cfg, err := storage.ParseStorageConfig(map[string]interface{}{
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
