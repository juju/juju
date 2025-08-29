// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	"github.com/juju/juju/internal/storage"
)

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

type storageSuite struct {
	BaseSuite
}

func (s *storageSuite) k8sProvider(c *tc.C, ctrl *gomock.Controller) storage.Provider {
	return kubernetes.StorageProvider(s.k8sClient, s.getNamespace())
}

func (s *storageSuite) TestValidateConfig(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	p := s.k8sProvider(c, ctrl)
	cfg, err := storage.NewConfig("name", constants.StorageProviderType, map[string]interface{}{
		"storage-class":       "my-storage",
		"storage-provisioner": "aws-storage",
		"storage-label":       "storage-fred",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.Attrs(), tc.DeepEquals, storage.Attrs{
		"storage-class":       "my-storage",
		"storage-provisioner": "aws-storage",
		"storage-label":       "storage-fred",
	})
}

func (s *storageSuite) TestValidateConfigError(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	p := s.k8sProvider(c, ctrl)
	cfg, err := storage.NewConfig("name", constants.StorageProviderType, map[string]interface{}{
		"storage-class":       "",
		"storage-provisioner": "aws-storage",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, tc.ErrorMatches, "storage-class must be specified if storage-provisioner is specified")
}

func (s *storageSuite) TestSupports(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	p := s.k8sProvider(c, ctrl)
	c.Assert(p.Supports(storage.StorageKindBlock), tc.IsTrue)
	c.Assert(p.Supports(storage.StorageKindFilesystem), tc.IsFalse)
}

func (s *storageSuite) TestScope(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	p := s.k8sProvider(c, ctrl)
	c.Assert(p.Scope(), tc.Equals, storage.ScopeEnviron)
}

func (s *storageSuite) TestDestroyVolumes(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockPersistentVolumes.EXPECT().Get(gomock.Any(), "vol-1", v1.GetOptions{}).
			Return(&core.PersistentVolume{
				Spec: core.PersistentVolumeSpec{
					ClaimRef: &core.ObjectReference{Namespace: "test", Name: "vol-1-pvc"},
				}}, nil),
		s.mockPersistentVolumeClaims.EXPECT().Delete(gomock.Any(), "vol-1-pvc", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),
		s.mockPersistentVolumes.EXPECT().Delete(gomock.Any(), "vol-1", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(nil),
	)

	p := s.k8sProvider(c, ctrl)
	vs, err := p.VolumeSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	errs, err := vs.DestroyVolumes(c.Context(), []string{"vol-1"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.DeepEquals, []error{nil})
}

func (s *storageSuite) TestDestroyVolumesNotFoundIgnored(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockPersistentVolumes.EXPECT().Get(gomock.Any(), "vol-1", v1.GetOptions{}).
			Return(&core.PersistentVolume{
				Spec: core.PersistentVolumeSpec{
					ClaimRef: &core.ObjectReference{Namespace: "test", Name: "vol-1-pvc"},
				}}, nil),
		s.mockPersistentVolumeClaims.EXPECT().Delete(gomock.Any(), "vol-1-pvc", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),
		s.mockPersistentVolumes.EXPECT().Delete(gomock.Any(), "vol-1", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),
	)

	p := s.k8sProvider(c, ctrl)
	vs, err := p.VolumeSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	errs, err := vs.DestroyVolumes(c.Context(), []string{"vol-1"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.DeepEquals, []error{nil})
}

func (s *storageSuite) TestListVolumes(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockPersistentVolumes.EXPECT().List(gomock.Any(), v1.ListOptions{}).
			Return(&core.PersistentVolumeList{Items: []core.PersistentVolume{
				{ObjectMeta: v1.ObjectMeta{Name: "vol-1"}}}}, nil),
	)

	p := s.k8sProvider(c, ctrl)
	vs, err := p.VolumeSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	vols, err := vs.ListVolumes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vols, tc.DeepEquals, []string{"vol-1"})
}

func (s *storageSuite) TestDescribeVolumes(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockPersistentVolumes.EXPECT().List(gomock.Any(), v1.ListOptions{}).
			Return(&core.PersistentVolumeList{Items: []core.PersistentVolume{
				{ObjectMeta: v1.ObjectMeta{Name: "vol-id"},
					Spec: core.PersistentVolumeSpec{
						PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain,
						Capacity:                      core.ResourceList{core.ResourceStorage: resource.MustParse("100Mi")}},
				}}}, nil),
	)

	p := s.k8sProvider(c, ctrl)
	vs, err := p.VolumeSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := vs.DescribeVolumes(c.Context(), []string{"vol-id"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []storage.DescribeVolumesResult{{
		VolumeInfo: &storage.VolumeInfo{VolumeId: "vol-id", Size: 66, Persistent: true},
	}})
}

func (s *storageSuite) TestValidateStorageProvider(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	prov := s.k8sProvider(c, ctrl)

	for _, t := range []struct {
		attrs map[string]interface{}
		err   string
	}{
		{
			attrs: map[string]interface{}{"storage-medium": "foo"},
			err:   `storage medium "foo" not valid`,
		},
		{
			attrs: nil,
		},
	} {
		err := prov.ValidateForK8s(t.attrs)
		if t.err == "" {
			c.Check(err, tc.ErrorIsNil)
		} else {
			c.Check(err, tc.ErrorMatches, t.err)
		}
	}
}

func (s *storageSuite) TestImportVolume(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"

	s.mockPersistentVolumes.EXPECT().
		Get(gomock.Any(), volId, v1.GetOptions{}).
		Return(
			&core.PersistentVolume{
				ObjectMeta: v1.ObjectMeta{Name: volId},
				Spec:       core.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain},
			}, nil)
	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(c.Context(), volId, "", make(map[string]string), false)
	c.Check(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestImportVolumeNotFound(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"

	s.mockPersistentVolumes.EXPECT().
		Get(gomock.Any(), volId, v1.GetOptions{}).
		Return(
			&core.PersistentVolume{
				ObjectMeta: v1.ObjectMeta{Name: volId},
				Spec:       core.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain},
			}, s.k8sNotFoundError())
	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(c.Context(), volId, "", make(map[string]string), false)
	c.Check(err, tc.ErrorMatches, "persistent volume \"fakeVolId\" not found")
}

func (s *storageSuite) TestImportVolumeInvalidReclaimPolicy(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"

	s.mockPersistentVolumes.EXPECT().
		Get(gomock.Any(), volId, v1.GetOptions{}).
		Return(
			&core.PersistentVolume{
				ObjectMeta: v1.ObjectMeta{Name: volId},
				Spec:       core.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimDelete},
			}, nil)
	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(c.Context(), volId, "", make(map[string]string), false)

	c.Check(err, tc.ErrorMatches, "importing volume \"fakeVolId\" with reclaim policy \"Delete\" not supported \\(must be \"Retain\"\\)")
}

func (s *storageSuite) TestImportVolumeAlreadyBound(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"

	s.mockPersistentVolumes.EXPECT().
		Get(gomock.Any(), volId, v1.GetOptions{}).
		Return(
			&core.PersistentVolume{
				ObjectMeta: v1.ObjectMeta{Name: volId},
				Spec: core.PersistentVolumeSpec{
					PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain,
					ClaimRef:                      &core.ObjectReference{},
				},
			}, nil)
	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(c.Context(), volId, "", make(map[string]string), false)
	c.Check(err, tc.ErrorMatches, "importing volume \"fakeVolId\" already bound to a claim not supported")
}

func (s *storageSuite) TestImportVolumeWithForce(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"
	pvcName := "my-pvc"

	// Mock PV that is bound to a PVC and has Delete reclaim policy
	pv := &core.PersistentVolume{
		ObjectMeta: v1.ObjectMeta{Name: volId},
		Spec: core.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimDelete,
			ClaimRef: &core.ObjectReference{
				Name:      pvcName,
				Namespace: s.namespace,
			},
		},
	}

	// Mock PVC that will be retrieved and validated before deletion
	pvc := &core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:      pvcName,
			Namespace: s.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "juju",
				"storage.juju.is/name":         "test-storage",
			},
		},
	}

	gomock.InOrder(
		s.mockPersistentVolumes.EXPECT().
			Get(gomock.Any(), volId, v1.GetOptions{}).
			Return(pv, nil),
		// First patch: Set reclaim policy to Retain
		s.mockPersistentVolumes.EXPECT().
			Patch(gomock.Any(), volId, types.StrategicMergePatchType, gomock.Any(), v1.PatchOptions{FieldManager: resources.JujuFieldManager}).
			Return(nil, nil),
		// Get PV again to verify reclaim policy was set
		s.mockPersistentVolumes.EXPECT().
			Get(gomock.Any(), volId, v1.GetOptions{}).
			Return(&core.PersistentVolume{
				ObjectMeta: v1.ObjectMeta{Name: volId},
				Spec: core.PersistentVolumeSpec{
					PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain,
					ClaimRef: &core.ObjectReference{
						Name:      pvcName,
						Namespace: s.namespace,
					},
				},
			}, nil),
		s.mockPersistentVolumeClaims.EXPECT().
			Get(gomock.Any(), pvcName, v1.GetOptions{}).
			Return(pvc, nil),
		s.mockPersistentVolumeClaims.EXPECT().
			Delete(gomock.Any(), pvcName, v1.DeleteOptions{}).
			Return(nil),
		// Second patch: Clear claimRef
		s.mockPersistentVolumes.EXPECT().
			Patch(gomock.Any(), volId, types.StrategicMergePatchType, gomock.Any(), v1.PatchOptions{FieldManager: resources.JujuFieldManager}).
			Return(nil, nil),
	)

	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(c.Context(), volId, "test-storage", make(map[string]string), true)
	c.Check(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestImportVolumeWithForceDeletePVCNotFound(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"
	pvcName := "my-pvc"

	// Mock PV that is bound to a PVC (but PVC doesn't exist)
	pv := &core.PersistentVolume{
		ObjectMeta: v1.ObjectMeta{
			Name: volId,
		},
		Spec: core.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain,
			ClaimRef: &core.ObjectReference{
				Name:      pvcName,
				Namespace: s.namespace,
			},
		},
	}

	gomock.InOrder(
		s.mockPersistentVolumes.EXPECT().
			Get(gomock.Any(), volId, v1.GetOptions{}).
			Return(pv, nil),
		s.mockPersistentVolumeClaims.EXPECT().
			Get(gomock.Any(), pvcName, v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockPersistentVolumes.EXPECT().
			Patch(gomock.Any(), volId, types.StrategicMergePatchType, gomock.Any(), v1.PatchOptions{FieldManager: resources.JujuFieldManager}).
			Return(nil, nil),
	)

	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(c.Context(), volId, "test-storage", make(map[string]string), true)
	c.Check(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestImportVolumeWithForceDeletePVCError(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"
	pvcName := "my-pvc"

	// Mock PV that is bound to a PVC
	pv := &core.PersistentVolume{
		ObjectMeta: v1.ObjectMeta{Name: volId},
		Spec: core.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain,
			ClaimRef: &core.ObjectReference{
				Name:      pvcName,
				Namespace: s.namespace,
			},
		},
	}

	// Mock PVC that will be retrieved and validated before deletion
	pvc := &core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:      pvcName,
			Namespace: s.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "juju",
				"storage.juju.is/name":         "test-storage",
			},
		},
	}

	gomock.InOrder(
		s.mockPersistentVolumes.EXPECT().
			Get(gomock.Any(), volId, v1.GetOptions{}).
			Return(pv, nil),
		s.mockPersistentVolumeClaims.EXPECT().
			Get(gomock.Any(), pvcName, v1.GetOptions{}).
			Return(pvc, nil),
		s.mockPersistentVolumeClaims.EXPECT().
			Delete(gomock.Any(), pvcName, v1.DeleteOptions{}).
			Return(errors.New("failed to delete PVC my-pvc")),
	)

	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(c.Context(), volId, "test-storage", make(map[string]string), true)
	c.Check(err, tc.ErrorMatches, "failed to delete PVC test/my-pvc: failed to delete PVC my-pvc")
}

func (s *storageSuite) TestImportVolumeWithForceUpdatePVError(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"
	pvcName := "my-pvc"

	// Mock PV that is bound to a PVC
	pv := &core.PersistentVolume{
		ObjectMeta: v1.ObjectMeta{Name: volId},
		Spec: core.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain,
			ClaimRef: &core.ObjectReference{
				Name:      pvcName,
				Namespace: s.namespace,
			},
		},
	}

	// Mock PVC that will be retrieved and validated before deletion
	pvc := &core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:      pvcName,
			Namespace: s.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "juju",
				"storage.juju.is/name":         "test-storage",
			},
		},
	}

	gomock.InOrder(
		s.mockPersistentVolumes.EXPECT().
			Get(gomock.Any(), volId, v1.GetOptions{}).
			Return(pv, nil),
		s.mockPersistentVolumeClaims.EXPECT().
			Get(gomock.Any(), pvcName, v1.GetOptions{}).
			Return(pvc, nil),
		s.mockPersistentVolumeClaims.EXPECT().
			Delete(gomock.Any(), pvcName, v1.DeleteOptions{}).
			Return(nil),
		s.mockPersistentVolumes.EXPECT().
			Patch(gomock.Any(), volId, types.StrategicMergePatchType, gomock.Any(), v1.PatchOptions{FieldManager: resources.JujuFieldManager}).
			Return(nil, errors.New("failed to patch PV my-pv")),
	)

	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(c.Context(), volId, "test-storage", make(map[string]string), true)
	c.Check(err, tc.ErrorMatches, "failed to patch PersistentVolume fakeVolId: failed to patch PV my-pv")
}

func (s *storageSuite) TestImportVolumeWithForceNoModificationsNeeded(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"

	// Mock PV that already has correct reclaim policy and no claimRef
	pv := &core.PersistentVolume{
		ObjectMeta: v1.ObjectMeta{Name: volId},
		Spec: core.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain,
			ClaimRef:                      nil,
		},
	}

	s.mockPersistentVolumes.EXPECT().
		Get(gomock.Any(), volId, v1.GetOptions{}).
		Return(pv, nil)

	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(c.Context(), volId, "test-storage", make(map[string]string), true)
	c.Check(err, tc.ErrorIsNil)
}
