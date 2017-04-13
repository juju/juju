// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"errors"
	"fmt"
	"time"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/oracle"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	gitjujutesting "github.com/juju/testing"
	"github.com/juju/utils/arch"
	//"github.com/juju/utils/clock"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
)

type environSuite struct{}

var _ = gc.Suite(&environSuite{})

// shamelessly copied from one of the OpenStack tests
var clk = gitjujutesting.NewClock(time.Time{})
var advancingClock = gitjujutesting.AutoAdvancingClock{clk, clk.Advance}

func (e *environSuite) TestNewOracleEnviron(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)
}

func (e *environSuite) TestAvailabilityZone(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	zones, err := environ.AvailabilityZones()
	c.Assert(err, gc.IsNil)
	c.Assert(zones, gc.NotNil)
}

func (e *environSuite) TestInstanceAvailabilityZoneNames(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	zones, err := environ.InstanceAvailabilityZoneNames([]instance.Id{
		instance.Id("0"),
	})
	c.Assert(err, gc.IsNil)
	c.Assert(zones, gc.NotNil)
}

func (e *environSuite) TestInstanceAvailabilityZoneNamesWithErrors(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		FakeEnvironAPI{
			FakeInstancer: FakeInstancer{
				InstanceErr: errors.New("FakeInstanceErr"),
			},
		},
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	_, err = environ.InstanceAvailabilityZoneNames([]instance.Id{instance.Id("0")})
	c.Assert(err, gc.NotNil)

	environ, err = oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		FakeEnvironAPI{
			FakeInstance: FakeInstance{
				AllErr: errors.New("FakeInstanceErr"),
			},
		},
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	_, err = environ.InstanceAvailabilityZoneNames([]instance.Id{
		instance.Id("0"),
		instance.Id("1"),
	})
	c.Assert(err, gc.NotNil)
}

func (e *environSuite) TestPrepareForBootstrap(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	ctx := envtesting.BootstrapContext(c)
	err = environ.PrepareForBootstrap(ctx)
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestPrepareForBootstrapWithErrors(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		FakeEnvironAPI{
			FakeAuthenticater: FakeAuthenticater{
				AuthenticateErr: errors.New("FakeAuthenticateErr"),
			},
		},
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	ctx := envtesting.BootstrapContext(c)
	err = environ.PrepareForBootstrap(ctx)
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
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
        &advancingClock,
		//clock.WallClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	ctx := envtesting.BootstrapContext(c)
	_, err = environ.Bootstrap(ctx,
		environs.BootstrapParams{
			ControllerConfig:     testing.FakeControllerConfig(),
			AvailableTools:       makeToolsList("xenial"),
			BootstrapSeries:      "xenial",
			BootstrapConstraints: constraints.MustParse("mem=3.5G"),
		})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestCreate(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	err = environ.Create(environs.CreateParams{
		ControllerUUID: "dsauhdiuashd",
	})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestCreateWithErrors(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		FakeEnvironAPI{
			FakeAuthenticater: FakeAuthenticater{
				AuthenticateErr: errors.New("FakeAuthenticateErr"),
			},
		},
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	err = environ.Create(environs.CreateParams{
		ControllerUUID: "daushdasd",
	})
	c.Assert(err, gc.NotNil)
}

func (e *environSuite) TestAdoptResources(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	err = environ.AdoptResources("", version.Number{})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestStopInstances(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	ids := []instance.Id{instance.Id("0")}
	err = environ.StopInstances(ids...)
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestAllInstances(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	_, err = environ.AllInstances()
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestMaintainInstance(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	err = environ.MaintainInstance(environs.StartInstanceParams{})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestConfig(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	cfg := environ.Config()
	c.Assert(cfg, gc.NotNil)
}

func (e *environSuite) TestConstraintsValidator(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	validator, err := environ.ConstraintsValidator()
	c.Assert(err, gc.IsNil)
	c.Assert(validator, gc.NotNil)
}

func (e *environSuite) TestSetConfig(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	err = environ.SetConfig(testing.ModelConfig(c))
	c.Assert(err, gc.NotNil)
}

func (e *environSuite) TestInstances(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	instances, err := environ.Instances([]instance.Id{instance.Id("0")})
	c.Assert(err, gc.IsNil)
	c.Assert(instances, gc.NotNil)
}

func (e *environSuite) TestConstrollerInstances(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	instances, err := environ.ControllerInstances("23123-3123-12312")
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
	c.Assert(instances, gc.IsNil)
}

func (e *environSuite) TestDestroy(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	err = environ.Destroy()
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestDestroyController(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	err = environ.DestroyController("231233-312-321-3312")
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestProvider(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	p := environ.Provider()
	c.Assert(p, gc.NotNil)
}

func (e *environSuite) TestPrecheckInstnace(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	err = environ.PrecheckInstance("", constraints.Value{}, "")
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestInstanceTypes(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	types, err := environ.InstanceTypes(constraints.Value{})
	c.Assert(err, gc.IsNil)
	c.Assert(types, gc.NotNil)
}
