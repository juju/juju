// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"time"

	"github.com/altoros/gosigma/mock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

type environInstanceSuite struct {
	testing.BaseSuite
	cloud      environscloudspec.CloudSpec
	baseConfig *config.Config

	callCtx context.ProviderCallContext
}

var _ = gc.Suite(&environInstanceSuite{})

func (s *environInstanceSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)

	mock.Start()

	s.cloud = fakeCloudSpec()
	s.cloud.Endpoint = mock.Endpoint("")

	attrs := testing.Attrs{
		"name": "testname",
		"uuid": "f54aac3a-9dcd-4a0c-86b5-24091478478c",
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
	s.callCtx = context.NewCloudCallContext()
}

func (s *environInstanceSuite) TearDownTest(c *gc.C) {
	mock.Reset()
	s.BaseSuite.TearDownTest(c)
}

func (s *environInstanceSuite) createEnviron(c *gc.C, cfg *config.Config) environs.Environ {
	s.PatchValue(&findInstanceImage, func([]*imagemetadata.ImageMetadata) (*imagemetadata.ImageMetadata, error) {
		img := &imagemetadata.ImageMetadata{
			Id: validImageId,
		}
		return img, nil
	})
	if cfg == nil {
		cfg = s.baseConfig
	}

	environ, err := environs.New(environs.OpenParams{
		Cloud:  s.cloud,
		Config: cfg,
	})

	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)
	return environ
}

func (s *environInstanceSuite) TestInstances(c *gc.C) {
	env := s.createEnviron(c, nil)

	instances, err := env.AllRunningInstances(s.callCtx)
	c.Assert(instances, gc.NotNil)
	c.Assert(err, gc.IsNil)
	c.Check(instances, gc.HasLen, 0)

	uuid0 := addTestClientServer(c, jujuMetaInstanceServer, "f54aac3a-9dcd-4a0c-86b5-24091478478c")
	uuid1 := addTestClientServer(c, jujuMetaInstanceController, "f54aac3a-9dcd-4a0c-86b5-24091478478c")
	addTestClientServer(c, jujuMetaInstanceServer, "other-model")
	addTestClientServer(c, jujuMetaInstanceController, "other-model")

	instances, err = env.AllRunningInstances(s.callCtx)
	c.Assert(instances, gc.NotNil)
	c.Assert(err, gc.IsNil)
	c.Check(instances, gc.HasLen, 2)

	ids := []instance.Id{instance.Id(uuid0), instance.Id(uuid1)}
	instances, err = env.Instances(s.callCtx, ids)
	c.Assert(instances, gc.NotNil)
	c.Assert(err, gc.IsNil)
	c.Check(instances, gc.HasLen, 2)

	ids = append(ids, instance.Id("fake-instance"))
	instances, err = env.Instances(s.callCtx, ids)
	c.Assert(instances, gc.NotNil)
	c.Assert(errors.Cause(err), gc.Equals, environs.ErrPartialInstances)
	c.Check(instances, gc.HasLen, 3)
	c.Check(instances[0], gc.NotNil)
	c.Check(instances[1], gc.NotNil)
	c.Check(instances[2], gc.IsNil)

	err = env.StopInstances(s.callCtx, ids...)
	c.Assert(err, gc.ErrorMatches, "404 Not Found.*")

	instances, err = env.Instances(s.callCtx, ids)
	c.Assert(instances, gc.NotNil)
	c.Assert(errors.Cause(err), gc.Equals, environs.ErrNoInstances)
	c.Check(instances, gc.HasLen, 3)
	c.Check(instances[0], gc.IsNil)
	c.Check(instances[1], gc.IsNil)
	c.Check(instances[2], gc.IsNil)
}

func (s *environInstanceSuite) TestInstancesFail(c *gc.C) {
	newClientFunc := newClient
	s.PatchValue(&newClient, func(spec environscloudspec.CloudSpec, uuid string) (*environClient, error) {
		spec.Endpoint = "https://0.1.2.3:2000/api/2.0/"
		cli, err := newClientFunc(spec, uuid)
		if cli != nil {
			cli.conn.ConnectTimeout(10 * time.Millisecond)
		}
		return cli, err
	})

	environ := s.createEnviron(c, nil)

	instances, err := environ.AllRunningInstances(s.callCtx)
	c.Assert(instances, gc.IsNil)
	c.Assert(err, gc.NotNil)

	instances, err = environ.Instances(s.callCtx, []instance.Id{instance.Id("123"), instance.Id("321")})
	c.Assert(instances, gc.IsNil)
	c.Assert(err, gc.NotNil)
}

func (s *environInstanceSuite) TestStartInstanceError(c *gc.C) {
	environ := s.createEnviron(c, nil)

	res, err := environ.StartInstance(s.callCtx, environs.StartInstanceParams{})
	c.Check(res, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "instance configuration is nil")

	toolsVal := &tools.Tools{
		Version: version.Binary{
			Release: "ubuntu",
		},
		URL: "https://0.1.2.3:2000/x.y.z.tgz",
	}

	res, err = environ.StartInstance(s.callCtx, environs.StartInstanceParams{
		InstanceConfig: &instancecfg.InstanceConfig{},
	})
	c.Check(res, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "agent binaries not found")

	icfg := &instancecfg.InstanceConfig{}
	err = icfg.SetTools(tools.List{toolsVal})
	c.Assert(err, jc.ErrorIsNil)
	res, err = environ.StartInstance(s.callCtx, environs.StartInstanceParams{
		Tools:          tools.List{toolsVal},
		InstanceConfig: icfg,
	})
	c.Check(res, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "cannot make user data: series \"\" not valid")
}
