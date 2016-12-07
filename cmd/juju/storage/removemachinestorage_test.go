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

type RemoveMachineStorageSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RemoveMachineStorageSuite{})

func (s *RemoveMachineStorageSuite) TestRemoveVolume(c *gc.C) {
	var fake fakeEntityDestroyer
	cmd := storage.NewRemoveVolumeCommand(fake.new)
	_, err := coretesting.RunCommand(c, cmd, "0", "1/1")
	c.Assert(err, jc.ErrorIsNil)
	fake.CheckCallNames(c, "NewEntityDestroyerCloser", "Destroy", "Close")
	fake.CheckCall(c, 1, "Destroy", []names.Tag{
		names.NewVolumeTag("0"),
		names.NewVolumeTag("1/1"),
	})
}

func (s *RemoveMachineStorageSuite) TestRemoveVolumeError(c *gc.C) {
	fake := fakeEntityDestroyer{results: []params.ErrorResult{
		{Error: &params.Error{Message: "foo"}},
		{Error: &params.Error{Message: "bar"}},
	}}
	cmd := storage.NewRemoveVolumeCommand(fake.new)
	_, err := coretesting.RunCommand(c, cmd, "0", "1/1")
	c.Assert(err, gc.ErrorMatches, "foo\nbar")
}

func (s *RemoveMachineStorageSuite) TestRemoveVolumeUnauthorizedError(c *gc.C) {
	var fake fakeEntityDestroyer
	fake.SetErrors(nil, &params.Error{Code: params.CodeUnauthorized, Message: "nope"})
	cmd := storage.NewRemoveVolumeCommand(fake.new)
	ctx, err := coretesting.RunCommand(c, cmd, "0")
	c.Assert(err, gc.ErrorMatches, "nope")
	c.Assert(coretesting.Stderr(ctx), gc.Equals, `
You do not have permission to remove storage.
You may ask an administrator to grant you access with "juju grant".

`)
}

func (s *RemoveMachineStorageSuite) TestRemoveVolumeInitErrors(c *gc.C) {
	s.testRemoveVolumeInitError(c, []string{}, "remove-volume requires a volume ID")
	s.testRemoveVolumeInitError(c, []string{"abc"}, `volume ID "abc" not valid`)
}

func (s *RemoveMachineStorageSuite) testRemoveVolumeInitError(c *gc.C, args []string, expect string) {
	cmd := storage.NewRemoveVolumeCommand(nil)
	_, err := coretesting.RunCommand(c, cmd, args...)
	c.Assert(err, gc.ErrorMatches, expect)
}

func (s *RemoveMachineStorageSuite) TestRemoveFilesystem(c *gc.C) {
	var fake fakeEntityDestroyer
	cmd := storage.NewRemoveFilesystemCommand(fake.new)
	_, err := coretesting.RunCommand(c, cmd, "0", "1/1")
	c.Assert(err, jc.ErrorIsNil)
	fake.CheckCallNames(c, "NewEntityDestroyerCloser", "Destroy", "Close")
	fake.CheckCall(c, 1, "Destroy", []names.Tag{
		names.NewFilesystemTag("0"),
		names.NewFilesystemTag("1/1"),
	})
}

func (s *RemoveMachineStorageSuite) TestRemoveFilesystemError(c *gc.C) {
	fake := fakeEntityDestroyer{results: []params.ErrorResult{
		{Error: &params.Error{Message: "foo"}},
		{Error: &params.Error{Message: "bar"}},
	}}
	cmd := storage.NewRemoveFilesystemCommand(fake.new)
	_, err := coretesting.RunCommand(c, cmd, "0", "1/1")
	c.Assert(err, gc.ErrorMatches, "foo\nbar")
}

func (s *RemoveMachineStorageSuite) TestRemoveFilesystemUnauthorizedError(c *gc.C) {
	var fake fakeEntityDestroyer
	fake.SetErrors(nil, &params.Error{Code: params.CodeUnauthorized, Message: "nope"})
	cmd := storage.NewRemoveFilesystemCommand(fake.new)
	ctx, err := coretesting.RunCommand(c, cmd, "0")
	c.Assert(err, gc.ErrorMatches, "nope")
	c.Assert(coretesting.Stderr(ctx), gc.Equals, `
You do not have permission to remove storage.
You may ask an administrator to grant you access with "juju grant".

`)
}

func (s *RemoveMachineStorageSuite) TestRemoveFilesystemInitErrors(c *gc.C) {
	s.testRemoveFilesystemInitError(c, []string{}, "remove-filesystem requires a filesystem ID")
	s.testRemoveFilesystemInitError(c, []string{"abc"}, `filesystem ID "abc" not valid`)
}

func (s *RemoveMachineStorageSuite) testRemoveFilesystemInitError(c *gc.C, args []string, expect string) {
	cmd := storage.NewRemoveFilesystemCommand(nil)
	_, err := coretesting.RunCommand(c, cmd, args...)
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

func (f *fakeEntityDestroyer) Destroy(tags []names.Tag) ([]params.ErrorResult, error) {
	f.MethodCall(f, "Destroy", tags)
	return f.results, f.NextErr()
}
