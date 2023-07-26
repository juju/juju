// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	containerlxd "github.com/juju/juju/container/lxd"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/lxd"
	coretesting "github.com/juju/juju/testing"
)

type environInstSuite struct {
	lxd.BaseSuite
}

var _ = gc.Suite(&environInstSuite{})

func (s *environInstSuite) TestInstancesOkay(c *gc.C) {
	ids := []instance.Id{"spam", "eggs", "ham"}
	var containers []containerlxd.Container
	var expected []instances.Instance
	for _, id := range ids {
		containers = append(containers, *s.NewContainer(c, string(id)))
		expected = append(expected, s.NewInstance(c, string(id)))
	}
	s.Client.Containers = containers

	insts, err := s.Env.Instances(context.NewEmptyCloudCallContext(), ids)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(insts, jc.DeepEquals, expected)
}

func (s *environInstSuite) TestInstancesAPI(c *gc.C) {
	ids := []instance.Id{"spam", "eggs", "ham"}
	s.Env.Instances(context.NewEmptyCloudCallContext(), ids)

	s.Stub.CheckCalls(c, []jujutesting.StubCall{{
		FuncName: "AliveContainers",
		Args: []interface{}{
			s.Prefix(),
		},
	}})
}

func (s *environInstSuite) TestInstancesEmptyArg(c *gc.C) {
	insts, err := s.Env.Instances(context.NewEmptyCloudCallContext(), nil)

	c.Check(insts, gc.HasLen, 0)
	c.Check(errors.Cause(err), gc.Equals, environs.ErrNoInstances)
}

func (s *environInstSuite) TestInstancesInstancesFailed(c *gc.C) {
	failure := errors.New("<unknown>")
	s.Stub.SetErrors(failure)

	ids := []instance.Id{"spam"}
	insts, err := s.Env.Instances(context.NewEmptyCloudCallContext(), ids)

	c.Check(insts, jc.DeepEquals, []instances.Instance{nil})
	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *environInstSuite) TestInstancesPartialMatch(c *gc.C) {
	container := s.NewContainer(c, "spam")
	expected := s.NewInstance(c, "spam")
	s.Client.Containers = []containerlxd.Container{*container}

	ids := []instance.Id{"spam", "eggs"}
	insts, err := s.Env.Instances(context.NewEmptyCloudCallContext(), ids)

	c.Check(insts, jc.DeepEquals, []instances.Instance{expected, nil})
	c.Check(errors.Cause(err), gc.Equals, environs.ErrPartialInstances)
}

func (s *environInstSuite) TestInstancesNoMatch(c *gc.C) {
	container := s.NewContainer(c, "spam")
	s.Client.Containers = []containerlxd.Container{*container}

	ids := []instance.Id{"eggs"}
	insts, err := s.Env.Instances(context.NewEmptyCloudCallContext(), ids)

	c.Check(insts, jc.DeepEquals, []instances.Instance{nil})
	c.Check(errors.Cause(err), gc.Equals, environs.ErrNoInstances)
}

func (s *environInstSuite) TestInstancesInvalidCredentials(c *gc.C) {
	var invalidCred = false
	// allInstances will ultimately return the error.
	s.Client.Stub.SetErrors(errTestUnAuth)

	ids := []instance.Id{"eggs"}
	_, err := s.Env.Instances(&context.CloudCallContext{
		InvalidateCredentialFunc: func(string) error {
			invalidCred = true
			return nil
		},
	}, ids)

	c.Check(err, gc.ErrorMatches, "not authorized")
	c.Assert(invalidCred, jc.IsTrue)
}

func (s *environInstSuite) TestControllerInstancesOkay(c *gc.C) {
	s.Client.Containers = []containerlxd.Container{*s.Container}

	ids, err := s.Env.ControllerInstances(context.NewEmptyCloudCallContext(), coretesting.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ids, jc.DeepEquals, []instance.Id{"spam"})
	s.BaseSuite.Client.CheckCallNames(c, "AliveContainers")
	s.BaseSuite.Client.CheckCall(
		c, 0, "AliveContainers", "juju-",
	)
}

func (s *environInstSuite) TestControllerInstancesNotBootstrapped(c *gc.C) {
	_, err := s.Env.ControllerInstances(context.NewEmptyCloudCallContext(), "not-used")

	c.Check(err, gc.Equals, environs.ErrNotBootstrapped)
}

func (s *environInstSuite) TestControllerInstancesMixed(c *gc.C) {
	other := containerlxd.Container{}
	s.Client.Containers = []containerlxd.Container{*s.Container}
	s.Client.Containers = []containerlxd.Container{*s.Container, other}

	ids, err := s.Env.ControllerInstances(context.NewEmptyCloudCallContext(), coretesting.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ids, jc.DeepEquals, []instance.Id{"spam"})
}

func (s *environInstSuite) TestControllerInvalidCredentials(c *gc.C) {
	var invalidCred = false
	// AliveContainers will return an error.
	s.Client.Stub.SetErrors(errTestUnAuth)

	_, err := s.Env.ControllerInstances(
		&context.CloudCallContext{
			InvalidateCredentialFunc: func(string) error {
				invalidCred = true
				return nil
			},
		}, coretesting.ControllerTag.Id())
	c.Check(err, gc.ErrorMatches, "not authorized")
	c.Assert(invalidCred, jc.IsTrue)
}

func (s *environInstSuite) TestAdoptResources(c *gc.C) {
	one := s.NewContainer(c, "smoosh")
	two := s.NewContainer(c, "guild-league")
	three := s.NewContainer(c, "tall-dwarfs")
	s.Client.Containers = []containerlxd.Container{*one, *two, *three}

	err := s.Env.AdoptResources(context.NewEmptyCloudCallContext(), "target-uuid", version.MustParse("3.4.5"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.BaseSuite.Client.Calls(), gc.HasLen, 4)
	s.BaseSuite.Client.CheckCall(c, 0, "AliveContainers", "juju-f75cba-")
	s.BaseSuite.Client.CheckCall(
		c, 1, "UpdateContainerConfig", "smoosh", map[string]string{"user.juju-controller-uuid": "target-uuid"})
	s.BaseSuite.Client.CheckCall(
		c, 2, "UpdateContainerConfig", "guild-league", map[string]string{"user.juju-controller-uuid": "target-uuid"})
	s.BaseSuite.Client.CheckCall(
		c, 3, "UpdateContainerConfig", "tall-dwarfs", map[string]string{"user.juju-controller-uuid": "target-uuid"})
}

func (s *environInstSuite) TestAdoptResourcesError(c *gc.C) {
	one := s.NewContainer(c, "smoosh")
	two := s.NewContainer(c, "guild-league")
	three := s.NewContainer(c, "tall-dwarfs")
	s.Client.Containers = []containerlxd.Container{*one, *two, *three}
	s.Client.SetErrors(nil, nil, errors.New("blammo"))

	err := s.Env.AdoptResources(context.NewEmptyCloudCallContext(), "target-uuid", version.MustParse("5.3.3"))
	c.Assert(err, gc.ErrorMatches, `failed to update controller for some instances: \[guild-league\]`)
	c.Assert(s.BaseSuite.Client.Calls(), gc.HasLen, 4)
	s.BaseSuite.Client.CheckCall(c, 0, "AliveContainers", "juju-f75cba-")
	s.BaseSuite.Client.CheckCall(
		c, 1, "UpdateContainerConfig", "smoosh", map[string]string{"user.juju-controller-uuid": "target-uuid"})
	s.BaseSuite.Client.CheckCall(
		c, 2, "UpdateContainerConfig", "guild-league", map[string]string{"user.juju-controller-uuid": "target-uuid"})
	s.BaseSuite.Client.CheckCall(
		c, 3, "UpdateContainerConfig", "tall-dwarfs", map[string]string{"user.juju-controller-uuid": "target-uuid"})
}

func (s *environInstSuite) TestAdoptResourcesInvalidResources(c *gc.C) {
	var invalidCred = false
	// allInstances will ultimately return the error.
	s.Client.Stub.SetErrors(errTestUnAuth)

	err := s.Env.AdoptResources(&context.CloudCallContext{
		InvalidateCredentialFunc: func(string) error {
			invalidCred = true
			return nil
		},
	}, "target-uuid", version.MustParse("3.4.5"))

	c.Check(err, gc.ErrorMatches, ".*not authorized")
	c.Assert(invalidCred, jc.IsTrue)
	s.BaseSuite.Client.CheckCall(c, 0, "AliveContainers", "juju-f75cba-")
}
