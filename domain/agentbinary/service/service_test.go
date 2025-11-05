// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	domainagentbinary "github.com/juju/juju/domain/agentbinary"
	domainagentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	baseStore *MockAgentBinaryStore
	state     *MockModelState
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.baseStore = NewMockAgentBinaryStore(ctrl)
	s.state = NewMockModelState(ctrl)
	c.Cleanup(func() {
		s.baseStore = nil
		s.state = nil
	})
	return ctrl
}

// TestGetAgentBinaryForVersionNotValid tests getting an agent binary for an
// invalid version. It is expected the caller is returned an error satisfying
// [coreerrors.NotValid].
func (s *serviceSuite) TestGetAgentBinaryForVersionNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	invalidVersion := domainagentbinary.Version{
		Architecture: domainagentbinary.AMD64,
		Number:       semversion.Zero,
	}
	svc := NewService(s.state, loggertesting.WrapCheckLog(c), s.baseStore)
	_, _, err := svc.GetAgentBinaryForVersion(c.Context(), invalidVersion)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetAgentBinaryForVersionNotFound tests getting an agent binary for a
// version that does not exist. It is expected the caller is returned an error
// satisfying [domainagentbinaryerrors.NotFound].
func (s *serviceSuite) TestGetAgentBinaryForVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	version := domainagentbinary.Version{
		Architecture: domainagentbinary.AMD64,
		Number:       tc.Must1(c, semversion.Parse, "4.0.0"),
	}

	s.state.EXPECT().GetAgentStream(gomock.Any()).Return(
		domainagentbinary.AgentStreamReleased, nil,
	)
	storeExp := s.baseStore.EXPECT()
	storeExp.GetAgentBinaryForVersionStreamSHA256(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return(nil, 0, "", domainagentbinaryerrors.NotFound)

	svc := NewService(s.state, loggertesting.WrapCheckLog(c), s.baseStore)
	_, _, err := svc.GetAgentBinaryForVersion(c.Context(), version)
	c.Check(err, tc.ErrorIs, domainagentbinaryerrors.NotFound)
}

// TestGetAgentBinaryForVersion tests getting an agent binary for a valid
// version in the store.
func (s *serviceSuite) TestGetAgentBinaryForVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	version := domainagentbinary.Version{
		Architecture: domainagentbinary.AMD64,
		Number:       tc.Must1(c, semversion.Parse, "4.0.0"),
	}
	stream := io.NopCloser(strings.NewReader("some data"))

	s.state.EXPECT().GetAgentStream(gomock.Any()).Return(
		domainagentbinary.AgentStreamReleased, nil,
	)
	storeExp := s.baseStore.EXPECT()
	storeExp.GetAgentBinaryForVersionStreamSHA256(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return(stream, 10, "sha256", nil)

	svc := NewService(s.state, loggertesting.WrapCheckLog(c), s.baseStore)
	gotStream, gotSize, err := svc.GetAgentBinaryForVersion(
		c.Context(), version,
	)
	c.Check(err, tc.ErrorIsNil)

	data, err := io.ReadAll(gotStream)
	c.Check(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, "some data")
	c.Check(gotSize, tc.Equals, int64(10))
}

// TestGetAvailableAgentBinariesForVersionNotValid tests getting available
// agent binaries for an invalid version. It is expected the caller is returned
// an error satisfying [coreerrors.NotValid].
func (s *serviceSuite) TestGetAvailableAgentBinariesForVersionNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state, loggertesting.WrapCheckLog(c), s.baseStore)
	_, err := svc.GetAvailableAgentBinaryiesForVersion(c.Context(), semversion.Zero)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetAvailableAgentBinariesForVersionSingleSource tests getting available
// agent binaries for a version from a single store sources.
func (s *serviceSuite) TestGetAvailableAgentBinariesForVersionSingleSource(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	version := tc.Must1(c, semversion.Parse, "4.0.0")

	s.state.EXPECT().GetAgentStream(gomock.Any()).Return(
		domainagentbinary.AgentStreamReleased, nil,
	)

	primaryStore := NewMockAgentBinaryStore(ctrl)
	exp := primaryStore.EXPECT()
	exp.GetAvailableForVersionInStream(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.AMD64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.ARM64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.S390X,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.RISCV64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.PPC64EL,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	svc := NewService(s.state, loggertesting.WrapCheckLog(c), primaryStore, nil)
	found, err := svc.GetAvailableAgentBinaryiesForVersion(c.Context(), version)
	c.Check(err, tc.ErrorIsNil)
	c.Check(found, tc.SameContents, []domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.AMD64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.ARM64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.S390X,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.RISCV64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.PPC64EL,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	})
}

// TestGetAvailableAgentBinariesForVersionMultipleSources tests getting
// available agent binaries for a version from multiple store sources.
func (s *serviceSuite) TestGetAvailableAgentBinariesForVersionMultipleSources(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	version := tc.Must1(c, semversion.Parse, "4.0.0")

	s.state.EXPECT().GetAgentStream(gomock.Any()).Return(
		domainagentbinary.AgentStreamReleased, nil,
	)

	primaryStore := NewMockAgentBinaryStore(ctrl)
	exp := primaryStore.EXPECT()
	exp.GetAvailableForVersionInStream(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.AMD64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.ARM64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	extStore1 := NewMockAgentBinaryStore(ctrl)
	exp = extStore1.EXPECT()
	exp.GetAvailableForVersionInStream(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.S390X,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	extStore2 := NewMockAgentBinaryStore(ctrl)
	exp = extStore2.EXPECT()
	exp.GetAvailableForVersionInStream(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.RISCV64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.PPC64EL,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	svc := NewService(
		s.state,
		loggertesting.WrapCheckLog(c),
		primaryStore,
		extStore1,
		extStore2,
	)
	found, err := svc.GetAvailableAgentBinaryiesForVersion(c.Context(), version)
	c.Check(err, tc.ErrorIsNil)
	c.Check(found, tc.SameContents, []domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.AMD64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.ARM64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.S390X,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.RISCV64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.PPC64EL,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	})
}

// TestGetAvailableAgentBinariesForVersionNotAllAvailable tests getting
// available agent binaries for a version from multiple store sources where not
// every architecture is available.
func (s *serviceSuite) TestGetAvailableAgentBinariesForVersionNotAllAvailable(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	version := tc.Must1(c, semversion.Parse, "4.0.0")

	s.state.EXPECT().GetAgentStream(gomock.Any()).Return(
		domainagentbinary.AgentStreamReleased, nil,
	)

	primaryStore := NewMockAgentBinaryStore(ctrl)
	exp := primaryStore.EXPECT()
	exp.GetAvailableForVersionInStream(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.AMD64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	extStore1 := NewMockAgentBinaryStore(ctrl)
	exp = extStore1.EXPECT()
	exp.GetAvailableForVersionInStream(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return(nil, nil)

	extStore2 := NewMockAgentBinaryStore(ctrl)
	exp = extStore2.EXPECT()
	exp.GetAvailableForVersionInStream(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.RISCV64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	svc := NewService(
		s.state,
		loggertesting.WrapCheckLog(c),
		primaryStore,
		extStore1,
		extStore2,
	)
	found, err := svc.GetAvailableAgentBinaryiesForVersion(c.Context(), version)
	c.Check(err, tc.ErrorIsNil)
	c.Check(found, tc.SameContents, []domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.AMD64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.RISCV64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	})
}

// TestGetAvailableAgentBinariesForVersionNone tests that when no stores have
// any agent binaries available for a version an empty result is returned.
func (s *serviceSuite) TestGetAvailableAgentBinariesForVersionNone(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	version := tc.Must1(c, semversion.Parse, "4.0.0")

	s.state.EXPECT().GetAgentStream(gomock.Any()).Return(
		domainagentbinary.AgentStreamReleased, nil,
	)

	primaryStore := NewMockAgentBinaryStore(ctrl)
	exp := primaryStore.EXPECT()
	exp.GetAvailableForVersionInStream(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return(nil, nil)

	extStore1 := NewMockAgentBinaryStore(ctrl)
	exp = extStore1.EXPECT()
	exp.GetAvailableForVersionInStream(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return(nil, nil)

	extStore2 := NewMockAgentBinaryStore(ctrl)
	exp = extStore2.EXPECT()
	exp.GetAvailableForVersionInStream(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return(nil, nil)

	svc := NewService(
		s.state,
		loggertesting.WrapCheckLog(c),
		primaryStore,
		extStore1,
		extStore2,
	)
	found, err := svc.GetAvailableAgentBinaryiesForVersion(c.Context(), version)
	c.Check(err, tc.ErrorIsNil)
	c.Check(found, tc.HasLen, 0)
}

// TestGetAndCacheExternalAgentBinaryVersionNotValid tests that passing a bad
// agent version returns to the caller a [coreerrors.NotValid] error.
func (s *serviceSuite) TestGetAndCacheExternalAgentBinaryVersionNotValid(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	badVersion := domainagentbinary.Version{
		Architecture: domainagentbinary.AMD64,
		Number:       semversion.Zero,
	}
	externalStore := NewMockAgentBinaryStore(ctrl)

	svc := NewService(s.state, loggertesting.WrapCheckLog(c), s.baseStore, externalStore)
	_, _, err := svc.GetAndCacheExternalAgentBinary(c.Context(), badVersion)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetAndCacheExternalAgentBinary is a happy path test for
// [Service.GetAndCacheExternalAgentBinary].
func (s *serviceSuite) TestGetAndCacheExternalAgentBinary(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	ver := domainagentbinary.Version{
		Architecture: domainagentbinary.AMD64,
		Number:       semversion.MustParse("4.0.1"),
	}

	extPayload := []byte("hello-agent-binary")
	extSize := int64(len(extPayload))
	extHash256Raw := sha256.Sum256(extPayload)
	extHash256 := hex.EncodeToString(extHash256Raw[:])
	extReader := io.NopCloser(bytes.NewReader(extPayload))

	stateExp := s.state.EXPECT()
	stateExp.GetAgentStream(gomock.Any()).Return(
		domainagentbinary.AgentStreamReleased, nil,
	)

	extStore := NewMockAgentBinaryStore(ctrl)
	extStoreExp := extStore.EXPECT()
	extStoreExp.GetAgentBinaryForVersionStreamSHA256(
		gomock.Any(), ver, domainagentbinary.AgentStreamReleased,
	).Return(extReader, extSize, extHash256, nil)

	var capturedStream io.Reader

	baseStoreExp := s.baseStore.EXPECT()
	baseStoreExp.AddAgentBinaryWithSHA256(
		gomock.Any(), gomock.Any(), ver, extSize, extHash256,
	).DoAndReturn(func(
		_ context.Context,
		s io.Reader,
		_ domainagentbinary.Version,
		_ int64,
		_ string,
	) error {
		copyStream := bytes.Buffer{}
		_, err := io.Copy(&copyStream, s)
		c.Assert(err, tc.ErrorIsNil)
		capturedStream = &copyStream
		return nil
	})
	baseStoreExp.GetAgentBinaryForSHA256(gomock.Any(), extHash256).DoAndReturn(
		func(context.Context, string) (io.ReadCloser, int64, error) {
			return io.NopCloser(capturedStream), extSize, nil
		},
	)

	svc := NewService(s.state, loggertesting.WrapCheckLog(c), s.baseStore, extStore)
	stream, size, err := svc.GetAndCacheExternalAgentBinary(c.Context(), ver)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(capturedStream, tc.NotNil)
	c.Check(size, tc.Equals, extSize)

	data, err := io.ReadAll(stream)
	c.Check(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, string("hello-agent-binary"))
}
