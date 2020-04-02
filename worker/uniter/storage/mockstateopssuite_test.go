// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/testing/checkers"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/check.v1"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/operation/mocks"
	"github.com/juju/juju/worker/uniter/storage"
)

type mockStateOpsSuite struct {
	storSt *storage.State

	mockStateOps *mocks.MockUnitStateReadWriter
}

func (s *mockStateOpsSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)
	s.mockStateOps = mocks.NewMockUnitStateReadWriter(ctlr)
	return ctlr
}

func (s *mockStateOpsSuite) expectSetState(c *gc.C, errStr string) {
	data, err := yaml.Marshal(storage.Storage(s.storSt))
	c.Assert(err, jc.ErrorIsNil)
	strStorageState := string(data)
	if errStr != "" {
		err = errors.New(`validation of uniter state: invalid operation state: ` + errStr)
	}

	mExp := s.mockStateOps.EXPECT()
	mExp.SetState(unitStateMatcher{c: c, expected: strStorageState}).Return(err)
}

func (s *mockStateOpsSuite) expectSetStateEmpty(c *gc.C) {
	var strStorageState string
	mExp := s.mockStateOps.EXPECT()
	mExp.SetState(unitStateMatcher{c: c, expected: strStorageState}).Return(nil)
}

func (s *mockStateOpsSuite) expectState(c *check.C) {
	data, err := yaml.Marshal(storage.Storage(s.storSt))
	c.Assert(err, checkers.ErrorIsNil)
	strStorageState := string(data)

	mExp := s.mockStateOps.EXPECT()
	mExp.State().Return(params.UnitStateResult{StorageState: strStorageState}, nil)
}

func (s *mockStateOpsSuite) expectStateNotFound() {
	mExp := s.mockStateOps.EXPECT()
	mExp.State().Return(params.UnitStateResult{StorageState: ""}, nil)
}

type unitStateMatcher struct {
	c        *gc.C
	expected string
}

func (m unitStateMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(params.SetUnitStateArg)
	if !ok {
		return false
	}

	if obtained.StorageState == nil || m.expected != *obtained.StorageState {
		m.c.Fatalf("unitStateMatcher: expected (%s) obtained (%s)", m.expected, *obtained.StorageState)
		return false
	}

	return true
}

func (m unitStateMatcher) String() string {
	return "Match the contents of the StorageState pointer in params.SetUnitStateArg"
}
