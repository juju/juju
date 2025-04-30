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
	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

type unitServiceSuite struct {
	baseSuite
}

var _ = gc.Suite(&unitServiceSuite{})

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
		AgentStatus: ptr(status.StatusInfo[status.UnitAgentStatusType]{
			Status:  status.UnitAgentStatusAllocating,
			Message: "agent status",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   ptr(now),
		}),
		WorkloadStatus: ptr(status.StatusInfo[status.WorkloadStatusType]{
			Status:  status.WorkloadStatusWaiting,
			Message: "workload status",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   ptr(now),
		}),
		K8sPodStatus: ptr(status.StatusInfo[status.K8sPodStatusType]{
			Status:  status.K8sPodStatusRunning,
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

func (s *unitServiceSuite) TestGetUnitRefreshAttributes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	attrs := application.UnitAttributes{
		Life: life.Alive,
	}
	s.state.EXPECT().GetUnitRefreshAttributes(gomock.Any(), unitName).Return(attrs, nil)

	refreshAttrs, err := s.service.GetUnitRefreshAttributes(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(refreshAttrs, gc.Equals, attrs)
}

func (s *unitServiceSuite) TestGetUnitRefreshAttributesInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("!!!")

	_, err := s.service.GetUnitRefreshAttributes(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *unitServiceSuite) TestGetUnitRefreshAttributesError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	attrs := application.UnitAttributes{
		Life: life.Alive,
	}
	s.state.EXPECT().GetUnitRefreshAttributes(gomock.Any(), unitName).Return(attrs, errors.Errorf("boom"))

	_, err := s.service.GetUnitRefreshAttributes(context.Background(), unitName)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *unitServiceSuite) TestGetAllUnitNames(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitNames := []coreunit.Name{"foo/666", "foo/667"}

	s.state.EXPECT().GetAllUnitNames(gomock.Any()).Return(unitNames, nil)

	names, err := s.service.GetAllUnitNames(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(names, jc.SameContents, unitNames)
}

func (s *unitServiceSuite) TestGetAllUnitNamesError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllUnitNames(gomock.Any()).Return(nil, errors.Errorf("boom"))

	_, err := s.service.GetAllUnitNames(context.Background())
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *unitServiceSuite) TestGetUnitNamesForApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appID := applicationtesting.GenApplicationUUID(c)
	unitNames := []coreunit.Name{"foo/666", "foo/667"}

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(appID, nil)
	s.state.EXPECT().GetUnitNamesForApplication(gomock.Any(), appID).Return(unitNames, nil)

	names, err := s.service.GetUnitNamesForApplication(context.Background(), appName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(names, jc.SameContents, unitNames)
}

func (s *unitServiceSuite) TestGetUnitNamesForApplicationNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("", applicationerrors.ApplicationNotFound)

	_, err := s.service.GetUnitNamesForApplication(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *unitServiceSuite) TestGetUnitNamesForApplicationDead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(appID, nil)
	s.state.EXPECT().GetUnitNamesForApplication(gomock.Any(), appID).Return(nil, applicationerrors.ApplicationIsDead)

	_, err := s.service.GetUnitNamesForApplication(context.Background(), appName)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationIsDead)
}

func (s *unitServiceSuite) TestGetUnitNamesOnMachineNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineNetNodeUUIDFromName(gomock.Any(), machine.Name("0")).Return("", applicationerrors.MachineNotFound)

	_, err := s.service.GetUnitNamesOnMachine(context.Background(), machine.Name("0"))
	c.Assert(err, jc.ErrorIs, applicationerrors.MachineNotFound)
}

func (s *unitServiceSuite) TestGetUnitNamesOnMachine(c *gc.C) {
	defer s.setupMocks(c).Finish()

	netNodeUUID := uuid.MustNewUUID().String()
	s.state.EXPECT().GetMachineNetNodeUUIDFromName(gomock.Any(), machine.Name("0")).Return(netNodeUUID, nil)
	s.state.EXPECT().GetUnitNamesForNetNode(gomock.Any(), netNodeUUID).Return([]coreunit.Name{"foo/666", "bar/667"}, nil)

	names, err := s.service.GetUnitNamesOnMachine(context.Background(), machine.Name("0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(names, jc.DeepEquals, []coreunit.Name{"foo/666", "bar/667"})
}

func (s *unitServiceSuite) TestAddSubordinateUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	appID := applicationtesting.GenApplicationUUID(c)
	unitName := unittesting.GenNewName(c, "principal/0")

	s.state.EXPECT().IsSubordinateApplication(gomock.Any(), appID).Return(true, nil)
	s.state.EXPECT().GetModelType(gomock.Any()).Return(coremodel.IAAS, nil)
	var foundApp coreapplication.ID
	var foundUnit coreunit.Name
	s.state.EXPECT().AddSubordinateUnit(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, arg application.SubordinateUnitArg) (coreunit.Name, error) {
			foundApp = arg.SubordinateAppID
			foundUnit = arg.PrincipalUnitName
			return "subordinate/0", nil
		},
	)

	// Act:
	err := s.service.AddSubordinateUnit(context.Background(), appID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foundApp, gc.Equals, appID)
	c.Assert(foundUnit, gc.Equals, unitName)
}

func (s *unitServiceSuite) TestAddSubordinateUnitUnitAlreadyHasSubordinate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	appID := applicationtesting.GenApplicationUUID(c)
	unitName := unittesting.GenNewName(c, "principal/0")
	s.state.EXPECT().IsSubordinateApplication(gomock.Any(), appID).Return(true, nil)
	s.state.EXPECT().GetModelType(gomock.Any()).Return(coremodel.IAAS, nil)
	s.state.EXPECT().AddSubordinateUnit(gomock.Any(), gomock.Any()).Return("", applicationerrors.UnitAlreadyHasSubordinate)

	// Act:
	err := s.service.AddSubordinateUnit(context.Background(), appID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
}

func (s *unitServiceSuite) TestAddSubordinateUnitServiceError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	appID := applicationtesting.GenApplicationUUID(c)
	unitName := unittesting.GenNewName(c, "principal/0")

	s.state.EXPECT().IsSubordinateApplication(gomock.Any(), appID).Return(true, nil)
	s.state.EXPECT().GetModelType(gomock.Any()).Return(coremodel.IAAS, nil)

	boom := errors.New("boom")
	s.state.EXPECT().AddSubordinateUnit(gomock.Any(), gomock.Any()).Return("", boom)

	// Act:
	err := s.service.AddSubordinateUnit(context.Background(), appID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIs, boom)
}

func (s *unitServiceSuite) TestAddSubordinateUnitApplicationNotSubordinate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	appID := applicationtesting.GenApplicationUUID(c)
	unitName := unittesting.GenNewName(c, "principal/0")
	s.state.EXPECT().IsSubordinateApplication(gomock.Any(), appID).Return(false, nil)

	// Act:
	err := s.service.AddSubordinateUnit(context.Background(), appID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotSubordinate)
}

func (s *unitServiceSuite) TestAddSubordinateUnitBadUnitName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	appID := applicationtesting.GenApplicationUUID(c)

	// Act:
	err := s.service.AddSubordinateUnit(context.Background(), appID, "bad-name")

	// Assert:
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *unitServiceSuite) TestAddSubordinateUnitBadAppName(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	unitName := unittesting.GenNewName(c, "principal/0")

	// Act:
	err := s.service.AddSubordinateUnit(context.Background(), "bad-app-uuid", unitName)

	// Assert:
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *unitServiceSuite) TestSetUnitWorkloadVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	workloadVersion := "v1.0.0"

	s.state.EXPECT().SetUnitWorkloadVersion(gomock.Any(), unitName, workloadVersion).Return(nil)

	err := s.service.SetUnitWorkloadVersion(context.Background(), unitName, workloadVersion)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *unitServiceSuite) TestSetUnitWorkloadVersionError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	workloadVersion := "v1.0.0"

	s.state.EXPECT().SetUnitWorkloadVersion(gomock.Any(), unitName, workloadVersion).Return(errors.Errorf("boom"))

	err := s.service.SetUnitWorkloadVersion(context.Background(), unitName, workloadVersion)
	c.Assert(err, gc.ErrorMatches, ".*boom")
}

func (s *unitServiceSuite) TestSetUnitWorkloadVersionInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("!!!")
	workloadVersion := "v1.0.0"

	err := s.service.SetUnitWorkloadVersion(context.Background(), unitName, workloadVersion)
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *unitServiceSuite) TestGetUnitWorkloadVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	workloadVersion := "v1.0.0"

	s.state.EXPECT().GetUnitWorkloadVersion(gomock.Any(), unitName).Return(workloadVersion, nil)

	version, err := s.service.GetUnitWorkloadVersion(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(version, gc.Equals, workloadVersion)
}

func (s *unitServiceSuite) TestGetUnitWorkloadVersionError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")

	s.state.EXPECT().GetUnitWorkloadVersion(gomock.Any(), unitName).Return("", errors.Errorf("boom"))

	_, err := s.service.GetUnitWorkloadVersion(context.Background(), unitName)
	c.Assert(err, gc.ErrorMatches, ".*boom")
}

func (s *unitServiceSuite) TestGetUnitPrincipal(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	principalUnitName := coreunit.Name("principal/666")
	s.state.EXPECT().GetUnitPrincipal(gomock.Any(), unitName).Return(principalUnitName, true, nil)

	u, ok, err := s.service.GetUnitPrincipal(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(u, gc.Equals, principalUnitName)
	c.Check(ok, jc.IsTrue)
}

func (s *unitServiceSuite) TestGetUnitPrincipalError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	boom := errors.New("boom")
	s.state.EXPECT().GetUnitPrincipal(gomock.Any(), unitName).Return("", false, boom)

	_, _, err := s.service.GetUnitPrincipal(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, boom)
}
