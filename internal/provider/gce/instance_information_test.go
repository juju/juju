// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"testing"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/internal/provider/gce/internal/google"
)

type instanceInformationSuite struct {
	BaseSuite
}

func TestInstanceInformationSuite(t *testing.T) {
	tc.Run(t, &instanceInformationSuite{})
}
func (s *instanceInformationSuite) TestInstanceTypesCacheExpiration(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").
		Return([]*computepb.Zone{{Name: new("us-east1")}}, nil).Times(3)

	now := time.Now()
	clk := testclock.NewClock(now)
	allInstTypes, err := env.getAllInstanceTypes(c.Context(), clk)
	c.Assert(err, tc.ErrorIsNil)

	// Cache miss
	cacheExpAt := env.instCacheExpireAt
	c.Assert(cacheExpAt.After(now), tc.IsTrue, tc.Commentf("expected a cache expiration time to be set"))

	// Cache hit
	cachedInstTypes, err := env.getAllInstanceTypes(c.Context(), clk)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(allInstTypes, tc.DeepEquals, cachedInstTypes, tc.Commentf("expected to get cached instance list"))
	c.Assert(env.instCacheExpireAt, tc.Equals, cacheExpAt, tc.Commentf("expected cache expiration timestamp not to be modified"))

	// Forced cache-miss after expiry.
	// NOTE(achilleasa): this will trigger a "advancing a clock with nothing waiting"
	// warning but that's a false positive; we just want to advance the clock
	// to test the cache expiry logic.
	clk.Advance(11 * time.Minute)
	_, err = env.getAllInstanceTypes(c.Context(), clk)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.instCacheExpireAt.After(cacheExpAt), tc.IsTrue, tc.Commentf("expected cache expiration to be updated"))
	c.Assert(env.instCacheExpireAt.After(clk.Now()), tc.IsTrue, tc.Commentf("expected cache expiration to be in the future"))
}

func (s *instanceInformationSuite) TestMachineTypeArch(c *tc.C) {
	// machineTypeArch must translate the GCE API architecture values, and
	// default to amd64 for unset or unknown values (historical behaviour).
	tests := []struct {
		gceArch  *string
		expected string
	}{
		{new("ARM64"), "arm64"},
		{new("arm64"), "arm64"}, // case-insensitive match
		{new("X86_64"), "amd64"},
		{new("ARCHITECTURE_UNSPECIFIED"), "amd64"},
		{new("amd64"), "amd64"},
		{new("RISCV64"), "amd64"}, // unknown values fall back to amd64
		{new(""), "amd64"},
		{nil, "amd64"},
	}
	for i, t := range tests {
		m := &computepb.MachineType{Architecture: t.gceArch}
		c.Logf("test %d: architecture=%q", i, m.GetArchitecture())
		c.Check(machineTypeArch(c.Context(), m), tc.Equals, t.expected)
	}
}

func (s *instanceInformationSuite) TestInstanceTypesFiltersByArch(c *tc.C) {
	// Regression test: machine types were all labelled amd64, so ARM64
	// families (e.g. t2a) matched arch=amd64 constraints and were then
	// rejected by GCE when the instance was created.
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").Return([]*computepb.Zone{{
		Name:   new("home-zone"),
		Status: new("UP"),
	}}, nil).Times(1)
	s.MockService.EXPECT().ListMachineTypes(gomock.Any(), "home-zone").Return([]*computepb.MachineType{{
		Id:           new(uint64(1)),
		Name:         new("e2-standard-2"),
		GuestCpus:    new(int32(2)),
		MemoryMb:     new(int32(8192)),
		Architecture: new("X86_64"),
	}, {
		Id:           new(uint64(2)),
		Name:         new("t2a-standard-2"),
		GuestCpus:    new(int32(2)),
		MemoryMb:     new(int32(8192)),
		Architecture: new("ARM64"),
	}, {
		Id:        new(uint64(3)),
		Name:      new("n2-standard-2"),
		GuestCpus: new(int32(2)),
		MemoryMb:  new(int32(8192)),
		// Architecture unset: must default to amd64.
	}}, nil).Times(1)

	// An amd64 constraint must exclude the ARM64 machine type.
	types, err := env.InstanceTypes(c.Context(), constraints.MustParse("arch=amd64"))
	c.Assert(err, tc.ErrorIsNil)
	names := make([]string, len(types.InstanceTypes))
	for i, it := range types.InstanceTypes {
		names[i] = it.Name
		c.Check(it.Arch, tc.Equals, "amd64")
	}
	c.Check(names, tc.DeepEquals, []string{"e2-standard-2", "n2-standard-2"})

	// An arm64 constraint must only match the ARM64 machine type.
	types, err = env.InstanceTypes(c.Context(), constraints.MustParse("arch=arm64"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(types.InstanceTypes, tc.HasLen, 1)
	c.Check(types.InstanceTypes[0].Name, tc.Equals, "t2a-standard-2")
	c.Check(types.InstanceTypes[0].Arch, tc.Equals, "arm64")

	// Without an arch constraint, all types match and are labelled with
	// their reported architecture.
	types, err = env.InstanceTypes(c.Context(), constraints.Value{})
	c.Assert(err, tc.ErrorIsNil)
	arches := make(map[string]string, len(types.InstanceTypes))
	for _, it := range types.InstanceTypes {
		arches[it.Name] = it.Arch
	}
	c.Check(arches, tc.DeepEquals, map[string]string{
		"e2-standard-2":  "amd64",
		"n2-standard-2":  "amd64",
		"t2a-standard-2": "arm64",
	})
}

func (s *instanceInformationSuite) TestEnsureDefaultConstraints(c *tc.C) {
	// Fill default cores and mem.
	cons := constraints.Value{}
	c.Assert(cons.String(), tc.Equals, ``)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `cores=2`)

	var err error
	// Do not fill default cores and mem if instance type is provided.
	cons, err = constraints.Parse(`instance-type=e2-medium`)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons.String(), tc.Equals, `instance-type=e2-medium`)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `instance-type=e2-medium`)

	// Do not fill default cores and mem if cores or/and mem are provided.
	cons, err = constraints.Parse(`cores=1 mem=1024M`) // smaller than defaults
	c.Assert(err, tc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `cores=1 mem=1024M`)

	cons, err = constraints.Parse(`cores=4 mem=4096M`)
	c.Assert(err, tc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `cores=4 mem=4096M`)

	cons, err = constraints.Parse(`cores=4`)
	c.Assert(err, tc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `cores=4`)

	cons, err = constraints.Parse(`mem=4096M`)
	c.Assert(err, tc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `cores=2 mem=4096M`)
}

func (s *instanceInformationSuite) TestInstanceTypesArch(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").
		Return([]*computepb.Zone{{Name: new("us-east1-b"), Status: new(google.StatusUp)}}, nil)
	s.MockService.EXPECT().ListMachineTypes(gomock.Any(), "us-east1-b").
		Return([]*computepb.MachineType{{
			Id:           new(uint64(1)),
			Name:         new("n4-standard-2"),
			Architecture: new(computepb.MachineType_X86_64.String()),
		}, {
			Id:           new(uint64(2)),
			Name:         new("c4a-standard-2"),
			Architecture: new(computepb.MachineType_ARM64.String()),
		}, {
			Id:   new(uint64(3)),
			Name: new("n1-standard-2"),
		}}, nil)

	instTypes, err := env.getAllInstanceTypes(c.Context(), testclock.NewClock(time.Now()))
	c.Assert(err, tc.ErrorIsNil)

	arches := make(map[string]string)
	for _, it := range instTypes {
		arches[it.Name] = it.Arch
	}
	c.Check(arches, tc.DeepEquals, map[string]string{
		"n4-standard-2":  arch.AMD64,
		"c4a-standard-2": arch.ARM64,
		"n1-standard-2":  arch.AMD64,
	})
}
