// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"time"

	"github.com/Altoros/gosigma/mock"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/loggo"
	gc "launchpad.net/gocheck"
)

type environInstanceSuite struct {
	testing.BaseSuite
	baseConfig *config.Config
}

var _ = gc.Suite(&environInstanceSuite{})

func (s *environInstanceSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)

	mock.Start()

	attrs := testing.Attrs{
		"name":     "testname",
		"region":   mock.Endpoint(""),
		"username": mock.TestUser,
		"password": mock.TestPassword,
	}
	s.baseConfig = newConfig(c, validAttrs().Merge(attrs))
}

func (s *environInstanceSuite) TearDownSuite(c *gc.C) {
	mock.Stop()
	s.BaseSuite.TearDownSuite(c)
}

func (s *environInstanceSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	ll := logger.LogLevel()
	logger.SetLogLevel(loggo.TRACE)
	s.AddCleanup(func(*gc.C) { logger.SetLogLevel(ll) })

	mock.Reset()
}

func (s *environInstanceSuite) TearDownTest(c *gc.C) {
	mock.Reset()
	s.BaseSuite.TearDownTest(c)
}

func (s *environInstanceSuite) createEnviron(c *gc.C, cfg *config.Config) environs.Environ {
	var emptyStorage environStorage
	s.PatchValue(&newStorage, func(*environConfig, *environClient) (*environStorage, error) {
		return &emptyStorage, nil
	})
	if cfg == nil {
		cfg = s.baseConfig
	}
	environ, err := environs.New(cfg)
	c.Assert(environ, gc.NotNil)
	c.Assert(err, gc.IsNil)
	return environ
}

func (s *environInstanceSuite) TestInstances(c *gc.C) {
	environ := s.createEnviron(c, nil)

	instances, err := environ.AllInstances()
	c.Assert(instances, gc.NotNil)
	c.Assert(err, gc.IsNil)
	c.Check(instances, gc.HasLen, 0)

	uuid0 := addTestClientServer(c, jujuMetaInstanceServer, "testname", "1.1.1.1")
	uuid1 := addTestClientServer(c, jujuMetaInstanceStateServer, "testname", "2.2.2.2")
	addTestClientServer(c, jujuMetaInstanceServer, "other-env", "0.1.1.1")
	addTestClientServer(c, jujuMetaInstanceStateServer, "other-env", "0.2.2.2")

	instances, err = environ.AllInstances()
	c.Assert(instances, gc.NotNil)
	c.Assert(err, gc.IsNil)
	c.Check(instances, gc.HasLen, 2)

	ids := []instance.Id{instance.Id(uuid0), instance.Id(uuid1)}
	instances, err = environ.Instances(ids)
	c.Assert(instances, gc.NotNil)
	c.Assert(err, gc.IsNil)
	c.Check(instances, gc.HasLen, 2)

	ids = append(ids, instance.Id("fake-instance"))
	instances, err = environ.Instances(ids)
	c.Assert(instances, gc.NotNil)
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Check(instances, gc.HasLen, 3)
	c.Check(instances[0], gc.NotNil)
	c.Check(instances[1], gc.NotNil)
	c.Check(instances[2], gc.IsNil)

	err = environ.StopInstances(ids...)
	c.Assert(err, gc.ErrorMatches, "404 Not Found.*")

	instances, err = environ.Instances(ids)
	c.Assert(instances, gc.NotNil)
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
	c.Check(instances, gc.HasLen, 3)
	c.Check(instances[0], gc.IsNil)
	c.Check(instances[1], gc.IsNil)
	c.Check(instances[2], gc.IsNil)
}

func (s *environInstanceSuite) TestInstancesFail(c *gc.C) {
	attrs := testing.Attrs{
		"name":     "testname",
		"region":   "https://0.1.2.3:2000/api/2.0/",
		"username": mock.TestUser,
		"password": mock.TestPassword,
	}
	baseConfig := newConfig(c, validAttrs().Merge(attrs))

	newClientFunc := newClient
	s.PatchValue(&newClient, func(cfg *environConfig) (*environClient, error) {
		cli, err := newClientFunc(cfg)
		if cli != nil {
			cli.conn.ConnectTimeout(10 * time.Millisecond)
		}
		return cli, err
	})

	environ := s.createEnviron(c, baseConfig)

	instances, err := environ.AllInstances()
	c.Assert(instances, gc.IsNil)
	c.Assert(err, gc.NotNil)

	instances, err = environ.Instances([]instance.Id{instance.Id("123"), instance.Id("321")})
	c.Assert(instances, gc.IsNil)
	c.Assert(err, gc.NotNil)
}

func (s *environInstanceSuite) TestAllocateAddress(c *gc.C) {
	environ := s.createEnviron(c, nil)

	addr, err := environ.AllocateAddress(instance.Id(""), network.Id(""))
	c.Check(addr, gc.Equals, network.Address{})
	c.Check(err, gc.ErrorMatches, "AllocateAddress not supported")
}

func (s *environInstanceSuite) TestStartInstanceError(c *gc.C) {
	environ := s.createEnviron(c, nil)

	inst, hw, ni, err := environ.StartInstance(environs.StartInstanceParams{})
	c.Check(inst, gc.IsNil)
	c.Check(hw, gc.IsNil)
	c.Check(ni, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "machine configuration is nil")

	inst, hw, ni, err = environ.StartInstance(environs.StartInstanceParams{
		MachineConfig: &cloudinit.MachineConfig{
			Networks: []string{"value"},
		},
	})
	c.Check(inst, gc.IsNil)
	c.Check(hw, gc.IsNil)
	c.Check(ni, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "starting instances with networks is not supported yet")

	inst, hw, ni, err = environ.StartInstance(environs.StartInstanceParams{
		MachineConfig: &cloudinit.MachineConfig{},
	})
	c.Check(inst, gc.IsNil)
	c.Check(hw, gc.IsNil)
	c.Check(ni, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "tools not found")

	inst, hw, ni, err = environ.StartInstance(environs.StartInstanceParams{
		Tools: tools.List{&tools.Tools{}},
		MachineConfig: &cloudinit.MachineConfig{
			Bootstrap: true,
			APIInfo:   &api.Info{},
		},
	})
	c.Check(inst, gc.IsNil)
	c.Check(hw, gc.IsNil)
	c.Check(ni, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "cannot complete machine configuration:.*")

	inst, hw, ni, err = environ.StartInstance(environs.StartInstanceParams{
		Tools:         tools.List{&tools.Tools{}},
		MachineConfig: &cloudinit.MachineConfig{},
	})
	c.Check(inst, gc.IsNil)
	c.Check(hw, gc.IsNil)
	c.Check(ni, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "failed start instance:.*")
}
