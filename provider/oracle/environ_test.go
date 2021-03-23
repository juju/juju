// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"errors"
	"fmt"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	gitjujutesting "github.com/juju/testing"
	"github.com/juju/utils/v2/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/oracle"
	oracletesting "github.com/juju/juju/provider/oracle/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

type environSuite struct {
	gitjujutesting.IsolationSuite
	env *oracle.OracleEnviron

	callCtx context.ProviderCallContext
}

func (e *environSuite) SetUpTest(c *gc.C) {
	var err error
	testEnvironAPI := oracletesting.DefaultEnvironAPI
	e.env, err = oracle.NewOracleEnviron(
		&oracle.EnvironProvider{},
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		testEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(e.env, gc.NotNil)

	// Setup the FakeInstance Name to match with the new
	// OracleEnviron.  Note, we are actually changing the
	// value in oracletesting.DefaultEnvironAPI.
	hostname, err := oracle.CreateHostname(e.env, "0")
	c.Assert(err, gc.IsNil)
	testEnvironAPI.FakeInstance.All.Result[0].Name = fmt.Sprintf("/Compute-a432100/sgiulitti@cloudbase.com/%s/ebc4ce91-56bb-4120-ba78-13762597f837", hostname)
	oracle.SetEnvironAPI(e.env, testEnvironAPI)
	e.callCtx = context.NewCloudCallContext()
}

var _ = gc.Suite(&environSuite{})

// shamelessly copied from one of the OpenStack tests
var clk = testclock.NewClock(time.Time{})
var advancingClock = testclock.AutoAdvancingClock{clk, clk.Advance}

func (e *environSuite) TestAvailabilityZone(c *gc.C) {
	zones, err := e.env.AvailabilityZones(e.callCtx)
	c.Assert(err, gc.IsNil)
	c.Assert(zones, gc.NotNil)
}

func (e *environSuite) TestInstanceAvailabilityZoneNames(c *gc.C) {
	zones, err := e.env.InstanceAvailabilityZoneNames(e.callCtx, []instance.Id{
		instance.Id("0"),
	})
	c.Assert(err, gc.IsNil)
	c.Assert(zones, gc.NotNil)
}

func (e *environSuite) TestInstanceAvailabilityZoneNamesWithErrors(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		&oracle.EnvironProvider{},
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		&oracletesting.FakeEnvironAPI{
			FakeInstancer: oracletesting.FakeInstancer{
				InstanceErr: errors.New("FakeInstanceErr"),
			},
		},
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	_, err = environ.InstanceAvailabilityZoneNames(e.callCtx, []instance.Id{instance.Id("0")})
	c.Assert(err, gc.NotNil)

	environ, err = oracle.NewOracleEnviron(
		&oracle.EnvironProvider{},
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		&oracletesting.FakeEnvironAPI{
			FakeInstance: oracletesting.FakeInstance{
				AllErr: errors.New("FakeInstanceErr"),
			},
		},
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	_, err = environ.InstanceAvailabilityZoneNames(e.callCtx, []instance.Id{
		instance.Id("0"),
		instance.Id("1"),
	})
	c.Assert(err, gc.NotNil)
}

func (e *environSuite) TestPrepareForBootstrap(c *gc.C) {
	ctx := envtesting.BootstrapContext(c)
	err := e.env.PrepareForBootstrap(ctx, "controller-1")
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestPrepareForBootstrapWithErrors(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		&oracle.EnvironProvider{},
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		&oracletesting.FakeEnvironAPI{
			FakeAuthenticater: oracletesting.FakeAuthenticater{
				AuthenticateErr: errors.New("FakeAuthenticateErr"),
			},
		},
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	ctx := envtesting.BootstrapContext(c)
	err = environ.PrepareForBootstrap(ctx, "controller-1")
	c.Assert(err, gc.NotNil)
}

func makeToolsList(series string) tools.List {
	var toolsVersion version.Binary
	toolsVersion.Number = version.MustParse("1.26.0")
	toolsVersion.Arch = arch.AMD64
	toolsVersion.Series = series
	return tools.List{{
		Version: toolsVersion,
		URL:     fmt.Sprintf("http://example.com/tools/juju-%s.tgz", toolsVersion),
		SHA256:  "1234567890abcdef",
		Size:    1024,
	}}
}

func (e *environSuite) TestBootstrap(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		&oracle.EnvironProvider{},
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		oracletesting.DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	ctx := envtesting.BootstrapContext(c)
	_, err = environ.Bootstrap(ctx, e.callCtx,
		environs.BootstrapParams{
			ControllerConfig:         testing.FakeControllerConfig(),
			AvailableTools:           makeToolsList("xenial"),
			BootstrapSeries:          "xenial",
			BootstrapConstraints:     constraints.MustParse("mem=3.5G"),
			SupportedBootstrapSeries: set.NewStrings("xenial").Union(testing.FakeSupportedJujuSeries),
		})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestCreate(c *gc.C) {
	err := e.env.Create(e.callCtx, environs.CreateParams{
		ControllerUUID: "dsauhdiuashd",
	})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestCreateWithErrors(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		&oracle.EnvironProvider{},
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		&oracletesting.FakeEnvironAPI{
			FakeAuthenticater: oracletesting.FakeAuthenticater{
				AuthenticateErr: errors.New("FakeAuthenticateErr"),
			},
		},
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	err = environ.Create(e.callCtx, environs.CreateParams{
		ControllerUUID: "daushdasd",
	})
	c.Assert(err, gc.NotNil)
}

func (e *environSuite) TestAdoptResources(c *gc.C) {
	err := e.env.AdoptResources(e.callCtx, "", version.Number{})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestStopInstances(c *gc.C) {
	hostname, err := oracle.CreateHostname(e.env, "0")
	c.Assert(err, gc.IsNil)
	ids := []instance.Id{instance.Id(hostname)}
	err = e.env.StopInstances(e.callCtx, ids...)
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestAllRunningInstances(c *gc.C) {
	_, err := e.env.AllRunningInstances(e.callCtx)
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestMaintainInstance(c *gc.C) {
	err := e.env.MaintainInstance(e.callCtx, environs.StartInstanceParams{})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestConfig(c *gc.C) {
	cfg := e.env.Config()
	c.Assert(cfg, gc.NotNil)
}

func (e *environSuite) TestConstraintsValidator(c *gc.C) {
	validator, err := e.env.ConstraintsValidator(e.callCtx)
	c.Assert(err, gc.IsNil)
	c.Assert(validator, gc.NotNil)
}

func (e *environSuite) TestSetConfig(c *gc.C) {
	err := e.env.SetConfig(testing.ModelConfig(c))
	c.Assert(err, gc.NotNil)
}

func (e *environSuite) TestInstances(c *gc.C) {
	hostname, err := oracle.CreateHostname(e.env, "0")
	c.Assert(err, gc.IsNil)
	instances, err := e.env.Instances(e.callCtx, []instance.Id{instance.Id(hostname)})
	c.Assert(err, gc.IsNil)
	c.Assert(instances, gc.NotNil)
}

func (e *environSuite) TestConstrollerInstances(c *gc.C) {
	instances, err := e.env.ControllerInstances(e.callCtx, "23123-3123-12312")
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
	c.Assert(instances, gc.IsNil)
}

func (e *environSuite) TestDestroy(c *gc.C) {
	err := e.env.Destroy(e.callCtx)
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestDestroyController(c *gc.C) {
	err := e.env.DestroyController(e.callCtx, "231233-312-321-3312")
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestProvider(c *gc.C) {
	p := e.env.Provider()
	c.Assert(p, gc.NotNil)
}

func (e *environSuite) TestPrecheckInstance(c *gc.C) {
	err := e.env.PrecheckInstance(e.callCtx, environs.PrecheckInstanceParams{})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestInstanceTypes(c *gc.C) {
	types, err := e.env.InstanceTypes(e.callCtx, constraints.Value{})
	c.Assert(err, gc.IsNil)
	c.Assert(types, gc.NotNil)
}
