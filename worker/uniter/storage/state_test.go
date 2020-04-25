// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/charm/v7/hooks"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/storage"
)

type stateSuite struct {
	tag1 names.StorageTag
	st   *storage.State
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.tag1 = names.NewStorageTag("test/1")
	s.st = storage.NewState()
}

func (s *stateSuite) TestAttached(c *gc.C) {
	_, found := s.st.Attached(s.tag1.Id())
	c.Assert(found, jc.IsFalse)
	s.st.Attach(s.tag1.Id())
	attached, found := s.st.Attached(s.tag1.Id())
	c.Assert(found, jc.IsTrue)
	c.Assert(attached, jc.IsTrue)
}

func (s *stateSuite) TestAttachedDetached(c *gc.C) {
	s.st.Attach(s.tag1.Id())
	err := s.st.Detach(s.tag1.Id())
	c.Assert(err, jc.ErrorIsNil)
	attached, found := s.st.Attached(s.tag1.Id())
	c.Assert(found, jc.IsTrue)
	c.Assert(attached, jc.IsFalse)
}

func (s *stateSuite) TestDetach(c *gc.C) {
	s.st.Attach(s.tag1.Id())
	attached, found := s.st.Attached(s.tag1.Id())
	c.Assert(found, jc.IsTrue)
	c.Assert(attached, jc.IsTrue)
	err := s.st.Detach(s.tag1.Id())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestDetachErr(c *gc.C) {
	err := s.st.Detach(s.tag1.Id())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *stateSuite) TestEmpty(c *gc.C) {
	c.Assert(s.st.Empty(), jc.IsTrue)
}

func (s *stateSuite) TestNotEmpty(c *gc.C) {
	s.st.Attach(s.tag1.Id())
	c.Assert(s.st.Empty(), jc.IsFalse)
}

func (s *stateSuite) TestValidateHookStorageDetaching(c *gc.C) {
	s.st.Attach(s.tag1.Id())
	hi := hook.Info{Kind: hooks.StorageDetaching, StorageId: s.tag1.Id()}
	err := s.st.ValidateHook(hi)
	c.Assert(err, jc.ErrorIsNil)

}

func (s *stateSuite) TestValidateHookStorageDetachingError(c *gc.C) {
	s.st.Attach(s.tag1.Id())
	err := s.st.Detach(s.tag1.Id())
	c.Assert(err, jc.ErrorIsNil)
	hi := hook.Info{Kind: hooks.StorageDetaching, StorageId: s.tag1.Id()}
	err = s.st.ValidateHook(hi)
	c.Assert(err, gc.NotNil)

}

func (s *stateSuite) TestValidateHookStorageAttached(c *gc.C) {
	hi := hook.Info{Kind: hooks.StorageAttached, StorageId: s.tag1.Id()}
	err := s.st.ValidateHook(hi)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestValidateHookStorageAttachedError(c *gc.C) {
	s.st.Attach(s.tag1.Id())
	hi := hook.Info{Kind: hooks.StorageAttached, StorageId: s.tag1.Id()}
	err := s.st.ValidateHook(hi)
	c.Assert(err, gc.NotNil)
}

type stateOpsSuite struct {
	mockStateOpsSuite

	tag1 names.StorageTag
	tag2 names.StorageTag
	tag3 names.StorageTag
}

var _ = gc.Suite(&stateOpsSuite{})

func (s *stateOpsSuite) SetUpSuite(c *gc.C) {
	s.tag1 = names.NewStorageTag("test/1")
	s.tag2 = names.NewStorageTag("test/2")
	s.tag3 = names.NewStorageTag("test/3")
}

func (s *stateOpsSuite) SetUpTest(c *gc.C) {
	s.storSt = storage.NewState()
	s.storSt.Attach(s.tag1.Id())
	s.storSt.Attach(s.tag2.Id())
	c.Assert(s.storSt.Detach(s.tag2.Id()), jc.ErrorIsNil)
	s.storSt.Attach(s.tag3.Id())
}

func (s *stateOpsSuite) TestRead(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectState(c)
	ops := storage.NewStateOps(s.mockStateOps)
	obtainedSt, err := ops.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storage.Storage(obtainedSt), gc.DeepEquals, storage.Storage(s.storSt))
}

func (s *stateOpsSuite) TestReadNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateNotFound()
	ops := storage.NewStateOps(s.mockStateOps)
	obtainedSt, err := ops.Read()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(obtainedSt, gc.NotNil)
}

func (s *stateOpsSuite) TestWrite(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSetState(c, "")
	ops := storage.NewStateOps(s.mockStateOps)
	err := ops.Write(s.storSt)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateOpsSuite) TestWriteEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSetStateEmpty(c)
	ops := storage.NewStateOps(s.mockStateOps)
	err := ops.Write(storage.NewState())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateOpsSuite) TestWriteNilState(c *gc.C) {
	defer s.setupMocks(c).Finish()
	ops := storage.NewStateOps(s.mockStateOps)
	err := ops.Write(nil)
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}
