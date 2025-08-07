// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/constraints"
)

func (s *applicationSuite) TestApplicationScaleStateful(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, false, false, "", func() {})

	c.Assert(app.Scale(20), jc.ErrorIsNil)
	ss, err := s.coreClient.AppsV1().StatefulSets(s.namespace).Get(
		context.Background(),
		s.appName,
		metav1.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*ss.Spec.Replicas, gc.Equals, int32(20))
}

func (s *applicationSuite) TestApplicationScaleStateless(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateless, false)
	s.assertEnsure(c, app, false, constraints.Value{}, false, false, "", func() {})

	c.Assert(app.Scale(20), jc.ErrorIsNil)
	dep, err := s.coreClient.AppsV1().Deployments(s.namespace).Get(
		context.Background(),
		s.appName,
		metav1.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*dep.Spec.Replicas, gc.Equals, int32(20))
}

func (s *applicationSuite) TestApplicationScaleStatefulLessThanZero(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, false, false, "", func() {})

	c.Assert(errors.IsNotValid(app.Scale(-1)), jc.IsTrue)
}

func (s *applicationSuite) TestCurrentScale(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, false, false, "", func() {})

	c.Assert(app.Scale(3), jc.ErrorIsNil)

	units, err := app.UnitsToRemove(context.Background(), 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, jc.SameContents, []string{"gitlab/1", "gitlab/2"})

	units, err = app.UnitsToRemove(context.Background(), 3)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)
}
