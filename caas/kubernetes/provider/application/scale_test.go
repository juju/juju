// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/constraints"
)

func (s *applicationSuite) TestApplicationScaleStateful(c *tc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, false, false, "", func() {})

	c.Assert(app.Scale(20), tc.ErrorIsNil)
	ss, err := s.client.AppsV1().StatefulSets(s.namespace).Get(
		context.Background(),
		s.appName,
		metav1.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*ss.Spec.Replicas, tc.Equals, int32(20))
}

func (s *applicationSuite) TestApplicationScaleStateless(c *tc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateless, false)
	s.assertEnsure(c, app, false, constraints.Value{}, false, false, "", func() {})

	c.Assert(app.Scale(20), tc.ErrorIsNil)
	dep, err := s.client.AppsV1().Deployments(s.namespace).Get(
		context.Background(),
		s.appName,
		metav1.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*dep.Spec.Replicas, tc.Equals, int32(20))
}

func (s *applicationSuite) TestApplicationScaleStatefulLessThanZero(c *tc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, false, false, "", func() {})

	c.Assert(app.Scale(-1), tc.ErrorIs, errors.NotValid)
}

func (s *applicationSuite) TestCurrentScale(c *tc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, false, false, "", func() {})

	c.Assert(app.Scale(3), tc.ErrorIsNil)

	units, err := app.UnitsToRemove(context.Background(), 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.SameContents, []string{"gitlab/1", "gitlab/2"})

	units, err = app.UnitsToRemove(context.Background(), 3)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 0)
}
