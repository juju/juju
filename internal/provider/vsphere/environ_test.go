// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/vsphere"
	"github.com/juju/juju/internal/testing"
)

type environSuite struct {
	EnvironFixture
}

var _ = tc.Suite(&environSuite{})

func (s *environSuite) TestBootstrap(c *tc.C) {
	s.PatchValue(&vsphere.Bootstrap, func(
		ctx environs.BootstrapContext,
		env environs.Environ,
		args environs.BootstrapParams,
	) (*environs.BootstrapResult, error) {
		return nil, errors.New("Bootstrap called")
	})

	_, err := s.env.Bootstrap(envtesting.BootstrapTestContext(c), environs.BootstrapParams{
		ControllerConfig: testing.FakeControllerConfig(),
	})
	c.Assert(err, tc.ErrorMatches, "Bootstrap called")

	// We dial a connection before calling calling Bootstrap,
	// in order to create the VM folder.
	s.dialStub.CheckCallNames(c, "Dial")
	s.client.CheckCallNames(c, "EnsureVMFolder", "Close")
	ensureVMFolderCall := s.client.Calls()[0]
	c.Assert(ensureVMFolderCall.Args, tc.HasLen, 3)
	c.Assert(ensureVMFolderCall.Args[0], tc.Implements, new(context.Context))
	c.Assert(ensureVMFolderCall.Args[2], tc.Equals,
		`Juju Controller (deadbeef-1bad-500d-9000-4b1d0d06f00d)/Model "testmodel" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)`,
	)
}

func (s *environSuite) TestDestroy(c *tc.C) {
	var destroyCalled bool
	s.PatchValue(&vsphere.DestroyEnv, func(env environs.Environ, ctx context.Context) error {
		destroyCalled = true
		s.client.CheckNoCalls(c)
		return nil
	})
	err := s.env.Destroy(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(destroyCalled, tc.IsTrue)
	s.client.CheckCallNames(c, "DestroyVMFolder", "Close")
	destroyVMFolderCall := s.client.Calls()[0]
	c.Assert(destroyVMFolderCall.Args, tc.HasLen, 2)
	c.Assert(destroyVMFolderCall.Args[0], tc.Implements, new(context.Context))
	c.Assert(destroyVMFolderCall.Args[1], tc.Equals,
		`Juju Controller (*)/Model "testmodel" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)`,
	)
}

func (s *environSuite) TestDestroyController(c *tc.C) {
	s.client.datastores = []mo.Datastore{{
		ManagedEntity: mo.ManagedEntity{Name: "foo"},
	}, {
		ManagedEntity: mo.ManagedEntity{Name: "bar"},
		Summary: types.DatastoreSummary{
			Accessible: true,
		},
	}, {
		ManagedEntity: mo.ManagedEntity{Name: "baz"},
		Summary: types.DatastoreSummary{
			Accessible: true,
		},
	}}

	var destroyCalled bool
	s.PatchValue(&vsphere.DestroyEnv, func(env environs.Environ, ctx context.Context) error {
		destroyCalled = true
		s.client.CheckNoCalls(c)
		return nil
	})
	err := s.env.DestroyController(context.Background(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(destroyCalled, tc.IsTrue)

	s.dialStub.CheckCallNames(c, "Dial")
	s.client.CheckCallNames(c,
		"DestroyVMFolder", "RemoveVirtualMachines", "DestroyVMFolder",
		"Close",
	)

	destroyModelVMFolderCall := s.client.Calls()[0]
	c.Assert(destroyModelVMFolderCall.Args, tc.HasLen, 2)
	c.Assert(destroyModelVMFolderCall.Args[0], tc.Implements, new(context.Context))
	c.Assert(destroyModelVMFolderCall.Args[1], tc.Equals,
		`Juju Controller (*)/Model "testmodel" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)`,
	)

	removeVirtualMachinesCall := s.client.Calls()[1]
	c.Assert(removeVirtualMachinesCall.Args, tc.HasLen, 2)
	c.Assert(removeVirtualMachinesCall.Args[0], tc.Implements, new(context.Context))
	c.Assert(removeVirtualMachinesCall.Args[1], tc.Equals,
		`Juju Controller (foo)/Model "*" (*)/*`,
	)

	destroyControllerVMFolderCall := s.client.Calls()[2]
	c.Assert(destroyControllerVMFolderCall.Args, tc.HasLen, 2)
	c.Assert(destroyControllerVMFolderCall.Args[0], tc.Implements, new(context.Context))
	c.Assert(destroyControllerVMFolderCall.Args[1], tc.Equals, `Juju Controller (foo)`)
}

func (s *environSuite) TestAdoptResources(c *tc.C) {
	err := s.env.AdoptResources(context.Background(), "foo", semversion.Number{})
	c.Assert(err, tc.ErrorIsNil)

	s.dialStub.CheckCallNames(c, "Dial")
	s.client.CheckCallNames(c, "MoveVMFolderInto", "Close")
	moveVMFolderIntoCall := s.client.Calls()[0]
	c.Assert(moveVMFolderIntoCall.Args, tc.HasLen, 3)
	c.Assert(moveVMFolderIntoCall.Args[0], tc.Implements, new(context.Context))
	c.Assert(moveVMFolderIntoCall.Args[1], tc.Equals, `Juju Controller (foo)`)
	c.Assert(moveVMFolderIntoCall.Args[2], tc.Equals,
		`Juju Controller (*)/Model "testmodel" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)`,
	)
}

func (s *environSuite) TestPrepareForBootstrap(c *tc.C) {
	err := s.env.PrepareForBootstrap(envtesting.BootstrapContext(context.Background(), c), "controller-1")
	c.Check(err, tc.ErrorIsNil)
}

func (s *environSuite) TestSupportsNetworking(c *tc.C) {
	_, ok := environs.SupportsNetworking(s.env)
	c.Assert(ok, tc.IsFalse)
}

func (s *environSuite) TestAdoptResourcesPermissionError(c *tc.C) {
	AssertInvalidatesCredential(c, s.client, func(ctx context.Context) error {
		return s.env.AdoptResources(ctx, "foo", semversion.Number{})
	})
}

func (s *environSuite) TestBootstrapPermissionError(c *tc.C) {
	AssertInvalidatesCredential(c, s.client, func(ctx context.Context) error {
		_, err := s.env.Bootstrap(nil, environs.BootstrapParams{
			ControllerConfig:        testing.FakeControllerConfig(),
			SupportedBootstrapBases: testing.FakeSupportedJujuBases,
		})
		return err
	})
}

func (s *environSuite) TestDestroyPermissionError(c *tc.C) {
	AssertInvalidatesCredential(c, s.client, func(ctx context.Context) error {
		return s.env.Destroy(ctx)
	})
}

func (s *environSuite) TestDestroyControllerPermissionError(c *tc.C) {
	AssertInvalidatesCredential(c, s.client, func(ctx context.Context) error {
		return s.env.DestroyController(ctx, "foo")
	})
}
