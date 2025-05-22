// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	stdtesting "testing"

	"github.com/canonical/lxd/shared/api"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	containerlxd "github.com/juju/juju/internal/container/lxd"
	"github.com/juju/juju/internal/provider/lxd"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
)

type storageSuite struct {
	lxd.BaseSuite

	provider storage.Provider
}

func TestStorageSuite(t *stdtesting.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) TestStorageProviderTypes(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Client.StorageIsSupported = false
	types, err := s.Env.StorageProviderTypes()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(types, tc.HasLen, 0)

	s.Client.StorageIsSupported = true
	types, err = s.Env.StorageProviderTypes()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(types, tc.DeepEquals, []storage.ProviderType{"lxd"})
}

func (s *storageSuite) TestStorageDefaultPools(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	pools := s.provider.DefaultPools()
	c.Assert(pools, tc.HasLen, 2)
	c.Assert(pools[0].Name(), tc.Equals, "lxd-zfs")
	c.Assert(pools[1].Name(), tc.Equals, "lxd-btrfs")
	s.Stub.CheckCallNames(c, "CreatePool", "CreatePool")
}

func (s *storageSuite) TestStorageDefaultPoolsDriverNotSupported(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Stub.SetErrors(
		errors.New("no zfs for you"),
		errors.NotFoundf("zfs storage pool"),
	)
	pools := s.provider.DefaultPools()
	c.Assert(pools, tc.HasLen, 1)
	c.Assert(pools[0].Name(), tc.Equals, "lxd-btrfs")
	s.Stub.CheckCallNames(c, "CreatePool", "GetStoragePool", "CreatePool")
}

func (s *storageSuite) TestVolumeSource(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	_, err := s.provider.VolumeSource(nil)
	c.Assert(err, tc.ErrorMatches, "volumes not supported")
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *storageSuite) TestFilesystemSource(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.filesystemSource(c, "pool")
}

func (s *storageSuite) TestSupports(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	c.Assert(s.provider.Supports(storage.StorageKindBlock), tc.IsFalse)
	c.Assert(s.provider.Supports(storage.StorageKindFilesystem), tc.IsTrue)
}

func (s *storageSuite) TestDynamic(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	c.Assert(s.provider.Dynamic(), tc.IsTrue)
}

func (s *storageSuite) TestScope(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	c.Assert(s.provider.Scope(), tc.Equals, storage.ScopeEnviron)
}

func (s *storageSuite) TestCreateFilesystems(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	source := s.filesystemSource(c, "source")
	results, err := source.CreateFilesystems(c.Context(), []storage.FilesystemParams{{
		Tag:      names.NewFilesystemTag("0"),
		Provider: "lxd",
		Size:     1024,
		ResourceTags: map[string]string{
			"key": "value",
		},
		Attributes: map[string]interface{}{
			"lxd-pool": "radiance",
			"driver":   "btrfs",
		},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Assert(results[0].Error, tc.ErrorIsNil)
	c.Assert(results[0].Filesystem, tc.DeepEquals, &storage.Filesystem{
		Tag: names.NewFilesystemTag("0"),
		FilesystemInfo: storage.FilesystemInfo{
			FilesystemId: "radiance:juju-f75cba-filesystem-0",
			Size:         1024,
		},
	})

	s.Stub.CheckCallNames(c, "CreatePool", "CreateVolume")
	s.Stub.CheckCall(c, 0, "CreatePool", "radiance", "btrfs", map[string]string(nil))
	s.Stub.CheckCall(c, 1, "CreateVolume", "radiance", "juju-f75cba-filesystem-0", map[string]string{
		"user.key": "value",
		"size":     "1024MiB",
	})
}

func (s *storageSuite) TestCreateFilesystemsPoolExists(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Stub.SetErrors(errors.New("pool already exists"))
	source := s.filesystemSource(c, "source")
	results, err := source.CreateFilesystems(c.Context(), []storage.FilesystemParams{{
		Tag:      names.NewFilesystemTag("0"),
		Provider: "lxd",
		Size:     1024,
		ResourceTags: map[string]string{
			"key": "value",
		},
		Attributes: map[string]interface{}{
			"lxd-pool": "radiance",
			"driver":   "dir",
		},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Error, tc.ErrorIsNil)
	c.Check(results[0].Filesystem, tc.DeepEquals, &storage.Filesystem{
		Tag:    names.NewFilesystemTag("0"),
		Volume: names.VolumeTag{},
		FilesystemInfo: storage.FilesystemInfo{
			FilesystemId: "radiance:juju-f75cba-filesystem-0",
			Size:         1024,
		},
	})

	s.Stub.CheckCallNames(c, "CreatePool", "GetStoragePool", "CreateVolume")
	s.Stub.CheckCall(c, 0, "CreatePool", "radiance", "dir", map[string]string(nil))
	s.Stub.CheckCall(c, 1, "GetStoragePool", "radiance")
	s.Stub.CheckCall(c, 2, "CreateVolume", "radiance", "juju-f75cba-filesystem-0", map[string]string{
		"user.key": "value",
	})
}

func (s *storageSuite) TestCreateFilesystemsInvalidCredentials(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)

	source := s.filesystemSource(c, "source")
	s.Client.Stub.SetErrors(nil, errTestUnAuth)
	results, err := source.CreateFilesystems(c.Context(), []storage.FilesystemParams{{
		Tag:      names.NewFilesystemTag("0"),
		Provider: "lxd",
		Size:     1024,
		ResourceTags: map[string]string{
			"key": "value",
		},
		Attributes: map[string]interface{}{
			"lxd-pool": "radiance",
			"driver":   "btrfs",
		},
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Error, tc.ErrorMatches, ".*not authorized")
	c.Check(results[0].Filesystem, tc.DeepEquals, (*storage.Filesystem)(nil))
}

func (s *storageSuite) TestDestroyFilesystems(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Stub.SetErrors(nil, errors.New("boom"))
	source := s.filesystemSource(c, "source")
	results, err := source.DestroyFilesystems(c.Context(), []string{
		"filesystem-0",
		"pool0:filesystem-0",
		"pool1:filesystem-1",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 3)
	c.Check(results[0], tc.ErrorMatches, `invalid filesystem ID "filesystem-0"; expected ID in format <lxd-pool>:<volume-name>`)
	c.Check(results[1], tc.ErrorIsNil)
	c.Check(results[2], tc.ErrorMatches, "boom")

	s.Stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "DeleteStoragePoolVolume", Args: []interface{}{"pool0", "custom", "filesystem-0"}},
		{FuncName: "DeleteStoragePoolVolume", Args: []interface{}{"pool1", "custom", "filesystem-1"}},
	})
}

func (s *storageSuite) TestDestroyFilesystemsInvalidCredentials(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)

	s.Client.Stub.SetErrors(errTestUnAuth)
	source := s.filesystemSource(c, "source")
	results, err := source.DestroyFilesystems(c.Context(), []string{
		"pool0:filesystem-0",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0], tc.ErrorMatches, "not authorized")
}

func (s *storageSuite) TestReleaseFilesystems(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Stub.SetErrors(nil, nil, nil, errors.New("boom"))
	s.Client.Volumes = map[string][]api.StorageVolume{
		"foo": {{
			Name: "filesystem-0",
			Config: map[string]string{
				"foo":                  "bar",
				"user.juju-model-uuid": "baz",
			},
		}, {
			Name: "filesystem-1",
			Config: map[string]string{
				"user.juju-controller-uuid": "qux",
				"user.juju-model-uuid":      "quux",
			},
		}},
	}

	source := s.filesystemSource(c, "source")
	results, err := source.ReleaseFilesystems(c.Context(), []string{
		"filesystem-0",
		"foo:filesystem-0",
		"foo:filesystem-1",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 3)
	c.Assert(results[0], tc.ErrorMatches, `invalid filesystem ID "filesystem-0"; expected ID in format <lxd-pool>:<volume-name>`)
	c.Assert(results[1], tc.ErrorIsNil)
	c.Assert(results[2], tc.ErrorMatches, `removing tags from volume "filesystem-1" in pool "foo": boom`)

	update0 := api.StorageVolumePut{
		Config: map[string]string{
			"foo": "bar",
		},
	}
	update1 := api.StorageVolumePut{
		Config: map[string]string{},
	}

	s.Stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "GetStoragePoolVolume", Args: []interface{}{"foo", "custom", "filesystem-0"}},
		{FuncName: "UpdateStoragePoolVolume", Args: []interface{}{"foo", "custom", "filesystem-0", update0, "eTag"}},
		{FuncName: "GetStoragePoolVolume", Args: []interface{}{"foo", "custom", "filesystem-1"}},
		{FuncName: "UpdateStoragePoolVolume", Args: []interface{}{"foo", "custom", "filesystem-1", update1, "eTag"}},
	})
}

func (s *storageSuite) TestReleaseFilesystemsInvalidCredentials(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)

	s.Stub.SetErrors(errTestUnAuth)

	source := s.filesystemSource(c, "source")
	results, err := source.ReleaseFilesystems(c.Context(), []string{
		"foo:filesystem-0",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0], tc.ErrorMatches, "not authorized")

	s.Stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "GetStoragePoolVolume", Args: []interface{}{"foo", "custom", "filesystem-0"}},
	})
}

func (s *storageSuite) TestAttachFilesystems(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	container := s.NewContainer(c, "inst-0")
	container.Devices = map[string]map[string]string{
		"filesystem-1": {
			"type":     "disk",
			"source":   "filesystem-1",
			"pool":     "pool",
			"path":     "/mnt/path",
			"readonly": "true",
		},
	}
	s.Client.Containers = []containerlxd.Container{*container}

	source := s.filesystemSource(c, "pool")
	results, err := source.AttachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		AttachmentParams: storage.AttachmentParams{
			Provider:   "lxd",
			Machine:    names.NewMachineTag("123"),
			InstanceId: "inst-0",
			ReadOnly:   true,
		},
		Filesystem:   names.NewFilesystemTag("0"),
		FilesystemId: "pool:filesystem-0",
		Path:         "/mnt/path",
	}, {
		AttachmentParams: storage.AttachmentParams{
			Provider:   "lxd",
			Machine:    names.NewMachineTag("123"),
			InstanceId: "inst-0",
			ReadOnly:   true,
		},
		Filesystem:   names.NewFilesystemTag("1"),
		FilesystemId: "pool:filesystem-1",
		Path:         "/mnt/socio",
	}, {
		AttachmentParams: storage.AttachmentParams{
			Provider:   "lxd",
			Machine:    names.NewMachineTag("42"),
			InstanceId: "inst-42",
		},
		Filesystem:   names.NewFilesystemTag("2"),
		FilesystemId: "pool:filesystem-2",
		Path:         "/mnt/psycho",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 3)
	c.Assert(results[0].Error, tc.ErrorIsNil)
	c.Assert(results[0].FilesystemAttachment, tc.DeepEquals, &storage.FilesystemAttachment{
		Filesystem: names.NewFilesystemTag("0"),
		Machine:    names.NewMachineTag("123"),
		FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
			Path:     "/mnt/path",
			ReadOnly: true,
		},
	})
	c.Assert(
		results[1].Error,
		tc.ErrorMatches,
		`attaching filesystem 1 to machine 123: container "inst-0" already has a device "filesystem-1"`)
	c.Assert(
		results[2].Error, tc.ErrorMatches, `attaching filesystem 2 to machine 42: instance "inst-42" not found`,
	)

	// TODO (manadart 2018-06-25) We need to check the device written to the
	// container as config.
	s.Stub.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "AliveContainers",
		Args:     []interface{}{"juju-f75cba-"},
	}, {
		FuncName: "WriteContainer",
		Args:     []interface{}{&s.Client.Containers[0]},
	}})
}

func (s *storageSuite) TestAttachFilesystemsInvalidCredentialsInstanceError(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil).MinTimes(1)

	s.Client.Stub.SetErrors(errTestUnAuth)

	container := s.NewContainer(c, "inst-0")
	container.Devices = map[string]map[string]string{
		"filesystem-1": {
			"type":     "disk",
			"source":   "filesystem-1",
			"pool":     "pool",
			"path":     "/mnt/path",
			"readonly": "true",
		},
	}
	s.Client.Containers = []containerlxd.Container{*container}

	source := s.filesystemSource(c, "pool")
	results, err := source.AttachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		AttachmentParams: storage.AttachmentParams{
			Provider:   "lxd",
			Machine:    names.NewMachineTag("123"),
			InstanceId: "inst-0",
			ReadOnly:   true,
		},
		Filesystem:   names.NewFilesystemTag("0"),
		FilesystemId: "pool:filesystem-0",
		Path:         "/mnt/path",
	}})
	c.Assert(err, tc.ErrorMatches, "not authorized")
	c.Assert(results, tc.HasLen, 0)
}

func (s *storageSuite) TestAttachFilesystemsInvalidCredentialsAttachingFilesystems(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)

	s.Client.Stub.SetErrors(nil, errTestUnAuth)

	container := s.NewContainer(c, "inst-0")
	container.Devices = map[string]map[string]string{
		"filesystem-1": {
			"type":     "disk",
			"source":   "filesystem-1",
			"pool":     "pool",
			"path":     "/mnt/path",
			"readonly": "true",
		},
	}
	s.Client.Containers = []containerlxd.Container{*container}

	source := s.filesystemSource(c, "pool")
	results, err := source.AttachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		AttachmentParams: storage.AttachmentParams{
			Provider:   "lxd",
			Machine:    names.NewMachineTag("123"),
			InstanceId: "inst-0",
			ReadOnly:   true,
		},
		Filesystem:   names.NewFilesystemTag("0"),
		FilesystemId: "pool:filesystem-0",
		Path:         "/mnt/path",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Error, tc.ErrorMatches, ".*not authorized")
	c.Check(results[0].FilesystemAttachment, tc.DeepEquals, (*storage.FilesystemAttachment)(nil))
}

func (s *storageSuite) TestDetachFilesystems(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	container := s.NewContainer(c, "inst-0")
	container.Devices = map[string]map[string]string{
		"filesystem-0": {
			"type":     "disk",
			"source":   "filesystem-0",
			"pool":     "pool",
			"path":     "/mnt/path",
			"readonly": "true",
		},
	}
	s.Client.Containers = []containerlxd.Container{*container}

	source := s.filesystemSource(c, "pool")
	results, err := source.DetachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		AttachmentParams: storage.AttachmentParams{
			Provider:   "lxd",
			Machine:    names.NewMachineTag("123"),
			InstanceId: "inst-0",
		},
		Filesystem:   names.NewFilesystemTag("0"),
		FilesystemId: "pool:filesystem-0",
	}, {
		AttachmentParams: storage.AttachmentParams{
			Provider:   "lxd",
			Machine:    names.NewMachineTag("123"),
			InstanceId: "inst-0",
		},
		Filesystem:   names.NewFilesystemTag("1"),
		FilesystemId: "pool:filesystem-1",
	}, {
		AttachmentParams: storage.AttachmentParams{
			Provider:   "lxd",
			Machine:    names.NewMachineTag("42"),
			InstanceId: "inst-42",
		},
		Filesystem:   names.NewFilesystemTag("2"),
		FilesystemId: "pool:filesystem-2",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 3)
	c.Assert(results[0], tc.ErrorIsNil)
	c.Assert(results[1], tc.ErrorIsNil)
	c.Assert(results[2], tc.ErrorIsNil)

	// TODO (manadart 2018-06-25) We need to check the container config to
	// ensure it represents the removed device.
	s.Stub.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "AliveContainers",
		Args:     []interface{}{"juju-f75cba-"},
	}, {
		FuncName: "WriteContainer",
		Args:     []interface{}{&s.Client.Containers[0]},
	}, {
		FuncName: "WriteContainer",
		Args:     []interface{}{&s.Client.Containers[0]},
	}})
}

func (s *storageSuite) TestDetachFilesystemsInvalidCredentialsInstanceErrors(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil).MinTimes(1)

	s.Client.Stub.SetErrors(errTestUnAuth)

	source := s.filesystemSource(c, "pool")
	results, err := source.DetachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		AttachmentParams: storage.AttachmentParams{
			Provider:   "lxd",
			Machine:    names.NewMachineTag("123"),
			InstanceId: "inst-0",
		},
		Filesystem:   names.NewFilesystemTag("0"),
		FilesystemId: "pool:filesystem-0",
	}})
	c.Assert(err, tc.ErrorMatches, "not authorized")
	c.Assert(results, tc.HasLen, 0)
}

func (s *storageSuite) TestDetachFilesystemsInvalidCredentialsDetachFilesystem(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)

	s.Client.Stub.SetErrors(nil, errTestUnAuth)

	container := s.NewContainer(c, "inst-0")
	container.Devices = map[string]map[string]string{
		"filesystem-0": {
			"type":     "disk",
			"source":   "filesystem-0",
			"pool":     "pool",
			"path":     "/mnt/path",
			"readonly": "true",
		},
	}
	s.Client.Containers = []containerlxd.Container{*container}

	source := s.filesystemSource(c, "pool")
	results, err := source.DetachFilesystems(c.Context(), []storage.FilesystemAttachmentParams{{
		AttachmentParams: storage.AttachmentParams{
			Provider:   "lxd",
			Machine:    names.NewMachineTag("123"),
			InstanceId: "inst-0",
		},
		Filesystem:   names.NewFilesystemTag("0"),
		FilesystemId: "pool:filesystem-0",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0], tc.ErrorMatches, ".*not authorized")
}

func (s *storageSuite) TestImportFilesystem(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	source := s.filesystemSource(c, "pool")
	c.Assert(source, tc.Implements, new(storage.FilesystemImporter))
	importer := source.(storage.FilesystemImporter)

	s.Client.Volumes = map[string][]api.StorageVolume{
		"foo": {{
			Name: "bar",
			Config: map[string]string{
				"size": "10GiB",
			},
		}},
	}

	info, err := importer.ImportFilesystem(c.Context(),
		"foo:bar", map[string]string{
			"baz": "qux",
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, storage.FilesystemInfo{
		FilesystemId: "foo:bar",
		Size:         10 * 1024,
	})

	update := api.StorageVolumePut{
		Config: map[string]string{
			"size":     "10GiB",
			"user.baz": "qux",
		},
	}
	s.Stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "GetStoragePoolVolume", Args: []interface{}{"foo", "custom", "bar"}},
		{FuncName: "UpdateStoragePoolVolume", Args: []interface{}{"foo", "custom", "bar", update, "eTag"}},
	})
}

func (s *storageSuite) TestImportFilesystemInvalidCredentialsGetPool(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)

	s.Client.Stub.SetErrors(errTestUnAuth)
	source := s.filesystemSource(c, "pool")

	c.Assert(source, tc.Implements, new(storage.FilesystemImporter))
	importer := source.(storage.FilesystemImporter)

	info, err := importer.ImportFilesystem(c.Context(),
		"foo:bar", map[string]string{
			"baz": "qux",
		})
	c.Assert(err, tc.ErrorMatches, ".*not authorized")
	c.Assert(info, tc.DeepEquals, storage.FilesystemInfo{})
}

func (s *storageSuite) TestImportFilesystemInvalidCredentialsUpdatePool(c *tc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)

	s.Client.Stub.SetErrors(nil, errTestUnAuth)
	source := s.filesystemSource(c, "pool")

	c.Assert(source, tc.Implements, new(storage.FilesystemImporter))
	importer := source.(storage.FilesystemImporter)

	s.Client.Volumes = map[string][]api.StorageVolume{
		"foo": {{
			Name: "bar",
			Config: map[string]string{
				"size": "10GiB",
			},
		}},
	}

	info, err := importer.ImportFilesystem(c.Context(),
		"foo:bar", map[string]string{
			"baz": "qux",
		})
	c.Assert(err, tc.ErrorMatches, ".*not authorized")
	c.Assert(info, tc.DeepEquals, storage.FilesystemInfo{})
}

func (s *storageSuite) SetupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.BaseSuite.SetupMocks(c)

	provider, err := s.Env.StorageProvider("lxd")
	c.Assert(err, tc.ErrorIsNil)
	s.provider = provider
	s.Stub.ResetCalls()
	return ctrl
}

func (s *storageSuite) filesystemSource(c *tc.C, pool string) storage.FilesystemSource {
	storageConfig, err := storage.NewConfig(pool, "lxd", nil)
	c.Assert(err, tc.ErrorIsNil)
	filesystemSource, err := s.provider.FilesystemSource(storageConfig)
	c.Assert(err, tc.ErrorIsNil)
	return filesystemSource
}
