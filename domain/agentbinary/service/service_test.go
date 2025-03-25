// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	coreerrors "github.com/juju/juju/core/errors"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain/agentbinary"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type serviceSuite struct {
	testing.IsolationSuite

	mockState             *MockState
	mockObjectStoreGetter *MockModelObjectStoreGetter
	mockObjectStore       *MockObjectStore
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockState = NewMockState(ctrl)
	s.mockObjectStore = NewMockObjectStore(ctrl)
	s.mockObjectStoreGetter = NewMockModelObjectStoreGetter(ctrl)
	s.mockObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.mockObjectStore, nil).AnyTimes()
	return ctrl
}

func (s *serviceSuite) TestAdd(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"tools/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockState.EXPECT().Add(gomock.Any(), agentbinary.Metadata{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(nil)

	svc := NewService(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = svc.Add(context.Background(), agentBinary, Metadata{
		Version: coreagentbinary.Version{
			Number: version.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		Size:   1234,
		SHA384: "test-sha384",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestAddFailedInvalidAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")

	svc := NewService(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err := svc.Add(context.Background(), agentBinary, Metadata{
		Version: coreagentbinary.Version{
			Arch: corearch.AMD64,
		},
		Size:   1234,
		SHA384: "test-sha384",
	})
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestAddFailedInvalidArch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")

	svc := NewService(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err := svc.Add(context.Background(), agentBinary, Metadata{
		Version: coreagentbinary.Version{
			Number: version.MustParse("4.6.8"),
			Arch:   "invalid-arch",
		},
		Size:   1234,
		SHA384: "test-sha384",
	})
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestAddFailedNotSupportedArch tests that the state returns an error when the architecture is not supported.
// This should not happen because the validation is done before calling the state.
// But just in case, we should still test it.
func (s *serviceSuite) TestAddFailedNotSupportedArchWithBinaryCleanUp(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"tools/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockState.EXPECT().Add(gomock.Any(), agentbinary.Metadata{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(coreerrors.NotSupported)
	s.mockObjectStore.EXPECT().Remove(gomock.Any(), "tools/4.6.8-amd64-test-sha384").Return(nil)

	svc := NewService(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = svc.Add(context.Background(), agentBinary, Metadata{
		Version: coreagentbinary.Version{
			Number: version.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		Size:   1234,
		SHA384: "test-sha384",
	})
	c.Assert(err, jc.ErrorIs, coreerrors.NotSupported)
}

// TestAddFailedObjectStoreUUIDNotFound tests that the state returns an error when the object store UUID is not found.
// This should not happen because the object store UUID is returned by the object store.
// But just in case, we should still test it.
func (s *serviceSuite) TestAddFailedObjectStoreUUIDNotFoundWithBinaryCleanUp(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"tools/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockState.EXPECT().Add(gomock.Any(), agentbinary.Metadata{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(coreerrors.NotFound)
	s.mockObjectStore.EXPECT().Remove(gomock.Any(), "tools/4.6.8-amd64-test-sha384").Return(nil)

	svc := NewService(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = svc.Add(context.Background(), agentBinary, Metadata{
		Version: coreagentbinary.Version{
			Number: version.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		Size:   1234,
		SHA384: "test-sha384",
	})
	c.Assert(err, jc.ErrorIs, coreerrors.NotFound)
}
