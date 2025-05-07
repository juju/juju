// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"regexp"

	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

type AttachStorageSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&AttachStorageSuite{})

func (s *AttachStorageSuite) TestAttach(c *tc.C) {
	fake := fakeEntityAttacher{results: []params.ErrorResult{
		{},
		{},
	}}
	cmd := storage.NewAttachStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	ctx, err := cmdtesting.RunCommand(c, cmd, "foo/0", "bar/1", "baz/2")
	c.Assert(err, tc.ErrorIsNil)
	fake.CheckCallNames(c, "NewEntityAttacherCloser", "Attach", "Close")
	fake.CheckCall(c, 1, "Attach", "foo/0", []string{"bar/1", "baz/2"})
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
attaching bar/1 to foo/0
attaching baz/2 to foo/0
`[1:])
}

func (s *AttachStorageSuite) TestAttachError(c *tc.C) {
	fake := fakeEntityAttacher{results: []params.ErrorResult{
		{Error: &params.Error{Message: "foo"}},
		{Error: &params.Error{Message: "bar"}},
	}}
	attachCmd := storage.NewAttachStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	ctx, err := cmdtesting.RunCommand(c, attachCmd, "baz/0", "qux/1", "quux/2")
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, tc.Equals, `failed to attach qux/1 to baz/0: foo
failed to attach quux/2 to baz/0: bar
`)
	c.Assert(err, tc.Equals, cmd.ErrSilent)
}

func (s *AttachStorageSuite) TestAttachUnauthorizedError(c *tc.C) {
	var fake fakeEntityAttacher
	fake.SetErrors(nil, &params.Error{Code: params.CodeUnauthorized, Message: "nope"})
	cmd := storage.NewAttachStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	ctx, err := cmdtesting.RunCommand(c, cmd, "foo/0", "bar/1")
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta("could not attach storage [bar/1]: nope"))
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
You do not have permission to attach storage.
You may ask an administrator to grant you access with "juju grant".

`)
}

func (s *AttachStorageSuite) TestAttachBlocked(c *tc.C) {
	var fake fakeEntityAttacher
	fake.SetErrors(nil, &params.Error{Code: params.CodeOperationBlocked, Message: "nope"})
	cmd := storage.NewAttachStorageCommandForTest(fake.new, jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, cmd, "foo/0", "bar/1")
	c.Assert(err.Error(), tc.Contains, `could not attach storage [bar/1]: nope`)
	c.Assert(err.Error(), tc.Contains, `All operations that change model have been disabled for the current model.`)
}

func (s *AttachStorageSuite) TestAttachInitErrors(c *tc.C) {
	s.testAttachInitError(c, []string{}, "attach-storage requires a unit ID and at least one storage ID")
	s.testAttachInitError(c, []string{"unit/0"}, "attach-storage requires a unit ID and at least one storage ID")
}

func (s *AttachStorageSuite) testAttachInitError(c *tc.C, args []string, expect string) {
	cmd := storage.NewAttachStorageCommandForTest(nil, jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, cmd, args...)
	c.Assert(err, tc.ErrorMatches, expect)
}

type fakeEntityAttacher struct {
	testing.Stub
	results []params.ErrorResult
}

func (f *fakeEntityAttacher) new(ctx context.Context) (storage.EntityAttacherCloser, error) {
	f.MethodCall(f, "NewEntityAttacherCloser")
	err := f.NextErr()
	return f, err
}

func (f *fakeEntityAttacher) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeEntityAttacher) Attach(ctx context.Context, unitId string, storageIds []string) ([]params.ErrorResult, error) {
	f.MethodCall(f, "Attach", unitId, storageIds)
	return f.results, f.NextErr()
}
