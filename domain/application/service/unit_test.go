// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"reflect"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	applicationtesting "github.com/juju/juju/core/application/testing"
	corearch "github.com/juju/juju/core/arch"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
)

type unitServiceSuite struct {
	baseSuite
}

var _ = gc.Suite(&unitServiceSuite{})

func (s *unitServiceSuite) TestSetUnitPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitPassword(gomock.Any(), unitUUID, application.PasswordInfo{
		PasswordHash:  "password",
		HashAlgorithm: 0,
	})

	err := s.service.SetUnitPassword(context.Background(), coreunit.Name("foo/666"), "password")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *unitServiceSuite) TestGetUnitUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := unittesting.GenUnitUUID(c)
	unitName := coreunit.Name("foo/666")
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(uuid, nil)

	u, err := s.service.GetUnitUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(u, gc.Equals, uuid)
}

func (s *unitServiceSuite) TestGetUnitUUIDErrors(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return("", applicationerrors.UnitNotFound)

	_, err := s.service.GetUnitUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

type registerArgMatcher struct {
	c   *gc.C
	arg application.RegisterCAASUnitArg
}

func (m registerArgMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(application.RegisterCAASUnitArg)
	if !ok {
		return false
	}

	m.c.Assert(obtained.PasswordHash, gc.Not(gc.Equals), "")
	obtained.PasswordHash = ""
	m.arg.PasswordHash = ""
	return reflect.DeepEqual(obtained, m.arg)
}

func (m registerArgMatcher) String() string {
	return pretty.Sprint(m.arg)
}

func (s *unitServiceSuite) TestRegisterCAASUnit(c *gc.C) {
	ctrl := s.setupMocksWithProvider(c,
		func(ctx context.Context) (Provider, error) {
			return s.provider, nil
		},
		func(ctx context.Context) (SupportedFeatureProvider, error) {
			return s.supportedFeaturesProvider, nil
		},
		func(ctx context.Context) (CAASApplicationProvider, error) {
			return s.caasApplicationProvider, nil
		})
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().Units().Return([]caas.Unit{{
		Id:      "foo-666",
		Address: "10.6.6.6",
		Ports:   []string{"8080"},
		FilesystemInfo: []caas.FilesystemInfo{{
			Volume: caas.VolumeInfo{VolumeId: "vol-666"},
		}},
	}}, nil)
	s.caasApplicationProvider.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)

	arg := application.RegisterCAASUnitArg{
		UnitName:                  "foo/666",
		PasswordHash:              "secret",
		ProviderID:                "foo-666",
		Address:                   ptr("10.6.6.6"),
		Ports:                     ptr([]string{"8080"}),
		OrderedScale:              true,
		OrderedId:                 666,
		StorageParentDir:          application.StorageParentDir,
		ObservedAttachedVolumeIDs: []string{"vol-666"},
	}
	s.state.EXPECT().RegisterCAASUnit(gomock.Any(), "foo", registerArgMatcher{arg: arg})

	p := application.RegisterCAASUnitParams{
		ApplicationName: "foo",
		ProviderID:      "foo-666",
	}
	unitName, password, err := s.service.RegisterCAASUnit(context.Background(), p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitName.String(), gc.Equals, "foo/666")
	c.Assert(password, gc.Not(gc.Equals), "")
}

func (s *unitServiceSuite) TestRegisterCAASUnitMissingProviderID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	p := application.RegisterCAASUnitParams{
		ApplicationName: "foo",
	}
	_, _, err := s.service.RegisterCAASUnit(context.Background(), p)
	c.Assert(err, gc.ErrorMatches, "provider id not valid")
}

func (s *unitServiceSuite) TestRegisterCAASUnitApplicationNoPods(c *gc.C) {
	ctrl := s.setupMocksWithProvider(c,
		func(ctx context.Context) (Provider, error) {
			return s.provider, nil
		},
		func(ctx context.Context) (SupportedFeatureProvider, error) {
			return s.supportedFeaturesProvider, nil
		},
		func(ctx context.Context) (CAASApplicationProvider, error) {
			return s.caasApplicationProvider, nil
		})
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().Units().Return([]caas.Unit{}, nil)
	s.caasApplicationProvider.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)

	p := application.RegisterCAASUnitParams{
		ApplicationName: "foo",
		ProviderID:      "foo-666",
	}
	_, _, err := s.service.RegisterCAASUnit(context.Background(), p)
	c.Assert(err, jc.ErrorIs, coreerrors.NotFound)
}

func (s *unitServiceSuite) TestUpdateCAASUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	unitName := coreunit.Name("foo/666")
	now := time.Now()

	expected := application.UpdateCAASUnitParams{
		ProviderID: ptr("provider-id"),
		Address:    ptr("10.6.6.6"),
		Ports:      ptr([]string{"666"}),
		AgentStatus: ptr(application.StatusInfo[application.UnitAgentStatusType]{
			Status:  application.UnitAgentStatusAllocating,
			Message: "agent status",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   ptr(now),
		}),
		WorkloadStatus: ptr(application.StatusInfo[application.WorkloadStatusType]{
			Status:  application.WorkloadStatusWaiting,
			Message: "workload status",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   ptr(now),
		}),
		CloudContainerStatus: ptr(application.StatusInfo[application.CloudContainerStatusType]{
			Status:  application.CloudContainerStatusRunning,
			Message: "container status",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   ptr(now),
		}),
	}

	params := UpdateCAASUnitParams{
		ProviderID: ptr("provider-id"),
		Address:    ptr("10.6.6.6"),
		Ports:      ptr([]string{"666"}),
		AgentStatus: ptr(corestatus.StatusInfo{
			Status:  corestatus.Allocating,
			Message: "agent status",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   ptr(now),
		}),
		WorkloadStatus: ptr(corestatus.StatusInfo{
			Status:  corestatus.Waiting,
			Message: "workload status",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   ptr(now),
		}),
		CloudContainerStatus: ptr(corestatus.StatusInfo{
			Status:  corestatus.Running,
			Message: "container status",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   ptr(now),
		}),
	}

	s.state.EXPECT().GetApplicationLife(gomock.Any(), "foo").Return(appID, life.Alive, nil)

	var unitArgs application.UpdateCAASUnitParams
	s.state.EXPECT().UpdateCAASUnit(gomock.Any(), unitName, gomock.Any()).DoAndReturn(func(_ context.Context, _ coreunit.Name, args application.UpdateCAASUnitParams) error {
		unitArgs = args
		return nil
	})

	err := s.service.UpdateCAASUnit(context.Background(), unitName, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitArgs, jc.DeepEquals, expected)
}

func (s *unitServiceSuite) TestUpdateCAASUnitNotAlive(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationLife(gomock.Any(), "foo").Return(id, life.Dying, nil)

	err := s.service.UpdateCAASUnit(context.Background(), coreunit.Name("foo/666"), UpdateCAASUnitParams{})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

// TestSetReportedUnitAgentVersionInvalid is here to assert that if pass a
// junk agent binary version to [Service.SetReportedUnitAgentVersion] we get
// back an error that satisfies [coreerrors.NotValid].
func (s *unitServiceSuite) TestSetReportedUnitAgentVersionInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetReportedUnitAgentVersion(
		context.Background(),
		coreunit.Name("foo/666"),
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
func (s *unitServiceSuite) TestSetReportedUnitAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// UnitNotFound error location 1.
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		"", applicationerrors.UnitNotFound,
	)

	err := s.service.SetReportedUnitAgentVersion(
		context.Background(),
		coreunit.Name("foo/666"),
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

	s.state.EXPECT().SetRunningAgentBinaryVersion(
		gomock.Any(),
		unitUUID,
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(applicationerrors.UnitNotFound)

	err = s.service.SetReportedUnitAgentVersion(
		context.Background(),
		coreunit.Name("foo/666"),
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
func (s *unitServiceSuite) TestSetReportedUnitAgentVersionDead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		coreunit.UUID(unitUUID.String()), nil,
	)

	s.state.EXPECT().SetRunningAgentBinaryVersion(
		gomock.Any(),
		coreunit.UUID(unitUUID.String()),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(applicationerrors.UnitIsDead)

	err := s.service.SetReportedUnitAgentVersion(
		context.Background(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitIsDead)
}

// TestSetReportedUnitAgentVersion asserts the happy path of
// [Service.SetReportedUnitAgentVersion].
func (s *unitServiceSuite) TestSetReportedUnitAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		coreunit.UUID(unitUUID.String()), nil,
	)

	s.state.EXPECT().SetRunningAgentBinaryVersion(
		gomock.Any(),
		coreunit.UUID(unitUUID.String()),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(nil)

	err := s.service.SetReportedUnitAgentVersion(
		context.Background(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIsNil)
}
