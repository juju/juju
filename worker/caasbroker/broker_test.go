// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasbroker_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasbroker"
	"github.com/juju/juju/worker/workertest"
)

type TrackerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&TrackerSuite{})

func (s *TrackerSuite) TestValidateObserver(c *gc.C) {
	config := caasbroker.Config{}
	s.testValidate(c, config, func(err error) {
		c.Check(err, jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, "nil Observer not valid")
	})
}

func (s *TrackerSuite) TestValidateNewBrokerFunc(c *gc.C) {
	config := caasbroker.Config{
		Observer: &runContext{},
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
			errors.New("no yuo"),
		},
	}
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			Observer:               context,
			NewContainerBrokerFunc: newMockBroker,
		})
		c.Check(err, gc.ErrorMatches, "cannot get cloud information: no yuo")
		c.Check(tracker, gc.IsNil)
		context.CheckCallNames(c, "CloudSpec")
	})
}

func (s *TrackerSuite) TestSuccess(c *gc.C) {
	fix := &fixture{}
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			Observer:               context,
			NewContainerBrokerFunc: newMockBroker,
		})
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.CleanKill(c, tracker)

		gotBroker := tracker.Broker()
		c.Assert(gotBroker, gc.NotNil)
	})
}

func (s *TrackerSuite) TestCloudSpec(c *gc.C) {
	cloudSpec := environs.CloudSpec{
		Name:   "foo",
		Type:   "bar",
		Region: "baz",
	}
	fix := &fixture{cloud: cloudSpec}
	fix.Run(c, func(context *runContext) {
		tracker, err := caasbroker.NewTracker(caasbroker.Config{
			Observer: context,
			NewContainerBrokerFunc: func(spec environs.CloudSpec) (caas.Broker, error) {
				c.Assert(spec, jc.DeepEquals, cloudSpec)
				return nil, errors.NotValidf("cloud spec")
			},
		})
		c.Check(err, gc.ErrorMatches, `cannot create caas broker: cloud spec not valid`)
		c.Check(tracker, gc.IsNil)
		context.CheckCallNames(c, "CloudSpec")
	})
}
