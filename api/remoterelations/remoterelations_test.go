// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/remoterelations"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&remoteRelationsSuite{})

type remoteRelationsSuite struct {
	coretesting.BaseSuite
}

func (s *remoteRelationsSuite) TestNewClient(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func (s *remoteRelationsSuite) TestWatchRemoteApplications(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchRemoteApplications")
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.WatchRemoteApplications()
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestWatchRemoteApplicationRelations(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchRemoteApplicationRelations")
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.WatchRemoteApplicationRelations("db2")
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestWatchRemoteApplicationInvalidApplication(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.WatchRemoteApplicationRelations("!@#")
	c.Assert(err, gc.ErrorMatches, `application name "!@#" not valid`)
}

func (s *remoteRelationsSuite) TestWatchLocalRelationUnits(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchLocalRelationUnits")
		c.Assert(result, gc.FitsTypeOf, &params.RelationUnitsWatchResults{})
		*(result.(*params.RelationUnitsWatchResults)) = params.RelationUnitsWatchResults{
			Results: []params.RelationUnitsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.WatchLocalRelationUnits("relation-wordpress:db mysql:db")
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestPublishLocalRelationChange(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "PublishLocalRelationChange")
		c.Check(arg, gc.DeepEquals, params.RemoteRelationsChanges{
			Changes: []params.RemoteRelationChangeEvent{{
				DepartedUnits: []int{1}}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	err := client.PublishLocalRelationChange(params.RemoteRelationChangeEvent{DepartedUnits: []int{1}})
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestExportEntities(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ExportEntities")
		c.Check(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-foo"}}})
		c.Assert(result, gc.FitsTypeOf, &params.RemoteEntityIdResults{})
		*(result.(*params.RemoteEntityIdResults)) = params.RemoteEntityIdResults{
			Results: []params.RemoteEntityIdResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	result, err := client.ExportEntities([]names.Tag{names.NewApplicationTag("foo")})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestExportEntitiesResultCount(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.RemoteEntityIdResults)) = params.RemoteEntityIdResults{
			Results: []params.RemoteEntityIdResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.ExportEntities([]names.Tag{names.NewApplicationTag("foo")})
	c.Check(err, gc.ErrorMatches, `expected 1 result\(s\), got 2`)
}

func (s *remoteRelationsSuite) TestRelationUnitSettings(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RelationUnitSettings")
		c.Check(arg, gc.DeepEquals, params.RelationUnits{RelationUnits: []params.RelationUnit{{Relation: "r", Unit: "u"}}})
		c.Assert(result, gc.FitsTypeOf, &params.SettingsResults{})
		*(result.(*params.SettingsResults)) = params.SettingsResults{
			Results: []params.SettingsResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	result, err := client.RelationUnitSettings([]params.RelationUnit{{Relation: "r", Unit: "u"}})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestRelationUnitSettingsResultsCount(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.SettingsResults)) = params.SettingsResults{
			Results: []params.SettingsResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.RelationUnitSettings([]params.RelationUnit{{Relation: "r", Unit: "u"}})
	c.Check(err, gc.ErrorMatches, `expected 1 result\(s\), got 2`)
}

func (s *remoteRelationsSuite) TestRelations(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Relations")
		c.Check(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "relation-foo.db#bar.db"}}})
		c.Assert(result, gc.FitsTypeOf, &params.RemoteRelationResults{})
		*(result.(*params.RemoteRelationResults)) = params.RemoteRelationResults{
			Results: []params.RemoteRelationResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	result, err := client.Relations([]string{"foo:db bar:db"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestRelationsResultsCount(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.RemoteRelationResults)) = params.RemoteRelationResults{
			Results: []params.RemoteRelationResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.Relations([]string{"foo:db bar:db"})
	c.Check(err, gc.ErrorMatches, `expected 1 result\(s\), got 2`)
}

func (s *remoteRelationsSuite) TestRemoteApplications(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RemoteApplications")
		c.Check(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-foo"}}})
		c.Assert(result, gc.FitsTypeOf, &params.RemoteApplicationResults{})
		*(result.(*params.RemoteApplicationResults)) = params.RemoteApplicationResults{
			Results: []params.RemoteApplicationResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	result, err := client.RemoteApplications([]string{"foo"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestRemoteApplicationsResultsCount(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.RemoteApplicationResults)) = params.RemoteApplicationResults{
			Results: []params.RemoteApplicationResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.RemoteApplications([]string{"foo"})
	c.Check(err, gc.ErrorMatches, `expected 1 result\(s\), got 2`)
}

func (s *remoteRelationsSuite) TestRegisterRemoteRelations(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RegisterRemoteRelations")
		c.Check(arg, gc.DeepEquals, params.RegisterRemoteRelations{
			Relations: []params.RegisterRemoteRelation{{OfferedApplicationName: "offeredapp"}}})
		c.Assert(result, gc.FitsTypeOf, &params.RemoteEntityIdResults{})
		*(result.(*params.RemoteEntityIdResults)) = params.RemoteEntityIdResults{
			Results: []params.RemoteEntityIdResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	result, err := client.RegisterRemoteRelations(params.RegisterRemoteRelation{OfferedApplicationName: "offeredapp"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestRegisterRemoteRelationCount(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.RemoteEntityIdResults)) = params.RemoteEntityIdResults{
			Results: []params.RemoteEntityIdResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.RegisterRemoteRelations(params.RegisterRemoteRelation{})
	c.Check(err, gc.ErrorMatches, `expected 1 result\(s\), got 2`)
}

func (s *remoteRelationsSuite) TestGetToken(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetTokens")
		c.Check(arg, gc.DeepEquals, params.GetTokenArgs{
			Args: []params.GetTokenArg{{ModelTag: coretesting.ModelTag.String(), Tag: "application-app"}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.GetToken(coretesting.ModelTag.Id(), names.NewApplicationTag("app"))
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestGetTokenCount(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.GetToken(coretesting.ModelTag.Id(), names.NewApplicationTag("app"))
	c.Check(err, gc.ErrorMatches, `expected 1 result, got 2`)
}

func (s *remoteRelationsSuite) TestImportRemoteEntity(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ImportRemoteEntities")
		c.Check(arg, gc.DeepEquals, params.ImportEntityArgs{
			Args: []params.ImportEntityArg{{ModelTag: coretesting.ModelTag.String(), Tag: "application-app", Token: "token"}}})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	err := client.ImportRemoteEntity(coretesting.ModelTag.Id(), names.NewApplicationTag("app"), "token")
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestImportRemoteEntityCount(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	err := client.ImportRemoteEntity(coretesting.ModelTag.Id(), names.NewApplicationTag("app"), "token")
	c.Check(err, gc.ErrorMatches, `expected 1 result, got 2`)
}

func (s *remoteRelationsSuite) TestWatchRemoteRelations(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchRemoteRelations")
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.WatchRemoteRelations()
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}
