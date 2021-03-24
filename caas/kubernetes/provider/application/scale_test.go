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
	s.assertEnsure(c, caas.DeploymentStateful, constraints.Value{}, func() {})
	app, _ := s.getApp(c, caas.DeploymentStateful, false)

	c.Assert(app.Scale(20), jc.ErrorIsNil)
	ss, err := s.client.AppsV1().StatefulSets(s.namespace).Get(
		context.Background(),
		s.appName,
		metav1.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*ss.Spec.Replicas, gc.Equals, int32(20))
}

func (s *applicationSuite) TestApplicationScaleStateless(c *gc.C) {
	s.assertEnsure(c, caas.DeploymentStateless, constraints.Value{}, func() {})
	app, _ := s.getApp(c, caas.DeploymentStateless, false)

	c.Assert(app.Scale(20), jc.ErrorIsNil)
	dep, err := s.client.AppsV1().Deployments(s.namespace).Get(
		context.Background(),
		s.appName,
		metav1.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*dep.Spec.Replicas, gc.Equals, int32(20))
}

func (s *applicationSuite) TestApplicationScaleStatefulLessThanZero(c *gc.C) {
	s.assertEnsure(c, caas.DeploymentStateful, constraints.Value{}, func() {})
	app, _ := s.getApp(c, caas.DeploymentStateful, false)

	c.Assert(errors.IsNotValid(app.Scale(-1)), jc.IsTrue)
}
