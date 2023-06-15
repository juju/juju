// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	stderrors "errors"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/application"
	apicharm "github.com/juju/juju/api/common/charm"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

const newBranchName = "new-branch"

type applicationSuite struct{}

var _ = gc.Suite(&applicationSuite{})

func (s *applicationSuite) TestDeploy(c *gc.C) {
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
				Placement:        []*instance.Placement{{"scope", "directive"}},
				EndpointBindings: map[string]string{"foo": "bar"},
				Storage:          map[string]storage.Constraints{"data": {Pool: "pool"}},
				AttachStorage:    []string{"storage-data-0"},
				Resources:        map[string]string{"foo": "bar"},
			},
		},
	}

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Deploy", deployArgs, result).SetArg(2, results).Return(nil)

	args := application.DeployArgs{
		CharmID: application.CharmID{
			URL: charm.MustParseURL("ch:a-charm-1"),
		},
		CharmOrigin: apicharm.Origin{
			Source: apicharm.OriginCharmHub,
		},
		ApplicationName:  "applicationA",
		NumUnits:         1,
		ConfigYAML:       "configYAML",
		Config:           map[string]string{"foo": "bar"},
		Cons:             constraints.MustParse("mem=4G"),
		Placement:        []*instance.Placement{{"scope", "directive"}},
		Storage:          map[string]storage.Constraints{"data": {Pool: "pool"}},
		AttachStorage:    []string{"data/0"},
		Resources:        map[string]string{"foo": "bar"},
		EndpointBindings: map[string]string{"foo": "bar"},
	}

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestDeployAlreadyExists(c *gc.C) {
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
				Placement:        []*instance.Placement{{"scope", "directive"}},
				EndpointBindings: map[string]string{"foo": "bar"},
				Storage:          map[string]storage.Constraints{"data": {Pool: "pool"}},
				AttachStorage:    []string{"storage-data-0"},
				Resources:        map[string]string{"foo": "bar"},
			},
		},
	}

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Deploy", deployArgs, result).SetArg(2, results).Return(nil)

	args := application.DeployArgs{
		CharmID: application.CharmID{
			URL: charm.MustParseURL("ch:a-charm-1"),
		},
		CharmOrigin: apicharm.Origin{
			Source: apicharm.OriginCharmHub,
		},
		ApplicationName:  "applicationA",
		NumUnits:         1,
		ConfigYAML:       "configYAML",
		Config:           map[string]string{"foo": "bar"},
		Cons:             constraints.MustParse("mem=4G"),
		Placement:        []*instance.Placement{{"scope", "directive"}},
		Storage:          map[string]storage.Constraints{"data": {Pool: "pool"}},
		AttachStorage:    []string{"data/0"},
		Resources:        map[string]string{"foo": "bar"},
		EndpointBindings: map[string]string{"foo": "bar"},
	}

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.Deploy(args)
	c.Assert(err, gc.ErrorMatches, `application already exists`)
}

func (s *applicationSuite) TestDeployAttachStorageMultipleUnits(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)

	args := application.DeployArgs{
		NumUnits:      2,
		AttachStorage: []string{"data/0"},
	}
	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.Deploy(args)
	c.Assert(err, gc.ErrorMatches, "cannot attach existing storage when more than one unit is requested")
}

func (s *applicationSuite) TestAddUnits(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	result := new(params.AddApplicationUnitsResults)
	results := params.AddApplicationUnitsResults{
		Units: []string{"foo/0"},
	}

	args := params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        1,
		Placement:       []*instance.Placement{{"scope", "directive"}},
		AttachStorage:   []string{"storage-data-0"},
	}

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("AddUnits", args, result).SetArg(2, results).Return(nil)
	client := application.NewClientFromCaller(mockFacadeCaller)
	units, err := client.AddUnits(application.AddUnitsParams{
		ApplicationName: "foo",
		NumUnits:        1,
		Placement:       []*instance.Placement{{"scope", "directive"}},
		AttachStorage:   []string{"data/0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, jc.DeepEquals, []string{"foo/0"})
}

func (s *applicationSuite) TestAddUnitsAttachStorageMultipleUnits(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.AddUnits(application.AddUnitsParams{
		NumUnits:      2,
		AttachStorage: []string{"data/0"},
	})
	c.Assert(err, gc.ErrorMatches, "cannot attach existing storage when more than one unit is requested")
}

func (s *applicationSuite) TestApplicationGetCharmURLOrigin(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	result := new(params.CharmURLOriginResult)
	results := params.CharmURLOriginResult{
		URL:    "ch:curl",
		Origin: params.CharmOrigin{Risk: "edge"},
	}
	args := params.ApplicationGet{
		ApplicationName: "application",
		BranchName:      newBranchName,
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("GetCharmURLOrigin", args, result).SetArg(2, results).Return(nil)
	client := application.NewClientFromCaller(mockFacadeCaller)

	curl, origin, err := client.GetCharmURLOrigin(newBranchName, "application")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.DeepEquals, charm.MustParseURL("ch:curl"))
	c.Assert(origin, gc.DeepEquals, apicharm.Origin{
		Risk: "edge",
	})
}

func (s *applicationSuite) TestSetCharm(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	toUint64Ptr := func(v uint64) *uint64 {
		return &v
	}

	args := params.ApplicationSetCharm{
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
		StorageConstraints: map[string]params.StorageConstraints{
			"a": {Pool: "radiant"},
			"b": {Count: toUint64Ptr(123)},
			"c": {Size: toUint64Ptr(123)},
		},
		Generation: newBranchName,
	}

	c.Assert(args.ConfigSettingsYAML, gc.Equals, "yaml")
	c.Assert(args.Force, gc.Equals, true)
	c.Assert(args.ForceBase, gc.Equals, true)
	c.Assert(args.ForceUnits, gc.Equals, true)
	c.Assert(args.StorageConstraints, jc.DeepEquals, map[string]params.StorageConstraints{
		"a": {Pool: "radiant"},
		"b": {Count: toUint64Ptr(123)},
		"c": {Size: toUint64Ptr(123)},
	})

	cfg := application.SetCharmConfig{
		ApplicationName: "application",
		CharmID: application.CharmID{
			URL: charm.MustParseURL("ch:application-1"),
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
		StorageConstraints: map[string]storage.Constraints{
			"a": {Pool: "radiant"},
			"b": {Count: 123},
			"c": {Size: 123},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SetCharm", args, nil).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.SetCharm(newBranchName, cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestDestroyApplications(c *gc.C) {
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
	results := params.DestroyApplicationResults{expectedResults}
	args := params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{
			{ApplicationTag: "application-foo", Force: true, MaxWait: &delay},
			{ApplicationTag: "application-bar", Force: true, MaxWait: &delay},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("DestroyApplication", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.DestroyApplications(application.DestroyApplicationsParams{
		Applications: []string{"foo", "bar"},
		Force:        true,
		MaxWait:      &delay,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestDestroyApplicationsArity(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("DestroyApplication", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.DestroyApplications(application.DestroyApplicationsParams{
		Applications: []string{"foo"},
	})
	c.Assert(err, gc.ErrorMatches, `expected 1 result\(s\), got 0`)
}

func (s *applicationSuite) TestDestroyApplicationsInvalidIds(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("DestroyApplication", args, result).SetArg(2, results).Return(nil)
	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.DestroyApplications(application.DestroyApplicationsParams{
		Applications: []string{"!", "foo"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestDestroyConsumedApplicationsArity(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("DestroyConsumedApplications", args, result).SetArg(2, results).Return(nil)
	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.DestroyConsumedApplication(destroyParams)
	c.Assert(err, gc.ErrorMatches, `expected 1 result\(s\), got 0`)
}

func (s *applicationSuite) TestDestroyConsumedApplications(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("DestroyConsumedApplications", args, result).SetArg(2, results).Return(nil)
	destroyParams := application.DestroyConsumedApplicationParams{
		[]string{"foo"}, false, &noWait,
	}
	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.DestroyConsumedApplication(destroyParams)
	c.Check(err, gc.ErrorMatches, "--force is required when --max-wait is provided")
	c.Check(res, gc.HasLen, 0)

	destroyParams = application.DestroyConsumedApplicationParams{
		[]string{"foo", "bar"}, force, &noWait,
	}
	res, err = client.DestroyConsumedApplication(destroyParams)
	c.Check(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 2)
}

func (s *applicationSuite) TestDestroyUnits(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("DestroyUnit", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.DestroyUnits(application.DestroyUnitsParams{
		Units:   []string{"foo/0", "bar/1"},
		Force:   true,
		MaxWait: &delay,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestDestroyUnitsArity(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("DestroyUnit", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.DestroyUnits(application.DestroyUnitsParams{
		Units: []string{"foo/0"},
	})
	c.Assert(err, gc.ErrorMatches, `expected 1 result\(s\), got 0`)
}

func (s *applicationSuite) TestDestroyUnitsInvalidIds(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("DestroyUnit", args, result).SetArg(2, results).Return(nil)
	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.DestroyUnits(application.DestroyUnitsParams{
		Units: []string{"!", "foo/0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestConsume(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	offer := params.ApplicationOfferDetails{
		SourceModelTag:         "source model",
		OfferName:              "an offer",
		OfferUUID:              "offer-uuid",
		OfferURL:               "offer url",
		ApplicationDescription: "description",
		Endpoints:              []params.RemoteEndpoint{{Name: "endpoint"}},
	}
	mac, err := apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	controllerInfo := &params.ExternalControllerInfo{
		ControllerTag: coretesting.ControllerTag.String(),
		Alias:         "controller-alias",
		Addrs:         []string{"192.168.1.0"},
		CACert:        coretesting.CACert,
	}

	result := new(params.ErrorResults)
	results := params.ErrorResults{Results: []params.ErrorResult{{}}}
	args := params.ConsumeApplicationArgs{
		Args: []params.ConsumeApplicationArg{
			{
				ApplicationAlias:        "alias",
				ApplicationOfferDetails: offer,
				Macaroon:                mac,
				AuthToken:               "auth-token",
				ControllerInfo:          controllerInfo,
			},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Consume", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	name, err := client.Consume(crossmodel.ConsumeApplicationArgs{
		Offer:            offer,
		ApplicationAlias: "alias",
		Macaroon:         mac,
		AuthToken:        "auth-token",
		ControllerInfo: &crossmodel.ControllerInfo{
			ControllerTag: coretesting.ControllerTag,
			Alias:         "controller-alias",
			Addrs:         controllerInfo.Addrs,
			CACert:        controllerInfo.CACert,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "alias")
}

func (s *applicationSuite) TestDestroyRelation(c *gc.C) {
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
		mockFacadeCaller.EXPECT().FacadeCall("DestroyRelation", args, nil).Return(nil)

		client := application.NewClientFromCaller(mockFacadeCaller)
		err := client.DestroyRelation(t.force, t.maxWait, "ep1", "ep2")
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *applicationSuite) TestDestroyRelationId(c *gc.C) {
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
		mockFacadeCaller.EXPECT().FacadeCall("DestroyRelation", args, nil).Return(nil)

		client := application.NewClientFromCaller(mockFacadeCaller)
		err := client.DestroyRelationId(123, t.force, t.maxWait)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *applicationSuite) TestSetRelationSuspended(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("SetRelationsSuspended", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.SetRelationSuspended([]int{123, 456}, true, "message")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestSetRelationSuspendedArity(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("SetRelationsSuspended", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.SetRelationSuspended([]int{123, 456}, true, "message")
	c.Assert(err, gc.ErrorMatches, "expected 2 results, got 1")
}

func (s *applicationSuite) TestAddRelation(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("AddRelation", args, result).SetArg(2, results).Return(nil)
	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.AddRelation([]string{"ep1", "ep2"}, []string{"cidr1", "cidr2"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Endpoints, jc.DeepEquals, map[string]params.CharmRelation{
		"ep1": {Name: "foo"},
	})
}

func (s *applicationSuite) TestGetConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ApplicationGetArgs{Args: []params.ApplicationGet{
		{ApplicationName: "foo", BranchName: newBranchName},
		{ApplicationName: "bar", BranchName: newBranchName},
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
	mockFacadeCaller.EXPECT().FacadeCall("CharmConfig", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.GetConfig(newBranchName, "foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, []map[string]interface{}{
		fooConfig, barConfig,
	})
}

func (s *applicationSuite) TestGetConstraints(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("GetConstraints", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.GetConstraints("foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, []constraints.Value{
		fooConstraints, barConstraints,
	})
}

func (s *applicationSuite) TestGetConstraintsError(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("GetConstraints", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.GetConstraints("foo", "bar")
	c.Assert(err, gc.ErrorMatches, `unable to get constraints for "bar": oh no`)
	c.Assert(res, gc.IsNil)
}

func (s *applicationSuite) TestSetConfig(c *gc.C) {
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
			Generation:      newBranchName,
		}}}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "FAIL"}},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SetConfigs", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.SetConfig(newBranchName, "foo", fooConfigYaml, fooConfig)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *applicationSuite) TestUnsetApplicationConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ApplicationConfigUnsetArgs{
		Args: []params.ApplicationUnset{{
			ApplicationName: "foo",
			Options:         []string{"option"},
			BranchName:      newBranchName,
		}},
	}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "FAIL"}},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("UnsetApplicationsConfig", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.UnsetApplicationConfig(newBranchName, "foo", []string{"option"})
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *applicationSuite) TestResolveUnitErrors(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("ResolveUnitErrors", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.ResolveUnitErrors(units, false, true)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestResolveUnitErrorsUnitsAll(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	units := []string{"mysql/0"}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.ResolveUnitErrors(units, true, false)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, "specifying units with all=true not supported")
}

func (s *applicationSuite) TestResolveUnitDuplicate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	units := []string{"mysql/0", "mysql/1", "mysql/0"}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.ResolveUnitErrors(units, false, false)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, "duplicate unit specified")
}

func (s *applicationSuite) TestResolveUnitErrorsInvalidUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	units := []string{"mysql"}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.ResolveUnitErrors(units, false, false)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `unit name "mysql" not valid`)
}

func (s *applicationSuite) TestResolveUnitErrorsAll(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("ResolveUnitErrors", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.ResolveUnitErrors(nil, true, false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestScaleApplication(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("ScaleApplications", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.ScaleApplication(application.ScaleApplicationParams{
		ApplicationName: "foo",
		Scale:           5,
		Force:           true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, params.ScaleApplicationResult{
		Info: &params.ScaleApplicationInfo{Scale: 5},
	})
}

func (s *applicationSuite) TestChangeScaleApplication(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("ScaleApplications", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.ScaleApplication(application.ScaleApplicationParams{
		ApplicationName: "foo",
		ScaleChange:     5,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, params.ScaleApplicationResult{
		Info: &params.ScaleApplicationInfo{Scale: 7},
	})
}

func (s *applicationSuite) TestScaleApplicationArity(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("ScaleApplications", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.ScaleApplication(application.ScaleApplicationParams{
		ApplicationName: "foo",
		Scale:           5,
	})
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *applicationSuite) TestScaleApplicationValidation(c *gc.C) {
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
		_, err := client.ScaleApplication(application.ScaleApplicationParams{
			ApplicationName: "foo",
			Scale:           test.scale,
			ScaleChange:     test.scaleChange,
		})
		c.Assert(err, gc.ErrorMatches, test.errorStr)
	}
}

func (s *applicationSuite) TestScaleApplicationError(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("ScaleApplications", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.ScaleApplication(application.ScaleApplicationParams{
		ApplicationName: "foo",
		Scale:           5,
	})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *applicationSuite) TestScaleApplicationCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	result := new(params.ScaleApplicationResults)
	results := params.ScaleApplicationResults{}
	args := params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{
			{ApplicationTag: "application-foo", Scale: 5},
		}}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ScaleApplications", args, result).SetArg(2, results).Return(errors.New("boom"))

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.ScaleApplication(application.ScaleApplicationParams{
		ApplicationName: "foo",
		Scale:           5,
	})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *applicationSuite) TestApplicationsInfoCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{make([]params.Entity, 0)}
	result := new(params.ApplicationInfoResults)
	results := params.ApplicationInfoResults{make([]params.ApplicationInfoResult, 0)}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ApplicationsInfo", args, result).SetArg(2, results).Return(errors.New("boom"))

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.ApplicationsInfo(nil)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *applicationSuite) TestApplicationsInfo(c *gc.C) {
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
					"juju-info": "myspace",
				},
				Remote: true,
			},
			},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ApplicationsInfo", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.ApplicationsInfo(
		[]names.ApplicationTag{
			names.NewApplicationTag("foo"),
			names.NewApplicationTag("bar"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, []params.ApplicationInfoResult{
		{Error: &params.Error{Message: "boom"}},
		{Result: &params.ApplicationResult{
			Tag:       "application-bar",
			Charm:     "charm-bar",
			Base:      params.Base{Name: "ubuntu", Channel: "12.10"},
			Channel:   "development",
			Principal: true,
			EndpointBindings: map[string]string{
				"juju-info": "myspace",
			},
			Remote: true,
		}},
	})
}

func (s *applicationSuite) TestApplicationsInfoResultMismatch(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("ApplicationsInfo", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.ApplicationsInfo(
		[]names.ApplicationTag{
			names.NewApplicationTag("foo"),
			names.NewApplicationTag("bar"),
		},
	)
	c.Assert(err, gc.ErrorMatches, "expected 2 results, got 3")
}

func (s *applicationSuite) TestUnitsInfoCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{make([]params.Entity, 0)}
	result := new(params.UnitInfoResults)
	results := params.UnitInfoResults{}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("UnitsInfo", args, result).SetArg(2, results).Return(errors.New("boom"))

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.UnitsInfo(nil)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *applicationSuite) TestUnitsInfo(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("UnitsInfo", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	res, err := client.UnitsInfo(
		[]names.UnitTag{
			names.NewUnitTag("foo/0"),
			names.NewUnitTag("bar/1"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, []application.UnitInfo{
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

func (s *applicationSuite) TestUnitsInfoResultMismatch(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("UnitsInfo", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	_, err := client.UnitsInfo(
		[]names.UnitTag{
			names.NewUnitTag("foo/0"),
			names.NewUnitTag("bar/1"),
		},
	)
	c.Assert(err, gc.ErrorMatches, "expected 2 results, got 3")
}

func (s *applicationSuite) TestExpose(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("Expose", args, nil).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.Expose("foo", map[string]params.ExposedEndpoint{
		"": {
			ExposeToCIDRs: []string{"0.0.0.0/0"},
		},
		"foo": {
			ExposeToSpaces: []string{"outer"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestUnexpose(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ApplicationUnexpose{
		ApplicationName:  "foo",
		ExposedEndpoints: []string{"foo"},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Unexpose", args, nil).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	err := client.Unexpose("foo", []string{"foo"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestLeader(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entity{Tag: names.NewApplicationTag("ubuntu").String()}
	result := new(params.StringResult)
	results := params.StringResult{Result: "ubuntu/42"}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Leader", args, result).SetArg(2, results).Return(nil)

	client := application.NewClientFromCaller(mockFacadeCaller)
	obtainedUnit, err := client.Leader("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedUnit, gc.Equals, "ubuntu/42")
}

func (s *applicationSuite) TestDeployFromRepository(c *gc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall("DeployFromRepository", args, result).SetArg(2, results).Return(nil)

	arg := application.DeployFromRepositoryArg{
		CharmName:       "ubuntu",
		ApplicationName: "jammy",
		Base:            &series.Base{OS: "ubuntu", Channel: series.Channel{Track: "22.04"}},
	}
	client := application.NewClientFromCaller(mockFacadeCaller)
	info, _, errs := client.DeployFromRepository(arg)
	c.Assert(errs, gc.HasLen, 3)
	c.Assert(errs[0], gc.ErrorMatches, "one")
	c.Assert(errs[1], gc.ErrorMatches, "two")
	c.Assert(errs[2], gc.ErrorMatches, "three")

	c.Assert(info, gc.DeepEquals, application.DeployInfo{
		Channel:      candidate,
		Architecture: "arm64",
		Base: series.Base{
			OS:      "ubuntu",
			Channel: series.Channel{Track: "22.04", Risk: "stable"},
		},
		EffectiveChannel: &stable,
		Name:             "ubuntu",
		Revision:         7,
	})

}
