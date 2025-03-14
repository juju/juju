// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"context"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/remoterelations"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
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
	c.Skip("Re-enable this test whenever CMR will be fully implemented and the related watcher rewired.")
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
	_, err := client.WatchRemoteApplications(context.Background())
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestWatchRemoteApplicationRelations(c *gc.C) {
	c.Skip("Re-enable this test whenever CMR will be fully implemented and the related watcher rewired.")
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
	_, err := client.WatchRemoteApplicationRelations(context.Background(), "db2")
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestWatchRemoteApplicationInvalidApplication(c *gc.C) {
	c.Skip("Re-enable this test whenever CMR will be fully implemented and the related watcher rewired.")
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.WatchRemoteApplicationRelations(context.Background(), "!@#")
	c.Assert(err, gc.ErrorMatches, `application name "!@#" not valid`)
}

func (s *remoteRelationsSuite) TestWatchLocalRelationChanges(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchLocalRelationChanges")
		c.Assert(result, gc.FitsTypeOf, &params.RemoteRelationWatchResults{})
		*(result.(*params.RemoteRelationWatchResults)) = params.RemoteRelationWatchResults{
			Results: []params.RemoteRelationWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.WatchLocalRelationChanges(context.Background(), "relation-wordpress:db mysql:db")
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
		c.Assert(result, gc.FitsTypeOf, &params.TokenResults{})
		*(result.(*params.TokenResults)) = params.TokenResults{
			Results: []params.TokenResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	result, err := client.ExportEntities(context.Background(), []names.Tag{names.NewApplicationTag("foo")})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestExportEntitiesResultCount(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.TokenResults)) = params.TokenResults{
			Results: []params.TokenResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.ExportEntities(context.Background(), []names.Tag{names.NewApplicationTag("foo")})
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
	result, err := client.Relations(context.Background(), []string{"foo:db bar:db"})
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
	_, err := client.Relations(context.Background(), []string{"foo:db bar:db"})
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
	result, err := client.RemoteApplications(context.Background(), []string{"foo"})
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
	_, err := client.RemoteApplications(context.Background(), []string{"foo"})
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
			Args: []params.GetTokenArg{{Tag: "application-app"}}})
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
	_, err := client.GetToken(context.Background(), names.NewApplicationTag("app"))
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
	_, err := client.GetToken(context.Background(), names.NewApplicationTag("app"))
	c.Check(err, gc.ErrorMatches, `expected 1 result, got 2`)
}

func (s *remoteRelationsSuite) TestImportRemoteEntity(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ImportRemoteEntities")
		c.Check(arg, gc.DeepEquals, params.RemoteEntityTokenArgs{
			Args: []params.RemoteEntityTokenArg{{Tag: "application-app", Token: "token"}}})
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
	err := client.ImportRemoteEntity(context.Background(), names.NewApplicationTag("app"), "token")
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
	err := client.ImportRemoteEntity(context.Background(), names.NewApplicationTag("app"), "token")
	c.Check(err, gc.ErrorMatches, `expected 1 result, got 2`)
}

func (s *remoteRelationsSuite) TestWatchRemoteRelations(c *gc.C) {
	c.Skip("Re-enable this test whenever CMR will be fully implemented and the related watcher rewired.")
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
	_, err := client.WatchRemoteRelations(context.Background())
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestConsumeRemoteRelationChange(c *gc.C) {
	var callCount int
	change := params.RemoteRelationChangeEvent{}
	changes := params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{change},
	}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ConsumeRemoteRelationChanges")
		c.Check(arg, jc.DeepEquals, changes)
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}}}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	err := client.ConsumeRemoteRelationChange(context.Background(), change)
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestControllerAPIInfoForModel(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ControllerAPIInfoForModels")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: coretesting.ModelTag.String()}}})
		c.Assert(result, gc.FitsTypeOf, &params.ControllerAPIInfoResults{})
		*(result.(*params.ControllerAPIInfoResults)) = params.ControllerAPIInfoResults{
			Results: []params.ControllerAPIInfoResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := remoterelations.NewClient(apiCaller)
	_, err := client.ControllerAPIInfoForModel(context.Background(), coretesting.ModelTag.Id())
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestSaveMacaroon(c *gc.C) {
	rel := names.NewRelationTag("mysql:db wordpress:db")
	mac, err := jujutesting.NewMacaroon("id")
	c.Check(err, jc.ErrorIsNil)
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SaveMacaroons")
		c.Assert(arg, gc.DeepEquals, params.EntityMacaroonArgs{Args: []params.EntityMacaroonArg{
			{Tag: rel.String(), Macaroon: mac}}})
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
	err = client.SaveMacaroon(context.Background(), rel, mac)
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestSetRemoteApplicationStatus(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetRemoteApplicationsStatus")
		c.Assert(arg, gc.DeepEquals, params.SetStatus{Entities: []params.EntityStatusArgs{
			{
				Tag:    names.NewApplicationTag("mysql").String(),
				Status: "blocked",
				Info:   "a message",
			}}})
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
	err := client.SetRemoteApplicationStatus(context.Background(), "mysql", status.Blocked, "a message")
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *remoteRelationsSuite) TestUpdateControllerForModelResultCount(c *gc.C) {
	apiCaller := testing.APICallerFunc(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Assert(request, gc.Equals, "UpdateControllersForModels")
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{
					{Error: &params.Error{Message: "FAIL"}},
					{Error: &params.Error{Message: "FAIL"}},
				},
			}
			return nil
		},
	)

	client := remoterelations.NewClient(apiCaller)
	err := client.UpdateControllerForModel(context.Background(), crossmodel.ControllerInfo{}, "some-model-uuid")
	c.Check(err, gc.ErrorMatches, `expected 1 result, got 2`)
}

func (s *remoteRelationsSuite) TestUpdateControllerForModelResultError(c *gc.C) {
	apiCaller := testing.APICallerFunc(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Assert(request, gc.Equals, "UpdateControllersForModels")
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{Error: &params.Error{Message: "FAIL"}}},
			}
			return nil
		},
	)

	client := remoterelations.NewClient(apiCaller)
	err := client.UpdateControllerForModel(context.Background(), crossmodel.ControllerInfo{}, "some-model-uuid")
	c.Check(err, gc.ErrorMatches, `FAIL`)
}

func (s *remoteRelationsSuite) TestUpdateControllerForModelResultSuccess(c *gc.C) {
	apiCaller := testing.APICallerFunc(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Assert(request, gc.Equals, "UpdateControllersForModels")
			*(result.(*params.ErrorResults)) = params.ErrorResults{Results: []params.ErrorResult{{}}}
			return nil
		},
	)

	client := remoterelations.NewClient(apiCaller)
	err := client.UpdateControllerForModel(context.Background(), crossmodel.ControllerInfo{}, "some-model-uuid")
	c.Check(err, jc.ErrorIsNil)
}

func (s *remoteRelationsSuite) TestConsumeRemoteSecretChange(c *gc.C) {
	var callCount int
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ConsumeRemoteSecretChanges")
		c.Check(arg, jc.DeepEquals, params.LatestSecretRevisionChanges{
			Changes: []params.SecretRevisionChange{{
				URI:            uri.String(),
				LatestRevision: 666,
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}}}
		callCount++
		return nil
	})

	changes := []watcher.SecretRevisionChange{{
		URI:      uri,
		Revision: 666,
	}}
	client := remoterelations.NewClient(apiCaller)
	err := client.ConsumeRemoteSecretChanges(context.Background(), changes)
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}
