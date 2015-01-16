// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment_test

import (
	"fmt"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/testing"
)

type fakeHASuite struct {
	testing.FakeJujuHomeSuite
	fake *fakeHAClient
}

func (s *fakeHASuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.fake = &fakeHAClient{numStateServers: -2}
}

type fakeHAClient struct {
	numStateServers int
	cons            constraints.Value
	series          string
	placement       []string
	result          params.StateServersChanges
}

func (f *fakeHAClient) Close() error {
	return nil
}

func (f *fakeHAClient) EnsureAvailability(numStateServers int, cons constraints.Value,
	series string, placement []string) (params.StateServersChanges, error) {

	f.numStateServers = numStateServers
	f.cons = cons
	f.series = series
	f.placement = placement

	if numStateServers <= 1 {
		return f.result, nil
	}

	// If numStateServers > 1, we need to pretend that we added some machines
	f.result.Maintained = append(f.result.Maintained, "machine-0")
	for i := 1; i < numStateServers; i++ {
		f.result.Added = append(f.result.Added, fmt.Sprintf("machine-%d", i))
	}

	return f.result, nil
}
