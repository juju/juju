// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	environscontext "github.com/juju/juju/environs/context"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/vsphere"
	"github.com/juju/juju/testing"
)

type environSuite struct {
	EnvironFixture
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) TestBootstrap(c *gc.C) {
	s.PatchValue(&vsphere.Bootstrap, func(
		ctx environs.BootstrapContext,
		env environs.Environ,
		callCtx environscontext.ProviderCallContext,
		args environs.BootstrapParams,
	) (*environs.BootstrapResult, error) {
		return nil, errors.New("Bootstrap called")
	})

	_, err := s.env.Bootstrap(nil, s.callCtx, environs.BootstrapParams{
		ControllerConfig: testing.FakeControllerConfig(),
	})
	c.Assert(err, gc.ErrorMatches, "Bootstrap called")

	// We dial a connection before calling calling Bootstrap,
	// in order to create the VM folder.
	s.dialStub.CheckCallNames(c, "Dial")
	s.client.CheckCallNames(c, "EnsureVMFolder", "Close")
	ensureVMFolderCall := s.client.Calls()[0]
	c.Assert(ensureVMFolderCall.Args, gc.HasLen, 2)
	c.Assert(ensureVMFolderCall.Args[0], gc.Implements, new(context.Context))
	c.Assert(ensureVMFolderCall.Args[1], gc.Equals,
		`Juju Controller (deadbeef-1bad-500d-9000-4b1d0d06f00d)/Model "testenv" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)`,
	)
}

func (s *environSuite) TestDestroy(c *gc.C) {
	var destroyCalled bool
	s.PatchValue(&vsphere.DestroyEnv, func(env environs.Environ, callCtx environscontext.ProviderCallContext) error {
		destroyCalled = true
		s.client.CheckNoCalls(c)
		return nil
	})
	err := s.env.Destroy(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(destroyCalled, jc.IsTrue)
	s.client.CheckCallNames(c, "DestroyVMFolder", "Close")
	destroyVMFolderCall := s.client.Calls()[0]
	c.Assert(destroyVMFolderCall.Args, gc.HasLen, 2)
	c.Assert(destroyVMFolderCall.Args[0], gc.Implements, new(context.Context))
	c.Assert(destroyVMFolderCall.Args[1], gc.Equals,
		`Juju Controller (*)/Model "testenv" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)`,
	)
}

func (s *environSuite) TestDestroyController(c *gc.C) {
	s.client.datastores = []*mo.Datastore{{
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
	s.PatchValue(&vsphere.DestroyEnv, func(env environs.Environ, callCtx environscontext.ProviderCallContext) error {
		destroyCalled = true
		s.client.CheckNoCalls(c)
		return nil
	})
	err := s.env.DestroyController(s.callCtx, "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(destroyCalled, jc.IsTrue)

	s.dialStub.CheckCallNames(c, "Dial")
	s.client.CheckCallNames(c,
		"DestroyVMFolder", "RemoveVirtualMachines", "DestroyVMFolder",
		"Datastores", "DeleteDatastoreFile", "DeleteDatastoreFile",
		"Close",
	)

	destroyModelVMFolderCall := s.client.Calls()[0]
	c.Assert(destroyModelVMFolderCall.Args, gc.HasLen, 2)
	c.Assert(destroyModelVMFolderCall.Args[0], gc.Implements, new(context.Context))
	c.Assert(destroyModelVMFolderCall.Args[1], gc.Equals,
		`Juju Controller (*)/Model "testenv" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)`,
	)

	removeVirtualMachinesCall := s.client.Calls()[1]
	c.Assert(removeVirtualMachinesCall.Args, gc.HasLen, 2)
	c.Assert(removeVirtualMachinesCall.Args[0], gc.Implements, new(context.Context))
	c.Assert(removeVirtualMachinesCall.Args[1], gc.Equals,
		`Juju Controller (foo)/Model "*" (*)/*`,
	)

	destroyControllerVMFolderCall := s.client.Calls()[2]
	c.Assert(destroyControllerVMFolderCall.Args, gc.HasLen, 2)
	c.Assert(destroyControllerVMFolderCall.Args[0], gc.Implements, new(context.Context))
	c.Assert(destroyControllerVMFolderCall.Args[1], gc.Equals, `Juju Controller (foo)`)

	deleteDatastoreFileCall1 := s.client.Calls()[4]
	c.Assert(deleteDatastoreFileCall1.Args, gc.HasLen, 2)
	c.Assert(deleteDatastoreFileCall1.Args[0], gc.Implements, new(context.Context))
	c.Assert(deleteDatastoreFileCall1.Args[1], gc.Equals, "[bar] juju-vmdks/foo")

	deleteDatastoreFileCall2 := s.client.Calls()[5]
	c.Assert(deleteDatastoreFileCall2.Args, gc.HasLen, 2)
	c.Assert(deleteDatastoreFileCall2.Args[0], gc.Implements, new(context.Context))
	c.Assert(deleteDatastoreFileCall2.Args[1], gc.Equals, "[baz] juju-vmdks/foo")
}

func (s *environSuite) TestAdoptResources(c *gc.C) {
	err := s.env.AdoptResources(s.callCtx, "foo", version.Number{})
	c.Assert(err, jc.ErrorIsNil)

	s.dialStub.CheckCallNames(c, "Dial")
	s.client.CheckCallNames(c, "MoveVMFolderInto", "Close")
	moveVMFolderIntoCall := s.client.Calls()[0]
	c.Assert(moveVMFolderIntoCall.Args, gc.HasLen, 3)
	c.Assert(moveVMFolderIntoCall.Args[0], gc.Implements, new(context.Context))
	c.Assert(moveVMFolderIntoCall.Args[1], gc.Equals, `Juju Controller (foo)`)
	c.Assert(moveVMFolderIntoCall.Args[2], gc.Equals,
		`Juju Controller (*)/Model "testenv" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)`,
	)
}

func (s *environSuite) TestPrepareForBootstrap(c *gc.C) {
	err := s.env.PrepareForBootstrap(envtesting.BootstrapContext(c))
	c.Check(err, jc.ErrorIsNil)
}

func (s *environSuite) TestSupportsNetworking(c *gc.C) {
	_, ok := environs.SupportsNetworking(s.env)
	c.Assert(ok, jc.IsFalse)
}
