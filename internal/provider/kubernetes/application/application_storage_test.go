// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/internal/provider/kubernetes/application"
	"github.com/juju/juju/storage"
)

// TestEnsureStorage deletes and reapplies a statefulset because it
// detects the current state of the world has not yet reached the desired state.
// In other words, the volume claim template in the current statefulset
// is different to the constructed volume claim template in the latest
// filesystems.
func (s *applicationSuite) TestEnsureStorage(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateful, false)
	defer ctrl.Finish()

	// The new storage has a size of 500 while the current storage has a size
	// of 100. This triggers a storage update because of a difference in size.
	filesystems := []storage.KubernetesFilesystemParams{
		{
			StorageName: "database",
			Size:        500,
			Provider:    "kubernetes",
			Attributes:  map[string]interface{}{"storage-class": "workload-storage"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			ResourceTags: map[string]string{"foo": "bar"},
		},
	}

	// Create the current sts with a pvc of size 100.
	_, err := s.client.AppsV1().StatefulSets("test").
		Create(context.Background(), &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gitlab",
				Namespace: "test",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "gitlab",
					"app.kubernetes.io/managed-by": "juju",
				},
				Annotations: map[string]string{
					"juju.is/version":  "3.5-beta1",
					"app.juju.is/uuid": "appuuid",
				},
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: pointer.Int32Ptr(3),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app.kubernetes.io/name": "gitlab",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      map[string]string{"app.kubernetes.io/name": "gitlab"},
						Annotations: map[string]string{"juju.is/version": "3.5-beta1"},
					},
					Spec: getPodSpec31(),
				},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "gitlab-database-appuuid",
							Labels: map[string]string{
								"storage.juju.is/name":         "database",
								"app.kubernetes.io/managed-by": "juju",
							},
							Annotations: map[string]string{
								"foo":                  "bar",
								"storage.juju.is/name": "database",
							}},
						Spec: corev1.PersistentVolumeClaimSpec{
							StorageClassName: pointer.StringPtr("test-workload-storage"),
							AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: k8sresource.MustParse("100Mi"),
								},
							},
						},
					},
				},
				PodManagementPolicy: appsv1.ParallelPodManagement,
				ServiceName:         "gitlab-endpoints",
			},
		}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	storageClassName := "test-workload-storage"
	pvcSpec := &corev1.PersistentVolumeClaimSpec{
		StorageClassName: &storageClassName,
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: k8sresource.MustParse("100Mi"),
			},
		},
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
	}
	expectedPVCBeforeUpdate := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gitlab-database-appuuid",
			Labels: map[string]string{
				"storage.juju.is/name":         "database",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{
				"foo":                  "bar",
				"storage.juju.is/name": "database",
			},
		},
		Spec: *pvcSpec,
	}
	// The expected pvc has size of 500Mi.
	newPVCSpec := &corev1.PersistentVolumeClaimSpec{
		StorageClassName: &storageClassName,
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: k8sresource.MustParse("500Mi"),
			},
		},
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
	}
	expectedPVCAfterUpdate := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gitlab-database-appuuid",
			Labels: map[string]string{
				"storage.juju.is/name":         "database",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{
				"foo":                  "bar",
				"storage.juju.is/name": "database",
			},
		},
		Spec: *newPVCSpec,
	}

	saveReplicaCount := func(appName string, replicaCount int) error {
		c.Assert(appName, gc.Equals, "gitlab")
		c.Assert(replicaCount, gc.Equals, 3)
		return nil
	}

	// Check volume claim template size before ensure storage. It should have size 100Mi.
	sts, err := s.client.AppsV1().StatefulSets("test").Get(context.Background(), "gitlab", metav1.GetOptions{})
	c.Assert(err, gc.IsNil)
	c.Assert(sts.Spec.VolumeClaimTemplates, gc.DeepEquals, []corev1.PersistentVolumeClaim{*expectedPVCBeforeUpdate})

	err = app.EnsureStorage(caas.ApplicationConfig{
		Filesystems:     filesystems,
		StorageUniqueID: "appuuid",
	}, saveReplicaCount)
	c.Assert(err, jc.ErrorIsNil)

	// Now after ensure storage, the volume claim template size is updated to 500Mi.
	sts, err = s.client.AppsV1().StatefulSets("test").Get(context.Background(), "gitlab", metav1.GetOptions{})
	c.Assert(err, gc.IsNil)
	c.Assert(sts.Spec.VolumeClaimTemplates, gc.DeepEquals, []corev1.PersistentVolumeClaim{*expectedPVCAfterUpdate})
}

// TestEnsureStorageMatchesDesiredStorage does not perform a storage update
// because there are no storage changes between the storage directives
// and the PVC in the statefulset. The current state of the world matches the
// desired state.
func (s *applicationSuite) TestEnsureStorageMatchesDesiredStorage(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateful, false)
	defer ctrl.Finish()

	// Current filesystem does not change. The size is still 100 same as what's in the
	// current statefulset.
	filesystems := []storage.KubernetesFilesystemParams{
		{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attributes:  map[string]interface{}{"storage-class": "workload-storage"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			ResourceTags: map[string]string{"foo": "bar"},
		},
	}
	// Create the current sts with a pvc of size 100.
	_, err := s.client.AppsV1().StatefulSets("test").
		Create(context.Background(), &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gitlab",
				Namespace: "test",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "gitlab",
					"app.kubernetes.io/managed-by": "juju",
				},
				Annotations: map[string]string{
					"juju.is/version":  "3.5-beta1",
					"app.juju.is/uuid": "appuuid",
				},
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: pointer.Int32Ptr(3),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app.kubernetes.io/name": "gitlab",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      map[string]string{"app.kubernetes.io/name": "gitlab"},
						Annotations: map[string]string{"juju.is/version": "3.5-beta1"},
					},
					Spec: getPodSpec31(),
				},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "gitlab-database-appuuid",
							Labels: map[string]string{
								"storage.juju.is/name":         "database",
								"app.kubernetes.io/managed-by": "juju",
							},
							Annotations: map[string]string{
								"foo":                  "bar",
								"storage.juju.is/name": "database",
							}},
						Spec: corev1.PersistentVolumeClaimSpec{
							StorageClassName: pointer.StringPtr("test-workload-storage"),
							AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: k8sresource.MustParse("100Mi"),
								},
							},
						},
					},
				},
				PodManagementPolicy: appsv1.ParallelPodManagement,
				ServiceName:         "gitlab-endpoints",
			},
		}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	storageClassName := "test-workload-storage"
	pvcSpec := &corev1.PersistentVolumeClaimSpec{
		StorageClassName: &storageClassName,
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: k8sresource.MustParse("100Mi"),
			},
		},
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
	}
	expectedPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gitlab-database-appuuid",
			Labels: map[string]string{
				"storage.juju.is/name":         "database",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{
				"foo":                  "bar",
				"storage.juju.is/name": "database",
			},
		},
		Spec: *pvcSpec,
	}

	saveReplicaCount := func(_ string, _ int) error {
		c.Fatal("saveReplicaCount should not be called because there is no storage change")
		return nil
	}

	// Check volume claim template size is 100Mi before update.
	sts, err := s.client.AppsV1().StatefulSets("test").Get(context.Background(), "gitlab", metav1.GetOptions{})
	c.Assert(err, gc.IsNil)
	c.Assert(sts.Spec.VolumeClaimTemplates, gc.DeepEquals, []corev1.PersistentVolumeClaim{*expectedPVC})

	err = app.EnsureStorage(caas.ApplicationConfig{
		Filesystems:     filesystems,
		StorageUniqueID: "appuuid",
	}, saveReplicaCount)
	c.Assert(err, jc.ErrorIsNil)

	// Check volume claim template size is still at 100Mi after update.
	sts, err = s.client.AppsV1().StatefulSets("test").Get(context.Background(), "gitlab", metav1.GetOptions{})
	c.Assert(err, gc.IsNil)
	c.Assert(sts.Spec.VolumeClaimTemplates, gc.DeepEquals, []corev1.PersistentVolumeClaim{*expectedPVC})
}

// TestEnsureStorageMissingStatefulset creates a statefulset from scratch
// because when updating the storage it identifies that a statefulset is missing.
func (s *applicationSuite) TestEnsureStorageMissingStatefulset(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateful, false)
	defer ctrl.Finish()

	appConfig, _, _ := s.createAppConfig(false,
		constraints.Value{}, true, false, defaultAgentVersion)

	saveReplicaCount := func(_ string, _ int) error {
		c.Fatal("saveReplicaCount should not be called because there is no storage change")
		return nil
	}

	storageClassName := "test-workload-storage"
	pvcSpec := &corev1.PersistentVolumeClaimSpec{
		StorageClassName: &storageClassName,
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: k8sresource.MustParse("100Mi"),
			},
		},
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
	}
	expectedPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gitlab-database-appuuid",
			Labels: map[string]string{
				"storage.juju.is/name":         "database",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{
				"foo":                  "bar",
				"storage.juju.is/name": "database",
			},
		},
		Spec: *pvcSpec,
	}

	_, err := s.client.AppsV1().StatefulSets("test").Get(context.Background(), "gitlab", metav1.GetOptions{})
	c.Assert(k8serrors.IsNotFound(err), gc.Equals, true)

	err = app.EnsureStorage(appConfig, saveReplicaCount)
	c.Assert(err, jc.ErrorIsNil)

	sts, err := s.client.AppsV1().StatefulSets("test").Get(context.Background(), "gitlab", metav1.GetOptions{})
	c.Assert(err, gc.IsNil)
	c.Assert(sts.Spec.VolumeClaimTemplates, gc.DeepEquals, []corev1.PersistentVolumeClaim{*expectedPVC})
}

// TestEnsureStorageFailToCreateStatefulset is a sad case where the saving replica
// returns an error causing the statefulset to not update.
func (s *applicationSuite) TestEnsureStorageFailToCreateStatefulset(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateful, false)
	defer ctrl.Finish()

	// New storage size of 500.
	filesystems := []storage.KubernetesFilesystemParams{
		{
			StorageName: "database",
			Size:        500,
			Provider:    "kubernetes",
			Attributes:  map[string]interface{}{"storage-class": "workload-storage"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			ResourceTags: map[string]string{"foo": "bar"},
		},
	}

	saveReplicaCount := func(_ string, count int) error {
		c.Assert(count, gc.Equals, 3)
		return errors.New("unexpected error")
	}

	storageClassName := "test-workload-storage"
	pvcSpec := &corev1.PersistentVolumeClaimSpec{
		StorageClassName: &storageClassName,
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: k8sresource.MustParse("100Mi"),
			},
		},
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
	}
	expectedPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gitlab-database-appuuid",
			Labels: map[string]string{
				"storage.juju.is/name":         "database",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{
				"foo":                  "bar",
				"storage.juju.is/name": "database",
			},
		},
		Spec: *pvcSpec,
	}

	// Create the current sts with a pvc of size 100.
	_, err := s.client.AppsV1().StatefulSets("test").
		Create(context.Background(), &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gitlab",
				Namespace: "test",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "gitlab",
					"app.kubernetes.io/managed-by": "juju",
				},
				Annotations: map[string]string{
					"juju.is/version":  "3.5-beta1",
					"app.juju.is/uuid": "appuuid",
				},
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: pointer.Int32Ptr(3),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app.kubernetes.io/name": "gitlab",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      map[string]string{"app.kubernetes.io/name": "gitlab"},
						Annotations: map[string]string{"juju.is/version": "3.5-beta1"},
					},
					Spec: getPodSpec31(),
				},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "gitlab-database-appuuid",
							Labels: map[string]string{
								"storage.juju.is/name":         "database",
								"app.kubernetes.io/managed-by": "juju",
							},
							Annotations: map[string]string{
								"foo":                  "bar",
								"storage.juju.is/name": "database",
							}},
						Spec: corev1.PersistentVolumeClaimSpec{
							StorageClassName: pointer.StringPtr("test-workload-storage"),
							AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: k8sresource.MustParse("100Mi"),
								},
							},
						},
					},
				},
				PodManagementPolicy: appsv1.ParallelPodManagement,
				ServiceName:         "gitlab-endpoints",
			},
		}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Check volume claim template size is at 100Mi before update.
	stsBeforeEnsure, err := s.client.AppsV1().StatefulSets("test").Get(context.Background(), "gitlab", metav1.GetOptions{})
	c.Assert(err, gc.IsNil)
	c.Assert(stsBeforeEnsure.Spec.VolumeClaimTemplates, gc.DeepEquals, []corev1.PersistentVolumeClaim{*expectedPVC})

	err = app.EnsureStorage(caas.ApplicationConfig{
		Filesystems:     filesystems,
		StorageUniqueID: "appuuid",
	}, saveReplicaCount)
	c.Assert(err, gc.ErrorMatches, `saving statefulset "gitlab" replica count: unexpected error`)

	// Check volume claim template size doesn't change after ensure storage
	// because we encountered an error.
	stsAfterEnsure, err := s.client.AppsV1().StatefulSets("test").Get(context.Background(), "gitlab", metav1.GetOptions{})
	c.Assert(err, gc.IsNil)
	c.Assert(stsAfterEnsure.Spec.VolumeClaimTemplates, gc.DeepEquals, stsBeforeEnsure.Spec.VolumeClaimTemplates)

}

func (s *applicationSuite) TestVolumeClaimTemplateMatch(c *gc.C) {
	storageClass := "sc-fast"
	otherStorageClass := "sc-slow"

	pvc := func(
		name string,
		size string,
		storageClassName *string,
		modes ...corev1.PersistentVolumeAccessMode,
	) corev1.PersistentVolumeClaim {
		requests := corev1.ResourceList{}
		if size != "" {
			requests[corev1.ResourceStorage] = k8sresource.MustParse(size)
		}
		return corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: storageClassName,
				AccessModes:      modes,
				Resources: corev1.VolumeResourceRequirements{
					Requests: requests,
				},
			},
		}
	}

	testCases := []struct {
		name    string
		current []corev1.PersistentVolumeClaim
		desired []corev1.PersistentVolumeClaim
		want    bool
	}{
		{
			name: "matches with different claim and access-mode order",
			current: []corev1.PersistentVolumeClaim{
				pvc("data-b", "1Gi", &storageClass, corev1.ReadWriteOnce, corev1.ReadOnlyMany),
				pvc("data-a", "2Gi", &storageClass, corev1.ReadWriteMany),
			},
			desired: []corev1.PersistentVolumeClaim{
				pvc("data-a", "2Gi", &storageClass, corev1.ReadWriteMany),
				pvc("data-b", "1Gi", &storageClass, corev1.ReadOnlyMany, corev1.ReadWriteOnce),
			},
			want: true,
		},
		{
			name: "different size does not match",
			current: []corev1.PersistentVolumeClaim{
				pvc("data-a", "1Gi", &storageClass, corev1.ReadWriteOnce),
			},
			desired: []corev1.PersistentVolumeClaim{
				pvc("data-a", "2Gi", &storageClass, corev1.ReadWriteOnce),
			},
			want: false,
		},
		{
			name: "different storage class does not match",
			current: []corev1.PersistentVolumeClaim{
				pvc("data-a", "1Gi", &storageClass, corev1.ReadWriteOnce),
			},
			desired: []corev1.PersistentVolumeClaim{
				pvc("data-a", "1Gi", &otherStorageClass, corev1.ReadWriteOnce),
			},
			want: false,
		},
		{
			name: "nil and non-nil storage class does not match",
			current: []corev1.PersistentVolumeClaim{
				pvc("data-a", "1Gi", nil, corev1.ReadWriteOnce),
			},
			desired: []corev1.PersistentVolumeClaim{
				pvc("data-a", "1Gi", &storageClass, corev1.ReadWriteOnce),
			},
			want: false,
		},
		{
			name: "different access modes do not match",
			current: []corev1.PersistentVolumeClaim{
				pvc("data-a", "1Gi", &storageClass, corev1.ReadWriteOnce),
			},
			desired: []corev1.PersistentVolumeClaim{
				pvc("data-a", "1Gi", &storageClass, corev1.ReadWriteMany),
			},
			want: false,
		},
		{
			name: "missing claim does not match",
			current: []corev1.PersistentVolumeClaim{
				pvc("data-a", "1Gi", &storageClass, corev1.ReadWriteOnce),
			},
			desired: []corev1.PersistentVolumeClaim{
				pvc("data-a", "1Gi", &storageClass, corev1.ReadWriteOnce),
				pvc("data-b", "1Gi", &storageClass, corev1.ReadWriteOnce),
			},
			want: false,
		},
		{
			name: "missing storage request matches when both absent",
			current: []corev1.PersistentVolumeClaim{
				pvc("data-a", "", &storageClass, corev1.ReadWriteOnce),
			},
			desired: []corev1.PersistentVolumeClaim{
				pvc("data-a", "", &storageClass, corev1.ReadWriteOnce),
			},
			want: true,
		},
	}

	for _, tc := range testCases {
		c.Assert(application.VolumeClaimTemplateMatch(tc.current, tc.desired), gc.Equals, tc.want, gc.Commentf(tc.name))
	}
}
