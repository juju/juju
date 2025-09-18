// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	core "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/storage"
)

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

type storageSuite struct {
	BaseSuite
}

func (s *storageSuite) k8sProvider() storage.Provider {
	return kubernetes.StorageProvider(s.k8sClient, s.getNamespace())
}

func (s *storageSuite) TestValidateConfig(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	p := s.k8sProvider()
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

	p := s.k8sProvider()
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

	p := s.k8sProvider()
	c.Assert(p.Supports(storage.StorageKindBlock), tc.IsFalse)
	c.Assert(p.Supports(storage.StorageKindFilesystem), tc.IsTrue)
}

func (s *storageSuite) TestScope(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	p := s.k8sProvider()
	c.Assert(p.Scope(), tc.Equals, storage.ScopeEnviron)
}

func (s *storageSuite) TestDestroyFilesystems(c *tc.C) {
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

	p := s.k8sProvider()
	fc, err := p.FilesystemSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	errs, err := fc.DestroyFilesystems(c.Context(), []string{"vol-1"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.DeepEquals, []error{nil})
}

func (s *storageSuite) TestDestroyFilesystemsNotFoundIgnored(c *tc.C) {
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

	p := s.k8sProvider()
	fc, err := p.FilesystemSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	errs, err := fc.DestroyFilesystems(c.Context(), []string{"vol-1"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.DeepEquals, []error{nil})
}

func (s *storageSuite) TestValidateStorageProvider(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	prov := s.k8sProvider()

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

func (s *storageSuite) TestImportFilesystem(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	fsId := "fakeFSId"

	s.mockPersistentVolumes.EXPECT().
		Get(gomock.Any(), fsId, v1.GetOptions{}).
		Return(
			&core.PersistentVolume{
				ObjectMeta: v1.ObjectMeta{Name: fsId},
				Spec:       core.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain},
			}, nil)
	prov := s.k8sProvider()
	fc, err := prov.FilesystemSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	_, err = fc.(storage.FilesystemImporter).
		ImportFilesystem(c.Context(), fsId, make(map[string]string))
	c.Check(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestImportFilesystemNotFound(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	fsId := "fakeFSId"

	s.mockPersistentVolumes.EXPECT().
		Get(gomock.Any(), fsId, v1.GetOptions{}).
		Return(
			&core.PersistentVolume{
				ObjectMeta: v1.ObjectMeta{Name: fsId},
				Spec:       core.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain},
			}, s.k8sNotFoundError())
	prov := s.k8sProvider()
	fc, err := prov.FilesystemSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	_, err = fc.(storage.FilesystemImporter).
		ImportFilesystem(c.Context(), fsId, make(map[string]string))
	c.Check(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *storageSuite) TestImportFilesystemInvalidReclaimPolicy(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	fsId := "fakeFSId"

	s.mockPersistentVolumes.EXPECT().
		Get(gomock.Any(), fsId, v1.GetOptions{}).
		Return(
			&core.PersistentVolume{
				ObjectMeta: v1.ObjectMeta{Name: fsId},
				Spec:       core.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimDelete},
			}, nil)
	prov := s.k8sProvider()
	fc, err := prov.FilesystemSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	_, err = fc.(storage.FilesystemImporter).
		ImportFilesystem(c.Context(), fsId, make(map[string]string))
	c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *storageSuite) TestImportFilesystemAlreadyBound(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	fsId := "fakeFSId"

	s.mockPersistentVolumes.EXPECT().
		Get(gomock.Any(), fsId, v1.GetOptions{}).
		Return(
			&core.PersistentVolume{
				ObjectMeta: v1.ObjectMeta{Name: fsId},
				Spec: core.PersistentVolumeSpec{
					PersistentVolumeReclaimPolicy: core.PersistentVolumeReclaimRetain,
					ClaimRef:                      &core.ObjectReference{},
				},
			}, nil)
	prov := s.k8sProvider()
	fc, err := prov.FilesystemSource(&storage.Config{})
	c.Assert(err, tc.ErrorIsNil)

	_, err = fc.(storage.FilesystemImporter).
		ImportFilesystem(c.Context(), fsId, make(map[string]string))
	c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
}
