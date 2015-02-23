// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/environment"
	"github.com/juju/juju/constraints"
	coretesting "github.com/juju/juju/testing"
)

type EnsureAvailabilitySuite struct {
	coretesting.FakeJujuHomeSuite
	fake *fakeHAClient
}

// Initialize numStateServers to an invalid number to validate
// that ensure-availability doesn't call into the API when its
// pre-checks fail
const invalidNumServers = -2

func (s *EnsureAvailabilitySuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.fake = &fakeHAClient{numStateServers: invalidNumServers}
}

type fakeHAClient struct {
	numStateServers int
	cons            constraints.Value
	err             error
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

	if f.err != nil {
		return f.result, f.err
	}

	if numStateServers == 1 {
		return f.result, nil
	}

	// In the real HAClient, specifying a numStateServers value of 0
	// indicates that the default value (3) should be used
	if numStateServers == 0 {
		numStateServers = 3
	}

	// If numStateServers > 1, we need to pretend that we added some machines
	f.result.Maintained = append(f.result.Maintained, "machine-0")
	for i := 1; i < numStateServers; i++ {
		f.result.Added = append(f.result.Added, fmt.Sprintf("machine-%d", i))
	}

	return f.result, nil
}

var _ = gc.Suite(&EnsureAvailabilitySuite{})

func (s *EnsureAvailabilitySuite) runEnsureAvailability(c *gc.C, args ...string) (*cmd.Context, error) {
	command := environment.NewEnsureAvailabilityCommand(s.fake)
	return coretesting.RunCommand(c, envcmd.Wrap(command), args...)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailability(c *gc.C) {
	ctx, err := s.runEnsureAvailability(c, "-n", "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals, "")

	c.Assert(s.fake.numStateServers, gc.Equals, 1)
	c.Assert(&s.fake.cons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(s.fake.series, gc.Equals, "")
	c.Assert(len(s.fake.placement), gc.Equals, 0)
}

func (s *EnsureAvailabilitySuite) TestBlockEnsureAvailability(c *gc.C) {
	s.fake.err = common.ErrOperationBlocked("TestBlockEnsureAvailability")
	_, err := s.runEnsureAvailability(c, "-n", "1")
	c.Assert(err, gc.ErrorMatches, cmd.ErrSilent.Error())

	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*TestBlockEnsureAvailability.*")
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityPlacementError(c *gc.C) {
	_, err := s.runEnsureAvailability(c, "-n", "1", "--to", "1")
	c.Assert(err, gc.ErrorMatches, `unsupported ensure-availability placement directive "1"`)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityFormatYaml(c *gc.C) {
	expected := map[string][]string{
		"maintained": {"0"},
		"added":      {"1", "2"},
	}

	ctx, err := s.runEnsureAvailability(c, "-n", "3", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fake.numStateServers, gc.Equals, 3)
	c.Assert(&s.fake.cons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(s.fake.series, gc.Equals, "")
	c.Assert(len(s.fake.placement), gc.Equals, 0)

	var result map[string][]string
	err = goyaml.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expected)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityFormatJson(c *gc.C) {
	expected := map[string][]string{
		"maintained": {"0"},
		"added":      {"1", "2"},
	}

	ctx, err := s.runEnsureAvailability(c, "-n", "3", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fake.numStateServers, gc.Equals, 3)
	c.Assert(&s.fake.cons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(s.fake.series, gc.Equals, "")
	c.Assert(len(s.fake.placement), gc.Equals, 0)

	var result map[string][]string
	err = json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expected)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityWithSeries(c *gc.C) {
	// Also test with -n 5 to validate numbers other than 1 and 3
	ctx, err := s.runEnsureAvailability(c, "--series", "series", "-n", "5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2, 3, 4\n\n")

	c.Assert(s.fake.numStateServers, gc.Equals, 5)
	c.Assert(&s.fake.cons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(s.fake.series, gc.Equals, "series")
	c.Assert(len(s.fake.placement), gc.Equals, 0)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityWithConstraints(c *gc.C) {
	ctx, err := s.runEnsureAvailability(c, "--constraints", "mem=4G", "-n", "3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n\n")

	c.Assert(s.fake.numStateServers, gc.Equals, 3)
	expectedCons := constraints.MustParse("mem=4G")
	c.Assert(s.fake.cons, gc.DeepEquals, expectedCons)
	c.Assert(s.fake.series, gc.Equals, "")
	c.Assert(len(s.fake.placement), gc.Equals, 0)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityWithPlacement(c *gc.C) {
	ctx, err := s.runEnsureAvailability(c, "--to", "valid", "-n", "3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n\n")

	c.Assert(s.fake.numStateServers, gc.Equals, 3)
	c.Assert(&s.fake.cons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(s.fake.series, gc.Equals, "")
	expectedPlacement := []string{"valid"}
	c.Assert(s.fake.placement, gc.DeepEquals, expectedPlacement)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityErrors(c *gc.C) {
	for _, n := range []int{-1, 2} {
		_, err := s.runEnsureAvailability(c, "-n", fmt.Sprint(n))
		c.Assert(err, gc.ErrorMatches, "must specify a number of state servers odd and non-negative")
	}

	// Verify that ensure-availability didn't call into the API
	c.Assert(s.fake.numStateServers, gc.Equals, invalidNumServers)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityAllows0(c *gc.C) {
	// If the number of state servers is specified as "0", the API will
	// then use the default number of 3.
	ctx, err := s.runEnsureAvailability(c, "-n", "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n\n")

	c.Assert(s.fake.numStateServers, gc.Equals, 0)
	c.Assert(&s.fake.cons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(s.fake.series, gc.Equals, "")
	c.Assert(len(s.fake.placement), gc.Equals, 0)
}

func (s *EnsureAvailabilitySuite) TestEnsureAvailabilityDefaultsTo0(c *gc.C) {
	// If the number of state servers is not specified, we pass in 0 to the
	// API.  The API will then use the default number of 3.
	ctx, err := s.runEnsureAvailability(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stdout(ctx), gc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n\n")

	c.Assert(s.fake.numStateServers, gc.Equals, 0)
	c.Assert(&s.fake.cons, jc.Satisfies, constraints.IsEmpty)
	c.Assert(s.fake.series, gc.Equals, "")
	c.Assert(len(s.fake.placement), gc.Equals, 0)
}
