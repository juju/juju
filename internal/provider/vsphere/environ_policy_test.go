// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"context"

	"github.com/juju/tc"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/vsphere/internal/vsphereclient"
)

type environPolSuite struct {
	EnvironFixture
}

var _ = tc.Suite(&environPolSuite{})

func (s *environPolSuite) TestConstraintsValidator(c *tc.C) {
	validator, err := s.env.ConstraintsValidator(context.Background())
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(unsupported, tc.HasLen, 0)
}

func (s *environPolSuite) TestConstraintsValidatorEmpty(c *tc.C) {
	validator, err := s.env.ConstraintsValidator(context.Background())
	c.Assert(err, tc.ErrorIsNil)

	unsupported, err := validator.Validate(constraints.Value{})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(unsupported, tc.HasLen, 0)
}

func (s *environPolSuite) TestConstraintsValidatorUnsupported(c *tc.C) {
	validator, err := s.env.ConstraintsValidator(context.Background())
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64 tags=foo virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(unsupported, tc.SameContents, []string{"tags", "virt-type"})
}

func (s *environPolSuite) TestConstraintsValidatorVocabArch(c *tc.C) {
	validator, err := s.env.ConstraintsValidator(context.Background())
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.MustParse("arch=ppc64el")
	_, err = validator.Validate(cons)
	c.Check(err, tc.ErrorMatches, "invalid constraint value: arch=ppc64el\nvalid values are:.*")
}

func (s *environPolSuite) TestPrecheckInstanceChecksPlacementZone(c *tc.C) {
	s.client.folders = makeFolders("/DC/host")
	err := s.env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Placement: "zone=some-zone",
	})
	c.Assert(err, tc.ErrorMatches, `availability zone "some-zone" not found`)

	s.client.computeResources = []vsphereclient.ComputeResource{
		{Resource: newComputeResource("z1"), Path: "/DC/host/z1"},
		{Resource: newComputeResource("z2"), Path: "/DC/host/z2"},
	}
	s.client.resourcePools = map[string][]*object.ResourcePool{
		"/DC/host/z1/...": {makeResourcePool("pool-1", "/DC/host/z1/Resources")},
	}
	err = s.env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Placement: "zone=z1",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceChecksConstraintZones(c *tc.C) {
	s.client.folders = makeFolders("/DC/host")
	s.client.computeResources = []vsphereclient.ComputeResource{
		{Resource: newComputeResource("z1"), Path: "/DC/host/z1"},
		{Resource: newComputeResource("z2"), Path: "/DC/host/z2"},
	}
	s.client.resourcePools = map[string][]*object.ResourcePool{
		"/DC/host/z1/...": {makeResourcePool("pool-1", "/DC/host/z1/Resources")},
		"/DC/host/z2/...": {
			// Check we don't get broken by trailing slashes.
			makeResourcePool("pool-2", "/DC/host/z2/Resources/"),
			makeResourcePool("pool-3", "/DC/host/z2/Resources/child"),
			makeResourcePool("pool-4", "/DC/host/z2/Resources/child/nested"),
			makeResourcePool("pool-5", "/DC/host/z2/Resources/child/nested/other/"),
		},
	}
	err := s.env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Constraints: constraints.MustParse("zones=z1,z2/child,z2/fish"),
	})
	c.Assert(err, tc.ErrorMatches, `availability zone "z2/fish" not found`)

	err = s.env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Constraints: constraints.MustParse("zones=z2/fish"),
	})
	c.Assert(err, tc.ErrorMatches, `availability zone "z2/fish" not found`)

	err = s.env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Constraints: constraints.MustParse("zones=z1,z2/child"),
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceChecksConstraintDatastore(c *tc.C) {
	s.client.datastores = []mo.Datastore{{
		ManagedEntity: mo.ManagedEntity{Name: "foo"},
	}, {
		ManagedEntity: mo.ManagedEntity{Name: "bar"},
		Summary: types.DatastoreSummary{
			Accessible: true,
		},
	}}

	err := s.env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Constraints: constraints.MustParse("root-disk-source=blam"),
	})
	c.Assert(err, tc.ErrorMatches, `datastore "blam" not found`)

	err = s.env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Constraints: constraints.MustParse("root-disk-source=foo"),
	})
	c.Assert(err, tc.ErrorMatches, `datastore "foo" not found`)

	err = s.env.PrecheckInstance(context.Background(), environs.PrecheckInstanceParams{
		Constraints: constraints.MustParse("root-disk-source=bar"),
	})
	c.Assert(err, tc.ErrorIsNil)
}
