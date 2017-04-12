// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/cmd"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/cmdtesting"
	"github.com/juju/juju/cmd/juju/storage"
)

type RemoveStorageSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RemoveStorageSuite{})

func (s *RemoveStorageSuite) TestRemoveStorage(c *gc.C) {
	fake := fakeEntityDestroyer{results: []params.ErrorResult{
		{},
		{},
	}}
	cmd := storage.NewRemoveStorageCommand(fake.new)
	ctx, err := cmdtesting.RunCommand(c, cmd, "pgdata/0", "pgdata/1")
	c.Assert(err, jc.ErrorIsNil)
	fake.CheckCallNames(c, "NewEntityDestroyerCloser", "Destroy", "Close")
	fake.CheckCall(c, 1, "Destroy", []string{"pgdata/0", "pgdata/1"})
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
removing pgdata/0
removing pgdata/1
`[1:])
}

func (s *RemoveStorageSuite) TestRemoveStorageError(c *gc.C) {
	fake := fakeEntityDestroyer{results: []params.ErrorResult{
		{Error: &params.Error{Message: "foo"}},
		{Error: &params.Error{Message: "bar"}},
	}}
	removeCmd := storage.NewRemoveStorageCommand(fake.new)
	ctx, err := cmdtesting.RunCommand(c, removeCmd, "pgdata/0", "pgdata/1")
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `failed to remove pgdata/0: foo
failed to remove pgdata/1: bar
`)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
}

func (s *RemoveStorageSuite) TestRemoveStorageUnauthorizedError(c *gc.C) {
	var fake fakeEntityDestroyer
	fake.SetErrors(nil, &params.Error{Code: params.CodeUnauthorized, Message: "nope"})
	cmd := storage.NewRemoveStorageCommand(fake.new)
	ctx, err := cmdtesting.RunCommand(c, cmd, "pgdata/0")
	c.Assert(err, gc.ErrorMatches, "nope")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
You do not have permission to remove storage.
You may ask an administrator to grant you access with "juju grant".

`)
}

func (s *RemoveStorageSuite) TestRemoveStorageInitErrors(c *gc.C) {
	s.testRemoveStorageInitError(c, []string{}, "remove-storage requires at least one storage ID")
}

func (s *RemoveStorageSuite) testRemoveStorageInitError(c *gc.C, args []string, expect string) {
	var fake fakeEntityDestroyer
	cmd := storage.NewRemoveStorageCommand(fake.new)
	_, err := cmdtesting.RunCommand(c, cmd, args...)
	c.Assert(err, gc.ErrorMatches, expect)
}

type fakeEntityDestroyer struct {
	testing.Stub
	results []params.ErrorResult
}

func (f *fakeEntityDestroyer) new() (storage.EntityDestroyerCloser, error) {
	f.MethodCall(f, "NewEntityDestroyerCloser")
	return f, f.NextErr()
}

func (f *fakeEntityDestroyer) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeEntityDestroyer) Destroy(ids []string) ([]params.ErrorResult, error) {
	f.MethodCall(f, "Destroy", ids)
	return f.results, f.NextErr()
}
