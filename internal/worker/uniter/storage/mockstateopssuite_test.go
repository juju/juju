// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/worker/uniter/operation/mocks"
	"github.com/juju/juju/internal/worker/uniter/storage"
	"github.com/juju/juju/rpc/params"
)

type mockStateOpsSuite struct {
	storSt *storage.State

	mockStateOps *mocks.MockUnitStateReadWriter
}

func (s *mockStateOpsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)
	s.mockStateOps = mocks.NewMockUnitStateReadWriter(ctlr)
	return ctlr
}

func (s *mockStateOpsSuite) expectSetState(c *tc.C, errStr string) {
	data, err := yaml.Marshal(storage.Storage(s.storSt))
	c.Assert(err, tc.ErrorIsNil)
	strStorageState := string(data)
	if errStr != "" {
		err = errors.New(`validation of uniter state: invalid operation state: ` + errStr)
	}

	mExp := s.mockStateOps.EXPECT()
	mExp.SetState(gomock.Any(), unitStateMatcher{c: c, expected: strStorageState}).Return(err)
}

func (s *mockStateOpsSuite) expectSetStateEmpty(c *tc.C) {
	var strStorageState string
	mExp := s.mockStateOps.EXPECT()
	mExp.SetState(gomock.Any(), unitStateMatcher{c: c, expected: strStorageState}).Return(nil)
}

func (s *mockStateOpsSuite) expectState(c *tc.C) {
	data, err := yaml.Marshal(storage.Storage(s.storSt))
	c.Assert(err, tc.ErrorIsNil)
	strStorageState := string(data)

	mExp := s.mockStateOps.EXPECT()
	mExp.State(gomock.Any()).Return(params.UnitStateResult{StorageState: strStorageState}, nil)
}

func (s *mockStateOpsSuite) expectStateNotFound() {
	mExp := s.mockStateOps.EXPECT()
	mExp.State(gomock.Any()).Return(params.UnitStateResult{StorageState: ""}, nil)
}

type unitStateMatcher struct {
	c        *tc.C
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
