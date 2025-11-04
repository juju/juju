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
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

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
	"github.com/juju/juju/internal/testhelpers"
)

type storeSuite struct {
	testhelpers.IsolationSuite

	mockAgentBinaryStoreState *MockAgentBinaryStoreState
	mockObjectStoreGetter     *MockModelObjectStoreGetter
	mockObjectStore           *MockObjectStore
}

func TestStoreSuite(t *testing.T) {
	tc.Run(t, &storeSuite{})
}

func (s *storeSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockAgentBinaryStoreState = NewMockAgentBinaryStoreState(ctrl)
	s.mockObjectStore = NewMockObjectStore(ctrl)
	s.mockObjectStoreGetter = NewMockModelObjectStoreGetter(ctrl)
	s.mockObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.mockObjectStore, nil).AnyTimes()
	return ctrl
}

func (s *storeSuite) TestAddAgentBinaryWithSHA384(c *tc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.0-beta1-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockAgentBinaryStoreState.EXPECT().RegisterAgentBinary(gomock.Any(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0-beta1",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(nil)

	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.AddAgentBinaryWithSHA384(c.Context(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.0-beta1"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storeSuite) TestAddAgentBinaryWithSHA384FailedInvalidAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")

	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err := store.AddAgentBinaryWithSHA384(c.Context(), agentBinary,
		coreagentbinary.Version{
			Arch: corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *storeSuite) TestAddAgentBinaryWithSHA384FailedInvalidArch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")

	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err := store.AddAgentBinaryWithSHA384(c.Context(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   "invalid-arch",
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestAddAgentBinaryWithSHA384IdempotentSave tests that the objectstore returns an error when the binary already exists.
// There must be a failure in previous calls. In a following retry, we pick up the existing binary from the
// object store and add it to the state.
func (s *storeSuite) TestAddAgentBinaryWithSHA384IdempotentSave(c *tc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return("", objectstoreerrors.ErrHashAndSizeAlreadyExists)
	s.mockAgentBinaryStoreState.EXPECT().GetObjectUUID(gomock.Any(), "agent-binaries/4.6.8-amd64-test-sha384").Return(objectStoreUUID, nil)
	s.mockAgentBinaryStoreState.EXPECT().RegisterAgentBinary(gomock.Any(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(nil)

	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.AddAgentBinaryWithSHA384(c.Context(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, tc.ErrorIsNil)
}

// TestAddAgentBinaryWithSHA384FailedNotSupportedArchWithBinaryCleanUp tests that the state returns an error when the architecture is not supported.
// This should not happen because the validation is done before calling the state.
// But just in case, we should still test it.
func (s *storeSuite) TestAddAgentBinaryWithSHA384FailedNotSupportedArchWithBinaryCleanUp(c *tc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockAgentBinaryStoreState.EXPECT().RegisterAgentBinary(gomock.Any(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(coreerrors.NotSupported)
	s.mockObjectStore.EXPECT().Remove(gomock.Any(), "agent-binaries/4.6.8-amd64-test-sha384").Return(nil)

	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.AddAgentBinaryWithSHA384(c.Context(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}

// TestAddAgentBinaryWithSHA384FailedObjectStoreUUIDNotFoundWithBinaryCleanUp tests that the state returns an error when the object store UUID is not found.
// This should not happen because the object store UUID is returned by the object store.
// But just in case, we should still test it.
func (s *storeSuite) TestAddAgentBinaryWithSHA384FailedObjectStoreUUIDNotFoundWithBinaryCleanUp(c *tc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockAgentBinaryStoreState.EXPECT().RegisterAgentBinary(gomock.Any(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(agentbinaryerrors.ObjectNotFound)
	s.mockObjectStore.EXPECT().Remove(gomock.Any(), "agent-binaries/4.6.8-amd64-test-sha384").Return(nil)

	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.AddAgentBinaryWithSHA384(c.Context(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, tc.ErrorIs, agentbinaryerrors.ObjectNotFound)
}

// TestAddAgentBinaryWithSHA384FailedAgentBinaryImmutableWithBinaryCleanUp tests that the state returns an error
// when the agent binary is immutable. The agent binary is immutable once it has been
// added. If we got this error, we should cleanup the newly added binary from the object store.
func (s *storeSuite) TestAddAgentBinaryWithSHA384FailedAgentBinaryImmutableWithBinaryCleanUp(c *tc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockAgentBinaryStoreState.EXPECT().RegisterAgentBinary(gomock.Any(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(agentbinaryerrors.AgentBinaryImmutable)
	s.mockObjectStore.EXPECT().Remove(gomock.Any(), "agent-binaries/4.6.8-amd64-test-sha384").Return(nil)

	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.AddAgentBinaryWithSHA384(c.Context(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, tc.ErrorIs, agentbinaryerrors.AgentBinaryImmutable)
}

func (s *storeSuite) TestAddAgentBinaryWithSHA384AlreadyExistsWithNoCleanup(c *tc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-test-sha384",
		agentBinary, int64(1234), "test-sha384",
	).Return(objectStoreUUID, nil)
	s.mockAgentBinaryStoreState.EXPECT().RegisterAgentBinary(gomock.Any(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(agentbinaryerrors.AlreadyExists)

	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.AddAgentBinaryWithSHA384(c.Context(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		1234,
		"test-sha384",
	)
	c.Assert(err, tc.ErrorIs, agentbinaryerrors.AlreadyExists)
}

func (s *storeSuite) calculateSHA(c *tc.C, content string) (string, string) {
	hasher256 := sha256.New()
	hasher384 := sha512.New384()
	_, err := io.Copy(io.MultiWriter(hasher256, hasher384), strings.NewReader(content))
	c.Assert(err, tc.ErrorIsNil)
	sha256Hash := hex.EncodeToString(hasher256.Sum(nil))
	sha384Hash := hex.EncodeToString(hasher384.Sum(nil))
	return sha256Hash, sha384Hash
}

func (s *storeSuite) TestAddAgentBinaryWithSHA256(c *tc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	size := int64(agentBinary.Len())
	sha256Hash, sha384Hash := s.calculateSHA(c, "test-agent-binary")
	objectStoreUUID, err := coreobjectstore.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.mockObjectStore.EXPECT().PutAndCheckHash(gomock.Any(),
		"agent-binaries/4.6.8-amd64-"+sha384Hash,
		gomock.Any(), size, sha384Hash,
	).DoAndReturn(func(_ context.Context, _ string, r io.Reader, _ int64, _ string) (coreobjectstore.UUID, error) {
		bytes, err := io.ReadAll(r)
		c.Check(err, tc.ErrorIsNil)
		c.Check(string(bytes), tc.Equals, "test-agent-binary")
		return objectStoreUUID, nil
	})
	s.mockAgentBinaryStoreState.EXPECT().RegisterAgentBinary(gomock.Any(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.6.8",
		Arch:            corearch.AMD64,
		ObjectStoreUUID: objectStoreUUID,
	}).Return(nil)

	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err = store.AddAgentBinaryWithSHA256(c.Context(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		size,
		sha256Hash,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storeSuite) TestAddAgentBinaryWithSHA256FailedInvalidSHA(c *tc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")

	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err := store.AddAgentBinaryWithSHA256(c.Context(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		17,
		"invalid-sha256",
	)
	c.Check(err, tc.ErrorIs, agentbinaryerrors.HashMismatch)

	s.mockObjectStore.EXPECT().PutAndCheckHash(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "invalid-sha",
	).Return(coreobjectstore.UUID(""), coreobjectstore.ErrHashMismatch)
	err = store.AddAgentBinaryWithSHA384(c.Context(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.6.8"),
			Arch:   corearch.AMD64,
		},
		17,
		"invalid-sha",
	)
	c.Check(err, tc.ErrorIs, agentbinaryerrors.HashMismatch)
}

func (s *storeSuite) TestAddAgentBinaryWithSHA256FailedInvalidAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	agentBinary := strings.NewReader("test-agent-binary")
	sha256Hash, _ := s.calculateSHA(c, "test-agent-binary")

	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	err := store.AddAgentBinaryWithSHA256(c.Context(), agentBinary,
		coreagentbinary.Version{
			Number: semversion.Zero,
			Arch:   corearch.AMD64,
		},
		1234,
		sha256Hash,
	)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *storeSuite) TestGetAgentBinaryUsingSHA256(c *tc.C) {
	defer s.setupMocks(c).Finish()
	sum := "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e"

	reader := io.NopCloser(strings.NewReader(""))
	s.mockAgentBinaryStoreState.EXPECT().CheckAgentBinarySHA256Exists(gomock.Any(), sum).Return(true, nil)
	s.mockObjectStore.EXPECT().GetBySHA256(gomock.Any(), sum).
		Return(reader, 0, nil)

	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	data, _, err := store.GetAgentBinaryUsingSHA256(c.Context(), sum)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, reader)
}

// TestGetAgentBinaryUsingSHA256NotFound asserts that if no agent binaries exist
// for a given sha we get back an error that satisfies
// [agentbinaryerrors.NotFound].
func (s *storeSuite) TestGetAgentBinaryUsingSHA256NotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	sum := "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e"

	// This first step tests the not found error via the state reporting that
	// it doesn't exist.
	s.mockAgentBinaryStoreState.EXPECT().CheckAgentBinarySHA256Exists(gomock.Any(), sum).Return(false, nil)

	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	_, _, err := store.GetAgentBinaryUsingSHA256(c.Context(), sum)
	c.Check(err, tc.ErrorIs, agentbinaryerrors.NotFound)

	// This second step tests the not found error via the object store reporting
	// that the object doesn't exist.
	s.mockAgentBinaryStoreState.EXPECT().CheckAgentBinarySHA256Exists(gomock.Any(), sum).Return(true, nil)
	s.mockObjectStore.EXPECT().GetBySHA256(gomock.Any(), sum).Return(
		nil, 0, intobjectstoreerrors.ObjectNotFound,
	)

	_, _, err = store.GetAgentBinaryUsingSHA256(c.Context(), sum)
	c.Check(err, tc.ErrorIs, agentbinaryerrors.NotFound)
}

// TestGetAgentBinaryUsingSHA256NoObjectStore is here as a protection mechanism.
// Because we are allowing the fetching of agent binaries based on sha it is
// possible that this interface could be used to fetch objects for a given sha
// that isn't related to agent binaries. This could and will pose a security
// risk.
//
// This test asserts that when the database says the sha doesn't exist the
// objectstore is never called.
func (s *storeSuite) TestGetAgentBinaryUsingSHA256NoObjectStore(c *tc.C) {
	defer s.setupMocks(c).Finish()
	sum := "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e"

	s.mockAgentBinaryStoreState.EXPECT().CheckAgentBinarySHA256Exists(gomock.Any(), sum).Return(false, nil)
	s.mockObjectStore.EXPECT().GetBySHA256(gomock.Any(), sum).DoAndReturn(
		func(_ context.Context, _ string) (io.ReadCloser, int64, error) {
			c.Fatal("should never have got this far")
			return nil, 0, nil
		},
	).AnyTimes()

	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	_, _, err := store.GetAgentBinaryUsingSHA256(c.Context(), sum)
	c.Check(err, tc.ErrorIs, agentbinaryerrors.NotFound)
}

func (s *storeSuite) TestGetAgentBinaryWithSHA256(c *tc.C) {
	defer s.setupMocks(c).Finish()
	sum := "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e"

	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.6.8"),
		Arch:   corearch.AMD64,
	}

	s.mockAgentBinaryStoreState.EXPECT().GetAgentBinarySHA256(gomock.Any(), ver, agentbinary.AgentStreamTesting).Return(true, sum, nil)
	agentBinary := strings.NewReader("test-agent-binary")
	data := io.NopCloser(agentBinary)
	s.mockObjectStore.EXPECT().GetBySHA256(gomock.Any(), sum).Return(
		data, agentBinary.Size(), nil,
	)

	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)

	reader, size, sha256Str, err := store.GetAgentBinaryWithSHA256(c.Context(), ver, agentbinary.AgentStreamTesting)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reader, tc.Equals, data)
	c.Assert(size, tc.Equals, agentBinary.Size())
	c.Assert(sha256Str, tc.Equals, sum)
}

func (s *storeSuite) TestGetAgentBinaryWithSHA256NotFoundInState(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.6.8"),
		Arch:   corearch.AMD64,
	}

	s.mockAgentBinaryStoreState.EXPECT().GetAgentBinarySHA256(gomock.Any(), ver, agentbinary.AgentStreamTesting).Return(false, "", nil)
	store := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)

	reader, size, sha256Str, err := store.GetAgentBinaryWithSHA256(c.Context(), ver, agentbinary.AgentStreamTesting)
	c.Assert(err, tc.ErrorMatches, `no agent binary found for version \"4.6.8-amd64\"`)
	c.Assert(reader, tc.Equals, nil)
	c.Assert(size, tc.Equals, int64(0))
	c.Assert(sha256Str, tc.Equals, "")
}

func (s *storeSuite) TestGetAgentBinaryWithSHA256NotFoundInStore(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.6.8"),
		Arch:   corearch.AMD64,
	}

	s.mockAgentBinaryStoreState.EXPECT().GetAgentBinarySHA256(gomock.Any(), ver, agentbinary.AgentStreamTesting).Return(true, "sha256", nil)
	s.mockObjectStore.EXPECT().GetBySHA256(gomock.Any(), "sha256").Return(nil, 0, intobjectstoreerrors.ObjectNotFound)

	binaryStore := NewAgentObjectStore(s.mockAgentBinaryStoreState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)
	reader, size, sha256Str, err := binaryStore.GetAgentBinaryWithSHA256(c.Context(), ver, agentbinary.AgentStreamTesting)

	c.Assert(err, tc.ErrorIs, agentbinaryerrors.NotFound)
	c.Assert(reader, tc.IsNil)
	c.Assert(size, tc.Equals, int64(0))
	c.Assert(sha256Str, tc.Equals, "")
}
