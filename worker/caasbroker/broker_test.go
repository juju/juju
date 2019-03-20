// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasbroker_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasbroker"
)

type TrackerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&TrackerSuite{})

func (s *TrackerSuite) TestValidateObserver(c *gc.C) {
	config := caasbroker.Config{}
	s.testValidate(c, config, func(err error) {
		c.Check(err, jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, "nil ConfigAPI not valid")
	})
}

func (s *TrackerSuite) TestValidateNewBrokerFunc(c *gc.C) {
	config := caasbroker.Config{
		ConfigAPI: &runContext{},
	}
	s.testValidate(c, config, func(err error) {
		c.Check(err, jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, "nil NewContainerBrokerFunc not valid")
	})
}

func (s *TrackerSuite) testValidate(c *gc.C, config caasbroker.Config, check func(err error)) {
	err := config.Validate()
	check(err)

	tracker, err := caasbroker.NewTracker(config)
	c.Check(tracker, gc.IsNil)
	check(err)
}

func (s *TrackerSuite) TestCloudSpecFails(c *gc.C) {
	fix := &fixture{
		observerErrs: []error{
			errors.New("no you"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI:              context,
			NewContainerBrokerFunc: newMockBroker,
		})
		c.Check(err, gc.ErrorMatches, "cannot get cloud information: no you")
		c.Check(tracker, gc.IsNil)
		context.CheckCallNames(c, "CloudSpec")
	})
}

func (s *TrackerSuite) validFixture() *fixture {
	cloudSpec := environs.CloudSpec{
		Name:   "foo",
		Type:   "bar",
		Region: "baz",
	}
	cfg := coretesting.FakeConfig()
	cfg["type"] = "kubernetes"
	cfg["uuid"] = utils.MustNewUUID().String()
	return &fixture{cloud: cloudSpec, config: cfg}
}

func (s *TrackerSuite) TestSuccess(c *gc.C) {
	fix := s.validFixture()
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI:              context,
			NewContainerBrokerFunc: newMockBroker,
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.CleanKill(c, tracker)

		gotBroker := tracker.Broker()
		c.Assert(gotBroker, gc.NotNil)
	})
}

func (s *TrackerSuite) TestInitialise(c *gc.C) {
	fix := s.validFixture()
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			ConfigAPI: context,
			NewContainerBrokerFunc: func(args environs.OpenParams) (caas.Broker, error) {
				c.Assert(args.Cloud, jc.DeepEquals, fix.cloud)
				c.Assert(args.Config.Name(), jc.DeepEquals, "testmodel")
				return nil, errors.NotValidf("cloud spec")
			},
		})
		c.Check(err, gc.ErrorMatches, `cannot create caas broker: cloud spec not valid`)
		c.Check(tracker, gc.IsNil)
		context.CheckCallNames(c, "CloudSpec", "Model", "ControllerConfig")
	})
}
