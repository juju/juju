// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

type DetachStorageSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&DetachStorageSuite{})

func (s *DetachStorageSuite) TestRemove(c *gc.C) {
	var fake fakeEntityDestroyer
	cmd := storage.NewDetachStorageCommand(fake.new)
	_, err := coretesting.RunCommand(c, cmd, "foo/0", "bar/1")
	c.Assert(err, jc.ErrorIsNil)
	fake.CheckCallNames(c, "NewEntityDestroyerCloser", "Destroy", "Close")
	fake.CheckCall(c, 1, "Destroy", []names.Tag{
		names.NewStorageTag("foo/0"),
		names.NewStorageTag("bar/1"),
	})
}

func (s *DetachStorageSuite) TestRemoveError(c *gc.C) {
	fake := fakeEntityDestroyer{results: []params.ErrorResult{
		{Error: &params.Error{Message: "foo"}},
		{Error: &params.Error{Message: "bar"}},
	}}
	cmd := storage.NewDetachStorageCommand(fake.new)
	_, err := coretesting.RunCommand(c, cmd, "baz/0", "qux/1")
	c.Assert(err, gc.ErrorMatches, "foo\nbar")
}

func (s *DetachStorageSuite) TestRemoveUnauthorizedError(c *gc.C) {
	var fake fakeEntityDestroyer
	fake.SetErrors(nil, &params.Error{Code: params.CodeUnauthorized, Message: "nope"})
	cmd := storage.NewDetachStorageCommand(fake.new)
	ctx, err := coretesting.RunCommand(c, cmd, "foo/0")
	c.Assert(err, gc.ErrorMatches, "nope")
	c.Assert(coretesting.Stderr(ctx), gc.Equals, `
You do not have permission to remove storage.
You may ask an administrator to grant you access with "juju grant".

`)
}

func (s *DetachStorageSuite) TestRemoveInitErrors(c *gc.C) {
	s.testRemoveInitError(c, []string{}, "remove-storage requires a storage ID")
	s.testRemoveInitError(c, []string{"abc"}, `storage ID "abc" not valid`)
}

func (s *DetachStorageSuite) testRemoveInitError(c *gc.C, args []string, expect string) {
	cmd := storage.NewDetachStorageCommand(nil)
	_, err := coretesting.RunCommand(c, cmd, args...)
	c.Assert(err, gc.ErrorMatches, expect)
}
