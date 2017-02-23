// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools/lxdclient"
)

type storageSuite struct {
	lxd.BaseSuite

	provider storage.Provider
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags("lxd-storage")
	s.BaseSuite.SetUpTest(c)
	s.Client.StorageIsSupported = true

	provider, err := s.Env.StorageProvider("lxd")
	c.Assert(err, jc.ErrorIsNil)
	s.provider = provider
	s.Stub.ResetCalls()
}

func (s *storageSuite) filesystemSource(c *gc.C, pool string) storage.FilesystemSource {
	storageConfig, err := storage.NewConfig(pool, "lxd", nil)
	c.Assert(err, jc.ErrorIsNil)
	filesystemSource, err := s.provider.FilesystemSource(storageConfig)
	c.Assert(err, jc.ErrorIsNil)
	return filesystemSource
}

func (s *storageSuite) TestStorageProviderTypes(c *gc.C) {
	s.Client.StorageIsSupported = false
	types, err := s.Env.StorageProviderTypes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types, gc.HasLen, 0)

	s.Client.StorageIsSupported = true
	types, err = s.Env.StorageProviderTypes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types, jc.DeepEquals, []storage.ProviderType{"lxd"})

	s.SetFeatureFlags( /*none*/ )
	types, err = s.Env.StorageProviderTypes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types, gc.HasLen, 0)
}

func (s *storageSuite) TestVolumeSource(c *gc.C) {
	_, err := s.provider.VolumeSource(nil)
	c.Assert(err, gc.ErrorMatches, "volumes not supported")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *storageSuite) TestFilesystemSource(c *gc.C) {
	s.filesystemSource(c, "pool")
}

func (s *storageSuite) TestSupports(c *gc.C) {
	c.Assert(s.provider.Supports(storage.StorageKindBlock), jc.IsFalse)
	c.Assert(s.provider.Supports(storage.StorageKindFilesystem), jc.IsTrue)
}

func (s *storageSuite) TestDynamic(c *gc.C) {
	c.Assert(s.provider.Dynamic(), jc.IsTrue)
}

func (s *storageSuite) TestScope(c *gc.C) {
	c.Assert(s.provider.Scope(), gc.Equals, storage.ScopeEnviron)
}

func (s *storageSuite) TestCreateFilesystems(c *gc.C) {
	source := s.filesystemSource(c, "radiance")
	results, err := source.CreateFilesystems([]storage.FilesystemParams{{
		Tag:      names.NewFilesystemTag("0"),
		Provider: "lxd",
		Size:     1024,
		ResourceTags: map[string]string{
			"key": "value",
		},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, jc.ErrorIsNil)
	c.Assert(results[0].Filesystem, jc.DeepEquals, &storage.Filesystem{
		names.NewFilesystemTag("0"),
		names.VolumeTag{},
		storage.FilesystemInfo{
			FilesystemId: "radiance:filesystem-0",
			Size:         1024,
		},
	})

	s.Stub.CheckCallNames(c, "VolumeCreate")
	s.Stub.CheckCall(c, 0, "VolumeCreate", "radiance", "filesystem-0", map[string]string{
		"user.key": "value",
	})
}

func (s *storageSuite) TestDestroyFilesystems(c *gc.C) {
	s.Stub.SetErrors(nil, errors.New("boom"))
	source := s.filesystemSource(c, "pool")
	results, err := source.DestroyFilesystems([]string{
		"notmypool:filesystem-0",
		"pool:filesystem-0",
		"pool:filesystem-1",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 3)
	c.Assert(results[0], gc.ErrorMatches, `filesystem ID "notmypool:filesystem-0" not valid`)
	c.Assert(results[1], jc.ErrorIsNil)
	c.Assert(results[2], gc.ErrorMatches, "boom")

	s.Stub.CheckCalls(c, []testing.StubCall{
		{"VolumeDelete", []interface{}{"pool", "filesystem-0"}},
		{"VolumeDelete", []interface{}{"pool", "filesystem-1"}},
	})
}

func (s *storageSuite) TestAttachFilesystems(c *gc.C) {
	raw := s.NewRawInstance(c, "inst-0")
	raw.Devices = map[string]map[string]string{
		"filesystem-1": map[string]string{
			"type":     "disk",
			"source":   "filesystem-1",
			"pool":     "pool",
			"path":     "/mnt/path",
			"readonly": "true",
		},
	}
	s.Client.Insts = []lxdclient.Instance{*raw}

	source := s.filesystemSource(c, "pool")
	results, err := source.AttachFilesystems([]storage.FilesystemAttachmentParams{{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 3)
	c.Assert(results[0].Error, jc.ErrorIsNil)
	c.Assert(results[0].FilesystemAttachment, jc.DeepEquals, &storage.FilesystemAttachment{
		names.NewFilesystemTag("0"),
		names.NewMachineTag("123"),
		storage.FilesystemAttachmentInfo{
			Path:     "/mnt/path",
			ReadOnly: true,
		},
	})
	c.Assert(results[1].Error, jc.ErrorIsNil)
	c.Assert(results[1].FilesystemAttachment, jc.DeepEquals, &storage.FilesystemAttachment{
		names.NewFilesystemTag("1"),
		names.NewMachineTag("123"),
		storage.FilesystemAttachmentInfo{
			Path:     "/mnt/socio",
			ReadOnly: true,
		},
	})
	c.Assert(
		results[2].Error,
		gc.ErrorMatches,
		`attaching filesystem 2 to machine 42: instance "inst-42" not found`,
	)

	s.Stub.CheckCalls(c, []testing.StubCall{{
		"Instances",
		[]interface{}{"juju-f75cba-", []string{"Starting", "Started", "Running", "Stopping", "Stopped"}},
	}, {
		"AttachDisk",
		[]interface{}{"inst-0", "filesystem-0", lxdclient.DiskDevice{
			Path:     "/mnt/path",
			Source:   "filesystem-0",
			Pool:     "pool",
			ReadOnly: true,
		}},
	}})
}

func (s *storageSuite) TestDetachFilesystems(c *gc.C) {
	raw := s.NewRawInstance(c, "inst-0")
	raw.Devices = map[string]map[string]string{
		"filesystem-0": map[string]string{
			"type":     "disk",
			"source":   "filesystem-0",
			"pool":     "pool",
			"path":     "/mnt/path",
			"readonly": "true",
		},
	}
	s.Client.Insts = []lxdclient.Instance{*raw}

	source := s.filesystemSource(c, "pool")
	results, err := source.DetachFilesystems([]storage.FilesystemAttachmentParams{{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 3)
	c.Assert(results[0], jc.ErrorIsNil)
	c.Assert(results[1], jc.ErrorIsNil)
	c.Assert(results[2], jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []testing.StubCall{{
		"Instances",
		[]interface{}{"juju-f75cba-", []string{"Starting", "Started", "Running", "Stopping", "Stopped"}},
	}, {
		"RemoveDevice", []interface{}{"inst-0", "filesystem-0"},
	}})
}
