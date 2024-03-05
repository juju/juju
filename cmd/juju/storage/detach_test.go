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
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

type DetachStorageSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&DetachStorageSuite{})

func (s *DetachStorageSuite) TestDetach(c *gc.C) {
	fake := fakeEntityDetacher{results: []params.ErrorResult{
		{},
		{},
	}}
	command := storage.NewDetachStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	ctx, err := cmdtesting.RunCommand(c, command, "foo/0", "bar/1")
	c.Assert(err, jc.ErrorIsNil)
	fake.CheckCallNames(c, "NewEntityDetacherCloser", "Detach", "Close")
	force := false
	fake.CheckCall(c, 1, "Detach", []string{"foo/0", "bar/1"}, &force, (*time.Duration)(nil))
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
detaching foo/0
detaching bar/1
`[1:])
}

func (s *DetachStorageSuite) TestDetachError(c *gc.C) {
	fake := fakeEntityDetacher{results: []params.ErrorResult{
		{Error: &params.Error{Message: "foo"}},
		{Error: &params.Error{Message: "bar"}},
	}}
	detachCmd := storage.NewDetachStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	ctx, err := cmdtesting.RunCommand(c, detachCmd, "baz/0", "qux/1")
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `failed to detach baz/0: foo
failed to detach qux/1: bar
`)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
}

func (s *DetachStorageSuite) TestDetachUnauthorizedError(c *gc.C) {
	var fake fakeEntityDetacher
	fake.SetErrors(nil, &params.Error{Code: params.CodeUnauthorized, Message: "nope"})
	command := storage.NewDetachStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	ctx, err := cmdtesting.RunCommand(c, command, "foo/0")
	c.Assert(err, gc.ErrorMatches, "nope")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
You do not have permission to detach storage.
You may ask an administrator to grant you access with "juju grant".

`)
}

func (s *DetachStorageSuite) TestDetachInitErrors(c *gc.C) {
	s.testDetachInitError(c, []string{}, "detach-storage requires at least one storage ID")
}

func (s *DetachStorageSuite) testDetachInitError(c *gc.C, args []string, expect string) {
	command := storage.NewDetachStorageCommandForTest(nil, jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, command, args...)
	c.Assert(err, gc.ErrorMatches, expect)
}

type fakeEntityDetacher struct {
	testing.Stub
	results []params.ErrorResult
}

func (f *fakeEntityDetacher) new() (storage.EntityDetacherCloser, error) {
	f.MethodCall(f, "NewEntityDetacherCloser")
	return f, f.NextErr()
}

func (f *fakeEntityDetacher) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeEntityDetacher) Detach(ids []string, force *bool, maxWait *time.Duration) ([]params.ErrorResult, error) {
	f.MethodCall(f, "Detach", ids, force, maxWait)
	return f.results, f.NextErr()
}
