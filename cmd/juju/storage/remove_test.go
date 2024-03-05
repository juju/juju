// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"time"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

type RemoveStorageSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RemoveStorageSuite{})

func (s *RemoveStorageSuite) TestRemoveStorage(c *gc.C) {
	fake := fakeStorageRemover{results: []params.ErrorResult{
		{},
		{},
	}}
	command := storage.NewRemoveStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	ctx, err := cmdtesting.RunCommand(c, command, "pgdata/0", "pgdata/1")
	c.Assert(err, jc.ErrorIsNil)
	fake.CheckCallNames(c, "NewStorageRemoverCloser", "Remove", "Close")
	force := false
	fake.CheckCall(c, 1, "Remove", []string{"pgdata/0", "pgdata/1"}, false, true, &force, (*time.Duration)(nil))
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
removing pgdata/0
removing pgdata/1
`[1:])
}

func (s *RemoveStorageSuite) TestRemoveStorageForce(c *gc.C) {
	fake := fakeStorageRemover{results: []params.ErrorResult{
		{},
		{},
	}}
	command := storage.NewRemoveStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, command, "--force", "pgdata/0", "pgdata/1")
	c.Assert(err, jc.ErrorIsNil)
	force := true
	fake.CheckCall(c, 1, "Remove", []string{"pgdata/0", "pgdata/1"}, true, true, &force, (*time.Duration)(nil))
}

func (s *RemoveStorageSuite) TestRemoveStorageNoDestroy(c *gc.C) {
	fake := fakeStorageRemover{results: []params.ErrorResult{
		{},
		{},
	}}
	command := storage.NewRemoveStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, command, "--no-destroy", "--force", "pgdata/0", "pgdata/1")
	c.Assert(err, jc.ErrorIsNil)
	force := true
	fake.CheckCall(c, 1, "Remove", []string{"pgdata/0", "pgdata/1"}, true, false, &force, (*time.Duration)(nil))
}

func (s *RemoveStorageSuite) TestRemoveStorageError(c *gc.C) {
	fake := fakeStorageRemover{results: []params.ErrorResult{
		{Error: &params.Error{Message: "foo"}},
		{Error: &params.Error{Message: "storage is attached", Code: params.CodeStorageAttached}},
	}}
	removeCmd := storage.NewRemoveStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	ctx, err := cmdtesting.RunCommand(c, removeCmd, "pgdata/0", "pgdata/1")
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `failed to remove pgdata/0: foo
failed to remove pgdata/1: storage is attached

Use the --force option to remove attached storage, or use
"juju detach-storage" to explicitly detach the storage
before removing.
`)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
}

func (s *RemoveStorageSuite) TestRemoveStorageCAASError(c *gc.C) {
	fake := fakeStorageRemover{results: []params.ErrorResult{
		{Error: &params.Error{Message: "foo"}},
		{Error: &params.Error{Message: "storage is attached", Code: params.CodeStorageAttached}},
	}}
	store := jujuclienttesting.MinimalStore()
	m := store.Models["arthur"].Models["king/sword"]
	m.ModelType = model.CAAS
	store.Models["arthur"].Models["king/sword"] = m
	removeCmd := storage.NewRemoveStorageCommandForTest(fake.new, store)
	ctx, err := cmdtesting.RunCommand(c, removeCmd, "pgdata/0", "pgdata/1")
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `failed to remove pgdata/0: foo
failed to remove pgdata/1: storage is attached
`)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
}

func (s *RemoveStorageSuite) TestRemoveStorageUnauthorizedError(c *gc.C) {
	var fake fakeStorageRemover
	fake.SetErrors(nil, &params.Error{Code: params.CodeUnauthorized, Message: "nope"})
	command := storage.NewRemoveStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	ctx, err := cmdtesting.RunCommand(c, command, "pgdata/0")
	c.Assert(err, gc.ErrorMatches, "nope")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
You do not have permission to remove storage.
You may ask an administrator to grant you access with "juju grant".

`)
}

func (s *RemoveStorageSuite) TestRemoveStorageInitErrors(c *gc.C) {
	s.testRemoveStorageInitError(c, []string{}, "remove-storage requires at least one storage ID")
	s.testRemoveStorageCAASInitError(c, []string{"--force", "storage/0"}, "forced detachment of storage on container models not supported")
}

func (s *RemoveStorageSuite) testRemoveStorageInitError(c *gc.C, args []string, expect string) {
	var fake fakeStorageRemover
	command := storage.NewRemoveStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, command, args...)
	c.Assert(err, gc.ErrorMatches, expect)
}

func (s *RemoveStorageSuite) testRemoveStorageCAASInitError(c *gc.C, args []string, expect string) {
	var fake fakeStorageRemover
	store := jujuclienttesting.MinimalStore()
	m := store.Models["arthur"].Models["king/sword"]
	m.ModelType = model.CAAS
	store.Models["arthur"].Models["king/sword"] = m
	command := storage.NewRemoveStorageCommandForTest(fake.new, store)
	_, err := cmdtesting.RunCommand(c, command, args...)
	c.Assert(err, gc.ErrorMatches, expect)
}

type fakeStorageRemover struct {
	testing.Stub
	results []params.ErrorResult
}

func (f *fakeStorageRemover) new() (storage.StorageRemoverCloser, error) {
	f.MethodCall(f, "NewStorageRemoverCloser")
	return f, f.NextErr()
}

func (f *fakeStorageRemover) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeStorageRemover) Remove(ids []string, destroyAttached, destroyStorage bool, force *bool, maxWait *time.Duration) ([]params.ErrorResult, error) {
	f.MethodCall(f, "Remove", ids, destroyAttached, destroyStorage, force, maxWait)
	return f.results, f.NextErr()
}
