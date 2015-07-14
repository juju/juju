// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/operation"
)

type UpdateStorageSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&UpdateStorageSuite{})

func (s *UpdateStorageSuite) TestPrepare(c *gc.C) {
	factory := operation.NewFactory(operation.FactoryParams{})
	op, err := factory.NewUpdateStorage(nil)
	c.Assert(err, jc.ErrorIsNil)
	state, err := op.Prepare(operation.State{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(state, gc.IsNil)
}

func (s *UpdateStorageSuite) TestExecuteError(c *gc.C) {
	updater := &mockStorageUpdater{err: errors.New("meep")}
	factory := operation.NewFactory(operation.FactoryParams{StorageUpdater: updater})

	tag0 := names.NewStorageTag("data/0")
	tag1 := names.NewStorageTag("data/1")
	tags := []names.StorageTag{tag0, tag1}
	op, err := factory.NewUpdateStorage(tags)
	c.Assert(err, jc.ErrorIsNil)
	state, err := op.Prepare(operation.State{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(state, gc.IsNil)

	state, err = op.Execute(operation.State{})
	c.Check(err, gc.ErrorMatches, "meep")
	c.Check(state, gc.IsNil)
	c.Check(updater.tags, jc.DeepEquals, [][]names.StorageTag{tags})
}

func (s *UpdateStorageSuite) TestExecuteSuccess(c *gc.C) {
	updater := &mockStorageUpdater{}
	factory := operation.NewFactory(operation.FactoryParams{StorageUpdater: updater})

	tag0 := names.NewStorageTag("data/0")
	tag1 := names.NewStorageTag("data/1")
	tags := []names.StorageTag{tag0, tag1}
	op, err := factory.NewUpdateStorage(tags)
	c.Assert(err, jc.ErrorIsNil)
	state, err := op.Prepare(operation.State{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(state, gc.IsNil)

	state, err = op.Execute(operation.State{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(state, gc.IsNil)
	c.Check(updater.tags, jc.DeepEquals, [][]names.StorageTag{tags})
}

func (s *UpdateStorageSuite) TestCommit(c *gc.C) {
	factory := operation.NewFactory(operation.FactoryParams{})
	op, err := factory.NewUpdateStorage(nil)
	c.Assert(err, jc.ErrorIsNil)
	state, err := op.Commit(operation.State{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(state, gc.IsNil)
}

func (s *UpdateStorageSuite) TestDoesNotNeedGlobalMachineLock(c *gc.C) {
	factory := operation.NewFactory(operation.FactoryParams{})
	op, err := factory.NewUpdateStorage(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.NeedsGlobalMachineLock(), jc.IsFalse)
}

type mockStorageUpdater struct {
	tags [][]names.StorageTag
	err  error
}

func (u *mockStorageUpdater) UpdateStorage(tags []names.StorageTag) error {
	u.tags = append(u.tags, tags)
	return u.err
}
