// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	io "io"
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
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	intobjectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
)

type storeSuite struct {
	testing.IsolationSuite

	mockState             *MockState
	mockObjectStoreGetter *MockControllerObjectStoreGetter
	mockObjectStore       *MockObjectStore
}

var _ = gc.Suite(&storeSuite{})

func (s *storeSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockState = NewMockState(ctrl)
	s.mockObjectStore = NewMockObjectStore(ctrl)
	s.mockObjectStoreGetter = NewMockControllerObjectStoreGetter(ctrl)
	s.mockObjectStoreGetter.EXPECT().GetControllerObjectStore(gomock.Any()).Return(s.mockObjectStore, nil).AnyTimes()
	return ctrl
}

func (s *storeSuite) TestAddAgentBinary(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.0-beta1-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockState.EXPECT().RegisterAgentBinary(gomock.Any(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0-beta1",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(nil)

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.AddAgentBinary(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.0-beta1"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storeSuite) TestAddAgentBinaryFailedInvalidAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err := store.AddAgentBinary(context.Background(), agentBinary,
		coreagentbinary.Version{
			Arch: corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *storeSuite) TestAddAgentBinaryFailedInvalidArch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err := store.AddAgentBinary(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   "invalid-arch",
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestAddAgentBinaryIdempotentSave tests that the objectstore returns an error when the binary already exists.
// There must be a failure in previous calls. In a following retry, we pick up the existing binary from the
// object store and add it to the state.
func (s *storeSuite) TestAddAgentBinaryIdempotentSave(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return("", objectstoreerrors.ErrHashAndSizeAlreadyExists)
	s.mockState.EXPECT().GetObjectUUID(gomock.Any(), "agent-binaries/4.6.8-amd64-test-sha384").Return(objectStoreUUID, nil)
	s.mockState.EXPECT().RegisterAgentBinary(gomock.Any(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(nil)

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.AddAgentBinary(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, jc.ErrorIsNil)
}

// TestAddAgentBinaryFailedNotSupportedArchWithBinaryCleanUp tests that the state returns an error when the architecture is not supported.
// This should not happen because the validation is done before calling the state.
// But just in case, we should still test it.
func (s *storeSuite) TestAddAgentBinaryFailedNotSupportedArchWithBinaryCleanUp(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockState.EXPECT().RegisterAgentBinary(gomock.Any(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(coreerrors.NotSupported)
	s.mockObjectStore.EXPECT().Remove(gomock.Any(), "agent-binaries/4.6.8-amd64-test-sha384").Return(nil)

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.AddAgentBinary(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, jc.ErrorIs, coreerrors.NotSupported)
}

// TestAddAgentBinaryFailedObjectStoreUUIDNotFoundWithBinaryCleanUp tests that the state returns an error when the object store UUID is not found.
// This should not happen because the object store UUID is returned by the object store.
// But just in case, we should still test it.
func (s *storeSuite) TestAddAgentBinaryFailedObjectStoreUUIDNotFoundWithBinaryCleanUp(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockState.EXPECT().RegisterAgentBinary(gomock.Any(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(agentbinaryerrors.ObjectNotFound)
	s.mockObjectStore.EXPECT().Remove(gomock.Any(), "agent-binaries/4.6.8-amd64-test-sha384").Return(nil)

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.AddAgentBinary(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, jc.ErrorIs, agentbinaryerrors.ObjectNotFound)
}

// TestAddAgentBinaryFailedAgentBinaryImmutableWithBinaryCleanUp tests that the state returns an error
// when the agent binary is immutable. The agent binary is immutable once it has been
// added. If we got this error, we should cleanup the newly added binary from the object store.
func (s *storeSuite) TestAddAgentBinaryFailedAgentBinaryImmutableWithBinaryCleanUp(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockState.EXPECT().RegisterAgentBinary(gomock.Any(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(agentbinaryerrors.AgentBinaryImmutable)
	s.mockObjectStore.EXPECT().Remove(gomock.Any(), "agent-binaries/4.6.8-amd64-test-sha384").Return(nil)

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.AddAgentBinary(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, jc.ErrorIs, agentbinaryerrors.AgentBinaryImmutable)
}

// TestAddAgentBinaryAlreadyExistsWithNoCleanup is testing that if we try and add an agent
// binary that already exists, we should get back an error satisfying
// [agentbinaryerrors.AlreadyExists] but the existing binary should be removed from the
// object store.
func (s *storeSuite) TestAddAgentBinaryAlreadyExistsWithNoCleanup(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockState.EXPECT().RegisterAgentBinary(gomock.Any(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(agentbinaryerrors.AlreadyExists)

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.AddAgentBinary(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, jc.ErrorIs, agentbinaryerrors.AlreadyExists)
}

func (s *storeSuite) calculateSHA(c *gc.C, content string) (string, string) {
	hasher256 := sha256.New()
	hasher384 := sha512.New384()
	_, err := io.Copy(io.MultiWriter(hasher256, hasher384), strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	sha256Hash := hex.EncodeToString(hasher256.Sum(nil))
	sha384Hash := hex.EncodeToString(hasher384.Sum(nil))
	return sha256Hash, sha384Hash
}

func (s *storeSuite) TestAddAgentBinaryWithSHA256(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	size := int64(agentBinary.Len())
	sha256Hash, sha384Hash := s.calculateSHA(c, "test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-"+sha384Hash,
		gomock.Any(), size, sha384Hash,
	).DoAndReturn(func(_ context.Context, _ string, r io.Reader, _ int64, _ string) (coreobjectstore.UUID, error) {
		bytes, err := io.ReadAll(r)
		c.Check(err, jc.ErrorIsNil)
		c.Check(string(bytes), gc.Equals, "test-agent-binary")
		return objectStoreUUID, nil
	})
	s.mockState.EXPECT().RegisterAgentBinary(gomock.Any(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(nil)

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.AddAgentBinaryWithSHA256(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		size,
		sha256Hash,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storeSuite) TestAddAgentBinaryWithSHA256FailedInvalidSHA(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err := store.AddAgentBinaryWithSHA256(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		1234,
		"invalid-sha256",
	)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *storeSuite) TestAddAgentBinaryWithSHA256FailedInvalidAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	sha256Hash, _ := s.calculateSHA(c, "test-agent-binary")

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err := store.AddAgentBinaryWithSHA256(context.Background(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.Zero,
			Arch:   corearch.AMD64,
		},
		1234,
		sha256Hash,
	)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestGetAgentBinaryForSHA256NoObjectStore is here as a protection mechanism.
// Because we are allowing the fetching of agent binaries based on sha it is
// possible that this interface could be used to fetch objects for a given sha
// that isn't related to agent binaries. This could and will pose a security
// risk.
//
// This test asserts that when the database says the sha doesn't exist the
// objectstore is never called.
func (s *storeSuite) TestGetAgentBinaryForSHA256NoObjectStore(c *gc.C) {
	defer s.setupMocks(c).Finish()
	sum := "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e"

	s.mockState.EXPECT().CheckAgentBinarySHA256Exists(gomock.Any(), sum).Return(false, nil)
	s.mockObjectStore.EXPECT().GetBySHA256(gomock.Any(), sum).DoAndReturn(
		func(_ context.Context, _ string) (io.ReadCloser, int64, error) {
			c.Fatal("should never have got this far")
			return nil, 0, nil
		},
	).AnyTimes()

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	_, _, err := store.GetAgentBinaryForSHA256(context.Background(), sum)
	c.Check(err, jc.ErrorIs, agentbinaryerrors.NotFound)
}

// TestGetAgentBinaryForSHA256NotFound asserts that if no agent binaries exist
// for a given sha we get back an error that satisfies
// [agentbinaryerrors.NotFound].
func (s *storeSuite) TestGetAgentBinaryForSHA256NotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	sum := "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e"

	// This first step tests the not found error via the state reporting that
	// it doesn't exist.
	s.mockState.EXPECT().CheckAgentBinarySHA256Exists(gomock.Any(), sum).Return(false, nil)

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	_, _, err := store.GetAgentBinaryForSHA256(context.Background(), sum)
	c.Check(err, jc.ErrorIs, agentbinaryerrors.NotFound)

	// This second step tests the not found error via the object store reporting
	// that the object doesn't exist.
	s.mockState.EXPECT().CheckAgentBinarySHA256Exists(gomock.Any(), sum).Return(true, nil)
	s.mockObjectStore.EXPECT().GetBySHA256(gomock.Any(), sum).Return(
		nil, 0, intobjectstoreerrors.ObjectNotFound,
	)

	_, _, err = store.GetAgentBinaryForSHA256(context.Background(), sum)
	c.Check(err, jc.ErrorIs, agentbinaryerrors.NotFound)
}

func (s *storeSuite) TestGetAgentBinaryForSHA256(c *gc.C) {
	defer s.setupMocks(c).Finish()
	sum := "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e"

	s.mockState.EXPECT().CheckAgentBinarySHA256Exists(gomock.Any(), sum).Return(true, nil)
	s.mockObjectStore.EXPECT().GetBySHA256(gomock.Any(), sum).Return(
		nil, 0, nil,
	)

	store := NewAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	_, _, err := store.GetAgentBinaryForSHA256(context.Background(), sum)
	c.Check(err, jc.ErrorIsNil)
}
