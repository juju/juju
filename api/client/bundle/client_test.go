// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/client/bundle"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

const apiVersion = 6

type bundleMockSuite struct {
	coretesting.BaseSuite
	bundleClient *bundle.Client
}

var _ = gc.Suite(&bundleMockSuite{})

func newClient(f basetesting.APICallerFunc, ver int) *bundle.Client {
	return bundle.NewClient(basetesting.BestVersionCaller{f, ver})
}

func (s *bundleMockSuite) TestGetChanges(c *gc.C) {
	bundleURL := "cs:bundle-url"
	bundleYAML := `applications:
	ubuntu:
		charm: cs:trusty/ubuntu
		series: trusty
		num_units: 1
		options:
			key: value
			series: xenial
		relations:
			- []`
	changes := []*params.BundleChange{
		{
			Id:       "addCharm-0",
			Method:   "addCharm",
			Args:     []interface{}{"cs:trusty/ubuntu", "trusty", ""},
			Requires: []string{},
		},
		{
			Id:     "deploy-1",
			Method: "deploy",
			Args: []interface{}{
				"$addCharm-0",
				"trusty",
				"ubuntu",
				map[string]interface{}{
					"key":    "value",
					"series": "xenial",
				},
				"",
				map[string]string{},
				map[string]string{},
				map[string]int{},
				1,
			},
			Requires: []string{"$addCharm-0"},
		},
	}
	client := newClient(
		func(objType string,
			version int,
			id,
			request string,
			args,
			response interface{},
		) error {
			c.Check(objType, gc.Equals, "Bundle")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "GetChanges")
			c.Assert(args, gc.Equals, params.BundleChangesParams{
				BundleDataYAML: bundleYAML,
				BundleURL:      bundleURL,
			})
			result := response.(*params.BundleChangesResults)
			result.Changes = changes
			return nil
		}, apiVersion,
	)
	result, err := client.GetChanges(bundleURL, bundleYAML)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Errors, gc.DeepEquals, []string(nil))
	c.Assert(result.Changes, gc.DeepEquals, changes)
}

func (s *bundleMockSuite) TestGetChangesReturnsErrors(c *gc.C) {
	bundleURL := "cs:bundle-url"
	bundleYAML := `applications:
	ubuntu:
		charm: cs:trusty/ubuntu
		series: trusty
		num_units: 1
		options:
			key: value
			series: xenial`
	client := newClient(
		func(objType string,
			version int,
			id,
			request string,
			args,
			response interface{},
		) error {
			c.Check(objType, gc.Equals, "Bundle")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "GetChanges")
			c.Assert(args, gc.Equals, params.BundleChangesParams{
				BundleDataYAML: bundleYAML,
				BundleURL:      bundleURL,
			})
			result := response.(*params.BundleChangesResults)
			result.Errors = []string{
				"Error returned from request",
			}
			return nil
		}, apiVersion,
	)
	result, err := client.GetChanges(bundleURL, bundleYAML)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Errors, gc.DeepEquals, []string{"Error returned from request"})
	c.Assert(result.Changes, gc.DeepEquals, []*params.BundleChange(nil))
}

func (s *bundleMockSuite) TestGetChangesMapArgs(c *gc.C) {
	bundleURL := "cs:bundle-url"
	bundleYAML := `applications:
	ubuntu:
		charm: cs:trusty/ubuntu
		series: trusty
		num_units: 1
		options:
			key: value
			series: xenial`
	changes := []*params.BundleChangesMapArgs{
		{
			Id:     "addCharm-0",
			Method: "addCharm",
			Args: map[string]interface{}{
				"charm":  "cs:trusty/ubuntu",
				"series": "trusty",
			},
			Requires: []string{},
		},
		{
			Id:     "deploy-1",
			Method: "deploy",
			Args: map[string]interface{}{
				"charm":     "$addCharm-0",
				"series":    "trusty",
				"num_units": "1",
				"options": map[string]interface{}{
					"key":    "value",
					"series": "xenial",
				},
			},
			Requires: []string{"$addCharm-0"},
		},
	}
	client := newClient(
		func(objType string,
			version int,
			id,
			request string,
			args,
			response interface{},
		) error {
			c.Check(objType, gc.Equals, "Bundle")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "GetChangesMapArgs")
			c.Assert(args, gc.Equals, params.BundleChangesParams{
				BundleDataYAML: bundleYAML,
				BundleURL:      bundleURL,
			})
			result := response.(*params.BundleChangesMapArgsResults)
			result.Changes = changes
			return nil
		}, apiVersion,
	)
	result, err := client.GetChangesMapArgs(bundleURL, bundleYAML)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Errors, gc.DeepEquals, []string(nil))
	c.Assert(result.Changes, gc.DeepEquals, changes)
}

func (s *bundleMockSuite) TestGetChangesMapArgsReturnsErrors(c *gc.C) {
	bundleURL := "cs:bundle-url"
	bundleYAML := `applications:
	ubuntu:
		charm: cs:trusty/ubuntu
		series: trusty
		num_units: 1
		options:
			key: value
			series: xenial
		relations:
			- []`
	client := newClient(
		func(objType string,
			version int,
			id,
			request string,
			args,
			response interface{},
		) error {
			c.Check(objType, gc.Equals, "Bundle")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "GetChangesMapArgs")
			c.Assert(args, gc.Equals, params.BundleChangesParams{
				BundleDataYAML: bundleYAML,
				BundleURL:      bundleURL,
			})
			result := response.(*params.BundleChangesMapArgsResults)
			result.Errors = []string{
				"Error returned from request",
			}
			return nil
		}, apiVersion,
	)
	result, err := client.GetChangesMapArgs(bundleURL, bundleYAML)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Errors, gc.DeepEquals, []string{"Error returned from request"})
	c.Assert(result.Changes, gc.DeepEquals, []*params.BundleChangesMapArgs(nil))
}

func (s *bundleMockSuite) TestExportBundleLatest(c *gc.C) {
	bundle := `applications:
	ubuntu:
		charm: cs:trusty/ubuntu
		series: trusty
		num_units: 1
		to:
			- \"0\"
		options:
			key: value
			series: xenial
		relations:
			- []`
	client := newClient(
		func(objType string, version int,
			id,
			request string,
			args,
			response interface{},
		) error {
			c.Assert(args, jc.DeepEquals, params.ExportBundleParams{
				IncludeCharmDefaults: true,
			})
			result := response.(*params.StringResult)
			result.Result = bundle
			return nil
		}, apiVersion,
	)
	result, err := client.ExportBundle(true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, bundle)
}
