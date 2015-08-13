// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"errors"
	"fmt"
	"strings"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/storage/provider/registry"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type DestroySuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&DestroySuite{})

func (s *DestroySuite) TestCannotGetInstances(c *gc.C) {
	env := &mockEnviron{
		allInstances: func() ([]instance.Instance, error) {
			return nil, fmt.Errorf("nope")
		},
		config: configGetter(c),
	}
	err := common.Destroy(env)
	c.Assert(err, gc.ErrorMatches, "destroying instances: nope")
}

func (s *DestroySuite) TestCannotStopInstances(c *gc.C) {
	env := &mockEnviron{
		allInstances: func() ([]instance.Instance, error) {
			return []instance.Instance{
				&mockInstance{id: "one"},
				&mockInstance{id: "another"},
			}, nil
		},
		stopInstances: func(ids []instance.Id) error {
			c.Assert(ids, gc.HasLen, 2)
			c.Assert(ids[0], gc.Equals, instance.Id("one"))
			c.Assert(ids[1], gc.Equals, instance.Id("another"))
			return fmt.Errorf("nah")
		},
		config: configGetter(c),
	}
	err := common.Destroy(env)
	c.Assert(err, gc.ErrorMatches, "destroying instances: nah")
}

func (s *DestroySuite) TestSuccessWhenStorageErrors(c *gc.C) {
	// common.Destroy doesn't touch provider/object storage anymore,
	// so failing storage should not affect success.
	env := &mockEnviron{
		storage: &mockStorage{removeAllErr: fmt.Errorf("noes!")},
		allInstances: func() ([]instance.Instance, error) {
			return []instance.Instance{
				&mockInstance{id: "one"},
				&mockInstance{id: "another"},
			}, nil
		},
		stopInstances: func(ids []instance.Id) error {
			c.Assert(ids, gc.HasLen, 2)
			c.Assert(ids[0], gc.Equals, instance.Id("one"))
			c.Assert(ids[1], gc.Equals, instance.Id("another"))
			return nil
		},
		config: configGetter(c),
	}
	err := common.Destroy(env)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DestroySuite) TestSuccess(c *gc.C) {
	s.PatchValue(&version.Current.Number, testing.FakeVersionNumber)
	stor := newStorage(s, c)
	err := stor.Put("somewhere", strings.NewReader("stuff"), 5)
	c.Assert(err, jc.ErrorIsNil)

	env := &mockEnviron{
		storage: stor,
		allInstances: func() ([]instance.Instance, error) {
			return []instance.Instance{
				&mockInstance{id: "one"},
			}, nil
		},
		stopInstances: func(ids []instance.Id) error {
			c.Assert(ids, gc.HasLen, 1)
			c.Assert(ids[0], gc.Equals, instance.Id("one"))
			return nil
		},
		config: configGetter(c),
	}
	err = common.Destroy(env)
	c.Assert(err, jc.ErrorIsNil)

	// common.Destroy doesn't touch provider/object storage anymore.
	r, err := stor.Get("somewhere")
	c.Assert(err, jc.ErrorIsNil)
	r.Close()
}

func (s *DestroySuite) TestSuccessWhenNoInstances(c *gc.C) {
	s.PatchValue(&version.Current.Number, testing.FakeVersionNumber)
	stor := newStorage(s, c)
	err := stor.Put("elsewhere", strings.NewReader("stuff"), 5)
	c.Assert(err, jc.ErrorIsNil)

	env := &mockEnviron{
		storage: stor,
		allInstances: func() ([]instance.Instance, error) {
			return nil, environs.ErrNoInstances
		},
		config: configGetter(c),
	}
	err = common.Destroy(env)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DestroySuite) TestDestroyEnvScopedVolumes(c *gc.C) {
	volumeSource := &dummy.VolumeSource{
		ListVolumesFunc: func() ([]string, error) {
			return []string{"vol-0", "vol-1", "vol-2"}, nil
		},
		DestroyVolumesFunc: func(ids []string) ([]error, error) {
			return make([]error, len(ids)), nil
		},
	}
	staticProvider := &dummy.StorageProvider{
		IsDynamic:    true,
		StorageScope: storage.ScopeEnviron,
		VolumeSourceFunc: func(*config.Config, *storage.Config) (storage.VolumeSource, error) {
			return volumeSource, nil
		},
	}
	registry.RegisterProvider("environ", staticProvider)
	defer registry.RegisterProvider("environ", nil)
	registry.RegisterEnvironStorageProviders("anything, really", "environ")
	defer registry.ResetEnvironStorageProviders("anything, really")

	env := &mockEnviron{
		config: configGetter(c),
		allInstances: func() ([]instance.Instance, error) {
			return nil, environs.ErrNoInstances
		},
	}
	err := common.Destroy(env)
	c.Assert(err, jc.ErrorIsNil)

	// common.Destroy will ignore machine-scoped storage providers.
	staticProvider.CheckCallNames(c, "Dynamic", "Scope", "Supports", "VolumeSource")
	volumeSource.CheckCalls(c, []gitjujutesting.StubCall{
		{"ListVolumes", nil},
		{"DestroyVolumes", []interface{}{[]string{"vol-0", "vol-1", "vol-2"}}},
	})
}

func (s *DestroySuite) TestDestroyVolumeErrors(c *gc.C) {
	volumeSource := &dummy.VolumeSource{
		ListVolumesFunc: func() ([]string, error) {
			return []string{"vol-0", "vol-1", "vol-2"}, nil
		},
		DestroyVolumesFunc: func(ids []string) ([]error, error) {
			return []error{
				nil,
				errors.New("cannot destroy vol-1"),
				errors.New("cannot destroy vol-2"),
			}, nil
		},
	}

	staticProvider := &dummy.StorageProvider{
		IsDynamic:    true,
		StorageScope: storage.ScopeEnviron,
		VolumeSourceFunc: func(*config.Config, *storage.Config) (storage.VolumeSource, error) {
			return volumeSource, nil
		},
	}
	registry.RegisterProvider("environ", staticProvider)
	defer registry.RegisterProvider("environ", nil)
	registry.RegisterEnvironStorageProviders("anything, really", "environ")
	defer registry.ResetEnvironStorageProviders("anything, really")

	env := &mockEnviron{
		config: configGetter(c),
		allInstances: func() ([]instance.Instance, error) {
			return nil, environs.ErrNoInstances
		},
	}
	err := common.Destroy(env)
	c.Assert(err, gc.ErrorMatches, "destroying storage: destroying volumes: cannot destroy vol-1, cannot destroy vol-2")
}

func (s *DestroySuite) TestIgnoreStaticVolumes(c *gc.C) {
	staticProvider := &dummy.StorageProvider{
		IsDynamic:    false,
		StorageScope: storage.ScopeEnviron,
	}
	registry.RegisterProvider("static", staticProvider)
	defer registry.RegisterProvider("static", nil)
	registry.RegisterEnvironStorageProviders("anything, really", "static")
	defer registry.ResetEnvironStorageProviders("anything, really")

	env := &mockEnviron{
		config: configGetter(c),
		allInstances: func() ([]instance.Instance, error) {
			return nil, environs.ErrNoInstances
		},
	}
	err := common.Destroy(env)
	c.Assert(err, jc.ErrorIsNil)

	// common.Destroy will ignore static storage providers.
	staticProvider.CheckCallNames(c, "Dynamic")
}

func (s *DestroySuite) TestIgnoreMachineScopedVolumes(c *gc.C) {
	staticProvider := &dummy.StorageProvider{
		IsDynamic:    true,
		StorageScope: storage.ScopeMachine,
	}
	registry.RegisterProvider("machine", staticProvider)
	defer registry.RegisterProvider("machine", nil)
	registry.RegisterEnvironStorageProviders("anything, really", "machine")
	defer registry.ResetEnvironStorageProviders("anything, really")

	env := &mockEnviron{
		config: configGetter(c),
		allInstances: func() ([]instance.Instance, error) {
			return nil, environs.ErrNoInstances
		},
	}
	err := common.Destroy(env)
	c.Assert(err, jc.ErrorIsNil)

	// common.Destroy will ignore machine-scoped storage providers.
	staticProvider.CheckCallNames(c, "Dynamic", "Scope")
}

func (s *DestroySuite) TestIgnoreNoVolumeSupport(c *gc.C) {
	staticProvider := &dummy.StorageProvider{
		IsDynamic:    true,
		StorageScope: storage.ScopeEnviron,
		SupportsFunc: func(storage.StorageKind) bool {
			return false
		},
	}
	registry.RegisterProvider("filesystem", staticProvider)
	defer registry.RegisterProvider("filesystem", nil)
	registry.RegisterEnvironStorageProviders("anything, really", "filesystem")
	defer registry.ResetEnvironStorageProviders("anything, really")

	env := &mockEnviron{
		config: configGetter(c),
		allInstances: func() ([]instance.Instance, error) {
			return nil, environs.ErrNoInstances
		},
	}
	err := common.Destroy(env)
	c.Assert(err, jc.ErrorIsNil)

	// common.Destroy will ignore storage providers that don't support
	// volumes (until we have persistent filesystems, that is).
	staticProvider.CheckCallNames(c, "Dynamic", "Scope", "Supports")
}
