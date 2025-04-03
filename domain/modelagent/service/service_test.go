// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	jujuversion "github.com/juju/juju/core/version"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/modelagent"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/errors"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/state/binarystorage"
)

type suite struct {
	testing.IsolationSuite
	state   *MockState
	storage *MockStorage
}

var _ = gc.Suite(&suite{})

func (s *suite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.storage = NewMockStorage(ctrl)
	return ctrl
}

// TestGetModelAgentVersionSuccess tests the happy path for
// Service.GetModelAgentVersion.
func (s *suite) TestGetModelAgentVersionSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expectedVersion, err := semversion.Parse("4.21.65")
	c.Assert(err, jc.ErrorIsNil)
	s.state.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(expectedVersion, nil)

	svc := NewProviderService(s.state, nil, nil)
	ver, err := svc.GetModelTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(ver, jc.DeepEquals, expectedVersion)
}

// TestGetModelAgentVersionNotFound tests that Service.GetModelAgentVersion
// returns an appropriate error when the agent version cannot be found.
func (s *suite) TestGetModelAgentVersionModelNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.Zero, modelagenterrors.AgentVersionNotFound)

	svc := NewProviderService(s.state, nil, nil)
	_, err := svc.GetModelTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetMachineTargetAgentVersion is asserting the happy path for getting
// a machine's target agent version.
func (s *suite) TestGetMachineTargetAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineName := coremachine.Name("0")
	uuid := uuid.MustNewUUID().String()
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.0"),
		Arch:   "amd64",
	}

	s.state.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(uuid, nil)
	s.state.EXPECT().GetMachineTargetAgentVersion(gomock.Any(), uuid).Return(ver, nil)

	rval, err := NewProviderService(s.state, nil, nil).GetMachineTargetAgentVersion(context.Background(), machineName)
	c.Check(err, jc.ErrorIsNil)
	c.Check(rval, gc.Equals, ver)
}

// TestGetMachineTargetAgentVersionNotFound is testing that the service
// returns a [machineerrors.MachineNotFound] error when no machine exists for
// a given name.
func (s *suite) TestGetMachineTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return(
		"", machineerrors.MachineNotFound,
	)

	_, err := NewProviderService(s.state, nil, nil).GetMachineTargetAgentVersion(
		context.Background(),
		coremachine.Name("0"),
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetUnitTargetAgentVersion is asserting the happy path for getting
// a unit's target agent version.
func (s *suite) TestGetUnitTargetAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.0"),
		Arch:   "amd64",
	}

	uuid := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/0")).Return(uuid, nil)
	s.state.EXPECT().GetUnitTargetAgentVersion(gomock.Any(), uuid).Return(ver, nil)

	rval, err := NewProviderService(s.state, nil, nil).GetUnitTargetAgentVersion(context.Background(), "foo/0")
	c.Check(err, jc.ErrorIsNil)
	c.Check(rval, gc.Equals, ver)
}

// TestGetUnitTargetAgentVersionNotFound is testing that the service
// returns a [applicationerrors.UnitNotFound] error when no unit exists for
// a given name.
func (s *suite) TestGetUnitTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/0")).Return(
		"", applicationerrors.UnitNotFound,
	)

	_, err := NewProviderService(s.state, nil, nil).GetUnitTargetAgentVersion(
		context.Background(),
		"foo/0",
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestWatchUnitTargetAgentVersionNotFound is testing that the service
// returns a [applicationerrors.UnitNotFound] error when no unit exists for
// a given name.
func (s *suite) TestWatchUnitTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/0")).Return(
		"", applicationerrors.UnitNotFound,
	)

	_, err := NewProviderService(s.state, nil, nil).WatchUnitTargetAgentVersion(
		context.Background(),
		"foo/0",
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestWatchMachineTargetAgentVersionNotFound is testing that the service
// returns a [machineerrors.MachineNotFound] error when no machine exists for
// a given name.
func (s *suite) TestWatchMachineTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return(
		"", machineerrors.MachineNotFound,
	)

	_, err := NewProviderService(s.state, nil, nil).WatchMachineTargetAgentVersion(context.Background(), "0")
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestSetMachineReportedAgentVersionInvalid is here to assert that if pass a
// junk agent binary version to [Service.SetMachineReportedAgentVersion] we get
// back an error that satisfies [coreerrors.NotValid].
func (s *suite) TestSetMachineReportedAgentVersionInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewProviderService(s.state, nil, nil).SetMachineReportedAgentVersion(
		context.Background(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.Zero,
		},
	)
	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestSetMachineReportedAgentVersionSuccess asserts that if we try to set the
// reported agent version for a machine that doesn't exist we get an error
// satisfying [machineerrors.MachineNotFound]. Because the service relied on
// state for producing this error we need to simulate this in two different
// locations to assert the full functionality.
func (s *suite) TestSetMachineReportedAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// MachineNotFound error location 1.
	s.state.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return(
		"", machineerrors.MachineNotFound,
	)

	err := NewProviderService(s.state, nil, nil).SetMachineReportedAgentVersion(
		context.Background(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)

	// MachineNotFound error location 2.
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return(
		machineUUID.String(), nil,
	)

	s.state.EXPECT().SetMachineRunningAgentBinaryVersion(
		gomock.Any(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(machineerrors.MachineNotFound)

	err = NewProviderService(s.state, nil, nil).SetMachineReportedAgentVersion(
		context.Background(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *suite) TestSetMachineReportedAgentVersionDead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return(
		machineUUID.String(), nil,
	)

	s.state.EXPECT().SetMachineRunningAgentBinaryVersion(
		gomock.Any(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(machineerrors.MachineIsDead)

	err = NewProviderService(s.state, nil, nil).SetMachineReportedAgentVersion(
		context.Background(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineIsDead)
}

// TestSetMachineReportedAgentVersion asserts the happy path of
// [Service.SetMachineReportedAgentVersion].
func (s *suite) TestSetMachineReportedAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return(
		machineUUID.String(), nil,
	)
	s.state.EXPECT().SetMachineRunningAgentBinaryVersion(
		gomock.Any(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(nil)

	err = NewProviderService(s.state, nil, nil).SetMachineReportedAgentVersion(
		context.Background(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIsNil)
}

// TestSetReportedUnitAgentVersionInvalid is here to assert that if pass a
// junk agent binary version to [Service.SetReportedUnitAgentVersion] we get
// back an error that satisfies [coreerrors.NotValid].
func (s *suite) TestSetReportedUnitAgentVersionInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewProviderService(s.state, nil, nil).SetUnitReportedAgentVersion(
		context.Background(),
		"foo/666",
		coreagentbinary.Version{
			Number: semversion.Zero,
		},
	)
	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestSetReportedUnitAgentVersionNotFound asserts that if we try to set the
// reported agent version for a unit that doesn't exist we get an error
// satisfying [applicationerrors.UnitNotFound]. Because the service relied on
// state for producing this error we need to simulate this in two different
// locations to assert the full functionality.
func (s *suite) TestSetReportedUnitAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// UnitNotFound error location 1.
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		"", applicationerrors.UnitNotFound,
	)

	err := NewProviderService(s.state, nil, nil).SetUnitReportedAgentVersion(
		context.Background(),
		"foo/666",
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)

	// UnitNotFound error location 2.
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		unitUUID, nil,
	)

	s.state.EXPECT().SetUnitRunningAgentBinaryVersion(
		gomock.Any(),
		unitUUID,
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(applicationerrors.UnitNotFound)

	err = NewProviderService(s.state, nil, nil).SetUnitReportedAgentVersion(
		context.Background(),
		"foo/666",
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestSetReportedUnitAgentVersionDead asserts that if we try to set the
// reported agent version for a dead unit we get an error satisfying
// [applicationerrors.UnitIsDead].
func (s *suite) TestSetReportedUnitAgentVersionDead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		coreunit.UUID(unitUUID.String()), nil,
	)

	s.state.EXPECT().SetUnitRunningAgentBinaryVersion(
		gomock.Any(),
		coreunit.UUID(unitUUID.String()),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(applicationerrors.UnitIsDead)

	err := NewProviderService(s.state, nil, nil).SetUnitReportedAgentVersion(
		context.Background(),
		"foo/666",
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitIsDead)
}

// TestSetReportedUnitAgentVersion asserts the happy path of
// [Service.SetReportedUnitAgentVersion].
func (s *suite) TestSetReportedUnitAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		coreunit.UUID(unitUUID.String()), nil,
	)

	s.state.EXPECT().SetUnitRunningAgentBinaryVersion(
		gomock.Any(),
		coreunit.UUID(unitUUID.String()),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(nil)

	err := NewProviderService(s.state, nil, nil).SetUnitReportedAgentVersion(
		context.Background(),
		"foo/666",
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIsNil)
}

type stubProvider struct {
	environs.BootstrapEnviron
}

func (stubProvider) Config() *config.Config {
	return &config.Config{}
}

func (s *suite) TestFindToolsMatchMajor(c *gc.C) {
	defer s.setupMocks(c).Finish()

	envtoolsList := coretools.List{
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.0-ubuntu-alpha"),
			Size:    2048,
			SHA256:  "badf00d",
		},
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.1-ubuntu-alpha"),
		},
	}
	storageMetadata := []binarystorage.Metadata{{
		Version: "123.456.0-ubuntu-alpha",
		Size:    1024,
		SHA256:  "feedface",
	}, {
		Version: "666.456.0-ubuntu-alpha",
		Size:    1024,
		SHA256:  "feedface666",
	}}
	s.storage.EXPECT().AllMetadata().Return(storageMetadata, nil)

	svc := NewProviderService(s.state, func(ctx context.Context) (ProviderWithAgentFinder, error) {
		return stubProvider{}, nil
	}, nil)

	svc.toolsFinder = func(_ context.Context, _ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, streams []string, filter coretools.Filter) (coretools.List, error) {
		c.Assert(major, gc.Equals, 123)
		c.Assert(minor, gc.Equals, 456)
		c.Assert(streams, gc.DeepEquals, []string{"released"})
		c.Assert(filter.OSType, gc.Equals, "ubuntu")
		c.Assert(filter.Arch, gc.Equals, "alpha")
		return envtoolsList, nil
	}

	result, err := svc.FindAgents(context.Background(), modelagent.FindAgentsParams{
		MajorVersion: 123,
		MinorVersion: 456,
		Arch:         "alpha",
		AgentStorage: s.storage,
		ToolsURLsGetter: func(ctx context.Context, v semversion.Binary) ([]string, error) {
			return []string{"tools:" + v.String()}, nil
		},
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, coretools.List{
		&coretools.Tools{
			Version: semversion.MustParseBinary(storageMetadata[0].Version),
			Size:    storageMetadata[0].Size,
			SHA256:  storageMetadata[0].SHA256,
			URL:     "tools:" + storageMetadata[0].Version,
		},
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.1-ubuntu-alpha"),
			URL:     "tools:123.456.1-ubuntu-alpha",
		},
	})
}

func (s *suite) TestFindToolsRequestAgentStream(c *gc.C) {
	defer s.setupMocks(c).Finish()

	envtoolsList := coretools.List{
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.0-ubuntu-alpha"),
			Size:    2048,
			SHA256:  "badf00d",
		},
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.1-ubuntu-alpha"),
		},
	}

	storageMetadata := []binarystorage.Metadata{{
		Version: "123.456.0-ubuntu-alpha",
		Size:    1024,
		SHA256:  "feedface",
	}}
	s.storage.EXPECT().AllMetadata().Return(storageMetadata, nil)

	svc := NewProviderService(s.state, func(ctx context.Context) (ProviderWithAgentFinder, error) {
		return stubProvider{}, nil
	}, nil)

	svc.toolsFinder = func(_ context.Context, _ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, streams []string, filter coretools.Filter) (coretools.List, error) {
		c.Assert(major, gc.Equals, 123)
		c.Assert(minor, gc.Equals, 456)
		c.Assert(streams, gc.DeepEquals, []string{"pretend"})
		c.Assert(filter.OSType, gc.Equals, "ubuntu")
		c.Assert(filter.Arch, gc.Equals, "alpha")
		return envtoolsList, nil
	}

	result, err := svc.FindAgents(context.Background(), modelagent.FindAgentsParams{
		MajorVersion: 123,
		MinorVersion: 456,
		Arch:         "alpha",
		AgentStream:  "pretend",
		AgentStorage: s.storage,
		ToolsURLsGetter: func(ctx context.Context, v semversion.Binary) ([]string, error) {
			return []string{"tools:" + v.String()}, nil
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, coretools.List{
		&coretools.Tools{
			Version: semversion.MustParseBinary(storageMetadata[0].Version),
			Size:    storageMetadata[0].Size,
			SHA256:  storageMetadata[0].SHA256,
			URL:     "tools:" + storageMetadata[0].Version,
		},
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.1-ubuntu-alpha"),
			URL:     "tools:123.456.1-ubuntu-alpha",
		},
	})
}

func (s *suite) TestFindToolsNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.storage.EXPECT().AllMetadata().Return(nil, nil)

	svc := NewProviderService(s.state, func(ctx context.Context) (ProviderWithAgentFinder, error) {
		return stubProvider{}, nil
	}, nil)
	svc.toolsFinder = func(_ context.Context, _ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, streams []string, filter coretools.Filter) (coretools.List, error) {
		return nil, coreerrors.NotFound
	}

	_, err := svc.FindAgents(context.Background(), modelagent.FindAgentsParams{
		MajorVersion: 123,
		MinorVersion: 456,
		Arch:         "alpha",
		AgentStream:  "pretend",
		AgentStorage: s.storage,
		ToolsURLsGetter: func(ctx context.Context, v semversion.Binary) ([]string, error) {
			return []string{"tools:" + v.String()}, nil
		},
	})
	c.Assert(err, jc.ErrorIs, coreerrors.NotFound)
}

func (s *suite) TestFindToolsExactInStorage(c *gc.C) {
	defer s.setupMocks(c).Finish()

	storageMetadata := []binarystorage.Metadata{
		{Version: "1.22-beta1-ubuntu-amd64"},
		{Version: "1.22.0-ubuntu-amd64"},
	}
	//s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	s.storage.EXPECT().AllMetadata().Return(storageMetadata, nil)
	s.PatchValue(&jujuversion.Current, semversion.MustParseBinary("1.22-beta1-ubuntu-amd64").Number)
	s.testFindToolsExact(c, true, true)

	s.storage.EXPECT().AllMetadata().Return(storageMetadata, nil)
	s.PatchValue(&jujuversion.Current, semversion.MustParseBinary("1.22.0-ubuntu-amd64").Number)
	s.testFindToolsExact(c, true, false)
}

func (s *suite) TestFindToolsExactNotInStorage(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.storage.EXPECT().AllMetadata().Return([]binarystorage.Metadata{}, nil)
	s.PatchValue(&jujuversion.Current, semversion.MustParse("1.22-beta1"))
	s.testFindToolsExact(c, false, true)

	s.storage.EXPECT().AllMetadata().Return([]binarystorage.Metadata{}, nil)
	s.PatchValue(&jujuversion.Current, semversion.MustParse("1.22.0"))
	s.testFindToolsExact(c, false, false)
}

func (s *suite) testFindToolsExact(c *gc.C, inStorage bool, develVersion bool) {

	svc := NewProviderService(s.state, func(ctx context.Context) (ProviderWithAgentFinder, error) {
		return stubProvider{}, nil
	}, nil)
	svc.toolsFinder = func(_ context.Context, _ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, streams []string, filter coretools.Filter) (coretools.List, error) {
		return nil, coreerrors.NotFound
	}

	var called bool
	current := coretesting.CurrentVersion()
	svc.toolsFinder = func(_ context.Context, _ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, streams []string, filter coretools.Filter) (coretools.List, error) {
		called = true
		c.Assert(filter.Number, gc.Equals, jujuversion.Current)
		c.Assert(filter.OSType, gc.Equals, current.Release)
		c.Assert(filter.Arch, gc.Equals, corearch.HostArch())
		if develVersion {
			c.Assert(streams, gc.DeepEquals, []string{"devel", "proposed", "released"})
		} else {
			c.Assert(streams, gc.DeepEquals, []string{"released"})
		}
		return nil, coreerrors.NotFound
	}

	_, err := svc.FindAgents(context.Background(), modelagent.FindAgentsParams{
		Number:       jujuversion.Current,
		Arch:         corearch.HostArch(),
		AgentStorage: s.storage,
		ToolsURLsGetter: func(ctx context.Context, v semversion.Binary) ([]string, error) {
			return []string{"tools:" + v.String()}, nil
		},
	})
	if inStorage {
		c.Assert(err, gc.IsNil)
		c.Assert(called, jc.IsFalse)
	} else {
		c.Assert(err, jc.ErrorIs, coreerrors.NotFound)
		c.Assert(called, jc.IsTrue)
	}
}

func (s *suite) TestFindToolsToolsStorageError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewProviderService(s.state, func(ctx context.Context) (ProviderWithAgentFinder, error) {
		return stubProvider{}, nil
	}, nil)

	var called bool
	svc.toolsFinder = func(_ context.Context, _ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, streams []string, filter coretools.Filter) (coretools.List, error) {
		called = true
		return nil, coreerrors.NotFound
	}

	s.storage.EXPECT().AllMetadata().Return(nil, errors.Errorf("AllMetadata failed"))

	_, err := svc.FindAgents(context.Background(), modelagent.FindAgentsParams{
		Number:       jujuversion.Current,
		Arch:         corearch.HostArch(),
		AgentStorage: s.storage,
		ToolsURLsGetter: func(ctx context.Context, v semversion.Binary) ([]string, error) {
			return []string{"tools:" + v.String()}, nil
		},
	})
	// ToolsStorage errors always cause FindAgents to bail. Only
	// if AllMetadata succeeds but returns nothing that matches
	// do we continue on to searching simplestreams.
	c.Assert(err, gc.ErrorMatches, "AllMetadata failed")
	c.Assert(called, jc.IsFalse)
}
