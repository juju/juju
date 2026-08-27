// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/internal/storage"
)

func (s *applicationSuite) TestApplicationScaleStateful(c *tc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, false, false, "", nil, func() {}, nil)

	c.Assert(app.Scale(20), tc.ErrorIsNil)
	ss, err := s.client.AppsV1().StatefulSets(s.namespace).Get(
		c.Context(),
		s.appName,
		metav1.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*ss.Spec.Replicas, tc.Equals, int32(20))
}

func (s *applicationSuite) TestApplicationScaleStateless(c *tc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateless, false)
	s.assertEnsure(c, app, false, constraints.Value{}, false, false, "", nil, func() {}, nil)

	c.Assert(app.Scale(20), tc.ErrorIsNil)
	dep, err := s.client.AppsV1().Deployments(s.namespace).Get(
		c.Context(),
		s.appName,
		metav1.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*dep.Spec.Replicas, tc.Equals, int32(20))
}

func (s *applicationSuite) TestApplicationScaleStatefulLessThanZero(c *tc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, false, false, "", nil, func() {}, nil)

	c.Assert(app.Scale(-1), tc.ErrorIs, errors.NotValid)
}

func (s *applicationSuite) TestEnsureControllerNonce(c *tc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	key := "controller-nonce-1"
	_, err := s.client.CoreV1().ConfigMaps(s.namespace).Create(c.Context(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: s.appName + "-configmap"},
		Data:       map[string]string{key: "persisted-nonce"},
	}, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	s.client.ClearActions()
	err = app.EnsureControllerNonce(c.Context(), 1, "persisted-nonce")
	c.Assert(err, tc.ErrorIsNil)
	actions := s.client.Actions()
	c.Assert(actions, tc.HasLen, 2)
	c.Check(actions[0].GetVerb(), tc.Equals, "get")
	c.Check(actions[1].GetVerb(), tc.Equals, "get")

	err = app.EnsureControllerNonce(c.Context(), 1, "reconciled-nonce")
	c.Assert(err, tc.ErrorIsNil)
	configMap, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(c.Context(), s.appName+"-configmap", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(configMap.Data[key], tc.Equals, "reconciled-nonce")
}

func (s *applicationSuite) TestEnsureControllerNonceReconcilesConfigMapVolume(c *tc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	configMapName := s.appName + "-configmap"
	_, err := s.client.CoreV1().ConfigMaps(s.namespace).Create(c.Context(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: configMapName},
		Data:       map[string]string{"controller-nonce-1": "persisted-nonce"},
	}, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.client.AppsV1().StatefulSets(s.namespace).Create(c.Context(), &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: s.appName},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Volumes: []corev1.Volume{{
					Name: "agent-conf",
					VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
						Items:                []corev1.KeyToPath{{Key: "controller-nonce-0", Path: "controller-nonce-0"}},
					}},
				}}},
			},
		},
	}, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	s.client.ClearActions()
	err = app.EnsureControllerNonce(c.Context(), 1, "persisted-nonce")
	c.Assert(err, tc.ErrorIsNil)
	actions := s.client.Actions()
	c.Assert(actions, tc.HasLen, 3)
	c.Check(actions[0].GetVerb(), tc.Equals, "get")
	c.Check(actions[1].GetVerb(), tc.Equals, "get")
	c.Check(actions[2].GetVerb(), tc.Equals, "update")

	statefulSet, err := s.client.AppsV1().StatefulSets(s.namespace).Get(c.Context(), s.appName, metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statefulSet.Spec.Template.Spec.Volumes[0].ConfigMap.Items, tc.IsNil)

	s.client.ClearActions()
	err = app.EnsureControllerNonce(c.Context(), 1, "persisted-nonce")
	c.Assert(err, tc.ErrorIsNil)
	actions = s.client.Actions()
	c.Assert(actions, tc.HasLen, 2)
	c.Check(actions[0].GetVerb(), tc.Equals, "get")
	c.Check(actions[1].GetVerb(), tc.Equals, "get")
}

func (s *applicationSuite) TestCurrentScale(c *tc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, false, false, "", nil, func() {}, nil)

	c.Assert(app.Scale(3), tc.ErrorIsNil)

	units, err := app.UnitsToRemove(c.Context(), 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.SameContents, []string{"gitlab/1", "gitlab/2"})

	units, err = app.UnitsToRemove(c.Context(), 3)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 0)
}

func (s *applicationSuite) TestEnsurePVCs(c *tc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, false, false, "", nil, func() {}, nil)

	// Test EnsurePVCs with filesystem params and unit attachments
	filesystems := []storage.KubernetesFilesystemParams{
		{
			StorageName: "database",
			Size:        1024, // 1GiB in MiB
			Provider:    storage.ProviderType("kubernetes"),
			Attributes:  map[string]any{"storage-class": "fast"},
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

	err := app.EnsurePVCs(filesystems, filesystemUnitAttachments, "uniqid")
	c.Assert(err, tc.ErrorIsNil)

	// Verify PVC was created
	pvcList, err := s.client.CoreV1().PersistentVolumeClaims(s.namespace).List(c.Context(), metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pvcList.Items, tc.HasLen, 1)

	pvc := pvcList.Items[0]
	c.Assert(pvc.Spec.VolumeName, tc.Equals, "test-volume-id")
	c.Assert(pvc.Name, tc.Matches, "gitlab-database-uniqid-gitlab-0")
}

func (s *applicationSuite) TestEnsurePVCsWithProvisionedAttachments(c *tc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, false, false, "", nil, func() {}, nil)

	// Test EnsurePVCs with filesystem params and unit attachments
	filesystems := []storage.KubernetesFilesystemParams{
		{
			StorageName: "database",
			Size:        1024, // 1GiB in MiB
			Provider:    storage.ProviderType("kubernetes"),
			Attributes:  map[string]any{"storage-class": "fast"},
			Attachments: []storage.KubernetesFilesystemAttachmentParams{
				{
					ProvisionedPVCNames: []string{"gitlab-database-uniqid-gitlab-0"},
				},
			},
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

	err := app.EnsurePVCs(filesystems, filesystemUnitAttachments, "uniqid")
	c.Assert(err, tc.ErrorIsNil)

	// Verify PVC was created
	pvcList, err := s.client.CoreV1().PersistentVolumeClaims(s.namespace).List(c.Context(), metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pvcList.Items, tc.HasLen, 1)

	pvc := pvcList.Items[0]
	c.Assert(pvc.Spec.VolumeName, tc.Equals, "test-volume-id")
	c.Assert(pvc.Name, tc.Matches, "gitlab-database-uniqid-gitlab-0")
}

func (s *applicationSuite) TestEnsurePVCsUnknownPVCNameFormat(c *tc.C) {
	app, _ := s.getApp(c, caas.DeploymentStateful, false)
	s.assertEnsure(c, app, false, constraints.Value{}, false, false, "", nil, func() {}, nil)

	// Test EnsurePVCs with filesystem params and unit attachments
	filesystems := []storage.KubernetesFilesystemParams{
		{
			StorageName: "database",
			Size:        1024, // 1GiB in MiB
			Provider:    storage.ProviderType("kubernetes"),
			Attributes:  map[string]any{"storage-class": "fast"},
			Attachments: []storage.KubernetesFilesystemAttachmentParams{
				{
					ProvisionedPVCNames: []string{"gitlab-database-uniqid-gitlabunknown-%#$0"},
				},
			},
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

	err := app.EnsurePVCs(filesystems, filesystemUnitAttachments, "uniqid")
	c.Assert(err, tc.ErrorMatches, `mapping pvc template names for app "gitlab".*`)

	// Verify PVC was not created
	pvcList, err := s.client.CoreV1().PersistentVolumeClaims(s.namespace).List(c.Context(), metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pvcList.Items, tc.HasLen, 0)
}
