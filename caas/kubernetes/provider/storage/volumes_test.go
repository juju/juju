// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"

	"github.com/juju/juju/caas/kubernetes/provider/storage"
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
