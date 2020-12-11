// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs_test

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs/ecsiface"
	"github.com/golang/mock/gomock"
	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	provider "github.com/juju/juju/caas/ecs"
	"github.com/juju/juju/caas/ecs/mocks"
	"github.com/juju/juju/cloud"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type baseSuite struct {
	testing.BaseSuite

	clock     *testclock.Clock
	environ   *provider.ECSEnviron
	cfg       *config.Config
	awsConfig *aws.Config

	ecsClient *mocks.MockECSAPI

	clusterName string
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.clusterName = "test-cluster"

	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"access-key":   "access-key",
		"secret-key":   "secret-key",
		"region":       "ap-southeast-2",
		"cluster-name": s.clusterName,
	})
	cloudSpec := environscloudspec.CloudSpec{
		Name:           "ecs",
		Type:           "ecs",
		Endpoint:       "some-host",
		Credential:     &cred,
		CACertificates: []string{testing.CACert},
	}
	var err error
	s.awsConfig, err = provider.CloudSpecToAWSConfig(cloudSpec)
	c.Assert(err, jc.ErrorIsNil)

	// init config for each test for easier changing config inside test.
	s.cfg, err = config.New(config.UseDefaults, testing.FakeConfig().Merge(testing.Attrs{
		config.NameKey: "test",
	}))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *baseSuite) TearDownTest(c *gc.C) {
	s.clock = nil
	s.environ = nil
	s.cfg = nil
	s.awsConfig = nil
	s.ecsClient = nil

	s.BaseSuite.TearDownTest(c)
}

func (s *baseSuite) setupController(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.ecsClient = mocks.NewMockECSAPI(ctrl)
	s.clock = testclock.NewClock(time.Time{})

	var err error
	s.environ, err = provider.NewEnviron(
		testing.ControllerTag.Id(), s.clusterName, s.clock,
		s.cfg, s.awsConfig,
		func(*aws.Config) (ecsiface.ECSAPI, error) {
			return s.ecsClient, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl
}
