// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

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
	cfg, err := storage.NewConfig("name", constants.StorageProviderType, map[string]any{
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
	cfg, err := storage.NewConfig("name", constants.StorageProviderType, map[string]any{
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
		attrs map[string]any
		err   string
	}{
		{
			attrs: map[string]any{"storage-medium": "foo"},
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
	c.Skip("TODO(storage): re-implement filesystem importing")
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

	_, err = fc.(storage.FilesystemImporter).ImportFilesystem(
		c.Context(), fsId, "mydata", make(map[string]string), false)
	c.Check(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestImportFilesystemNotFound(c *tc.C) {
	c.Skip("TODO(storage): re-implement filesystem importing")
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

	_, err = fc.(storage.FilesystemImporter).ImportFilesystem(
		c.Context(), fsId, "mydata", make(map[string]string), false)
	c.Check(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *storageSuite) TestImportFilesystemInvalidReclaimPolicy(c *tc.C) {
	c.Skip("TODO(storage): re-implement filesystem importing")
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

	_, err = fc.(storage.FilesystemImporter).ImportFilesystem(
		c.Context(), fsId, "mydata", make(map[string]string), false)
	c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *storageSuite) TestImportFilesystemAlreadyBound(c *tc.C) {
	c.Skip("TODO(storage): re-implement filesystem importing")
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

	_, err = fc.(storage.FilesystemImporter).ImportFilesystem(
		c.Context(), fsId, "mydata", make(map[string]string), false)
	c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
}

// TestAttachFilesystems tests that the correct mount path corresponds to the
// path that is mounted inside the charm container.
func (s *storageSuite) TestAttachFilesystems(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	prov := s.k8sProvider()
	fs, err := prov.FilesystemSource(nil)
	c.Check(err, tc.ErrorIsNil)

	providerId := "vault-k8s-certs-dd246a-vault-k8s-0"
	params := []storage.FilesystemAttachmentParams{
		{
			AttachmentParams: storage.AttachmentParams{
				Provider:   "kubernetes",
				ProviderId: &providerId,
				Machine:    names.NewMachineTag("unit-vault-k8s-0"),
				InstanceId: "vault-k8s-0",
				ReadOnly:   false,
			},
			Filesystem:           names.NewFilesystemTag("1"),
			FilesystemProviderId: "pvc-8b255462-6fe1-4308-a6a7-dfb46f41d62b",
			Path:                 "/var/lib/juju/storage/09f5ee7d-4cb7-4866-876a-4216014e1283",
		},
	}

	s.mockPersistentVolumeClaims.EXPECT().Get(
		gomock.Any(),
		*params[0].ProviderId,
		gomock.Any(),
	).Return(&core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{Name: *params[0].ProviderId},
	}, nil)
	s.mockPods.EXPECT().Get(
		gomock.Any(),
		params[0].InstanceId.String(),
		gomock.Any(),
	).Return(&core.Pod{
		Spec: core.PodSpec{
			Containers: []core.Container{
				{
					Name: "charm",
					VolumeMounts: []core.VolumeMount{
						{
							MountPath: "/var/lib/pebble/default",
							Name:      "charm-data",
						},
						{
							MountPath: "/var/log/juju",
							Name:      "charm-data",
						},
						{
							MountPath: "/var/lib/juju/storage/certs-0",
							Name:      "vault-k8s-certs-dd246a",
						},
					},
				},
			},
			Volumes: []core.Volume{
				{
					Name: "vault-k8s-certs-dd246a",
					VolumeSource: core.VolumeSource{
						PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
							ClaimName: providerId,
						},
					},
				},
			},
		},
	}, nil)

	res, err := fs.AttachFilesystems(c.Context(), params)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(res, tc.HasLen, 1)
	c.Check(res[0].Error, tc.ErrorIsNil)
	c.Check(res[0].FilesystemAttachment, tc.DeepEquals, &storage.FilesystemAttachment{
		Filesystem: params[0].Filesystem,
		Machine:    params[0].Machine,
		FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
			Path:     "/var/lib/juju/storage/certs-0",
			ReadOnly: false,
		},
	})
}

// TestAttachFilesystemsErrorMissingProviderID tests that it should indicate
// an error if the provider ID is missing.
func (s *storageSuite) TestAttachFilesystemsErrorMissingProviderID(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	prov := s.k8sProvider()
	fs, err := prov.FilesystemSource(nil)
	c.Check(err, tc.ErrorIsNil)

	params := []storage.FilesystemAttachmentParams{
		{
			AttachmentParams: storage.AttachmentParams{
				Provider:   "kubernetes",
				ProviderId: nil,
				Machine:    names.NewMachineTag("unit-vault-k8s-0"),
				InstanceId: "vault-k8s-0",
				ReadOnly:   false,
			},
			Filesystem:           names.NewFilesystemTag("1"),
			FilesystemProviderId: "pvc-8b255462-6fe1-4308-a6a7-dfb46f41d62b",
			Path:                 "/var/lib/juju/storage/09f5ee7d-4cb7-4866-876a-4216014e1283",
		},
	}

	res, err := fs.AttachFilesystems(c.Context(), params)
	c.Check(err, tc.ErrorIsNil)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 1)
	c.Check(res[0].Error, tc.ErrorIs, storage.FilesystemAttachParamsIncomplete)
}

// TestAttachFilesystemsErrorMissingInstanceID tests that it should indicate
// an error if the instance ID is missing.
func (s *storageSuite) TestAttachFilesystemsErrorMissingInstanceID(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	prov := s.k8sProvider()
	fs, err := prov.FilesystemSource(nil)
	c.Check(err, tc.ErrorIsNil)

	providerId := "vault-k8s-certs-dd246a-vault-k8s-0"
	params := []storage.FilesystemAttachmentParams{
		{
			AttachmentParams: storage.AttachmentParams{
				Provider:   "kubernetes",
				ProviderId: &providerId,
				Machine:    names.NewMachineTag("unit-vault-k8s-0"),
				InstanceId: "",
				ReadOnly:   false,
			},
			Filesystem:           names.NewFilesystemTag("1"),
			FilesystemProviderId: "pvc-8b255462-6fe1-4308-a6a7-dfb46f41d62b",
			Path:                 "/var/lib/juju/storage/09f5ee7d-4cb7-4866-876a-4216014e1283",
		},
	}

	res, err := fs.AttachFilesystems(c.Context(), params)
	c.Check(err, tc.ErrorIsNil)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 1)
	c.Check(res[0].Error, tc.ErrorIs, storage.FilesystemAttachParamsIncomplete)
}

// TestAttachFilesystemsErrorMissingPod tests that it should indicate
// an error if the kubernetes pod couldn't be found.
func (s *storageSuite) TestAttachFilesystemsErrorMissingPod(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	prov := s.k8sProvider()
	fs, err := prov.FilesystemSource(nil)
	c.Check(err, tc.ErrorIsNil)

	providerId := "vault-k8s-certs-dd246a-vault-k8s-0"
	params := []storage.FilesystemAttachmentParams{
		{
			AttachmentParams: storage.AttachmentParams{
				Provider:   "kubernetes",
				ProviderId: &providerId,
				Machine:    names.NewMachineTag("unit-vault-k8s-0"),
				InstanceId: "vault-k8s-0",
				ReadOnly:   false,
			},
			Filesystem:           names.NewFilesystemTag("1"),
			FilesystemProviderId: "pvc-8b255462-6fe1-4308-a6a7-dfb46f41d62b",
			Path:                 "/var/lib/juju/storage/09f5ee7d-4cb7-4866-876a-4216014e1283",
		},
	}

	s.mockPersistentVolumeClaims.EXPECT().Get(
		gomock.Any(),
		*params[0].ProviderId,
		gomock.Any(),
	).Return(&core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{Name: *params[0].ProviderId},
	}, nil)
	s.mockPods.EXPECT().Get(
		gomock.Any(),
		params[0].InstanceId.String(),
		gomock.Any(),
	).Return(nil, k8serrors.NewNotFound(schema.GroupResource{},
		params[0].InstanceId.String()))

	res, err := fs.AttachFilesystems(c.Context(), params)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 1)
	c.Check(res[0].Error, tc.ErrorMatches, `.* kubernetes Pod "vault-k8s-0" not found`)
}

// TestAttachFilesystemsErrorMissingCharmContainer tests that it should indicate
// an error if the charm container couldn't be found in the pod spec.
func (s *storageSuite) TestAttachFilesystemsErrorMissingCharmContainer(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	prov := s.k8sProvider()
	fs, err := prov.FilesystemSource(nil)
	c.Check(err, tc.ErrorIsNil)

	providerId := "vault-k8s-certs-dd246a-vault-k8s-0"
	params := []storage.FilesystemAttachmentParams{
		{
			AttachmentParams: storage.AttachmentParams{
				Provider:   "kubernetes",
				ProviderId: &providerId,
				Machine:    names.NewMachineTag("unit-vault-k8s-0"),
				InstanceId: "vault-k8s-0",
				ReadOnly:   false,
			},
			Filesystem:           names.NewFilesystemTag("1"),
			FilesystemProviderId: "pvc-8b255462-6fe1-4308-a6a7-dfb46f41d62b",
			Path:                 "/var/lib/juju/storage/09f5ee7d-4cb7-4866-876a-4216014e1283",
		},
	}

	s.mockPersistentVolumeClaims.EXPECT().Get(
		gomock.Any(),
		*params[0].ProviderId,
		gomock.Any(),
	).Return(&core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{Name: *params[0].ProviderId},
	}, nil)
	s.mockPods.EXPECT().Get(
		gomock.Any(),
		params[0].InstanceId.String(),
		gomock.Any(),
	).Return(&core.Pod{
		Spec: core.PodSpec{
			Containers: []core.Container{
				{
					Name: "vault",
					VolumeMounts: []core.VolumeMount{
						{
							MountPath: "/var/lib/pebble/default",
							Name:      "charm-data",
						},
						{
							MountPath: "/var/log/juju",
							Name:      "charm-data",
						},
						{
							MountPath: "/var/lib/juju/storage/certs-0",
							Name:      "vault-k8s-certs-dd246a",
						},
					},
				},
			},
			Volumes: []core.Volume{
				{
					Name: "vault-k8s-certs-dd246a",
					VolumeSource: core.VolumeSource{
						PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
							ClaimName: providerId,
						},
					},
				},
			},
		},
	}, nil)

	res, err := fs.AttachFilesystems(c.Context(), params)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 1)
	c.Check(res[0].Error, tc.ErrorMatches, `.* missing charm container`)
}

// TestAttachFilesystemsErrorMissingVolume tests that it should indicate
// an error if the volume couldn't be found by matching the given pvcName (providerId).
func (s *storageSuite) TestAttachFilesystemsErrorMissingVolume(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	prov := s.k8sProvider()
	fs, err := prov.FilesystemSource(nil)
	c.Check(err, tc.ErrorIsNil)

	providerId := "vault-k8s-certs-dd246a-vault-k8s-0"
	params := []storage.FilesystemAttachmentParams{
		{
			AttachmentParams: storage.AttachmentParams{
				Provider:   "kubernetes",
				ProviderId: &providerId,
				Machine:    names.NewMachineTag("unit-vault-k8s-0"),
				InstanceId: "vault-k8s-0",
				ReadOnly:   false,
			},
			Filesystem:           names.NewFilesystemTag("1"),
			FilesystemProviderId: "pvc-8b255462-6fe1-4308-a6a7-dfb46f41d62b",
			Path:                 "/var/lib/juju/storage/09f5ee7d-4cb7-4866-876a-4216014e1283",
		},
	}

	s.mockPersistentVolumeClaims.EXPECT().Get(
		gomock.Any(),
		*params[0].ProviderId,
		gomock.Any(),
	).Return(&core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{Name: *params[0].ProviderId},
	}, nil)
	s.mockPods.EXPECT().Get(
		gomock.Any(),
		params[0].InstanceId.String(),
		gomock.Any(),
	).Return(&core.Pod{
		Spec: core.PodSpec{
			Containers: []core.Container{
				{
					Name: "charm",
					VolumeMounts: []core.VolumeMount{
						{
							MountPath: "/var/lib/pebble/default",
							Name:      "charm-data",
						},
						{
							MountPath: "/var/log/juju",
							Name:      "charm-data",
						},
						{
							MountPath: "/var/lib/juju/storage/certs-0",
							Name:      "vault-k8s-certs-dd246a",
						},
					},
				},
			},
			Volumes: []core.Volume{
				{
					Name: "vault-k8s-certs-dd246a",
					VolumeSource: core.VolumeSource{
						PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
							ClaimName: "claim-name-doesnt-match",
						},
					},
				},
			},
		},
	}, nil)

	res, err := fs.AttachFilesystems(c.Context(), params)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 1)
	c.Check(res[0].Error, tc.ErrorMatches, `.* missing pod volume which references claim "vault-k8s-certs-dd246a-vault-k8s-0"`)
}

// TestAttachFilesystemsErrorMissingVolumeMount tests that it should indicate
// an error if the volume mount of the charm container couldn't be found by
// matching the volume name.
func (s *storageSuite) TestAttachFilesystemsErrorMissingVolumeMount(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	prov := s.k8sProvider()
	fs, err := prov.FilesystemSource(nil)
	c.Check(err, tc.ErrorIsNil)

	providerId := "vault-k8s-certs-dd246a-vault-k8s-0"
	params := []storage.FilesystemAttachmentParams{
		{
			AttachmentParams: storage.AttachmentParams{
				Provider:   "kubernetes",
				ProviderId: &providerId,
				Machine:    names.NewMachineTag("unit-vault-k8s-0"),
				InstanceId: "vault-k8s-0",
				ReadOnly:   false,
			},
			Filesystem:           names.NewFilesystemTag("1"),
			FilesystemProviderId: "pvc-8b255462-6fe1-4308-a6a7-dfb46f41d62b",
			Path:                 "/var/lib/juju/storage/09f5ee7d-4cb7-4866-876a-4216014e1283",
		},
	}

	s.mockPersistentVolumeClaims.EXPECT().Get(
		gomock.Any(),
		*params[0].ProviderId,
		gomock.Any(),
	).Return(&core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{Name: *params[0].ProviderId},
	}, nil)
	s.mockPods.EXPECT().Get(
		gomock.Any(),
		params[0].InstanceId.String(),
		gomock.Any(),
	).Return(&core.Pod{
		Spec: core.PodSpec{
			Containers: []core.Container{
				{
					Name: "charm",
					VolumeMounts: []core.VolumeMount{
						{
							MountPath: "/var/lib/pebble/default",
							Name:      "charm-data",
						},
						{
							MountPath: "/var/log/juju",
							Name:      "charm-data",
						},
						{
							MountPath: "/var/lib/juju/storage/certs-0",
							Name:      "volume-name-doesnt-match",
						},
					},
				},
			},
			Volumes: []core.Volume{
				{
					Name: "vault-k8s-certs-dd246a",
					VolumeSource: core.VolumeSource{
						PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
							ClaimName: providerId,
						},
					},
				},
			},
		},
	}, nil)

	res, err := fs.AttachFilesystems(c.Context(), params)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 1)
	c.Check(res[0].Error, tc.ErrorMatches, `.* missing pod volume mount "vault-k8s-certs-dd246a"`)
}
