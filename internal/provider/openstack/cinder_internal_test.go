// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type cinderConfigSuite struct {
	testhelpers.IsolationSuite
}

func TestCinderConfigSuite(t *stdtesting.T) {
	tc.Run(t, &cinderConfigSuite{})
}

func (s *cinderConfigSuite) TestNewCinderConfigEmpty(c *tc.C) {
	cfg, err := newCinderConfig(nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg, tc.DeepEquals, &cinderConfig{})
}

func (s *cinderConfigSuite) TestNewCinderConfigVolumeType(c *tc.C) {
	cfg, err := newCinderConfig(map[string]any{
		"volume-type": "Ceph",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg, tc.DeepEquals, &cinderConfig{volumeType: "Ceph"})
}

func (s *cinderConfigSuite) TestNewCinderConfigFull(c *tc.C) {
	cfg, err := newCinderConfig(map[string]any{
		"volume-type": "Ceph",
		"disk-bus":    "scsi",
		"tag":         "root",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg, tc.DeepEquals, &cinderConfig{
		volumeType: "Ceph",
		diskBus:    "scsi",
		tag:        "root",
	})
}

func (s *cinderConfigSuite) TestNewCinderConfigInvalidDiskBus(c *tc.C) {
	_, err := newCinderConfig(map[string]any{
		"disk-bus": "pcie",
	})
	c.Assert(err, tc.ErrorMatches, "validating Cinder storage config: .*")
}
