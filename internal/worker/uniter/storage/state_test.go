// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/storage"
)

type stateSuite struct {
	tag1 names.StorageTag
	st   *storage.State
}

var _ = tc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.tag1 = names.NewStorageTag("test/1")
	s.st = storage.NewState()
}

func (s *stateSuite) TestAttached(c *tc.C) {
	_, found := s.st.Attached(s.tag1.Id())
	c.Assert(found, jc.IsFalse)
	s.st.Attach(s.tag1.Id())
	attached, found := s.st.Attached(s.tag1.Id())
	c.Assert(found, jc.IsTrue)
	c.Assert(attached, jc.IsTrue)
}

func (s *stateSuite) TestAttachedDetached(c *tc.C) {
	s.st.Attach(s.tag1.Id())
	err := s.st.Detach(s.tag1.Id())
	c.Assert(err, jc.ErrorIsNil)
	attached, found := s.st.Attached(s.tag1.Id())
	c.Assert(found, jc.IsTrue)
	c.Assert(attached, jc.IsFalse)
}

func (s *stateSuite) TestDetach(c *tc.C) {
	s.st.Attach(s.tag1.Id())
	attached, found := s.st.Attached(s.tag1.Id())
	c.Assert(found, jc.IsTrue)
	c.Assert(attached, jc.IsTrue)
	err := s.st.Detach(s.tag1.Id())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestDetachErr(c *tc.C) {
	err := s.st.Detach(s.tag1.Id())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *stateSuite) TestEmpty(c *tc.C) {
	c.Assert(s.st.Empty(), jc.IsTrue)
}

func (s *stateSuite) TestNotEmpty(c *tc.C) {
	s.st.Attach(s.tag1.Id())
	c.Assert(s.st.Empty(), jc.IsFalse)
}

func (s *stateSuite) TestValidateHookStorageDetaching(c *tc.C) {
	s.st.Attach(s.tag1.Id())
	hi := hook.Info{Kind: hooks.StorageDetaching, StorageId: s.tag1.Id()}
	err := s.st.ValidateHook(hi)
	c.Assert(err, jc.ErrorIsNil)

}

func (s *stateSuite) TestValidateHookStorageDetachingError(c *tc.C) {
	s.st.Attach(s.tag1.Id())
	err := s.st.Detach(s.tag1.Id())
	c.Assert(err, jc.ErrorIsNil)
	hi := hook.Info{Kind: hooks.StorageDetaching, StorageId: s.tag1.Id()}
	err = s.st.ValidateHook(hi)
	c.Assert(err, tc.NotNil)

}

func (s *stateSuite) TestValidateHookStorageAttached(c *tc.C) {
	hi := hook.Info{Kind: hooks.StorageAttached, StorageId: s.tag1.Id()}
	err := s.st.ValidateHook(hi)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestValidateHookStorageAttachedError(c *tc.C) {
	s.st.Attach(s.tag1.Id())
	hi := hook.Info{Kind: hooks.StorageAttached, StorageId: s.tag1.Id()}
	err := s.st.ValidateHook(hi)
	c.Assert(err, tc.NotNil)
}

type stateOpsSuite struct {
	mockStateOpsSuite

	tag1 names.StorageTag
	tag2 names.StorageTag
	tag3 names.StorageTag
}

var _ = tc.Suite(&stateOpsSuite{})

func (s *stateOpsSuite) SetUpSuite(c *tc.C) {
	s.tag1 = names.NewStorageTag("test/1")
	s.tag2 = names.NewStorageTag("test/2")
	s.tag3 = names.NewStorageTag("test/3")
}

func (s *stateOpsSuite) SetUpTest(c *tc.C) {
	s.storSt = storage.NewState()
	s.storSt.Attach(s.tag1.Id())
	s.storSt.Attach(s.tag2.Id())
	c.Assert(s.storSt.Detach(s.tag2.Id()), jc.ErrorIsNil)
	s.storSt.Attach(s.tag3.Id())
}

func (s *stateOpsSuite) TestRead(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectState(c)
	ops := storage.NewStateOps(s.mockStateOps)
	obtainedSt, err := ops.Read(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storage.Storage(obtainedSt), tc.DeepEquals, storage.Storage(s.storSt))
}

func (s *stateOpsSuite) TestReadNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateNotFound()
	ops := storage.NewStateOps(s.mockStateOps)
	obtainedSt, err := ops.Read(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(obtainedSt, tc.NotNil)
}

func (s *stateOpsSuite) TestWrite(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSetState(c, "")
	ops := storage.NewStateOps(s.mockStateOps)
	err := ops.Write(context.Background(), s.storSt)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateOpsSuite) TestWriteEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSetStateEmpty(c)
	ops := storage.NewStateOps(s.mockStateOps)
	err := ops.Write(context.Background(), storage.NewState())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateOpsSuite) TestWriteNilState(c *tc.C) {
	defer s.setupMocks(c).Finish()
	ops := storage.NewStateOps(s.mockStateOps)
	err := ops.Write(context.Background(), nil)
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}
