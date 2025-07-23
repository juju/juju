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
	"github.com/juju/juju/storage"
)

func (s *applicationSuite) TestApplicationScaleStateful(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, nil, false, false, "", func() {})

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
	app, _ := s.getApp(c, caas.DeploymentStateless, false)
	s.assertEnsure(c, app, false, constraints.Value{}, nil, false, false, "", func() {})

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
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, nil, false, false, "", func() {})

	c.Assert(errors.IsNotValid(app.Scale(-1)), jc.IsTrue)
}

func (s *applicationSuite) TestCurrentScale(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, nil, false, false, "", func() {})

	c.Assert(app.Scale(3), jc.ErrorIsNil)

	units, err := app.UnitsToRemove(context.Background(), 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, jc.SameContents, []string{"gitlab/1", "gitlab/2"})

	units, err = app.UnitsToRemove(context.Background(), 3)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)
}

func (s *applicationSuite) TestEnsurePVC(c *gc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, nil, false, false, "", func() {})

	// Test EnsurePVC with filesystem params and unit attachments
	filesystems := []storage.KubernetesFilesystemParams{
		{
			StorageName: "database",
			Size:        1024, // 1GiB in MiB
			Provider:    storage.ProviderType("kubernetes"),
			Attributes:  map[string]interface{}{"storage-class": "fast"},
		},
	}

	filesystemUnitAttachments := map[string][]storage.KubernetesFilesystemUnitAttachmentParams{
		"database": {
			{
				UnitName: "gitlab/0",
				VolumeId: "test-volume-id",
			},
		},
	}

	_, err := app.EnsurePVC(filesystems, filesystemUnitAttachments)
	c.Assert(err, jc.ErrorIsNil)

	// Verify PVC was created
	pvcList, err := s.client.CoreV1().PersistentVolumeClaims(s.namespace).List(context.Background(), metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pvcList.Items, gc.HasLen, 1)

	pvc := pvcList.Items[0]
	c.Assert(pvc.Spec.VolumeName, gc.Equals, "test-volume-id")
	c.Assert(pvc.Name, gc.Matches, "gitlab-database-.*-gitlab-0")
}
