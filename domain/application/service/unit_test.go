// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"reflect"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	coreerrors "github.com/juju/juju/core/errors"
	corelife "github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
)

type unitServiceSuite struct {
	baseSuite
}

func TestUnitServiceSuite(t *stdtesting.T) {
	tc.Run(t, &unitServiceSuite{})
}

func (s *unitServiceSuite) TestGetUnitUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := unittesting.GenUnitUUID(c)
	unitName := coreunit.Name("foo/666")
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(uuid, nil)

	u, err := s.service.GetUnitUUID(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(u, tc.Equals, uuid)
}

func (s *unitServiceSuite) TestGetUnitUUIDErrors(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return("", applicationerrors.UnitNotFound)

	_, err := s.service.GetUnitUUID(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

type registerArgMatcher struct {
	arg application.RegisterCAASUnitArg
}

func (m registerArgMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(application.RegisterCAASUnitArg)
	if !ok {
		return false
	}

	obtained.PasswordHash = ""
	m.arg.PasswordHash = ""
	return reflect.DeepEqual(obtained, m.arg)
}

func (m registerArgMatcher) String() string {
	return pretty.Sprint(m.arg)
}

func (s *unitServiceSuite) TestRegisterCAASUnit(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
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
	s.caasProvider.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)

	arg := application.RegisterCAASUnitArg{
		UnitName:                  "foo/666",
		PasswordHash:              "secret",
		ProviderID:                "foo-666",
		Address:                   ptr("10.6.6.6"),
		Ports:                     ptr([]string{"8080"}),
		OrderedScale:              true,
		OrderedId:                 666,
		ObservedAttachedVolumeIDs: []string{"vol-666"},
	}
	s.state.EXPECT().RegisterCAASUnit(gomock.Any(), "foo", registerArgMatcher{arg: arg})

	p := application.RegisterCAASUnitParams{
		ApplicationName: "foo",
		ProviderID:      "foo-666",
	}
	unitName, password, err := s.service.RegisterCAASUnit(c.Context(), p)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitName.String(), tc.Equals, "foo/666")
	c.Assert(password, tc.Not(tc.Equals), "")
}

func (s *unitServiceSuite) TestRegisterCAASUnitMissingProviderID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	p := application.RegisterCAASUnitParams{
		ApplicationName: "foo",
	}
	_, _, err := s.service.RegisterCAASUnit(c.Context(), p)
	c.Assert(err, tc.ErrorMatches, "provider id not valid")
}

func (s *unitServiceSuite) TestRegisterCAASUnitApplicationNoPods(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().Units().Return([]caas.Unit{}, nil)
	s.caasProvider.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)

	p := application.RegisterCAASUnitParams{
		ApplicationName: "foo",
		ProviderID:      "foo-666",
	}
	_, _, err := s.service.RegisterCAASUnit(c.Context(), p)
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *unitServiceSuite) TestUpdateCAASUnit(c *tc.C) {
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

	s.state.EXPECT().GetApplicationLifeByName(gomock.Any(), "foo").Return(appID, life.Alive, nil)

	var unitArgs application.UpdateCAASUnitParams
	s.state.EXPECT().UpdateCAASUnit(gomock.Any(), unitName, gomock.Any()).DoAndReturn(func(_ context.Context, _ coreunit.Name, args application.UpdateCAASUnitParams) error {
		unitArgs = args
		return nil
	})

	err := s.service.UpdateCAASUnit(c.Context(), unitName, params)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(unitArgs, tc.DeepEquals, expected)
}

func (s *unitServiceSuite) TestUpdateCAASUnitNotAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationLifeByName(gomock.Any(), "foo").Return(id, life.Dying, nil)

	err := s.service.UpdateCAASUnit(c.Context(), coreunit.Name("foo/666"), UpdateCAASUnitParams{})
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *unitServiceSuite) TestGetUnitRefreshAttributes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	attrs := application.UnitAttributes{
		Life: life.Alive,
	}
	s.state.EXPECT().GetUnitRefreshAttributes(gomock.Any(), unitName).Return(attrs, nil)

	refreshAttrs, err := s.service.GetUnitRefreshAttributes(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(refreshAttrs, tc.Equals, attrs)
}

func (s *unitServiceSuite) TestGetUnitRefreshAttributesInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("!!!")

	_, err := s.service.GetUnitRefreshAttributes(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *unitServiceSuite) TestGetUnitRefreshAttributesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	attrs := application.UnitAttributes{
		Life: life.Alive,
	}
	s.state.EXPECT().GetUnitRefreshAttributes(gomock.Any(), unitName).Return(attrs, errors.Errorf("boom"))

	_, err := s.service.GetUnitRefreshAttributes(c.Context(), unitName)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *unitServiceSuite) TestGetAllUnitNames(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitNames := []coreunit.Name{"foo/666", "foo/667"}

	s.state.EXPECT().GetAllUnitNames(gomock.Any()).Return(unitNames, nil)

	names, err := s.service.GetAllUnitNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(names, tc.SameContents, unitNames)
}

func (s *unitServiceSuite) TestGetAllUnitNamesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllUnitNames(gomock.Any()).Return(nil, errors.Errorf("boom"))

	_, err := s.service.GetAllUnitNames(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *unitServiceSuite) TestGetUnitNamesForApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appID := applicationtesting.GenApplicationUUID(c)
	unitNames := []coreunit.Name{"foo/666", "foo/667"}

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(appID, nil)
	s.state.EXPECT().GetUnitNamesForApplication(gomock.Any(), appID).Return(unitNames, nil)

	names, err := s.service.GetUnitNamesForApplication(c.Context(), appName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(names, tc.SameContents, unitNames)
}

func (s *unitServiceSuite) TestGetUnitNamesForApplicationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("", applicationerrors.ApplicationNotFound)

	_, err := s.service.GetUnitNamesForApplication(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *unitServiceSuite) TestGetUnitNamesForApplicationDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "foo"
	appID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(appID, nil)
	s.state.EXPECT().GetUnitNamesForApplication(gomock.Any(), appID).Return(nil, applicationerrors.ApplicationIsDead)

	_, err := s.service.GetUnitNamesForApplication(c.Context(), appName)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationIsDead)
}

func (s *unitServiceSuite) TestGetUnitNamesOnMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineNetNodeUUIDFromName(gomock.Any(), coremachine.Name("0")).Return("", applicationerrors.MachineNotFound)

	_, err := s.service.GetUnitNamesOnMachine(c.Context(), coremachine.Name("0"))
	c.Assert(err, tc.ErrorIs, applicationerrors.MachineNotFound)
}

func (s *unitServiceSuite) TestGetUnitNamesOnMachineInvalidMachineName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetUnitNamesOnMachine(c.Context(), coremachine.Name(""))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *unitServiceSuite) TestGetUnitNamesOnMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	netNodeUUID := "net-node-uuid"
	s.state.EXPECT().GetMachineNetNodeUUIDFromName(gomock.Any(), coremachine.Name("0")).Return(netNodeUUID, nil)
	s.state.EXPECT().GetUnitNamesForNetNode(gomock.Any(), netNodeUUID).Return([]coreunit.Name{"foo/666", "bar/667"}, nil)

	names, err := s.service.GetUnitNamesOnMachine(c.Context(), coremachine.Name("0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(names, tc.DeepEquals, []coreunit.Name{"foo/666", "bar/667"})
}

func (s *unitServiceSuite) TestAddIAASSubordinateUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	appID := applicationtesting.GenApplicationUUID(c)
	unitName := unittesting.GenNewName(c, "principal/0")

	s.state.EXPECT().IsSubordinateApplication(gomock.Any(), appID).Return(true, nil)
	var foundApp coreapplication.ID
	var foundUnit coreunit.Name
	s.state.EXPECT().AddIAASSubordinateUnit(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, arg application.SubordinateUnitArg) (coreunit.Name, []coremachine.Name, error) {
			foundApp = arg.SubordinateAppID
			foundUnit = arg.PrincipalUnitName
			return "subordinate/0", nil, nil
		},
	)

	// Act:
	err := s.service.AddIAASSubordinateUnit(c.Context(), appID, unitName)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(foundApp, tc.Equals, appID)
	c.Assert(foundUnit, tc.Equals, unitName)
}

func (s *unitServiceSuite) TestAddIAASSubordinateUnitUnitAlreadyHasSubordinate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	appID := applicationtesting.GenApplicationUUID(c)
	unitName := unittesting.GenNewName(c, "principal/0")
	s.state.EXPECT().IsSubordinateApplication(gomock.Any(), appID).Return(true, nil)
	s.state.EXPECT().AddIAASSubordinateUnit(gomock.Any(), gomock.Any()).Return("", nil, applicationerrors.UnitAlreadyHasSubordinate)

	// Act:
	err := s.service.AddIAASSubordinateUnit(c.Context(), appID, unitName)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitServiceSuite) TestAddIAASSubordinateUnitServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	appID := applicationtesting.GenApplicationUUID(c)
	unitName := unittesting.GenNewName(c, "principal/0")

	s.state.EXPECT().IsSubordinateApplication(gomock.Any(), appID).Return(true, nil)

	boom := errors.New("boom")
	s.state.EXPECT().AddIAASSubordinateUnit(gomock.Any(), gomock.Any()).Return("", nil, boom)

	// Act:
	err := s.service.AddIAASSubordinateUnit(c.Context(), appID, unitName)

	// Assert:
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *unitServiceSuite) TestAddIAASSubordinateUnitApplicationNotSubordinate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	appID := applicationtesting.GenApplicationUUID(c)
	unitName := unittesting.GenNewName(c, "principal/0")
	s.state.EXPECT().IsSubordinateApplication(gomock.Any(), appID).Return(false, nil)

	// Act:
	err := s.service.AddIAASSubordinateUnit(c.Context(), appID, unitName)

	// Assert:
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotSubordinate)
}

func (s *unitServiceSuite) TestAddIAASSubordinateUnitBadUnitName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	appID := applicationtesting.GenApplicationUUID(c)

	// Act:
	err := s.service.AddIAASSubordinateUnit(c.Context(), appID, "bad-name")

	// Assert:
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *unitServiceSuite) TestAddIAASSubordinateUnitBadAppName(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	unitName := unittesting.GenNewName(c, "principal/0")

	// Act:
	err := s.service.AddIAASSubordinateUnit(c.Context(), "bad-app-uuid", unitName)

	// Assert:
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *unitServiceSuite) TestSetUnitWorkloadVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	workloadVersion := "v1.0.0"

	s.state.EXPECT().SetUnitWorkloadVersion(gomock.Any(), unitName, workloadVersion).Return(nil)

	err := s.service.SetUnitWorkloadVersion(c.Context(), unitName, workloadVersion)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitServiceSuite) TestSetUnitWorkloadVersionError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	workloadVersion := "v1.0.0"

	s.state.EXPECT().SetUnitWorkloadVersion(gomock.Any(), unitName, workloadVersion).Return(errors.Errorf("boom"))

	err := s.service.SetUnitWorkloadVersion(c.Context(), unitName, workloadVersion)
	c.Assert(err, tc.ErrorMatches, ".*boom")
}

func (s *unitServiceSuite) TestSetUnitWorkloadVersionInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("!!!")
	workloadVersion := "v1.0.0"

	err := s.service.SetUnitWorkloadVersion(c.Context(), unitName, workloadVersion)
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *unitServiceSuite) TestGetUnitWorkloadVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	workloadVersion := "v1.0.0"

	s.state.EXPECT().GetUnitWorkloadVersion(gomock.Any(), unitName).Return(workloadVersion, nil)

	version, err := s.service.GetUnitWorkloadVersion(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(version, tc.Equals, workloadVersion)
}

func (s *unitServiceSuite) TestGetUnitWorkloadVersionError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")

	s.state.EXPECT().GetUnitWorkloadVersion(gomock.Any(), unitName).Return("", errors.Errorf("boom"))

	_, err := s.service.GetUnitWorkloadVersion(c.Context(), unitName)
	c.Assert(err, tc.ErrorMatches, ".*boom")
}

func (s *unitServiceSuite) TestGetUnitPrincipal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	principalUnitName := coreunit.Name("principal/666")
	s.state.EXPECT().GetUnitPrincipal(gomock.Any(), unitName).Return(principalUnitName, true, nil)

	u, ok, err := s.service.GetUnitPrincipal(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(u, tc.Equals, principalUnitName)
	c.Check(ok, tc.IsTrue)
}

func (s *unitServiceSuite) TestGetUnitPrincipalError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	boom := errors.New("boom")
	s.state.EXPECT().GetUnitPrincipal(gomock.Any(), unitName).Return("", false, boom)

	_, _, err := s.service.GetUnitPrincipal(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *unitServiceSuite) TestGetUnitMachineName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	s.state.EXPECT().GetUnitMachineName(gomock.Any(), unitName).Return("0", nil)

	name, err := s.service.GetUnitMachineName(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(name, tc.Equals, coremachine.Name("0"))
}

func (s *unitServiceSuite) TestGetUnitMachineNameError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	boom := errors.New("boom")
	s.state.EXPECT().GetUnitMachineName(gomock.Any(), unitName).Return("", boom)

	_, err := s.service.GetUnitMachineName(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *unitServiceSuite) TestGetUnitMachineUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	s.state.EXPECT().GetUnitMachineUUID(gomock.Any(), unitName).Return("fake-uuid", nil)

	uuid, err := s.service.GetUnitMachineUUID(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, coremachine.UUID("fake-uuid"))
}

func (s *unitServiceSuite) TestGetUnitMachineUUIDError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	boom := errors.New("boom")
	s.state.EXPECT().GetUnitMachineUUID(gomock.Any(), unitName).Return("", boom)

	_, err := s.service.GetUnitMachineUUID(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *unitServiceSuite) TestGetUnitK8sPodInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	ports := []string{"666", "668"}
	s.state.EXPECT().GetUnitK8sPodInfo(gomock.Any(), unitName).Return(application.K8sPodInfo{
		ProviderID: "some-id",
		Address:    "10.6.6.6",
		Ports:      ports,
	}, nil)

	info, err := s.service.GetUnitK8sPodInfo(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.ProviderID, tc.Equals, network.Id("some-id"))
	c.Check(info.Address, tc.Equals, "10.6.6.6")
	c.Check(info.Ports, tc.DeepEquals, ports)
}

func (s *unitServiceSuite) TestGetUnitK8sPodInfoUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")

	s.state.EXPECT().GetUnitK8sPodInfo(gomock.Any(), unitName).Return(application.K8sPodInfo{}, applicationerrors.UnitNotFound)

	_, err := s.service.GetUnitK8sPodInfo(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitServiceSuite) TestGetUnitSubordinates(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	names := []coreunit.Name{"sub/667"}
	s.state.EXPECT().GetUnitSubordinates(gomock.Any(), unitName).Return(names, nil)

	foundNames, err := s.service.GetUnitSubordinates(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(foundNames, tc.DeepEquals, names)
}

func (s *unitServiceSuite) TestGetUnitSubordinatesUnitNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetUnitSubordinates(c.Context(), "bad-name")
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *unitServiceSuite) TestGetUnitSubordinatesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	boom := errors.New("boom")
	s.state.EXPECT().GetUnitSubordinates(gomock.Any(), unitName).Return(nil, boom)

	_, err := s.service.GetUnitSubordinates(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *unitServiceSuite) TestGetAllUnitLifeForApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)

	allUnitDomainLife := map[coreunit.Name]life.Life{
		coreunit.Name("foo/0"): life.Alive,
		coreunit.Name("foo/1"): life.Dying,
		coreunit.Name("foo/2"): life.Dead,
	}
	s.state.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appID).
		Return(allUnitDomainLife, nil)

	allUnitLife, err := s.service.GetAllUnitLifeForApplication(c.Context(), appID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(allUnitLife, tc.DeepEquals, map[coreunit.Name]corelife.Value{
		coreunit.Name("foo/0"): corelife.Alive,
		coreunit.Name("foo/1"): corelife.Dying,
		coreunit.Name("foo/2"): corelife.Dead,
	})
}

func (s *unitServiceSuite) TestGetAllUnitLifeForApplicationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)

	boom := errors.New("boom")
	s.state.EXPECT().GetAllUnitLifeForApplication(gomock.Any(), appID).
		Return(nil, boom)

	allUnitLife, err := s.service.GetAllUnitLifeForApplication(c.Context(), appID)
	c.Assert(err, tc.ErrorIs, boom)
	c.Check(allUnitLife, tc.IsNil)
}
