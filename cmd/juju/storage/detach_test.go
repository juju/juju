// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

type DetachStorageSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&DetachStorageSuite{})

func (s *DetachStorageSuite) TestDetach(c *tc.C) {
	fake := fakeEntityDetacher{results: []params.ErrorResult{
		{},
		{},
	}}
	command := storage.NewDetachStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	ctx, err := cmdtesting.RunCommand(c, command, "foo/0", "bar/1")
	c.Assert(err, tc.ErrorIsNil)
	fake.CheckCallNames(c, "NewEntityDetacherCloser", "Detach", "Close")
	force := false
	fake.CheckCall(c, 1, "Detach", []string{"foo/0", "bar/1"}, &force, (*time.Duration)(nil))
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
detaching foo/0
detaching bar/1
`[1:])
}

func (s *DetachStorageSuite) TestDetachError(c *tc.C) {
	fake := fakeEntityDetacher{results: []params.ErrorResult{
		{Error: &params.Error{Message: "foo"}},
		{Error: &params.Error{Message: "bar"}},
	}}
	detachCmd := storage.NewDetachStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	ctx, err := cmdtesting.RunCommand(c, detachCmd, "baz/0", "qux/1")
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, tc.Equals, `failed to detach baz/0: foo
failed to detach qux/1: bar
`)
	c.Assert(err, tc.Equals, cmd.ErrSilent)
}

func (s *DetachStorageSuite) TestDetachUnauthorizedError(c *tc.C) {
	var fake fakeEntityDetacher
	fake.SetErrors(nil, &params.Error{Code: params.CodeUnauthorized, Message: "nope"})
	command := storage.NewDetachStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	ctx, err := cmdtesting.RunCommand(c, command, "foo/0")
	c.Assert(err, tc.ErrorMatches, "nope")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
You do not have permission to detach storage.
You may ask an administrator to grant you access with "juju grant".

`)
}

func (s *DetachStorageSuite) TestDetachInitErrors(c *tc.C) {
	s.testDetachInitError(c, []string{}, "detach-storage requires at least one storage ID")
}

func (s *DetachStorageSuite) testDetachInitError(c *tc.C, args []string, expect string) {
	command := storage.NewDetachStorageCommandForTest(nil, jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, command, args...)
	c.Assert(err, tc.ErrorMatches, expect)
}

type fakeEntityDetacher struct {
	testhelpers.Stub
	results []params.ErrorResult
}

func (f *fakeEntityDetacher) new(ctx context.Context) (storage.EntityDetacherCloser, error) {
	f.MethodCall(f, "NewEntityDetacherCloser")
	err := f.NextErr()
	return f, err
}

func (f *fakeEntityDetacher) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeEntityDetacher) Detach(ctx context.Context, ids []string, force *bool, maxWait *time.Duration) ([]params.ErrorResult, error) {
	f.MethodCall(f, "Detach", ids, force, maxWait)
	return f.results, f.NextErr()
}
