// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	containerlxd "github.com/juju/juju/internal/container/lxd"
	"github.com/juju/juju/internal/provider/lxd"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type environInstSuite struct {
	lxd.BaseSuite
}

func TestEnvironInstSuite(t *stdtesting.T) {
	tc.Run(t, &environInstSuite{})
}

func (s *environInstSuite) TestInstancesOkay(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	ids := []instance.Id{"spam", "eggs", "ham"}
	var containers []containerlxd.Container
	var expected []instances.Instance
	for _, id := range ids {
		containers = append(containers, *s.NewContainer(c, string(id)))
		expected = append(expected, s.NewInstance(c, string(id)))
	}
	s.Client.Containers = containers

	insts, err := s.Env.Instances(c.Context(), ids)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(insts, tc.DeepEquals, expected)
}

func (s *environInstSuite) TestInstancesAPI(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	ids := []instance.Id{"spam", "eggs", "ham"}
	s.Env.Instances(c.Context(), ids)

	s.Stub.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "AliveContainers",
		Args: []interface{}{
			s.Prefix(),
		},
	}})
}

func (s *environInstSuite) TestInstancesEmptyArg(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	insts, err := s.Env.Instances(c.Context(), nil)

	c.Check(insts, tc.HasLen, 0)
	c.Check(errors.Cause(err), tc.Equals, environs.ErrNoInstances)
}

func (s *environInstSuite) TestInstancesInstancesFailed(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	failure := errors.New("<unknown>")
	s.Stub.SetErrors(failure)

	ids := []instance.Id{"spam"}
	insts, err := s.Env.Instances(c.Context(), ids)

	c.Check(insts, tc.DeepEquals, []instances.Instance{nil})
	c.Check(errors.Cause(err), tc.Equals, failure)
}

func (s *environInstSuite) TestInstancesPartialMatch(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	container := s.NewContainer(c, "spam")
	expected := s.NewInstance(c, "spam")
	s.Client.Containers = []containerlxd.Container{*container}

	ids := []instance.Id{"spam", "eggs"}
	insts, err := s.Env.Instances(c.Context(), ids)

	c.Check(insts, tc.DeepEquals, []instances.Instance{expected, nil})
	c.Check(errors.Cause(err), tc.Equals, environs.ErrPartialInstances)
}

func (s *environInstSuite) TestInstancesNoMatch(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	container := s.NewContainer(c, "spam")
	s.Client.Containers = []containerlxd.Container{*container}

	ids := []instance.Id{"eggs"}
	insts, err := s.Env.Instances(c.Context(), ids)

	c.Check(insts, tc.DeepEquals, []instances.Instance{nil})
	c.Check(errors.Cause(err), tc.Equals, environs.ErrNoInstances)
}

func (s *environInstSuite) TestInstancesInvalidCredentials(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)
	// allInstances will ultimately return the error.
	s.Client.Stub.SetErrors(errTestUnAuth)

	ids := []instance.Id{"eggs"}
	_, err := s.Env.Instances(c.Context(), ids)

	c.Check(err, tc.ErrorMatches, "not authorized")
}

func (s *environInstSuite) TestControllerInstancesOkay(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Client.Containers = []containerlxd.Container{*s.Container}

	ids, err := s.Env.ControllerInstances(c.Context(), coretesting.ControllerTag.Id())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(ids, tc.DeepEquals, []instance.Id{"spam"})
	s.BaseSuite.Client.CheckCallNames(c, "AliveContainers")
	s.BaseSuite.Client.CheckCall(
		c, 0, "AliveContainers", "juju-",
	)
}

func (s *environInstSuite) TestControllerInstancesNotBootstrapped(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	_, err := s.Env.ControllerInstances(c.Context(), "not-used")

	c.Check(err, tc.Equals, environs.ErrNotBootstrapped)
}

func (s *environInstSuite) TestControllerInstancesMixed(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	other := containerlxd.Container{}
	s.Client.Containers = []containerlxd.Container{*s.Container}
	s.Client.Containers = []containerlxd.Container{*s.Container, other}

	ids, err := s.Env.ControllerInstances(c.Context(), coretesting.ControllerTag.Id())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(ids, tc.DeepEquals, []instance.Id{"spam"})
}

func (s *environInstSuite) TestControllerInvalidCredentials(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)

	// AliveContainers will return an error.
	s.Client.Stub.SetErrors(errTestUnAuth)

	_, err := s.Env.ControllerInstances(c.Context(), coretesting.ControllerTag.Id())
	c.Check(err, tc.ErrorMatches, "not authorized")
}

func (s *environInstSuite) TestAdoptResources(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	one := s.NewContainer(c, "smoosh")
	two := s.NewContainer(c, "guild-league")
	three := s.NewContainer(c, "tall-dwarfs")
	s.Client.Containers = []containerlxd.Container{*one, *two, *three}

	err := s.Env.AdoptResources(c.Context(), "target-uuid", semversion.MustParse("3.4.5"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.BaseSuite.Client.Calls(), tc.HasLen, 4)
	s.BaseSuite.Client.CheckCall(c, 0, "AliveContainers", "juju-f75cba-")
	s.BaseSuite.Client.CheckCall(
		c, 1, "UpdateContainerConfig", "smoosh", map[string]string{"user.juju-controller-uuid": "target-uuid"})
	s.BaseSuite.Client.CheckCall(
		c, 2, "UpdateContainerConfig", "guild-league", map[string]string{"user.juju-controller-uuid": "target-uuid"})
	s.BaseSuite.Client.CheckCall(
		c, 3, "UpdateContainerConfig", "tall-dwarfs", map[string]string{"user.juju-controller-uuid": "target-uuid"})
}

func (s *environInstSuite) TestAdoptResourcesError(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	one := s.NewContainer(c, "smoosh")
	two := s.NewContainer(c, "guild-league")
	three := s.NewContainer(c, "tall-dwarfs")
	s.Client.Containers = []containerlxd.Container{*one, *two, *three}
	s.Client.SetErrors(nil, nil, errors.New("blammo"))

	err := s.Env.AdoptResources(c.Context(), "target-uuid", semversion.MustParse("5.3.3"))
	c.Assert(err, tc.ErrorMatches, `failed to update controller for some instances: \[guild-league\]`)
	c.Assert(s.BaseSuite.Client.Calls(), tc.HasLen, 4)
	s.BaseSuite.Client.CheckCall(c, 0, "AliveContainers", "juju-f75cba-")
	s.BaseSuite.Client.CheckCall(
		c, 1, "UpdateContainerConfig", "smoosh", map[string]string{"user.juju-controller-uuid": "target-uuid"})
	s.BaseSuite.Client.CheckCall(
		c, 2, "UpdateContainerConfig", "guild-league", map[string]string{"user.juju-controller-uuid": "target-uuid"})
	s.BaseSuite.Client.CheckCall(
		c, 3, "UpdateContainerConfig", "tall-dwarfs", map[string]string{"user.juju-controller-uuid": "target-uuid"})
}

func (s *environInstSuite) TestAdoptResourcesInvalidResources(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)

	// allInstances will ultimately return the error.
	s.Client.Stub.SetErrors(errTestUnAuth)

	err := s.Env.AdoptResources(c.Context(), "target-uuid", semversion.MustParse("3.4.5"))

	c.Check(err, tc.ErrorMatches, ".*not authorized")
	s.BaseSuite.Client.CheckCall(c, 0, "AliveContainers", "juju-f75cba-")
}
