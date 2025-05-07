// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/application"
	apicharm "github.com/juju/juju/api/common/charm"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type applicationSuite struct{}

var _ = tc.Suite(&applicationSuite{})

func (s *applicationSuite) TestDeploy(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	result := new(params.ErrorResults)
	results := params.ErrorResults{Results: make([]params.ErrorResult, 1)}

	deployArgs := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{
			{
				CharmURL:         "ch:a-charm-1",
				CharmOrigin:      &params.CharmOrigin{Source: "charm-hub"},
				ApplicationName:  "applicationA",
				NumUnits:         1,
				ConfigYAML:       "configYAML",
				Config:           map[string]string{"foo": "bar"},
				Constraints:      constraints.MustParse("mem=4G"),
				Placement:        []*instance.Placement{{Scope: "scope", Directive: "directive"}},
				EndpointBindings: map[string]string{"foo": "bar"},
				Storage:          map[string]storage.Directive{"data": {Pool: "pool"}},
				AttachStorage:    []string{"storage-data-0"},
				Resources:        map[string]string{"foo": "bar"},
			},
		},
	}

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Deploy", deployArgs, result).SetArg(3, results).Return(nil)

	args := application.DeployArgs{
		CharmID: application.CharmID{
			URL: "ch:a-charm-1",
		},
		CharmOrigin: apicharm.Origin{
			Source: apicharm.OriginCharmHub,
		},
		ApplicationName:  "applicationA",
		NumUnits:         1,
		ConfigYAML:       "configYAML",
		Config:           map[string]string{"foo": "bar"},
		Cons:             constraints.MustParse("mem=4G"),
		Placement:        []*instance.Placement{{Scope: "scope", Directive: "directive"}},
		Storage:          map[string]storage.Directive{"data": {Pool: "pool"}},
		AttachStorage:    []string{"data/0"},
		Resources:        map[string]string{"foo": "bar"},
		EndpointBindings: map[string]string{"foo": "bar"},
	}

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.Deploy(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestDeployAlreadyExists(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	result := new(params.ErrorResults)
	results := params.ErrorResults{Results: []params.ErrorResult{{Error: &params.Error{Message: "application already exists", Code: params.CodeAlreadyExists}}}}

	deployArgs := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{
			{
				CharmURL:         "ch:a-charm-1",
				CharmOrigin:      &params.CharmOrigin{Source: "charm-hub"},
				ApplicationName:  "applicationA",
				NumUnits:         1,
				ConfigYAML:       "configYAML",
				Config:           map[string]string{"foo": "bar"},
				Constraints:      constraints.MustParse("mem=4G"),
				Placement:        []*instance.Placement{{Scope: "scope", Directive: "directive"}},
				EndpointBindings: map[string]string{"foo": "bar"},
				Storage:          map[string]storage.Directive{"data": {Pool: "pool"}},
				AttachStorage:    []string{"storage-data-0"},
				Resources:        map[string]string{"foo": "bar"},
			},
		},
	}

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Deploy", deployArgs, result).SetArg(3, results).Return(nil)

	args := application.DeployArgs{
		CharmID: application.CharmID{
			URL: "ch:a-charm-1",
		},
		CharmOrigin: apicharm.Origin{
			Source: apicharm.OriginCharmHub,
		},
		ApplicationName:  "applicationA",
		NumUnits:         1,
		ConfigYAML:       "configYAML",
		Config:           map[string]string{"foo": "bar"},
		Cons:             constraints.MustParse("mem=4G"),
		Placement:        []*instance.Placement{{Scope: "scope", Directive: "directive"}},
		Storage:          map[string]storage.Directive{"data": {Pool: "pool"}},
		AttachStorage:    []string{"data/0"},
		Resources:        map[string]string{"foo": "bar"},
		EndpointBindings: map[string]string{"foo": "bar"},
	}

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.Deploy(context.Background(), args)
	c.Assert(err, tc.ErrorMatches, `application already exists`)
}

func (s *applicationSuite) TestDeployAttachStorageMultipleUnits(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)

	args := application.DeployArgs{
		NumUnits:      2,
		AttachStorage: []string{"data/0"},
	}
	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.Deploy(context.Background(), args)
	c.Assert(err, tc.ErrorMatches, "cannot attach existing storage when more than one unit is requested")
}

func (s *applicationSuite) TestAddUnits(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	result := new(params.AddApplicationUnitsResults)
	results := params.AddApplicationUnitsResults{
		Units: []string{"foo/0"},
	}

	args := params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        1,
		Placement:       []*instance.Placement{{Scope: "scope", Directive: "directive"}},
		AttachStorage:   []string{"storage-data-0"},
	}

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddUnits", args, result).SetArg(3, results).Return(nil)
	client := application.NewClientFromCaller(mockFacadeCaller)
	units, err := client.AddUnits(context.Background(), application.AddUnitsParams{
		ApplicationName: "foo",
		NumUnits:        1,
		Placement:       []*instance.Placement{{Scope: "scope", Directive: "directive"}},
		AttachStorage:   []string{"data/0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, jc.DeepEquals, []string{"foo/0"})
}

func (s *applicationSuite) TestAddUnitsAttachStorageMultipleUnits(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.AddUnits(context.Background(), application.AddUnitsParams{
		NumUnits:      2,
		AttachStorage: []string{"data/0"},
	})
	c.Assert(err, tc.ErrorMatches, "cannot attach existing storage when more than one unit is requested")
}

func (s *applicationSuite) TestApplicationGetCharmURLOrigin(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	result := new(params.CharmURLOriginResult)
	results := params.CharmURLOriginResult{
		URL:    "ch:curl",
		Origin: params.CharmOrigin{Risk: "edge"},
	}
	args := params.ApplicationGet{
		ApplicationName: "application",
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "GetCharmURLOrigin", args, result).SetArg(3, results).Return(nil)
	client := application.NewClientFromCaller(mockFacadeCaller)

	curl, origin, err := client.GetCharmURLOrigin(context.Background(), "application")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, tc.DeepEquals, charm.MustParseURL("ch:curl"))
	c.Assert(origin, tc.DeepEquals, apicharm.Origin{
		Risk: "edge",
	})
}

func (s *applicationSuite) TestSetCharm(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	toUint64Ptr := func(v uint64) *uint64 {
		return &v
	}

	args := params.ApplicationSetCharmV2{
		ApplicationName: "application",
		CharmURL:        "ch:application-1",
		CharmOrigin: &params.CharmOrigin{
			Source: "charm-hub",
			Risk:   "edge",
		},
		Channel: "edge",
		ConfigSettings: map[string]string{
			"a": "b",
			"c": "d",
		},
		ConfigSettingsYAML: "yaml",
		Force:              true,
		ForceBase:          true,
		ForceUnits:         true,
		StorageDirectives: map[string]params.StorageDirectives{
			"a": {Pool: "radiant"},
			"b": {Count: toUint64Ptr(123)},
			"c": {Size: toUint64Ptr(123)},
		},
	}

	c.Assert(args.ConfigSettingsYAML, tc.Equals, "yaml")
	c.Assert(args.Force, tc.Equals, true)
	c.Assert(args.ForceBase, tc.Equals, true)
	c.Assert(args.ForceUnits, tc.Equals, true)
	c.Assert(args.StorageDirectives, jc.DeepEquals, map[string]params.StorageDirectives{
		"a": {Pool: "radiant"},
		"b": {Count: toUint64Ptr(123)},
		"c": {Size: toUint64Ptr(123)},
	})

	cfg := application.SetCharmConfig{
		ApplicationName: "application",
		CharmID: application.CharmID{
			URL: "ch:application-1",
			Origin: apicharm.Origin{
				Source: "charm-hub",
				Risk:   "edge",
			},
		},
		ConfigSettings: map[string]string{
			"a": "b",
			"c": "d",
		},
		ConfigSettingsYAML: "yaml",
		Force:              true,
		ForceBase:          true,
		ForceUnits:         true,
		StorageDirectives: map[string]storage.Directive{
			"a": {Pool: "radiant"},
			"b": {Count: 123},
			"c": {Size: 123},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetCharm", args, nil).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.SetCharm(context.Background(), cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestDestroyApplications(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedResults := []params.DestroyApplicationResult{{
		Error: &params.Error{Message: "boo"},
	}, {
		Info: &params.DestroyApplicationInfo{
			DestroyedStorage: []params.Entity{{Tag: "storage-pgdata-0"}},
			DetachedStorage:  []params.Entity{{Tag: "storage-pgdata-1"}},
			DestroyedUnits:   []params.Entity{{Tag: "unit-bar-1"}},
		},
	}}
	delay := 1 * time.Minute
	result := new(params.DestroyApplicationResults)
	results := params.DestroyApplicationResults{Results: expectedResults}
	args := params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{
			{ApplicationTag: "application-foo", Force: true, MaxWait: &delay},
			{ApplicationTag: "application-bar", Force: true, MaxWait: &delay},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DestroyApplication", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.DestroyApplications(context.Background(), application.DestroyApplicationsParams{
		Applications: []string{"foo", "bar"},
		Force:        true,
		MaxWait:      &delay,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestDestroyApplicationsArity(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	result := new(params.DestroyApplicationResults)
	results := params.DestroyApplicationResults{}
	args := params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{
			{ApplicationTag: "application-foo"},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DestroyApplication", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.DestroyApplications(context.Background(), application.DestroyApplicationsParams{
		Applications: []string{"foo"},
	})
	c.Assert(err, tc.ErrorMatches, `expected 1 result\(s\), got 0`)
}

func (s *applicationSuite) TestDestroyApplicationsInvalidIds(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	expectedResults := []params.DestroyApplicationResult{{
		Error: &params.Error{Message: `application name "!" not valid`},
	}, {
		Info: &params.DestroyApplicationInfo{},
	}}
	result := new(params.DestroyApplicationResults)
	results := params.DestroyApplicationResults{expectedResults[1:]}

	applications := []params.DestroyApplicationParams{
		{ApplicationTag: names.NewApplicationTag("foo").String()},
	}
	args := params.DestroyApplicationsParams{Applications: applications}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DestroyApplication", args, result).SetArg(3, results).Return(nil)
	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.DestroyApplications(context.Background(), application.DestroyApplicationsParams{
		Applications: []string{"!", "foo"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestDestroyConsumedApplicationsArity(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	result := new(params.ErrorResults)
	results := params.ErrorResults{}
	force := false
	args := params.DestroyConsumedApplicationsParams{
		Applications: []params.DestroyConsumedApplicationParams{
			{ApplicationTag: names.NewApplicationTag("foo").String(), Force: &force},
		},
	}
	destroyParams := application.DestroyConsumedApplicationParams{
		[]string{"foo"}, false, nil,
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DestroyConsumedApplications", args, result).SetArg(3, results).Return(nil)
	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.DestroyConsumedApplication(context.Background(), destroyParams)
	c.Assert(err, tc.ErrorMatches, `expected 1 result\(s\), got 0`)
}

func (s *applicationSuite) TestDestroyConsumedApplications(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	noWait := 1 * time.Minute
	force := true
	result := new(params.ErrorResults)
	expectedResults := []params.ErrorResult{{}, {}}
	results := params.ErrorResults{expectedResults}
	args := params.DestroyConsumedApplicationsParams{
		Applications: []params.DestroyConsumedApplicationParams{
			{ApplicationTag: "application-foo", Force: &force, MaxWait: &noWait},
			{ApplicationTag: "application-bar", Force: &force, MaxWait: &noWait},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DestroyConsumedApplications", args, result).SetArg(3, results).Return(nil)
	destroyParams := application.DestroyConsumedApplicationParams{
		[]string{"foo"}, false, &noWait,
	}
	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.DestroyConsumedApplication(context.Background(), destroyParams)
	c.Check(err, tc.ErrorMatches, "--force is required when --max-wait is provided")
	c.Check(res, tc.HasLen, 0)

	destroyParams = application.DestroyConsumedApplicationParams{
		[]string{"foo", "bar"}, force, &noWait,
	}
	res, err = client.DestroyConsumedApplication(context.Background(), destroyParams)
	c.Check(err, jc.ErrorIsNil)
	c.Check(res, tc.HasLen, 2)
}

func (s *applicationSuite) TestDestroyUnits(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedResults := []params.DestroyUnitResult{{
		Error: &params.Error{Message: "boo"},
	}, {
		Info: &params.DestroyUnitInfo{
			DestroyedStorage: []params.Entity{{Tag: "storage-pgdata-0"}},
			DetachedStorage:  []params.Entity{{Tag: "storage-pgdata-1"}},
		},
	}}
	result := new(params.DestroyUnitResults)
	results := params.DestroyUnitResults{expectedResults}
	delay := 1 * time.Minute

	args := params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{
			{UnitTag: "unit-foo-0", Force: true, MaxWait: &delay},
			{UnitTag: "unit-bar-1", Force: true, MaxWait: &delay},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DestroyUnit", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.DestroyUnits(context.Background(), application.DestroyUnitsParams{
		Units:   []string{"foo/0", "bar/1"},
		Force:   true,
		MaxWait: &delay,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestDestroyUnitsArity(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	result := new(params.DestroyUnitResults)
	results := params.DestroyUnitResults{}
	args := params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{
			{UnitTag: names.NewUnitTag("foo/0").String()},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DestroyUnit", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.DestroyUnits(context.Background(), application.DestroyUnitsParams{
		Units: []string{"foo/0"},
	})
	c.Assert(err, tc.ErrorMatches, `expected 1 result\(s\), got 0`)
}

func (s *applicationSuite) TestDestroyUnitsInvalidIds(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedResults := []params.DestroyUnitResult{{
		Error: &params.Error{Message: `unit ID "!" not valid`},
	}, {
		Info: &params.DestroyUnitInfo{},
	}}
	result := new(params.DestroyUnitResults)
	results := params.DestroyUnitResults{expectedResults[1:]}
	args := params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{
			{UnitTag: names.NewUnitTag("foo/0").String()},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DestroyUnit", args, result).SetArg(3, results).Return(nil)
	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.DestroyUnits(context.Background(), application.DestroyUnitsParams{
		Units: []string{"!", "foo/0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestConsume(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	offer := params.ApplicationOfferDetailsV5{
		SourceModelTag:         "source model",
		OfferName:              "an offer",
		OfferUUID:              "offer-uuid",
		OfferURL:               "offer url",
		ApplicationDescription: "description",
		Endpoints:              []params.RemoteEndpoint{{Name: "endpoint"}},
	}
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	controllerInfo := &params.ExternalControllerInfo{
		ControllerTag: coretesting.ControllerTag.String(),
		Alias:         "controller-alias",
		Addrs:         []string{"192.168.1.0"},
		CACert:        coretesting.CACert,
	}

	result := new(params.ErrorResults)
	results := params.ErrorResults{Results: []params.ErrorResult{{}}}
	args := params.ConsumeApplicationArgsV5{
		Args: []params.ConsumeApplicationArgV5{
			{
				ApplicationAlias:          "alias",
				ApplicationOfferDetailsV5: offer,
				Macaroon:                  mac,
				ControllerInfo:            controllerInfo,
			},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Consume", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	name, err := client.Consume(context.Background(), crossmodel.ConsumeApplicationArgs{
		Offer:            offer,
		ApplicationAlias: "alias",
		Macaroon:         mac,
		ControllerInfo: &crossmodel.ControllerInfo{
			ControllerUUID: coretesting.ControllerTag.Id(),
			Alias:          "controller-alias",
			Addrs:          controllerInfo.Addrs,
			CACert:         controllerInfo.CACert,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, tc.Equals, "alias")
}

func (s *applicationSuite) TestDestroyRelation(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	false_ := false
	true_ := true
	zero := time.Minute * 1
	for _, t := range []struct {
		force   *bool
		maxWait *time.Duration
	}{
		{},
		{force: &true_},
		{force: &false_},
		{maxWait: &zero},
		{force: &false_, maxWait: &zero},
		{force: &true_, maxWait: &zero},
	} {
		args := params.DestroyRelation{
			Endpoints: []string{"ep1", "ep2"},
			Force:     t.force,
			MaxWait:   t.maxWait,
		}
		mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
		mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DestroyRelation", args, nil).Return(nil)

		client := application.NewClientFromCaller(mockFacadeCaller)
		err := client.DestroyRelation(context.Background(), t.force, t.maxWait, "ep1", "ep2")
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *applicationSuite) TestDestroyRelationId(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	false_ := false
	true_ := true
	zero := time.Minute * 1
	for _, t := range []struct {
		force   *bool
		maxWait *time.Duration
	}{
		{},
		{force: &true_},
		{force: &false_},
		{maxWait: &zero},
		{force: &false_, maxWait: &zero},
		{force: &true_, maxWait: &zero},
	} {
		args := params.DestroyRelation{
			RelationId: 123,
			Force:      t.force,
			MaxWait:    t.maxWait,
		}
		mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
		mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DestroyRelation", args, nil).Return(nil)

		client := application.NewClientFromCaller(mockFacadeCaller)
		err := client.DestroyRelationId(context.Background(), 123, t.force, t.maxWait)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *applicationSuite) TestSetRelationSuspended(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.RelationSuspendedArgs{
		Args: []params.RelationSuspendedArg{
			{
				RelationId: 123,
				Suspended:  true,
				Message:    "message",
			}, {
				RelationId: 456,
				Suspended:  true,
				Message:    "message",
			}},
	}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}, {}},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetRelationsSuspended", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.SetRelationSuspended(context.Background(), []int{123, 456}, true, "message")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestSetRelationSuspendedArity(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.RelationSuspendedArgs{
		Args: []params.RelationSuspendedArg{
			{
				RelationId: 123,
				Suspended:  true,
				Message:    "message",
			}, {
				RelationId: 456,
				Suspended:  true,
				Message:    "message",
			}},
	}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetRelationsSuspended", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.SetRelationSuspended(context.Background(), []int{123, 456}, true, "message")
	c.Assert(err, tc.ErrorMatches, "expected 2 results, got 1")
}

func (s *applicationSuite) TestAddRelation(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.AddRelation{
		Endpoints: []string{"ep1", "ep2"},
		ViaCIDRs:  []string{"cidr1", "cidr2"},
	}
	result := new(params.AddRelationResults)
	results := params.AddRelationResults{
		Endpoints: map[string]params.CharmRelation{
			"ep1": {Name: "foo"},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddRelation", args, result).SetArg(3, results).Return(nil)
	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.AddRelation(context.Background(), []string{"ep1", "ep2"}, []string{"cidr1", "cidr2"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Endpoints, jc.DeepEquals, map[string]params.CharmRelation{
		"ep1": {Name: "foo"},
	})
}

func (s *applicationSuite) TestGetConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ApplicationGetArgs{Args: []params.ApplicationGet{
		{ApplicationName: "foo"},
		{ApplicationName: "bar"},
	}}
	fooConfig := map[string]interface{}{
		"outlook": map[string]interface{}{
			"description": "No default outlook.",
			"source":      "unset",
			"type":        "string",
		},
		"skill-level": map[string]interface{}{
			"description": "A number indicating skill.",
			"source":      "user",
			"type":        "int",
			"value":       42,
		}}
	barConfig := map[string]interface{}{
		"title": map[string]interface{}{
			"default":     "My Title",
			"description": "A descriptive title used for the application.",
			"source":      "user",
			"type":        "string",
			"value":       "bar",
		},
		"username": map[string]interface{}{
			"default":     "admin001",
			"description": "The name of the initial account (given admin permissions).",
			"source":      "default",
			"type":        "string",
			"value":       "admin001",
		},
	}
	result := new(params.ApplicationGetConfigResults)
	results := params.ApplicationGetConfigResults{
		Results: []params.ConfigResult{
			{Config: fooConfig}, {Config: barConfig},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "CharmConfig", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.GetConfig(context.Background(), "foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, []map[string]interface{}{
		fooConfig, barConfig,
	})
}

func (s *applicationSuite) TestGetConstraints(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fooConstraints := constraints.MustParse("mem=4G")
	barConstraints := constraints.MustParse("mem=128G", "cores=64")
	result := new(params.ApplicationGetConstraintsResults)
	results := params.ApplicationGetConstraintsResults{
		Results: []params.ApplicationConstraint{
			{Constraints: fooConstraints}, {Constraints: barConstraints},
		},
	}
	args := params.Entities{
		Entities: []params.Entity{
			{"application-foo"}, {"application-bar"},
		}}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "GetConstraints", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.GetConstraints(context.Background(), "foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, []constraints.Value{
		fooConstraints, barConstraints,
	})
}

func (s *applicationSuite) TestGetConstraintsError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fooConstraints := constraints.MustParse("mem=4G")
	args := params.Entities{
		Entities: []params.Entity{
			{"application-foo"}, {"application-bar"},
		}}
	result := new(params.ApplicationGetConstraintsResults)
	results := params.ApplicationGetConstraintsResults{
		Results: []params.ApplicationConstraint{
			{Constraints: fooConstraints},
			{Error: &params.Error{Message: "oh no"}},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "GetConstraints", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.GetConstraints(context.Background(), "foo", "bar")
	c.Assert(err, tc.ErrorMatches, `unable to get constraints for "bar": oh no`)
	c.Assert(res, tc.IsNil)
}

func (s *applicationSuite) TestSetConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fooConfig := map[string]string{
		"foo":   "bar",
		"level": "high",
	}
	fooConfigYaml := "foo"
	args := params.ConfigSetArgs{
		Args: []params.ConfigSet{{
			ApplicationName: "foo",
			Config:          fooConfig,
			ConfigYAML:      fooConfigYaml,
		}}}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "FAIL"}},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetConfigs", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.SetConfig(context.Background(), "foo", fooConfigYaml, fooConfig)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *applicationSuite) TestUnsetApplicationConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ApplicationConfigUnsetArgs{
		Args: []params.ApplicationUnset{{
			ApplicationName: "foo",
			Options:         []string{"option"},
		}},
	}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "FAIL"}},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UnsetApplicationsConfig", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.UnsetApplicationConfig(context.Background(), "foo", []string{"option"})
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *applicationSuite) TestResolveUnitErrors(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UnitsResolved{
		Retry: true,
		Tags: params.Entities{
			Entities: []params.Entity{
				{Tag: "unit-mysql-0"},
				{Tag: "unit-mysql-1"},
			},
		},
	}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, 1),
	}
	units := []string{"mysql/0", "mysql/1"}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ResolveUnitErrors", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.ResolveUnitErrors(context.Background(), units, false, true)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestResolveUnitErrorsUnitsAll(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	units := []string{"mysql/0"}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.ResolveUnitErrors(context.Background(), units, true, false)
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Equals, "specifying units with all=true not supported")
}

func (s *applicationSuite) TestResolveUnitDuplicate(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	units := []string{"mysql/0", "mysql/1", "mysql/0"}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.ResolveUnitErrors(context.Background(), units, false, false)
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Equals, "duplicate unit specified")
}

func (s *applicationSuite) TestResolveUnitErrorsInvalidUnit(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	units := []string{"mysql"}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.ResolveUnitErrors(context.Background(), units, false, false)
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Equals, `unit name "mysql" not valid`)
}

func (s *applicationSuite) TestResolveUnitErrorsAll(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UnitsResolved{
		All: true,
	}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, 1),
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ResolveUnitErrors", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.ResolveUnitErrors(context.Background(), nil, true, false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestScaleApplication(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{
			{ApplicationTag: "application-foo", Scale: 5, Force: true},
		}}
	result := new(params.ScaleApplicationResults)
	results := params.ScaleApplicationResults{
		Results: []params.ScaleApplicationResult{
			{Info: &params.ScaleApplicationInfo{Scale: 5}},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ScaleApplications", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.ScaleApplication(context.Background(), application.ScaleApplicationParams{
		ApplicationName: "foo",
		Scale:           5,
		Force:           true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, params.ScaleApplicationResult{
		Info: &params.ScaleApplicationInfo{Scale: 5},
	})
}

func (s *applicationSuite) TestChangeScaleApplication(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{
			{ApplicationTag: "application-foo", ScaleChange: 5},
		}}
	result := new(params.ScaleApplicationResults)
	results := params.ScaleApplicationResults{
		Results: []params.ScaleApplicationResult{
			{Info: &params.ScaleApplicationInfo{Scale: 7}},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ScaleApplications", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.ScaleApplication(context.Background(), application.ScaleApplicationParams{
		ApplicationName: "foo",
		ScaleChange:     5,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, params.ScaleApplicationResult{
		Info: &params.ScaleApplicationInfo{Scale: 7},
	})
}

func (s *applicationSuite) TestScaleApplicationArity(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{
			{ApplicationTag: "application-foo", Scale: 5},
		}}
	result := new(params.ScaleApplicationResults)
	results := params.ScaleApplicationResults{
		Results: []params.ScaleApplicationResult{
			{Info: &params.ScaleApplicationInfo{Scale: 5}},
			{Info: &params.ScaleApplicationInfo{Scale: 3}},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ScaleApplications", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.ScaleApplication(context.Background(), application.ScaleApplicationParams{
		ApplicationName: "foo",
		Scale:           5,
	})
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
}

func (s *applicationSuite) TestScaleApplicationValidation(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	client := application.NewClientFromCaller(mockFacadeCaller)
	for i, test := range []struct {
		scale       int
		scaleChange int
		errorStr    string
	}{{
		scale:       5,
		scaleChange: 5,
		errorStr:    "requesting both scale and scale-change not valid",
	}, {
		scale:       -1,
		scaleChange: 0,
		errorStr:    "scale < 0 not valid",
	}} {
		c.Logf("test #%d", i)
		_, err := client.ScaleApplication(context.Background(), application.ScaleApplicationParams{
			ApplicationName: "foo",
			Scale:           test.scale,
			ScaleChange:     test.scaleChange,
		})
		c.Assert(err, tc.ErrorMatches, test.errorStr)
	}
}

func (s *applicationSuite) TestScaleApplicationError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	result := new(params.ScaleApplicationResults)
	results := params.ScaleApplicationResults{
		Results: []params.ScaleApplicationResult{
			{Error: &params.Error{Message: "boom"}},
		},
	}
	args := params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{
			{ApplicationTag: "application-foo", Scale: 5},
		}}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ScaleApplications", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.ScaleApplication(context.Background(), application.ScaleApplicationParams{
		ApplicationName: "foo",
		Scale:           5,
	})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *applicationSuite) TestScaleApplicationCallError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	result := new(params.ScaleApplicationResults)
	results := params.ScaleApplicationResults{}
	args := params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{
			{ApplicationTag: "application-foo", Scale: 5},
		}}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ScaleApplications", args, result).SetArg(3, results).Return(errors.New("boom"))

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.ScaleApplication(context.Background(), application.ScaleApplicationParams{
		ApplicationName: "foo",
		Scale:           5,
	})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *applicationSuite) TestApplicationsInfoCallError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{make([]params.Entity, 0)}
	result := new(params.ApplicationInfoResults)
	results := params.ApplicationInfoResults{make([]params.ApplicationInfoResult, 0)}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ApplicationsInfo", args, result).SetArg(3, results).Return(errors.New("boom"))

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.ApplicationsInfo(context.Background(), nil)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *applicationSuite) TestApplicationsInfo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "application-foo"},
			{Tag: "application-bar"},
		}}
	result := new(params.ApplicationInfoResults)
	results := params.ApplicationInfoResults{
		Results: []params.ApplicationInfoResult{
			{Error: &params.Error{Message: "boom"}},
			{Result: &params.ApplicationResult{
				Tag:       "application-bar",
				Charm:     "charm-bar",
				Base:      params.Base{Name: "ubuntu", Channel: "12.10"},
				Channel:   "development",
				Principal: true,
				EndpointBindings: map[string]string{
					relation.JujuInfo: "myspace",
				},
				Remote: true,
			},
			},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ApplicationsInfo", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.ApplicationsInfo(
		context.Background(),
		[]names.ApplicationTag{
			names.NewApplicationTag("foo"),
			names.NewApplicationTag("bar"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, []params.ApplicationInfoResult{
		{Error: &params.Error{Message: "boom"}},
		{Result: &params.ApplicationResult{
			Tag:       "application-bar",
			Charm:     "charm-bar",
			Base:      params.Base{Name: "ubuntu", Channel: "12.10"},
			Channel:   "development",
			Principal: true,
			EndpointBindings: map[string]string{
				relation.JujuInfo: "myspace",
			},
			Remote: true,
		}},
	})
}

func (s *applicationSuite) TestApplicationsInfoResultMismatch(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{[]params.Entity{
		{Tag: names.NewApplicationTag("foo").String()},
		{Tag: names.NewApplicationTag("bar").String()},
	}}
	result := new(params.ApplicationInfoResults)
	results := params.ApplicationInfoResults{
		Results: []params.ApplicationInfoResult{
			{Error: &params.Error{Message: "boom"}},
			{Error: &params.Error{Message: "boom again"}},
			{Result: &params.ApplicationResult{Tag: "application-bar"}},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ApplicationsInfo", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.ApplicationsInfo(
		context.Background(),
		[]names.ApplicationTag{
			names.NewApplicationTag("foo"),
			names.NewApplicationTag("bar"),
		},
	)
	c.Assert(err, tc.ErrorMatches, "expected 2 results, got 3")
}

func (s *applicationSuite) TestUnitsInfoCallError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{make([]params.Entity, 0)}
	result := new(params.UnitInfoResults)
	results := params.UnitInfoResults{}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UnitsInfo", args, result).SetArg(3, results).Return(errors.New("boom"))

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.UnitsInfo(context.Background(), nil)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *applicationSuite) TestUnitsInfo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "unit-foo-0"},
			{Tag: "unit-bar-1"},
		}}
	result := new(params.UnitInfoResults)
	results := params.UnitInfoResults{
		Results: []params.UnitInfoResult{
			{Error: &params.Error{Message: "boom"}},
			{Result: &params.UnitResult{
				Tag:             "unit-bar-1",
				WorkloadVersion: "666",
				Machine:         "1",
				OpenedPorts:     []string{"80"},
				PublicAddress:   "10.0.0.1",
				Charm:           "charm-bar",
				Leader:          true,
				RelationData: []params.EndpointRelationData{{
					Endpoint:        "db",
					CrossModel:      true,
					RelatedEndpoint: "server",
					ApplicationData: map[string]interface{}{"foo": "bar"},
					UnitRelationData: map[string]params.RelationData{
						"baz": {
							InScope:  true,
							UnitData: map[string]interface{}{"hello": "world"},
						},
					},
				}},
				ProviderId: "provider-id",
				Address:    "192.168.1.1",
			}},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UnitsInfo", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.UnitsInfo(
		context.Background(),
		[]names.UnitTag{
			names.NewUnitTag("foo/0"),
			names.NewUnitTag("bar/1"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, []application.UnitInfo{
		{Error: stderrors.New("boom")},
		{
			Tag:             "unit-bar-1",
			WorkloadVersion: "666",
			Machine:         "1",
			OpenedPorts:     []string{"80"},
			PublicAddress:   "10.0.0.1",
			Charm:           "charm-bar",
			Leader:          true,
			RelationData: []application.EndpointRelationData{{
				Endpoint:        "db",
				CrossModel:      true,
				RelatedEndpoint: "server",
				ApplicationData: map[string]interface{}{"foo": "bar"},
				UnitRelationData: map[string]application.RelationData{
					"baz": {
						InScope:  true,
						UnitData: map[string]interface{}{"hello": "world"},
					},
				},
			}},
			ProviderId: "provider-id",
			Address:    "192.168.1.1",
		},
	})
}

func (s *applicationSuite) TestUnitsInfoResultMismatch(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "unit-foo-0"},
			{Tag: "unit-bar-1"},
		}}
	result := new(params.UnitInfoResults)
	results := params.UnitInfoResults{Results: []params.UnitInfoResult{
		{}, {}, {},
	}}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UnitsInfo", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.UnitsInfo(
		context.Background(),
		[]names.UnitTag{
			names.NewUnitTag("foo/0"),
			names.NewUnitTag("bar/1"),
		},
	)
	c.Assert(err, tc.ErrorMatches, "expected 2 results, got 3")
}

func (s *applicationSuite) TestExpose(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ApplicationExpose{
		ApplicationName: "foo",
		ExposedEndpoints: map[string]params.ExposedEndpoint{
			"": {
				ExposeToCIDRs: []string{"0.0.0.0/0"},
			},
			"foo": {
				ExposeToSpaces: []string{"outer"},
			},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Expose", args, nil).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.Expose(
		context.Background(),
		"foo", map[string]params.ExposedEndpoint{
			"": {
				ExposeToCIDRs: []string{"0.0.0.0/0"},
			},
			"foo": {
				ExposeToSpaces: []string{"outer"},
			},
		})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestUnexpose(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ApplicationUnexpose{
		ApplicationName:  "foo",
		ExposedEndpoints: []string{"foo"},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Unexpose", args, nil).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.Unexpose(context.Background(), "foo", []string{"foo"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestLeader(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entity{Tag: names.NewApplicationTag("ubuntu").String()}
	result := new(params.StringResult)
	results := params.StringResult{Result: "ubuntu/42"}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Leader", args, result).SetArg(3, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	obtainedUnit, err := client.Leader(context.Background(), "ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedUnit, tc.Equals, "ubuntu/42")
}

func (s *applicationSuite) TestDeployFromRepository(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.DeployFromRepositoryArgs{
		Args: []params.DeployFromRepositoryArg{{
			ApplicationName: "jammy",
			CharmName:       "ubuntu",
			Base: &params.Base{
				Name:    "ubuntu",
				Channel: "22.04",
			},
		}},
	}
	stable := "stable"
	candidate := "candidate"
	result := new(params.DeployFromRepositoryResults)
	results := params.DeployFromRepositoryResults{
		Results: []params.DeployFromRepositoryResult{{
			Errors: []*params.Error{
				{Message: "one"},
				{Message: "two"},
				{Message: "three"},
			},
			Info: params.DeployFromRepositoryInfo{
				Channel:      candidate,
				Architecture: "arm64",
				Base: params.Base{
					Name:    "ubuntu",
					Channel: "22.04",
				},
				EffectiveChannel: &stable,
				Name:             "ubuntu",
				Revision:         7,
			},
			PendingResourceUploads: nil,
		}},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DeployFromRepository", args, result).SetArg(3, results).Return(nil)

	arg := application.DeployFromRepositoryArg{
		CharmName:       "ubuntu",
		ApplicationName: "jammy",
		Base:            &corebase.Base{OS: "ubuntu", Channel: corebase.Channel{Track: "22.04"}},
	}
	client := application.NewClientFromCaller(mockFacadeCaller)
	info, _, errs := client.DeployFromRepository(context.Background(), arg)
	c.Assert(errs, tc.HasLen, 3)
	c.Assert(errs[0], tc.ErrorMatches, "one")
	c.Assert(errs[1], tc.ErrorMatches, "two")
	c.Assert(errs[2], tc.ErrorMatches, "three")

	c.Assert(info, tc.DeepEquals, application.DeployInfo{
		Channel:      candidate,
		Architecture: "arm64",
		Base: corebase.Base{
			OS:      "ubuntu",
			Channel: corebase.Channel{Track: "22.04", Risk: "stable"},
		},
		EffectiveChannel: &stable,
		Name:             "ubuntu",
		Revision:         7,
	})

}
