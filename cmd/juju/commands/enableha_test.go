// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/juju/tc"
	goyaml "gopkg.in/yaml.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type EnableHASuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	fake *fakeHAClient
}

// invalidNumServers is a number of controllers that would
// never be generated by the enable-ha command.
const invalidNumServers = -2

func (s *EnableHASuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	// Initialize numControllers to an invalid number to validate
	// that enable-ha doesn't call into the API when its
	// pre-checks fail
	s.fake = &fakeHAClient{numControllers: invalidNumServers}
}

type fakeHAClient struct {
	numControllers int
	cons           constraints.Value
	err            error
	placement      []string
	result         params.ControllersChanges
}

func (f *fakeHAClient) Close() error {
	return nil
}

func (f *fakeHAClient) EnableHA(ctx context.Context, numControllers int, cons constraints.Value, placement []string) (
	params.ControllersChanges, error,
) {
	f.numControllers = numControllers
	f.cons = cons
	f.placement = placement

	if f.err != nil {
		return f.result, f.err
	}

	if numControllers == 1 {
		return f.result, nil
	}

	// In the real HAClient, specifying a numControllers value of 0
	// indicates that the default value (3) should be used
	if numControllers == 0 {
		numControllers = 3
	}

	f.result.Maintained = append(f.result.Maintained, "machine-0")

	for _, p := range placement {
		m, err := instance.ParsePlacement(p)
		if err == nil && m.Scope == instance.MachineScope {
			f.result.Converted = append(f.result.Converted, "machine-"+m.Directive)
		}
	}

	// We may need to pretend that we added some machines.
	for i := len(f.result.Converted) + 1; i < numControllers; i++ {
		f.result.Added = append(f.result.Added, fmt.Sprintf("machine-%d", i))
	}

	return f.result, nil
}
func TestEnableHASuite(t *testing.T) {
	tc.Run(t, &EnableHASuite{})
}

func (s *EnableHASuite) runEnableHA(c *tc.C, args ...string) (*cmd.Context, error) {
	command := &enableHACommand{newHAClientFunc: func(ctx context.Context) (MakeHAClient, error) { return s.fake, nil }}
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "arthur"
	store.Controllers["arthur"] = jujuclient.ControllerDetails{}
	store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "admin/controller",
		Models: map[string]jujuclient.ModelDetails{"admin/controller": {
			ModelType: model.IAAS,
			ModelUUID: coretesting.ModelTag.Id(),
		}},
	}
	store.Accounts["arthur"] = jujuclient.AccountDetails{
		User: "king",
	}
	command.SetClientStore(store)
	return cmdtesting.RunCommand(c, modelcmd.WrapController(command), args...)
}

func (s *EnableHASuite) TestEnableHA(c *tc.C) {
	ctx, err := s.runEnableHA(c, "-n", "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")

	c.Assert(s.fake.numControllers, tc.Equals, 1)
	c.Assert(&s.fake.cons, tc.Satisfies, constraints.IsEmpty)
	c.Assert(len(s.fake.placement), tc.Equals, 0)
}

func (s *EnableHASuite) TestBlockEnableHA(c *tc.C) {
	s.fake.err = apiservererrors.OperationBlockedError("TestBlockEnableHA")
	_, err := s.runEnableHA(c, "-n", "1")
	c.Assert(err, tc.NotNil)
	c.Assert(strings.Contains(err.Error(), "All operations that change model have been disabled for the current model"), tc.IsTrue)
}

func (s *EnableHASuite) TestEnableHAFormatYaml(c *tc.C) {
	expected := map[string][]string{
		"maintained": {"0"},
		"added":      {"1", "2"},
	}

	ctx, err := s.runEnableHA(c, "-n", "3", "--format", "yaml")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.fake.numControllers, tc.Equals, 3)
	c.Assert(&s.fake.cons, tc.Satisfies, constraints.IsEmpty)
	c.Assert(len(s.fake.placement), tc.Equals, 0)

	var result map[string][]string
	err = goyaml.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &result)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, expected)
}

func (s *EnableHASuite) TestEnableHAFormatJson(c *tc.C) {
	expected := map[string][]string{
		"maintained": {"0"},
		"added":      {"1", "2"},
	}

	ctx, err := s.runEnableHA(c, "-n", "3", "--format", "json")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.fake.numControllers, tc.Equals, 3)
	c.Assert(&s.fake.cons, tc.Satisfies, constraints.IsEmpty)
	c.Assert(len(s.fake.placement), tc.Equals, 0)

	var result map[string][]string
	err = json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &result)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, expected)
}

func (s *EnableHASuite) TestEnableHAWithFive(c *tc.C) {
	// Also test with -n 5 to validate numbers other than 1 and 3
	ctx, err := s.runEnableHA(c, "-n", "5")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2, 3, 4\n")

	c.Assert(s.fake.numControllers, tc.Equals, 5)
	c.Assert(&s.fake.cons, tc.Satisfies, constraints.IsEmpty)
	c.Assert(len(s.fake.placement), tc.Equals, 0)
}

func (s *EnableHASuite) TestEnableHAWithConstraints(c *tc.C) {
	ctx, err := s.runEnableHA(c, "--constraints", "mem=4G", "-n", "3")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n")

	c.Assert(s.fake.numControllers, tc.Equals, 3)
	expectedCons := constraints.MustParse("mem=4G")
	c.Assert(s.fake.cons, tc.DeepEquals, expectedCons)
	c.Assert(len(s.fake.placement), tc.Equals, 0)
}

func (s *EnableHASuite) TestEnableHAWithMultipleConstraints(c *tc.C) {
	ctx, err := s.runEnableHA(c, "--constraints", "cores=4", "--constraints", "mem=4G", "-n", "3")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n")

	c.Assert(s.fake.numControllers, tc.Equals, 3)
	expectedCons := constraints.MustParse("cores=4 mem=4G")
	c.Assert(s.fake.cons, tc.DeepEquals, expectedCons)
	c.Assert(len(s.fake.placement), tc.Equals, 0)
}

func (s *EnableHASuite) TestEnableHAWithPlacement(c *tc.C) {
	ctx, err := s.runEnableHA(c, "--to", "valid", "-n", "3")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n")

	c.Assert(s.fake.numControllers, tc.Equals, 3)
	c.Assert(&s.fake.cons, tc.Satisfies, constraints.IsEmpty)
	expectedPlacement := []string{"valid"}
	c.Assert(s.fake.placement, tc.DeepEquals, expectedPlacement)
}

func (s *EnableHASuite) TestEnableHAErrors(c *tc.C) {
	for _, n := range []int{-1, 2} {
		_, err := s.runEnableHA(c, "-n", fmt.Sprint(n))
		c.Assert(err, tc.ErrorMatches, "must specify a number of controllers odd and non-negative")
	}

	// Verify that enable-ha didn't call into the API
	c.Assert(s.fake.numControllers, tc.Equals, invalidNumServers)
}

func (s *EnableHASuite) TestEnableHAErrorsWithInvalidPlacement(c *tc.C) {
	_, err := s.runEnableHA(c, "--to", "in,,valid", "-n", "3")
	c.Assert(err, tc.ErrorMatches, "empty placement directive passed to enable-ha")

	// Verify that enable-ha didn't call into the API
	c.Assert(s.fake.numControllers, tc.Equals, invalidNumServers)
}

func (s *EnableHASuite) TestEnableHAAllows0(c *tc.C) {
	// If the number of controllers is specified as "0", the API will
	// then use the default number of 3.
	ctx, err := s.runEnableHA(c, "-n", "0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n")

	c.Assert(s.fake.numControllers, tc.Equals, 0)
	c.Assert(&s.fake.cons, tc.Satisfies, constraints.IsEmpty)
	c.Assert(len(s.fake.placement), tc.Equals, 0)
}

func (s *EnableHASuite) TestEnableHADefaultsTo0(c *tc.C) {
	// If the number of controllers is not specified, we pass in 0 to the
	// API.  The API will then use the default number of 3.
	ctx, err := s.runEnableHA(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals,
		"maintaining machines: 0\n"+
			"adding machines: 1, 2\n")

	c.Assert(s.fake.numControllers, tc.Equals, 0)
	c.Assert(&s.fake.cons, tc.Satisfies, constraints.IsEmpty)
	c.Assert(len(s.fake.placement), tc.Equals, 0)
}

func (s *EnableHASuite) TestEnableHAToExisting(c *tc.C) {
	ctx, err := s.runEnableHA(c, "--to", "1,2")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, `
maintaining machines: 0
converting machines: 1, 2
`[1:])

	c.Check(s.fake.numControllers, tc.Equals, 0)
	c.Check(&s.fake.cons, tc.Satisfies, constraints.IsEmpty)
	c.Check(len(s.fake.placement), tc.Equals, 2)
}

func (s *EnableHASuite) TestEnableHADisallowsSeries(c *tc.C) {
	// We do not allow --series as an argument. This test ensures it is not
	// inadvertently added back.
	ctx, err := s.runEnableHA(c, "-n", "0", "--series", "xenian")
	c.Assert(err, tc.ErrorMatches, "option provided but not defined: --series")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}
