// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"time"

	"github.com/altoros/gosigma/mock"
	"github.com/juju/loggo"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
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
		"uuid":     "f54aac3a-9dcd-4a0c-86b5-24091478478c",
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
	s.PatchValue(&findInstanceImage, func(env *environ, ic *imagemetadata.ImageConstraint) (*imagemetadata.ImageMetadata, error) {
		img := &imagemetadata.ImageMetadata{
			Id: validImageId,
		}
		return img, nil
	})
	if cfg == nil {
		cfg = s.baseConfig
	}
	environ, err := environs.New(cfg)

	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)
	return environ
}

func (s *environInstanceSuite) TestInstances(c *gc.C) {
	env := s.createEnviron(c, nil)

	instances, err := env.AllInstances()
	c.Assert(instances, gc.NotNil)
	c.Assert(err, gc.IsNil)
	c.Check(instances, gc.HasLen, 0)

	uuid0 := addTestClientServer(c, jujuMetaInstanceServer, "f54aac3a-9dcd-4a0c-86b5-24091478478c")
	uuid1 := addTestClientServer(c, jujuMetaInstanceStateServer, "f54aac3a-9dcd-4a0c-86b5-24091478478c")
	addTestClientServer(c, jujuMetaInstanceServer, "other-env")
	addTestClientServer(c, jujuMetaInstanceStateServer, "other-env")

	instances, err = env.AllInstances()
	c.Assert(instances, gc.NotNil)
	c.Assert(err, gc.IsNil)
	c.Check(instances, gc.HasLen, 2)

	ids := []instance.Id{instance.Id(uuid0), instance.Id(uuid1)}
	instances, err = env.Instances(ids)
	c.Assert(instances, gc.NotNil)
	c.Assert(err, gc.IsNil)
	c.Check(instances, gc.HasLen, 2)

	ids = append(ids, instance.Id("fake-instance"))
	instances, err = env.Instances(ids)
	c.Assert(instances, gc.NotNil)
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Check(instances, gc.HasLen, 3)
	c.Check(instances[0], gc.NotNil)
	c.Check(instances[1], gc.NotNil)
	c.Check(instances[2], gc.IsNil)

	err = env.StopInstances(ids...)
	c.Assert(err, gc.ErrorMatches, "404 Not Found.*")

	instances, err = env.Instances(ids)
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

func (s *environInstanceSuite) TestStartInstanceError(c *gc.C) {
	environ := s.createEnviron(c, nil)

	res, err := environ.StartInstance(environs.StartInstanceParams{})
	c.Check(res, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "instance configuration is nil")

	toolsVal := &tools.Tools{
		Version: version.Binary{
			Series: "trusty",
		},
	}
	res, err = environ.StartInstance(environs.StartInstanceParams{
		InstanceConfig: &instancecfg.InstanceConfig{
			Networks: []string{"value"},
			Tools:    toolsVal,
		},
	})
	c.Check(res, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "starting instances with networks is not supported yet")

	res, err = environ.StartInstance(environs.StartInstanceParams{
		InstanceConfig: &instancecfg.InstanceConfig{},
	})
	c.Check(res, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "tools not found")

	res, err = environ.StartInstance(environs.StartInstanceParams{
		Tools:          tools.List{toolsVal},
		InstanceConfig: &instancecfg.InstanceConfig{Tools: toolsVal},
	})
	c.Check(res, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "cannot make user data: series \"\" not valid")
}
