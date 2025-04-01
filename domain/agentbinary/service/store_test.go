// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	coreerrors "github.com/juju/juju/core/errors"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type storeSuite struct {
	testing.IsolationSuite

	mockState             *MockState
	mockObjectStoreGetter *MockModelObjectStoreGetter
	mockObjectStore       *MockObjectStore
}

var _ = gc.Suite(&storeSuite{})

func (s *storeSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockState = NewMockState(ctrl)
	s.mockObjectStore = NewMockObjectStore(ctrl)
	s.mockObjectStoreGetter = NewMockModelObjectStoreGetter(ctrl)
	s.mockObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.mockObjectStore, nil).AnyTimes()
	return ctrl
}

func (s *storeSuite) TestAdd(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.0-beta1-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockState.EXPECT().Add(gomock.Any(), agentbinary.Metadata{
		Version:         "4.0-beta1",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(nil)

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.Add(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.0-beta1"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storeSuite) TestAddFailedInvalidAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err := store.Add(context.Background(), agentBinary,
		coreagentbinary.Version{
			Arch: corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *storeSuite) TestAddFailedInvalidArch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err := store.Add(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   "invalid-arch",
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestAddFailedNotSupportedArch tests that the state returns an error when the architecture is not supported.
// This should not happen because the validation is done before calling the state.
// But just in case, we should still test it.
func (s *storeSuite) TestAddFailedNotSupportedArchWithBinaryCleanUp(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockState.EXPECT().Add(gomock.Any(), agentbinary.Metadata{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(coreerrors.NotSupported)
	s.mockObjectStore.EXPECT().Remove(gomock.Any(), "agent-binaries/4.6.8-amd64-test-sha384").Return(nil)

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.Add(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, jc.ErrorIs, coreerrors.NotSupported)
}

// TestAddFailedObjectStoreUUIDNotFound tests that the state returns an error when the object store UUID is not found.
// This should not happen because the object store UUID is returned by the object store.
// But just in case, we should still test it.
func (s *storeSuite) TestAddFailedObjectStoreUUIDNotFoundWithBinaryCleanUp(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockState.EXPECT().Add(gomock.Any(), agentbinary.Metadata{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(agentbinaryerrors.ObjectNotFound)
	s.mockObjectStore.EXPECT().Remove(gomock.Any(), "agent-binaries/4.6.8-amd64-test-sha384").Return(nil)

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.Add(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, jc.ErrorIs, agentbinaryerrors.ObjectNotFound)
}

// TestAddAlreadyExistsWithCleanup is testing that if we try and add an agent
// binary that already exists, we should get back an error satisfying
// [agentbinaryerrors.AlreadyExists] and the binary should be removed from the
// object store.
func (s *storeSuite) TestAddAlreadyExistsWithCleanup(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockState.EXPECT().Add(gomock.Any(), agentbinary.Metadata{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(agentbinaryerrors.AlreadyExists)
	s.mockObjectStore.EXPECT().Remove(gomock.Any(), "agent-binaries/4.6.8-amd64-test-sha384").Return(nil)

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.Add(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, jc.ErrorIs, agentbinaryerrors.AlreadyExists)
}

// TODO: the AddWithSHA256 is currently not implemented yet.
// More tests should be added when the implementation is done in JUJU-7734.
func (s *storeSuite) TestAddWithSHA256(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-test-sha256",
		agentBinary, int64(1234), "test-sha256",
	).Return(objectStoreUUID, nil)
	s.mockState.EXPECT().Add(gomock.Any(), agentbinary.Metadata{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(nil)

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.AddWithSHA256(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha256",
	)
	c.Assert(err, jc.ErrorIsNil)
}
