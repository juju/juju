// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"

	"github.com/juju/juju/core/instance"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider/dummy"
	"github.com/juju/juju/internal/testing"
)

type DestroySuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&DestroySuite{})

func (s *DestroySuite) TestCannotGetInstances(c *tc.C) {
	env := &mockEnviron{
		allInstances: func(context.Context) ([]instances.Instance, error) {
			return nil, fmt.Errorf("nope")
		},
		config: configGetter(c),
	}
	err := common.Destroy(env, context.Background())
	c.Assert(err, tc.ErrorMatches, "destroying instances: nope")
}

func (s *DestroySuite) TestCannotStopInstances(c *tc.C) {
	env := &mockEnviron{
		allInstances: func(context.Context) ([]instances.Instance, error) {
			return []instances.Instance{
				&mockInstance{id: "one"},
				&mockInstance{id: "another"},
			}, nil
		},
		stopInstances: func(ctx context.Context, ids []instance.Id) error {
			c.Assert(ids, tc.HasLen, 2)
			c.Assert(ids[0], tc.Equals, instance.Id("one"))
			c.Assert(ids[1], tc.Equals, instance.Id("another"))
			return fmt.Errorf("nah")
		},
		config: configGetter(c),
	}
	err := common.Destroy(env, context.Background())
	c.Assert(err, tc.ErrorMatches, "destroying instances: nah")
}

func (s *DestroySuite) TestSuccessWhenStorageErrors(c *tc.C) {
	// common.Destroy doesn't touch provider/object storage anymore,
	// so failing storage should not affect success.
	env := &mockEnviron{
		storage: &mockStorage{removeAllErr: fmt.Errorf("noes!")},
		allInstances: func(context.Context) ([]instances.Instance, error) {
			return []instances.Instance{
				&mockInstance{id: "one"},
				&mockInstance{id: "another"},
			}, nil
		},
		stopInstances: func(ctx context.Context, ids []instance.Id) error {
			c.Assert(ids, tc.HasLen, 2)
			c.Assert(ids[0], tc.Equals, instance.Id("one"))
			c.Assert(ids[1], tc.Equals, instance.Id("another"))
			return nil
		},
		config: configGetter(c),
	}
	err := common.Destroy(env, context.Background())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DestroySuite) TestSuccess(c *tc.C) {
	s.PatchValue(&jujuversion.Current, testing.FakeVersionNumber)
	stor := newStorage(s, c)
	err := stor.Put("somewhere", strings.NewReader("stuff"), 5)
	c.Assert(err, tc.ErrorIsNil)

	env := &mockEnviron{
		storage: stor,
		allInstances: func(context.Context) ([]instances.Instance, error) {
			return []instances.Instance{
				&mockInstance{id: "one"},
			}, nil
		},
		stopInstances: func(ctx context.Context, ids []instance.Id) error {
			c.Assert(ids, tc.HasLen, 1)
			c.Assert(ids[0], tc.Equals, instance.Id("one"))
			return nil
		},
		config: configGetter(c),
	}
	err = common.Destroy(env, context.Background())
	c.Assert(err, tc.ErrorIsNil)

	// common.Destroy doesn't touch provider/object storage anymore.
	r, err := stor.Get("somewhere")
	c.Assert(err, tc.ErrorIsNil)
	r.Close()
}

func (s *DestroySuite) TestSuccessWhenNoInstances(c *tc.C) {
	s.PatchValue(&jujuversion.Current, testing.FakeVersionNumber)
	stor := newStorage(s, c)
	err := stor.Put("elsewhere", strings.NewReader("stuff"), 5)
	c.Assert(err, tc.ErrorIsNil)

	env := &mockEnviron{
		storage: stor,
		allInstances: func(context.Context) ([]instances.Instance, error) {
			return nil, environs.ErrNoInstances
		},
		config: configGetter(c),
	}
	err = common.Destroy(env, context.Background())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DestroySuite) TestDestroyEnvScopedVolumes(c *tc.C) {
	volumeSource := &dummy.VolumeSource{
		ListVolumesFunc: func(ctx context.Context) ([]string, error) {
			return []string{"vol-0", "vol-1", "vol-2"}, nil
		},
		DestroyVolumesFunc: func(ctx context.Context, ids []string) ([]error, error) {
			return make([]error, len(ids)), nil
		},
	}
	storageProvider := &dummy.StorageProvider{
		IsDynamic:    true,
		StorageScope: storage.ScopeEnviron,
		VolumeSourceFunc: func(*storage.Config) (storage.VolumeSource, error) {
			return volumeSource, nil
		},
	}

	env := &mockEnviron{
		config: configGetter(c),
		allInstances: func(context.Context) ([]instances.Instance, error) {
			return nil, environs.ErrNoInstances
		},
		storageProviders: storage.StaticProviderRegistry{
			map[storage.ProviderType]storage.Provider{
				"environ": storageProvider,
			},
		},
	}
	err := common.Destroy(env, context.Background())
	c.Assert(err, tc.ErrorIsNil)

	// common.Destroy will ignore machine-scoped storage providers.
	storageProvider.CheckCallNames(c, "Dynamic", "Scope", "Supports", "VolumeSource")
	volumeSource.CheckCalls(c, []jujutesting.StubCall{
		{"ListVolumes", []interface{}{context.Background()}},
		{"DestroyVolumes", []interface{}{context.Background(), []string{"vol-0", "vol-1", "vol-2"}}},
	})
}

func (s *DestroySuite) TestDestroyVolumeErrors(c *tc.C) {
	volumeSource := &dummy.VolumeSource{
		ListVolumesFunc: func(ctx context.Context) ([]string, error) {
			return []string{"vol-0", "vol-1", "vol-2"}, nil
		},
		DestroyVolumesFunc: func(ctx context.Context, ids []string) ([]error, error) {
			return []error{
				nil,
				errors.New("cannot destroy vol-1"),
				errors.New("cannot destroy vol-2"),
			}, nil
		},
	}

	storageProvider := &dummy.StorageProvider{
		IsDynamic:    true,
		StorageScope: storage.ScopeEnviron,
		VolumeSourceFunc: func(*storage.Config) (storage.VolumeSource, error) {
			return volumeSource, nil
		},
	}

	env := &mockEnviron{
		config: configGetter(c),
		allInstances: func(context.Context) ([]instances.Instance, error) {
			return nil, environs.ErrNoInstances
		},
		storageProviders: storage.StaticProviderRegistry{
			map[storage.ProviderType]storage.Provider{
				"environ": storageProvider,
			},
		},
	}
	err := common.Destroy(env, context.Background())
	c.Assert(err, tc.ErrorMatches, "destroying storage: destroying volumes: cannot destroy vol-1, cannot destroy vol-2")
}

func (s *DestroySuite) TestIgnoreStaticVolumes(c *tc.C) {
	staticProvider := &dummy.StorageProvider{
		IsDynamic:    false,
		StorageScope: storage.ScopeEnviron,
	}

	env := &mockEnviron{
		config: configGetter(c),
		allInstances: func(context.Context) ([]instances.Instance, error) {
			return nil, environs.ErrNoInstances
		},
		storageProviders: storage.StaticProviderRegistry{
			map[storage.ProviderType]storage.Provider{
				"static": staticProvider,
			},
		},
	}
	err := common.Destroy(env, context.Background())
	c.Assert(err, tc.ErrorIsNil)

	// common.Destroy will ignore static storage providers.
	staticProvider.CheckCallNames(c, "Dynamic")
}

func (s *DestroySuite) TestIgnoreMachineScopedVolumes(c *tc.C) {
	staticProvider := &dummy.StorageProvider{
		IsDynamic:    true,
		StorageScope: storage.ScopeMachine,
	}

	env := &mockEnviron{
		config: configGetter(c),
		allInstances: func(context.Context) ([]instances.Instance, error) {
			return nil, environs.ErrNoInstances
		},
		storageProviders: storage.StaticProviderRegistry{
			map[storage.ProviderType]storage.Provider{
				"static": staticProvider,
			},
		},
	}
	err := common.Destroy(env, context.Background())
	c.Assert(err, tc.ErrorIsNil)

	// common.Destroy will ignore machine-scoped storage providers.
	staticProvider.CheckCallNames(c, "Dynamic", "Scope")
}

func (s *DestroySuite) TestIgnoreNoVolumeSupport(c *tc.C) {
	staticProvider := &dummy.StorageProvider{
		IsDynamic:    true,
		StorageScope: storage.ScopeEnviron,
		SupportsFunc: func(storage.StorageKind) bool {
			return false
		},
	}

	env := &mockEnviron{
		config: configGetter(c),
		allInstances: func(context.Context) ([]instances.Instance, error) {
			return nil, environs.ErrNoInstances
		},
		storageProviders: storage.StaticProviderRegistry{
			map[storage.ProviderType]storage.Provider{
				"static": staticProvider,
			},
		},
	}
	err := common.Destroy(env, context.Background())
	c.Assert(err, tc.ErrorIsNil)

	// common.Destroy will ignore storage providers that don't support
	// volumes (until we have persistent filesystems, that is).
	staticProvider.CheckCallNames(c, "Dynamic", "Scope", "Supports")
}
