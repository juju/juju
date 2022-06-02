// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/storage"
	storageprovider "github.com/juju/juju/storage/provider"
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
		VolumeInfo: &storage.VolumeInfo{VolumeId: "vol-id", Size: 68, Persistent: true},
	}})
}

func (s *storageSuite) TestValidateStorageProvider(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	c1, _ := storage.NewConfig("c1",
		storageprovider.RootfsProviderType,
		nil)
	c2, _ := storage.NewConfig("c2",
		storageprovider.TmpfsProviderType,
		nil)
	c3, _ := storage.NewConfig("c3",
		storageprovider.LoopProviderType,
		nil)
	c4, _ := storage.NewConfig("c4",
		storageprovider.TmpfsProviderType,
		map[string]interface{}{"storage-medium": "foo"})

	for _, t := range []struct {
		config *storage.Config
		err    string
	}{
		{
			config: c1,
		},
		{
			config: c2,
		},
		{
			config: c3,
			err:    `storage provider type "loop" not valid`,
		},
		{
			config: c4,
			err:    `storage medium "foo" not valid`,
		},
	} {
		p := s.k8sProvider(c, ctrl)

		err := p.ValidateConfig(t.config)
		if t.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, t.err)
		}
	}

}
