// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/storage"
)

var _ = gc.Suite(&storageSuite{})

type storageSuite struct {
	BaseSuite
}

func (s *storageSuite) k8sProvider(c *gc.C, ctrl *gomock.Controller) storage.Provider {
	return provider.StorageProvider(s.k8sClient, s.getNamespace())
}

func (s *storageSuite) TestValidateConfig(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	p := s.k8sProvider(c, ctrl)
	cfg, err := storage.NewConfig("name", constants.StorageProviderType, map[string]interface{}{
		"storage-class":       "my-storage",
		"storage-provisioner": "aws-storage",
		"storage-label":       "storage-fred",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Attrs(), jc.DeepEquals, storage.Attrs{
		"storage-class":       "my-storage",
		"storage-provisioner": "aws-storage",
		"storage-label":       "storage-fred",
	})
}

func (s *storageSuite) TestValidateConfigError(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	p := s.k8sProvider(c, ctrl)
	cfg, err := storage.NewConfig("name", constants.StorageProviderType, map[string]interface{}{
		"storage-class":       "",
		"storage-provisioner": "aws-storage",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, gc.ErrorMatches, "storage-class must be specified if storage-provisioner is specified")
}

func (s *storageSuite) TestSupports(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	p := s.k8sProvider(c, ctrl)
	c.Assert(p.Supports(storage.StorageKindBlock), jc.IsTrue)
	c.Assert(p.Supports(storage.StorageKindFilesystem), jc.IsFalse)
}

func (s *storageSuite) TestScope(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	p := s.k8sProvider(c, ctrl)
	c.Assert(p.Scope(), gc.Equals, storage.ScopeEnviron)
}

func (s *storageSuite) TestDestroyVolumes(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	errs, err := vs.DestroyVolumes(&context.CloudCallContext{}, []string{"vol-1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})
}

func (s *storageSuite) TestDestroyVolumesNotFoundIgnored(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	errs, err := vs.DestroyVolumes(&context.CloudCallContext{}, []string{"vol-1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})
}

func (s *storageSuite) TestListVolumes(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockPersistentVolumes.EXPECT().List(gomock.Any(), v1.ListOptions{}).
			Return(&core.PersistentVolumeList{Items: []core.PersistentVolume{
				{ObjectMeta: v1.ObjectMeta{Name: "vol-1"}}}}, nil),
	)

	p := s.k8sProvider(c, ctrl)
	vs, err := p.VolumeSource(&storage.Config{})
	c.Assert(err, jc.ErrorIsNil)

	vols, err := vs.ListVolumes(&context.CloudCallContext{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vols, jc.DeepEquals, []string{"vol-1"})
}

func (s *storageSuite) TestDescribeVolumes(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	result, err := vs.DescribeVolumes(&context.CloudCallContext{}, []string{"vol-id"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []storage.DescribeVolumesResult{{
		VolumeInfo: &storage.VolumeInfo{VolumeId: "vol-id", Size: 66, Persistent: true},
	}})
}

func (s *storageSuite) TestValidateStorageProvider(c *gc.C) {
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
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, t.err)
		}
	}
}

func (s *storageSuite) TestImportVolume(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"

	s.mockPersistentVolumes.EXPECT().
		Get(gomock.Any(), volId, v1.GetOptions{}).
		Return(
			&core.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: volId},
				Spec:       core.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain},
			}, nil)
	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, jc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(&context.CloudCallContext{}, volId, make(map[string]string), false)
	c.Check(err, jc.ErrorIsNil)
}

func (s *storageSuite) TestImportVolumeNotFound(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"

	s.mockPersistentVolumes.EXPECT().
		Get(gomock.Any(), volId, v1.GetOptions{}).
		Return(
			&core.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: volId},
				Spec:       core.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain},
			}, s.k8sNotFoundError())
	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, jc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(&context.CloudCallContext{}, volId, make(map[string]string), false)
	c.Check(err, gc.ErrorMatches, "persistent volume \"fakeVolId\" not found")
}

func (s *storageSuite) TestImportVolumeInvalidReclaimPolicy(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"

	s.mockPersistentVolumes.EXPECT().
		Get(gomock.Any(), volId, v1.GetOptions{}).
		Return(
			&core.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: volId},
				Spec:       core.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimDelete},
			}, nil)
	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, jc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(&context.CloudCallContext{}, volId, make(map[string]string), false)

	c.Check(err, gc.ErrorMatches, "importing volume \"fakeVolId\" with reclaim policy \"Delete\" not supported \\(must be \"Retain\"\\)")
}

func (s *storageSuite) TestImportVolumeAlreadyBound(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"

	s.mockPersistentVolumes.EXPECT().
		Get(gomock.Any(), volId, v1.GetOptions{}).
		Return(
			&core.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: volId},
				Spec: core.PersistentVolumeSpec{
					PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain,
					ClaimRef:                      &core.ObjectReference{},
				},
			}, nil)
	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, jc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(&context.CloudCallContext{}, volId, make(map[string]string), false)
	c.Check(err, gc.ErrorMatches, "importing volume \"fakeVolId\" already bound to a claim not supported")
}

func (s *storageSuite) TestImportVolumeWithForce(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"
	pvcName := "my-pvc"

	// Mock PV that is bound to a PVC and has Delete reclaim policy
	pv := &core.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: volId},
		Spec: core.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimDelete,
			ClaimRef: &core.ObjectReference{
				Name:      pvcName,
				Namespace: s.namespace,
			},
		},
	}

	// Expected PV after force import (reclaim policy changed to Retain, claimRef cleared)
	updatedPV := &core.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: volId},
		Spec: core.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain,
			ClaimRef:                      nil,
		},
	}

	// Mock PVC that will be retrieved and validated before deletion
	pvc := &core.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: s.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "juju",
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
			Update(gomock.Any(), updatedPV, v1.UpdateOptions{}).
			Return(updatedPV, nil),
	)

	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, jc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(&context.CloudCallContext{}, volId, make(map[string]string), true)
	c.Check(err, jc.ErrorIsNil)
}

func (s *storageSuite) TestImportVolumeWithForceDeletePVCNotFound(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"
	pvcName := "my-pvc"

	// Mock PV that is bound to a PVC (but PVC doesn't exist)
	pv := &core.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: volId},
		Spec: core.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain,
			ClaimRef: &core.ObjectReference{
				Name:      pvcName,
				Namespace: s.namespace,
			},
		},
	}

	// Expected PV after force import (claimRef cleared)
	updatedPV := &core.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: volId},
		Spec: core.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain,
			ClaimRef:                      nil,
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
			Update(gomock.Any(), updatedPV, v1.UpdateOptions{}).
			Return(updatedPV, nil),
	)

	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, jc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(&context.CloudCallContext{}, volId, make(map[string]string), true)
	c.Check(err, jc.ErrorIsNil)
}

func (s *storageSuite) TestImportVolumeWithForceDeletePVCError(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"
	pvcName := "my-pvc"

	// Mock PV that is bound to a PVC
	pv := &core.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: volId},
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
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: s.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "juju",
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
	c.Assert(err, jc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(&context.CloudCallContext{}, volId, make(map[string]string), true)
	c.Check(err, gc.ErrorMatches, "failed to delete PVC test/my-pvc: failed to delete PVC my-pvc")
}

func (s *storageSuite) TestImportVolumeWithForceUpdatePVError(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"
	pvcName := "my-pvc"

	// Mock PV that is bound to a PVC
	pv := &core.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: volId},
		Spec: core.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain,
			ClaimRef: &core.ObjectReference{
				Name:      pvcName,
				Namespace: s.namespace,
			},
		},
	}

	// Expected PV after force import (claimRef cleared)
	updatedPV := &core.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: volId},
		Spec: core.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain,
			ClaimRef:                      nil,
		},
	}

	// Mock PVC that will be retrieved and validated before deletion
	pvc := &core.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: s.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "juju",
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
			Update(gomock.Any(), updatedPV, v1.UpdateOptions{}).
			Return(nil, errors.New("failed to update PV my-pv")),
	)

	prov := s.k8sProvider(c, ctrl)
	vs, err := prov.VolumeSource(&storage.Config{})
	c.Assert(err, jc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(&context.CloudCallContext{}, volId, make(map[string]string), true)
	c.Check(err, gc.ErrorMatches, "failed to update PersistentVolume fakeVolId: failed to update PV my-pv")
}

func (s *storageSuite) TestImportVolumeWithForceNoModificationsNeeded(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	volId := "fakeVolId"

	// Mock PV that already has correct reclaim policy and no claimRef
	pv := &core.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: volId},
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
	c.Assert(err, jc.ErrorIsNil)

	_, err = vs.(storage.VolumeImporter).
		ImportVolume(&context.CloudCallContext{}, volId, make(map[string]string), true)
	c.Check(err, jc.ErrorIsNil)
}
