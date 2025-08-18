// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"os"
	"strings"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiapplication "github.com/juju/juju/api/client/application"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type AddUnitSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake *fakeApplicationAddUnitAPI

	store *jujuclient.MemStore
}

type fakeApplicationAddUnitAPI struct {
	envType       string
	application   string
	numUnits      int
	placement     []*instance.Placement
	attachStorage []string
	err           error
}

func (f *fakeApplicationAddUnitAPI) Close() error {
	return nil
}

func (f *fakeApplicationAddUnitAPI) ModelUUID() string {
	return "fake-uuid"
}

func (f *fakeApplicationAddUnitAPI) AddUnits(args apiapplication.AddUnitsParams) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	if args.ApplicationName != f.application {
		return nil, errors.NotFoundf("application %q", args.ApplicationName)
	}

	f.numUnits += args.NumUnits
	f.placement = args.Placement
	f.attachStorage = args.AttachStorage
	return nil, nil
}

func (f *fakeApplicationAddUnitAPI) ScaleApplication(args apiapplication.ScaleApplicationParams) (params.ScaleApplicationResult, error) {
	if f.err != nil {
		return params.ScaleApplicationResult{}, f.err
	}
	if args.ApplicationName != f.application {
		return params.ScaleApplicationResult{}, errors.NotFoundf("application %q", args.ApplicationName)
	}
	f.numUnits += args.ScaleChange
	return params.ScaleApplicationResult{}, nil
}

func (f *fakeApplicationAddUnitAPI) ModelGet() (map[string]interface{}, error) {
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": f.envType,
		"name": "dummy",
	})
	if err != nil {
		return nil, err
	}

	return cfg.AllAttrs(), nil
}

func (s *AddUnitSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fake = &fakeApplicationAddUnitAPI{
		application: "some-application-name",
		numUnits:    1,
		envType:     "dummy",
	}
	s.store = jujuclienttesting.MinimalStore()
}

var _ = gc.Suite(&AddUnitSuite{})

var initAddUnitErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: []string{"some-application-name", "-n", "0"},
		err:  `--num-units must be a positive integer`,
	}, {
		args: []string{},
		err:  `no application specified`,
	}, {
		args: []string{"some-application-name", "--to", "1,#:foo"},
		err:  `invalid --to parameter "#:foo"`,
	}, {
		args: []string{"some-application-name", "--attach-storage", "foo/0", "-n", "2"},
		err:  `--attach-storage cannot be used with -n`,
	}, {
		args: []string{"some-application-name", "--to", "4,5,,"},
		err:  `invalid --to parameter "4,5,,"`,
	},
}

func (s *AddUnitSuite) TestInitErrors(c *gc.C) {
	for i, t := range initAddUnitErrorTests {
		c.Logf("test %d", i)
		err := cmdtesting.InitCommand(application.NewAddUnitCommandForTest(s.fake, s.store), t.args)
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

// Must error at init when the model type is known (and args are invalid)
func (s *AddUnitSuite) TestInitErrorsForCAAS(c *gc.C) {
	m := s.store.Models["arthur"].Models["king/sword"]
	m.ModelType = model.CAAS
	s.store.Models["arthur"].Models["king/sword"] = m
	err := cmdtesting.InitCommand(application.NewAddUnitCommandForTest(s.fake, s.store), []string{"some-application-name", "--to", "lxd:1"})
	c.Check(err, gc.ErrorMatches, "k8s models only support --num-units")
}

func (s *AddUnitSuite) runAddUnit(c *gc.C, args ...string) error {
	_, err := cmdtesting.RunCommand(c, application.NewAddUnitCommandForTest(s.fake, s.store), args...)
	return err
}

func (s *AddUnitSuite) TestAddUnit(c *gc.C) {
	err := s.runAddUnit(c, "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 2)

	err = s.runAddUnit(c, "--num-units", "2", "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 4)
}

func (s *AddUnitSuite) TestAddUnitWithPlacement(c *gc.C) {
	err := s.runAddUnit(c, "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 2)

	err = s.runAddUnit(c, "--num-units", "2", "--to", "123,lxd:1,1/lxd/2,foo", "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 4)
	c.Assert(s.fake.placement, jc.DeepEquals, []*instance.Placement{
		{"#", "123"},
		{"lxd", "1"},
		{"#", "1/lxd/2"},
		{"fake-uuid", "foo"},
	})
}

func (s *AddUnitSuite) TestAddUnitAttachStorage(c *gc.C) {
	err := s.runAddUnit(c, "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 2)
	c.Assert(s.fake.attachStorage, gc.HasLen, 0)

	err = s.runAddUnit(c, "some-application-name", "--attach-storage", "foo/0,bar/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 3)
	c.Assert(s.fake.attachStorage, jc.DeepEquals, []string{"foo/0", "bar/1"})
}

func (s *AddUnitSuite) TestBlockAddUnit(c *gc.C) {
	// Block operation
	s.fake.err = apiservererrors.OperationBlockedError("TestBlockAddUnit")
	s.runAddUnit(c, "some-application-name")

	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*TestBlockAddUnit.*")
}

func (s *AddUnitSuite) TestUnauthorizedMentionsJujuGrant(c *gc.C) {
	s.fake.err = &params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	}
	ctx, _ := cmdtesting.RunCommand(c, application.NewAddUnitCommandForTest(
		s.fake, jujuclienttesting.MinimalStore()), "some-application-name")
	errString := strings.Replace(cmdtesting.Stderr(ctx), "\n", " ", -1)
	c.Assert(errString, gc.Matches, `.*juju grant.*`)
}

func (s *AddUnitSuite) TestForceMachine(c *gc.C) {
	err := s.runAddUnit(c, "some-application-name", "--to", "3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 2)
	c.Assert(s.fake.placement[0].Directive, gc.Equals, "3")

	err = s.runAddUnit(c, "some-application-name", "--to", "23")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 3)
	c.Assert(s.fake.placement[0].Directive, gc.Equals, "23")
}

func (s *AddUnitSuite) TestForceMachineNewContainer(c *gc.C) {
	err := s.runAddUnit(c, "some-application-name", "--to", "lxd:1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 2)
	c.Assert(s.fake.placement[0].Directive, gc.Equals, "1")
	c.Assert(s.fake.placement[0].Scope, gc.Equals, "lxd")
}

func (s *AddUnitSuite) TestNameChecks(c *gc.C) {
	assertMachineOrNewContainer := func(s string, expect bool) {
		c.Logf("%s -> %v", s, expect)
		c.Assert(application.IsMachineOrNewContainer(s), gc.Equals, expect)
	}
	assertMachineOrNewContainer("0", true)
	assertMachineOrNewContainer("00", false)
	assertMachineOrNewContainer("1", true)
	assertMachineOrNewContainer("0/lxd/0", true)
	assertMachineOrNewContainer("lxd:0", true)
	assertMachineOrNewContainer("lxd:lxd:0", false)
	assertMachineOrNewContainer("kvm:0/lxd/1", true)
	assertMachineOrNewContainer("lxd:", false)
	assertMachineOrNewContainer(":lxd", false)
	assertMachineOrNewContainer("0/lxd/", false)
	assertMachineOrNewContainer("0/lxd", false)
	assertMachineOrNewContainer("kvm:0/lxd", false)
	assertMachineOrNewContainer("0/lxd/01", false)
	assertMachineOrNewContainer("0/lxd/10", true)
	assertMachineOrNewContainer("0/kvm/4", true)
}

func (s *AddUnitSuite) TestCAASAllowsNumUnitsOnly(c *gc.C) {
	expectedError := "k8s models only support --num-units"
	m := s.store.Models["arthur"].Models["king/sword"]
	m.ModelType = model.CAAS
	s.store.Models["arthur"].Models["king/sword"] = m

	err := s.runAddUnit(c, "some-application-name", "--to", "lxd:1")
	c.Assert(err, gc.ErrorMatches, expectedError)

	err = s.runAddUnit(c, "some-application-name", "--to", "lxd:1", "-n", "2")
	c.Assert(err, gc.ErrorMatches, expectedError)

	err = s.runAddUnit(c, "some-application-name", "--attach-storage", "foo/0")
	c.Assert(err, gc.ErrorMatches, expectedError)

	err = s.runAddUnit(c, "some-application-name", "--attach-storage", "foo/0", "-n", "2")
	c.Assert(err, gc.ErrorMatches, expectedError)

	err = s.runAddUnit(c, "some-application-name", "--attach-storage", "foo/0", "-n", "2", "--to", "lxd:1")
	c.Assert(err, gc.ErrorMatches, expectedError)

	err = s.runAddUnit(c, "some-application-name", "--num-units", "2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AddUnitSuite) TestCAASAddUnitNotSupported(c *gc.C) {
	m := s.store.Models["arthur"].Models["king/sword"]
	m.ModelType = model.CAAS
	s.store.Models["arthur"].Models["king/sword"] = m

	s.fake.err = apiservererrors.ServerError(errors.NotSupportedf(`scale a "daemon" charm`))
	err := s.runAddUnit(c, "some-application-name")
	c.Check(err, gc.ErrorMatches, `can not add unit: scale a "daemon" charm not supported`)
}
func (s *AddUnitSuite) TestUnknownModelCallsRefresh(c *gc.C) {
	called := false
	refresh := func(jujuclient.ClientStore, string) error {
		called = true
		return nil
	}
	cmd := application.NewAddUnitCommandForTestWithRefresh(s.fake, s.store, refresh)
	_, err := cmdtesting.RunCommand(c, cmd, "-m", "nope", "no-app")
	c.Check(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "model arthur:king/nope not found")
}

func (s *AddUnitSuite) TestCAASAddUnitAttachStorage(c *gc.C) {
	s.SetFeatureFlags(feature.K8SAttachStorage)
	defer func() {
		// Unset feature flag
		os.Unsetenv(osenv.JujuFeatureFlagEnvKey)
		featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
	}()
	m := s.store.Models["arthur"].Models["king/sword"]
	m.ModelType = model.CAAS
	s.store.Models["arthur"].Models["king/sword"] = m

	err := s.runAddUnit(c, "some-application-name", "--attach-storage", "foo/0")
	c.Check(err, jc.ErrorIsNil)
}
