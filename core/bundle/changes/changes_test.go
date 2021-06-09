// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bundlechanges_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/juju/charm/v9"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	bundlechanges "github.com/juju/juju/core/bundle/changes"
)

type changesSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&changesSuite{})

func (s *changesSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	err := loggo.ConfigureLoggers("bundlechanges=trace")
	c.Assert(err, jc.ErrorIsNil)
}

// record holds expected information about the contents of a change value.
type record struct {
	Id       string
	Requires []string
	Method   string
	Params   interface{}
	GUIArgs  []interface{}
	Args     map[string]interface{}
}

func (s *changesSuite) TestMinimalBundle(c *gc.C) {
	content := `
        applications:
            django:
                charm: django
   `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "django",
		},
		GUIArgs: []interface{}{"django", "", ""},
		Args: map[string]interface{}{
			"charm": "django",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"",
			"django",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
		},
		Requires: []string{"addCharm-0"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestMinimalBundleWithChannels(c *gc.C) {
	content := `
        applications:
            django:
                charm: django
                channel: edge
   `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:   "django",
			Channel: "edge",
		},
		GUIArgs: []interface{}{"django", "", "edge"},
		Args: map[string]interface{}{
			"channel": "edge",
			"charm":   "django",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Channel:     "edge",
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"",
			"django",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"edge",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"channel":     "edge",
		},
		Requires: []string{"addCharm-0"},
	}}

	s.assertParseData(c, content, expected)
}
func (s *changesSuite) TestBundleURLAnnotationSet(c *gc.C) {
	content := `
        applications:
            django:
                charm: django`

	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "django",
		},
		GUIArgs: []interface{}{"django", "", ""},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"",
			"django",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "setAnnotations-2",
		Method: "setAnnotations",
		Params: bundlechanges.SetAnnotationsParams{
			Id:         "$deploy-1",
			EntityType: "application",
			Annotations: map[string]string{
				"bundleURL": "cs:bundle/blog",
			},
		},
		GUIArgs: []interface{}{
			"$deploy-1",
			"application",
			map[string]string{
				"bundleURL": "cs:bundle/blog",
			},
		},
		Requires: []string{"deploy-1"},
	}}

	data, err := charm.ReadBundleData(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	err = data.Verify(nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	// Retrieve the changes, and convert them to a sequence of records.
	changes, err := bundlechanges.FromData(bundlechanges.ChangesConfig{
		Bundle:    data,
		BundleURL: "cs:bundle/blog",
		Logger:    loggo.GetLogger("bundlechanges"),
	})
	c.Assert(err, jc.ErrorIsNil)
	records := make([]record, len(changes))
	for i, change := range changes {
		r := record{
			Id:       change.Id(),
			Requires: change.Requires(),
			Method:   change.Method(),
			GUIArgs:  change.GUIArgs(),
			Params:   copyParams(change),
		}
		records[i] = r
	}
	c.Check(records, jc.DeepEquals, expected)
}

func (s *changesSuite) TestMinimalBundleWithDevices(c *gc.C) {
	content := `
        applications:
            django:
                charm: django
   `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "django",
		},
		GUIArgs: []interface{}{"django", "", ""},
		Args: map[string]interface{}{
			"charm": "django",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"",
			"django",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
		},
		Requires: []string{"addCharm-0"},
	}}

	s.assertParseDataWithDevices(c, content, expected)
}

func (s *changesSuite) TestMinimalBundleWithOffer(c *gc.C) {
	content := `
saas:
  keystone:
    url: production:admin/info.keystone
applications:
  apache2:
    charm: "cs:apache2-26"
--- #overlay
applications:
  apache2:
    offers:
      offer1:
        endpoints:
          - "apache-website"
          - "apache-proxy"
   `
	expected := []record{
		{
			Id:     "addCharm-0",
			Method: "addCharm",
			Params: bundlechanges.AddCharmParams{
				Charm: "cs:apache2-26",
			},
			GUIArgs: []interface{}{"cs:apache2-26", "", ""},
			Args: map[string]interface{}{
				"charm": "cs:apache2-26",
			},
		},
		{
			Id:     "deploy-1",
			Method: "deploy",
			Params: bundlechanges.AddApplicationParams{
				Charm:       "$addCharm-0",
				Application: "apache2",
			},
			GUIArgs: []interface{}{
				"$addCharm-0",
				"",
				"apache2",
				map[string]interface{}{},
				"",
				map[string]string{},
				map[string]string{},
				map[string]int{},
				0,
				"",
			},
			Args: map[string]interface{}{
				"application": "apache2",
				"charm":       "$addCharm-0",
			},
			Requires: []string{"addCharm-0"},
		},
		{
			Id:     "consumeOffer-2",
			Method: "consumeOffer",
			Params: bundlechanges.ConsumeOfferParams{
				URL:             "production:admin/info.keystone",
				ApplicationName: "keystone",
			},
			GUIArgs: []interface{}{"production:admin/info.keystone", "keystone"},
			Args: map[string]interface{}{
				"application-name": "keystone",
				"url":              "production:admin/info.keystone",
			},
			Requires: []string{},
		},
		{
			Id:     "createOffer-3",
			Method: "createOffer",
			Params: bundlechanges.CreateOfferParams{
				Application: "apache2",
				Endpoints: []string{
					"apache-website",
					"apache-proxy",
				},
				OfferName: "offer1",
			},
			GUIArgs: []interface{}{"apache2", []string{"apache-website", "apache-proxy"}, "offer1"},
			Args: map[string]interface{}{
				"application": "apache2",
				"endpoints": []interface{}{
					"apache-website",
					"apache-proxy",
				},
				"offer-name": "offer1",
			},
			Requires: []string{"deploy-1"},
		},
	}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestMinimalBundleWithOfferAndPreDeployedApp(c *gc.C) {
	content := `
applications:
  apache2:
    charm: "cs:apache2-26"
--- #overlay
applications:
  apache2:
    offers:
      offer1:
        endpoints:
          - "apache-website"
          - "apache-proxy"
   `

	// We have already deployed apache2 so we only expect the offer to be
	// added to the model.
	deployedModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"apache2": {
				Name:  "apache-2",
				Charm: "cs:apache2-26",
			},
		},
	}
	expected := []record{
		{
			Id:     "createOffer-0",
			Method: "createOffer",
			Params: bundlechanges.CreateOfferParams{
				Application: "apache2",
				Endpoints: []string{
					"apache-website",
					"apache-proxy",
				},
				OfferName: "offer1",
			},
			GUIArgs: []interface{}{"apache2", []string{"apache-website", "apache-proxy"}, "offer1"},
			Args: map[string]interface{}{
				"application": "apache2",
				"endpoints": []interface{}{
					"apache-website",
					"apache-proxy",
				},
				"offer-name": "offer1",
			},
		},
	}

	s.assertParseDataWithModel(c, deployedModel, content, expected)
}

func (s *changesSuite) TestMinimalBundleWithOfferACL(c *gc.C) {
	content := `
applications:
  apache2:
    charm: "cs:apache2-26"
--- #overlay
applications:
  apache2:
    offers:
      offer1:
        endpoints:
          - "apache-website"
          - "apache-proxy"
        acl:
          foo: consume
   `
	expected := []record{
		{
			Id:     "addCharm-0",
			Method: "addCharm",
			Params: bundlechanges.AddCharmParams{
				Charm: "cs:apache2-26",
			},
			GUIArgs: []interface{}{"cs:apache2-26", "", ""},
			Args: map[string]interface{}{
				"charm": "cs:apache2-26",
			},
		},
		{
			Id:     "deploy-1",
			Method: "deploy",
			Params: bundlechanges.AddApplicationParams{
				Charm:       "$addCharm-0",
				Application: "apache2",
			},
			GUIArgs: []interface{}{
				"$addCharm-0",
				"",
				"apache2",
				map[string]interface{}{},
				"",
				map[string]string{},
				map[string]string{},
				map[string]int{},
				0,
				"",
			},
			Args: map[string]interface{}{
				"application": "apache2",
				"charm":       "$addCharm-0",
			},
			Requires: []string{"addCharm-0"},
		},
		{
			Id:     "createOffer-2",
			Method: "createOffer",
			Params: bundlechanges.CreateOfferParams{
				Application: "apache2",
				Endpoints: []string{
					"apache-website",
					"apache-proxy",
				},
				OfferName: "offer1",
			},
			GUIArgs: []interface{}{"apache2", []string{"apache-website", "apache-proxy"}, "offer1"},
			Args: map[string]interface{}{
				"application": "apache2",
				"endpoints": []interface{}{
					"apache-website",
					"apache-proxy",
				},
				"offer-name": "offer1",
			},
			Requires: []string{"deploy-1"},
		},
		{
			Id:     "grantOfferAccess-3",
			Method: "grantOfferAccess",
			Params: bundlechanges.GrantOfferAccessParams{
				User:   "foo",
				Access: "consume",
				Offer:  "offer1",
			},
			GUIArgs: []interface{}{"foo", "consume", "offer1"},
			Args: map[string]interface{}{
				"access": "consume",
				"offer":  "offer1",
				"user":   "foo",
			},
			Requires: []string{"createOffer-2"},
		},
	}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestMinimalBundleWithOfferUpdate(c *gc.C) {
	content := `
applications:
  apache2:
    charm: "cs:apache2-26"
--- #overlay
applications:
  apache2:
    offers:
      offer1:
        endpoints:
          - "apache-website"
          - "apache-proxy"
   `
	expected := []record{
		{
			Id:     "createOffer-0",
			Method: "createOffer",
			Params: bundlechanges.CreateOfferParams{
				Application: "apache2",
				Endpoints: []string{
					"apache-website",
					"apache-proxy",
				},
				OfferName: "offer1",
				Update:    true,
			},
			GUIArgs: []interface{}{"apache2", []string{"apache-website", "apache-proxy"}, "offer1"},
			Args: map[string]interface{}{
				"application": "apache2",
				"endpoints": []interface{}{
					"apache-website",
					"apache-proxy",
				},
				"offer-name": "offer1",
				"update":     true,
			},
		},
	}

	curModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"apache2": {
				Name:   "apache2",
				Charm:  "cs:apache2-26",
				Offers: []string{"offer1"},
			},
		},
	}
	s.assertParseDataWithModel(c, curModel, content, expected)
}

func (s *changesSuite) TestMinimalBundleWithOfferAndRelations(c *gc.C) {
	content := `
saas:
  mysql:
    url: production:admin/info.mysql
applications:
  apache2:
    charm: "cs:apache2-26"
relations:
- - apache2:db
  - mysql:db
   `
	expected := []record{
		{
			Id:     "addCharm-0",
			Method: "addCharm",
			Params: bundlechanges.AddCharmParams{
				Charm: "cs:apache2-26",
			},
			GUIArgs: []interface{}{"cs:apache2-26", "", ""},
			Args: map[string]interface{}{
				"charm": "cs:apache2-26",
			},
		},
		{
			Id:     "deploy-1",
			Method: "deploy",
			Params: bundlechanges.AddApplicationParams{
				Charm:       "$addCharm-0",
				Application: "apache2",
			},
			GUIArgs: []interface{}{
				"$addCharm-0",
				"",
				"apache2",
				map[string]interface{}{},
				"",
				map[string]string{},
				map[string]string{},
				map[string]int{},
				0,
				"",
			},
			Args: map[string]interface{}{
				"application": "apache2",
				"charm":       "$addCharm-0",
			},
			Requires: []string{"addCharm-0"},
		},
		{
			Id:     "consumeOffer-2",
			Method: "consumeOffer",
			Params: bundlechanges.ConsumeOfferParams{
				URL:             "production:admin/info.mysql",
				ApplicationName: "mysql",
			},
			GUIArgs: []interface{}{"production:admin/info.mysql", "mysql"},
			Args: map[string]interface{}{
				"application-name": "mysql",
				"url":              "production:admin/info.mysql",
			},
			Requires: []string{},
		},
		{
			Id:     "addRelation-3",
			Method: "addRelation",
			Params: bundlechanges.AddRelationParams{
				Endpoint1: "$deploy-1:db",
				Endpoint2: "$consumeOffer-2:db",
			},
			GUIArgs: []interface{}{"$deploy-1:db", "$consumeOffer-2:db"},
			Args: map[string]interface{}{
				"endpoint1": "$deploy-1:db",
				"endpoint2": "$consumeOffer-2:db",
			},
			Requires: []string{"deploy-1", "consumeOffer-2"},
		},
	}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestSimpleBundle(c *gc.C) {
	content := `
        applications:
            mediawiki:
                charm: cs:precise/mediawiki-10
                num_units: 1
                expose: true
                options:
                    debug: false
                annotations:
                    gui-x: "609"
                    gui-y: "-15"
                resources:
                    data: 3
            mysql:
                charm: cs:precise/mysql-28
                num_units: 1
                resources:
                  data: "./resources/data.tar"
        series: trusty
        relations:
            - - mediawiki:db
              - mysql:db
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:precise/mediawiki-10",
			Series: "precise",
		},
		GUIArgs: []interface{}{"cs:precise/mediawiki-10", "precise", ""},
		Args: map[string]interface{}{
			"charm":  "cs:precise/mediawiki-10",
			"series": "precise",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "mediawiki",
			Series:      "precise",
			Options:     map[string]interface{}{"debug": false},
			Resources:   map[string]int{"data": 3},
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"precise",
			"mediawiki",
			map[string]interface{}{"debug": false},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{"data": 3},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "mediawiki",
			"charm":       "$addCharm-0",
			"options": map[string]interface{}{
				"debug": false,
			},
			"resources": map[string]interface{}{
				"data": float64(3),
			},
			"series": "precise",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "expose-2",
		Method: "expose",
		Params: bundlechanges.ExposeParams{
			Application: "$deploy-1",
		},
		GUIArgs: []interface{}{"$deploy-1", nil},
		Args: map[string]interface{}{
			"application": "$deploy-1",
		},
		Requires: []string{"deploy-1"},
	}, {
		Id:     "setAnnotations-3",
		Method: "setAnnotations",
		Params: bundlechanges.SetAnnotationsParams{
			Id:          "$deploy-1",
			EntityType:  bundlechanges.ApplicationType,
			Annotations: map[string]string{"gui-x": "609", "gui-y": "-15"},
		},
		GUIArgs: []interface{}{
			"$deploy-1",
			"application",
			map[string]string{"gui-x": "609", "gui-y": "-15"},
		},
		Args: map[string]interface{}{
			"annotations": map[string]interface{}{
				"gui-x": "609",
				"gui-y": "-15",
			},
			"entity-type": "application",
			"id":          "$deploy-1",
		},
		Requires: []string{"deploy-1"},
	}, {
		Id:     "addCharm-4",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:precise/mysql-28",
			Series: "precise",
		},
		GUIArgs: []interface{}{"cs:precise/mysql-28", "precise", ""},
		Args: map[string]interface{}{
			"charm":  "cs:precise/mysql-28",
			"series": "precise",
		},
	}, {
		Id:     "deploy-5",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:          "$addCharm-4",
			Application:    "mysql",
			Series:         "precise",
			LocalResources: map[string]string{"data": "./resources/data.tar"},
		},
		GUIArgs: []interface{}{
			"$addCharm-4",
			"precise",
			"mysql",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "mysql",
			"charm":       "$addCharm-4",
			"local-resources": map[string]interface{}{
				"data": "./resources/data.tar",
			},
			"series": "precise",
		},
		Requires: []string{"addCharm-4"},
	}, {
		Id:     "addRelation-6",
		Method: "addRelation",
		Params: bundlechanges.AddRelationParams{
			Endpoint1: "$deploy-1:db",
			Endpoint2: "$deploy-5:db",
		},
		GUIArgs: []interface{}{"$deploy-1:db", "$deploy-5:db"},
		Args: map[string]interface{}{
			"endpoint1": "$deploy-1:db",
			"endpoint2": "$deploy-5:db",
		},
		Requires: []string{"deploy-1", "deploy-5"},
	}, {
		Id:     "addUnit-7",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
		},
		GUIArgs: []interface{}{"$deploy-1", nil},
		Args: map[string]interface{}{
			"application": "$deploy-1",
		},
		Requires: []string{"deploy-1"},
	}, {
		Id:     "addUnit-8",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-5",
		},
		GUIArgs: []interface{}{"$deploy-5", nil},
		Args: map[string]interface{}{
			"application": "$deploy-5",
		},
		Requires: []string{"deploy-5"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestSimpleBundleWithDevices(c *gc.C) {
	content := `
        applications:
            mediawiki:
                charm: cs:precise/mediawiki-10
                num_units: 1
                expose: true
                options:
                    debug: false
                annotations:
                    gui-x: "609"
                    gui-y: "-15"
                resources:
                    data: 3
            mysql:
                charm: cs:precise/mysql-28
                num_units: 1
                resources:
                  data: "./resources/data.tar"
        series: trusty
        relations:
            - - mediawiki:db
              - mysql:db
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:precise/mediawiki-10",
			Series: "precise",
		},
		GUIArgs: []interface{}{"cs:precise/mediawiki-10", "precise", ""},
		Args: map[string]interface{}{
			"charm":  "cs:precise/mediawiki-10",
			"series": "precise",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "mediawiki",
			Series:      "precise",
			Options:     map[string]interface{}{"debug": false},
			Resources:   map[string]int{"data": 3},
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"precise",
			"mediawiki",
			map[string]interface{}{"debug": false},
			"",
			map[string]string{},
			map[string]string{},
			map[string]string{},
			map[string]int{"data": 3},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "mediawiki",
			"charm":       "$addCharm-0",
			"options": map[string]interface{}{
				"debug": false,
			},
			"resources": map[string]interface{}{
				"data": float64(3),
			},
			"series": "precise",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "expose-2",
		Method: "expose",
		Params: bundlechanges.ExposeParams{
			Application: "$deploy-1",
		},
		GUIArgs: []interface{}{"$deploy-1", nil},
		Args: map[string]interface{}{
			"application": "$deploy-1",
		},
		Requires: []string{"deploy-1"},
	}, {
		Id:     "setAnnotations-3",
		Method: "setAnnotations",
		Params: bundlechanges.SetAnnotationsParams{
			Id:          "$deploy-1",
			EntityType:  bundlechanges.ApplicationType,
			Annotations: map[string]string{"gui-x": "609", "gui-y": "-15"},
		},
		GUIArgs: []interface{}{
			"$deploy-1",
			"application",
			map[string]string{"gui-x": "609", "gui-y": "-15"},
		},
		Args: map[string]interface{}{
			"annotations": map[string]interface{}{
				"gui-x": "609",
				"gui-y": "-15",
			},
			"entity-type": "application",
			"id":          "$deploy-1",
		},
		Requires: []string{"deploy-1"},
	}, {
		Id:     "addCharm-4",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:precise/mysql-28",
			Series: "precise",
		},
		GUIArgs: []interface{}{"cs:precise/mysql-28", "precise", ""},
		Args: map[string]interface{}{
			"charm":  "cs:precise/mysql-28",
			"series": "precise",
		},
	}, {
		Id:     "deploy-5",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:          "$addCharm-4",
			Application:    "mysql",
			Series:         "precise",
			LocalResources: map[string]string{"data": "./resources/data.tar"},
		},
		GUIArgs: []interface{}{
			"$addCharm-4",
			"precise",
			"mysql",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "mysql",
			"charm":       "$addCharm-4",
			"local-resources": map[string]interface{}{
				"data": "./resources/data.tar",
			},
			"series": "precise",
		},
		Requires: []string{"addCharm-4"},
	}, {
		Id:     "addRelation-6",
		Method: "addRelation",
		Params: bundlechanges.AddRelationParams{
			Endpoint1: "$deploy-1:db",
			Endpoint2: "$deploy-5:db",
		},
		GUIArgs: []interface{}{"$deploy-1:db", "$deploy-5:db"},
		Args: map[string]interface{}{
			"endpoint1": "$deploy-1:db",
			"endpoint2": "$deploy-5:db",
		},
		Requires: []string{"deploy-1", "deploy-5"},
	}, {
		Id:     "addUnit-7",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
		},
		GUIArgs: []interface{}{"$deploy-1", nil},
		Args: map[string]interface{}{
			"application": "$deploy-1",
		},
		Requires: []string{"deploy-1"},
	}, {
		Id:     "addUnit-8",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-5",
		},
		GUIArgs: []interface{}{"$deploy-5", nil},
		Args: map[string]interface{}{
			"application": "$deploy-5",
		},
		Requires: []string{"deploy-5"},
	}}

	s.assertParseDataWithDevices(c, content, expected)
}

func (s *changesSuite) TestKubernetesBundle(c *gc.C) {
	content := `
        bundle: kubernetes
        applications:
            mediawiki:
                charm: cs:mediawiki-k8s-10
                num_units: 1
                expose: true
                options:
                    debug: false
                annotations:
                    gui-x: "609"
                    gui-y: "-15"
                resources:
                    data: 3
            mysql:
                charm: cs:mysql-k8s-28
                num_units: 2
                resources:
                  data: "./resources/data.tar"
        relations:
            - - mediawiki:db
              - mysql:db
        `
	// float64 is used here because that's what the JSON specification falls
	// back to, there is no int!
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:mediawiki-k8s-10",
			Series: "kubernetes",
		},
		GUIArgs: []interface{}{"cs:mediawiki-k8s-10", "kubernetes", ""},
		Args: map[string]interface{}{
			"charm":  "cs:mediawiki-k8s-10",
			"series": "kubernetes",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "mediawiki",
			Series:      "kubernetes",
			NumUnits:    1,
			Options:     map[string]interface{}{"debug": false},
			Resources:   map[string]int{"data": 3},
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"kubernetes",
			"mediawiki",
			map[string]interface{}{"debug": false},
			"",
			map[string]string{},
			map[string]string{},
			map[string]string{},
			map[string]int{"data": 3},
			1,
			"",
		},
		Args: map[string]interface{}{
			"application": "mediawiki",
			"charm":       "$addCharm-0",
			"num-units":   float64(1),
			"options": map[string]interface{}{
				"debug": false,
			},
			"resources": map[string]interface{}{
				"data": float64(3),
			},
			"series": "kubernetes",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "expose-2",
		Method: "expose",
		Params: bundlechanges.ExposeParams{
			Application: "$deploy-1",
		},
		GUIArgs: []interface{}{"$deploy-1", nil},
		Args: map[string]interface{}{
			"application": "$deploy-1",
		},
		Requires: []string{"deploy-1"},
	}, {
		Id:     "setAnnotations-3",
		Method: "setAnnotations",
		Params: bundlechanges.SetAnnotationsParams{
			Id:          "$deploy-1",
			EntityType:  bundlechanges.ApplicationType,
			Annotations: map[string]string{"gui-x": "609", "gui-y": "-15"},
		},
		GUIArgs: []interface{}{
			"$deploy-1",
			"application",
			map[string]string{"gui-x": "609", "gui-y": "-15"},
		},
		Args: map[string]interface{}{
			"annotations": map[string]interface{}{
				"gui-x": "609",
				"gui-y": "-15",
			},
			"entity-type": "application",
			"id":          "$deploy-1",
		},
		Requires: []string{"deploy-1"},
	}, {
		Id:     "addCharm-4",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:mysql-k8s-28",
			Series: "kubernetes",
		},
		GUIArgs: []interface{}{"cs:mysql-k8s-28", "kubernetes", ""},
		Args: map[string]interface{}{
			"charm":  "cs:mysql-k8s-28",
			"series": "kubernetes",
		},
	}, {
		Id:     "deploy-5",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:          "$addCharm-4",
			Application:    "mysql",
			Series:         "kubernetes",
			NumUnits:       2,
			LocalResources: map[string]string{"data": "./resources/data.tar"},
		},
		GUIArgs: []interface{}{
			"$addCharm-4",
			"kubernetes",
			"mysql",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]string{},
			map[string]int{},
			2,
			"",
		},
		Args: map[string]interface{}{
			"application": "mysql",
			"charm":       "$addCharm-4",
			"local-resources": map[string]interface{}{
				"data": "./resources/data.tar",
			},
			"num-units": float64(2),
			"series":    "kubernetes",
		},
		Requires: []string{"addCharm-4"},
	}, {
		Id:     "addRelation-6",
		Method: "addRelation",
		Params: bundlechanges.AddRelationParams{
			Endpoint1: "$deploy-1:db",
			Endpoint2: "$deploy-5:db",
		},
		GUIArgs: []interface{}{"$deploy-1:db", "$deploy-5:db"},
		Args: map[string]interface{}{
			"endpoint1": "$deploy-1:db",
			"endpoint2": "$deploy-5:db",
		},
		Requires: []string{"deploy-1", "deploy-5"},
	}}

	s.assertParseDataWithDevices(c, content, expected)
}

func (s *changesSuite) TestSameCharmReused(c *gc.C) {
	content := `
        applications:
            mediawiki:
                charm: cs:precise/mediawiki-10
                num_units: 1
            otherwiki:
                charm: cs:precise/mediawiki-10
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:precise/mediawiki-10",
			Series: "precise",
		},
		GUIArgs: []interface{}{"cs:precise/mediawiki-10", "precise", ""},
		Args: map[string]interface{}{
			"charm":  "cs:precise/mediawiki-10",
			"series": "precise",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "mediawiki",
			Series:      "precise",
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"precise",
			"mediawiki",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "mediawiki",
			"charm":       "$addCharm-0",
			"series":      "precise",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "deploy-2",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "otherwiki",
			Series:      "precise",
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"precise",
			"otherwiki",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "otherwiki",
			"charm":       "$addCharm-0",
			"series":      "precise",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addUnit-3",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
		},
		GUIArgs: []interface{}{"$deploy-1", nil},
		Args: map[string]interface{}{
			"application": "$deploy-1",
		},
		Requires: []string{"deploy-1"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestMachinesAndUnitsPlacementWithBindings(c *gc.C) {
	content := `
        applications:
            django:
                charm: cs:trusty/django-42
                num_units: 2
                bindings:
                    "": foo
                    http: bar
                to:
                    - 1
                    - lxc:2
                constraints: spaces=baz cpu-cores=4 cpu-power=42
            haproxy:
                charm: cs:trusty/haproxy-47
                num_units: 2
                expose: yes
                to:
                    - lxc:django/0
                    - new
                options:
                    bad: wolf
                    number: 42.47
        machines:
            1:
                series: trusty
            2:
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:trusty/django-42",
			Series: "trusty",
		},
		GUIArgs: []interface{}{"cs:trusty/django-42", "trusty", ""},
		Args: map[string]interface{}{
			"charm":  "cs:trusty/django-42",
			"series": "trusty",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:            "$addCharm-0",
			Application:      "django",
			Series:           "trusty",
			Constraints:      "spaces=baz cpu-cores=4 cpu-power=42",
			EndpointBindings: map[string]string{"": "foo", "http": "bar"},
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"trusty",
			"django",
			map[string]interface{}{},
			"spaces=baz cpu-cores=4 cpu-power=42",
			map[string]string{},
			map[string]string{"": "foo", "http": "bar"},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"constraints": "spaces=baz cpu-cores=4 cpu-power=42",
			"endpoint-bindings": map[string]interface{}{
				"":     "foo",
				"http": "bar",
			},
			"series": "trusty",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:trusty/haproxy-47",
			Series: "trusty",
		},
		GUIArgs: []interface{}{"cs:trusty/haproxy-47", "trusty", ""},
		Args: map[string]interface{}{
			"charm":  "cs:trusty/haproxy-47",
			"series": "trusty",
		},
	}, {
		Id:     "deploy-3",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-2",
			Application: "haproxy",
			Series:      "trusty",
			Options:     map[string]interface{}{"bad": "wolf", "number": 42.47},
		},
		GUIArgs: []interface{}{
			"$addCharm-2",
			"trusty",
			"haproxy",
			map[string]interface{}{"bad": "wolf", "number": 42.47},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "haproxy",
			"charm":       "$addCharm-2",
			"options": map[string]interface{}{
				"bad":    "wolf",
				"number": 42.47,
			},
			"series": "trusty",
		},
		Requires: []string{"addCharm-2"},
	}, {
		Id:     "expose-4",
		Method: "expose",
		Params: bundlechanges.ExposeParams{
			Application: "$deploy-3",
		},
		GUIArgs: []interface{}{"$deploy-3", nil},
		Args: map[string]interface{}{
			"application": "$deploy-3",
		},
		Requires: []string{"deploy-3"},
	}, {
		Id:     "addMachines-5",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Series: "trusty",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{Series: "trusty"},
		},
		Args: map[string]interface{}{
			"series": "trusty",
		},
	}, {
		Id:     "addMachines-6",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{},
		},
		Args: map[string]interface{}{},
	}, {
		Id:     "addUnit-7",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-5",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addMachines-5"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-5",
		},
		Requires: []string{"deploy-1", "addMachines-5"},
	}, {
		Id:     "addMachines-11",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "lxc",
			Series:        "trusty",
			ParentId:      "$addMachines-6",
			Constraints:   "spaces=bar,baz,foo cpu-cores=4 cpu-power=42",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				ContainerType: "lxc",
				Series:        "trusty",
				ParentId:      "$addMachines-6",
				Constraints:   "spaces=bar,baz,foo cpu-cores=4 cpu-power=42",
			},
		},
		Args: map[string]interface{}{
			"constraints":    "spaces=bar,baz,foo cpu-cores=4 cpu-power=42",
			"container-type": "lxc",
			"parent-id":      "$addMachines-6",
			"series":         "trusty",
		},
		Requires: []string{"addMachines-6"},
	}, {
		Id:     "addMachines-12",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "lxc",
			Series:        "trusty",
			ParentId:      "$addUnit-7",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				ContainerType: "lxc",
				Series:        "trusty",
				ParentId:      "$addUnit-7",
			},
		},
		Args: map[string]interface{}{
			"container-type": "lxc",
			"parent-id":      "$addUnit-7",
			"series":         "trusty",
		},
		Requires: []string{"addUnit-7"},
	}, {
		Id:     "addMachines-13",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Series: "trusty",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				Series: "trusty",
			},
		},
		Args: map[string]interface{}{
			"series": "trusty",
		},
	}, {
		Id:     "addUnit-8",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-11",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addMachines-11"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-11",
		},
		Requires: []string{"deploy-1", "addMachines-11", "addUnit-7"},
	}, {
		Id:     "addUnit-9",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
			To:          "$addMachines-12",
		},
		GUIArgs: []interface{}{"$deploy-3", "$addMachines-12"},
		Args: map[string]interface{}{
			"application": "$deploy-3",
			"to":          "$addMachines-12",
		},
		Requires: []string{"deploy-3", "addMachines-12"},
	}, {
		Id:     "addUnit-10",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
			To:          "$addMachines-13",
		},
		GUIArgs: []interface{}{"$deploy-3", "$addMachines-13"},
		Args: map[string]interface{}{
			"application": "$deploy-3",
			"to":          "$addMachines-13",
		},
		Requires: []string{"deploy-3", "addMachines-13", "addUnit-9"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestMachinesWithConstraintsAndAnnotations(c *gc.C) {
	content := `
        applications:
            django:
                charm: cs:trusty/django-42
                num_units: 2
                to:
                    - 1
                    - new
        machines:
            1:
                constraints: "cpu-cores=4"
                annotations:
                    foo: bar
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:trusty/django-42",
			Series: "trusty",
		},
		GUIArgs: []interface{}{"cs:trusty/django-42", "trusty", ""},
		Args: map[string]interface{}{
			"charm":  "cs:trusty/django-42",
			"series": "trusty",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Series:      "trusty",
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"trusty",
			"django",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"series":      "trusty",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addMachines-2",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Constraints: "cpu-cores=4",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				Constraints: "cpu-cores=4",
			},
		},
		Args: map[string]interface{}{
			"constraints": "cpu-cores=4",
		},
	}, {
		Id:     "setAnnotations-3",
		Method: "setAnnotations",
		Params: bundlechanges.SetAnnotationsParams{
			Id:          "$addMachines-2",
			EntityType:  bundlechanges.MachineType,
			Annotations: map[string]string{"foo": "bar"},
		},
		GUIArgs: []interface{}{
			"$addMachines-2",
			"machine",
			map[string]string{"foo": "bar"},
		},
		Args: map[string]interface{}{
			"annotations": map[string]interface{}{
				"foo": "bar",
			},
			"entity-type": "machine",
			"id":          "$addMachines-2",
		},
		Requires: []string{"addMachines-2"},
	}, {
		Id:     "addUnit-4",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-2",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addMachines-2"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-2",
		},
		Requires: []string{"deploy-1", "addMachines-2"},
	}, {
		Id:     "addMachines-6",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Series: "trusty",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				Series: "trusty",
			},
		},
		Args: map[string]interface{}{
			"series": "trusty",
		},
	}, {
		Id:     "addUnit-5",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-6",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addMachines-6"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-6",
		},
		Requires: []string{"deploy-1", "addMachines-6", "addUnit-4"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestEndpointWithoutRelationName(c *gc.C) {
	content := `
        applications:
            mediawiki:
                charm: cs:precise/mediawiki-10
            mysql:
                charm: cs:precise/mysql-28
                constraints: mem=42G
        relations:
            - - mediawiki:db
              - mysql
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:precise/mediawiki-10",
			Series: "precise",
		},
		GUIArgs: []interface{}{"cs:precise/mediawiki-10", "precise", ""},
		Args: map[string]interface{}{
			"charm":  "cs:precise/mediawiki-10",
			"series": "precise",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "mediawiki",
			Series:      "precise",
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"precise",
			"mediawiki",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "mediawiki",
			"charm":       "$addCharm-0",
			"series":      "precise",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:precise/mysql-28",
			Series: "precise",
		},
		GUIArgs: []interface{}{"cs:precise/mysql-28", "precise", ""},
		Args: map[string]interface{}{
			"charm":  "cs:precise/mysql-28",
			"series": "precise",
		},
	}, {
		Id:     "deploy-3",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-2",
			Application: "mysql",
			Series:      "precise",
			Constraints: "mem=42G",
		},
		GUIArgs: []interface{}{
			"$addCharm-2",
			"precise",
			"mysql",
			map[string]interface{}{},
			"mem=42G",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "mysql",
			"charm":       "$addCharm-2",
			"constraints": "mem=42G",
			"series":      "precise",
		},
		Requires: []string{"addCharm-2"},
	}, {
		Id:     "addRelation-4",
		Method: "addRelation",
		Params: bundlechanges.AddRelationParams{
			Endpoint1: "$deploy-1:db",
			Endpoint2: "$deploy-3",
		},
		GUIArgs: []interface{}{"$deploy-1:db", "$deploy-3"},
		Args: map[string]interface{}{
			"endpoint1": "$deploy-1:db",
			"endpoint2": "$deploy-3",
		},
		Requires: []string{"deploy-1", "deploy-3"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestUnitPlacedInApplication(c *gc.C) {
	content := `
        applications:
            wordpress:
                charm: wordpress
                num_units: 3
            django:
                charm: cs:trusty/django-42
                num_units: 2
                to: [wordpress]
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:trusty/django-42",
			Series: "trusty",
		},
		GUIArgs: []interface{}{"cs:trusty/django-42", "trusty", ""},
		Args: map[string]interface{}{
			"charm":  "cs:trusty/django-42",
			"series": "trusty",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Series:      "trusty",
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"trusty",
			"django",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"series":      "trusty",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "wordpress",
		},
		GUIArgs: []interface{}{"wordpress", "", ""},
		Args: map[string]interface{}{
			"charm": "wordpress",
		},
	}, {
		Id:     "deploy-3",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-2",
			Application: "wordpress",
		},
		GUIArgs: []interface{}{
			"$addCharm-2",
			"",
			"wordpress",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "wordpress",
			"charm":       "$addCharm-2",
		},
		Requires: []string{"addCharm-2"},
	}, {
		Id:     "addUnit-6",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
		},
		GUIArgs: []interface{}{"$deploy-3", nil},
		Args: map[string]interface{}{
			"application": "$deploy-3",
		},
		Requires: []string{"deploy-3"},
	}, {
		Id:     "addUnit-7",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
		},
		GUIArgs: []interface{}{"$deploy-3", nil},
		Args: map[string]interface{}{
			"application": "$deploy-3",
		},
		Requires: []string{"deploy-3", "addUnit-6"},
	}, {
		Id:     "addUnit-8",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
		},
		GUIArgs: []interface{}{"$deploy-3", nil},
		Args: map[string]interface{}{
			"application": "$deploy-3",
		},
		Requires: []string{"deploy-3", "addUnit-7"},
	}, {
		Id:     "addUnit-4",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addUnit-6",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addUnit-6"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addUnit-6",
		},
		Requires: []string{"deploy-1", "addUnit-6"},
	}, {
		Id:     "addUnit-5",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addUnit-7",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addUnit-7"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addUnit-7",
		},
		Requires: []string{"deploy-1", "addUnit-7", "addUnit-4"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestUnitPlacedInApplicationWithDevices(c *gc.C) {
	content := `
        applications:
            wordpress:
                charm: wordpress
                num_units: 3
            django:
                charm: cs:trusty/django-42
                num_units: 2
                to: [wordpress]
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:trusty/django-42",
			Series: "trusty",
		},
		GUIArgs: []interface{}{"cs:trusty/django-42", "trusty", ""},
		Args: map[string]interface{}{
			"charm":  "cs:trusty/django-42",
			"series": "trusty",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Series:      "trusty",
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"trusty",
			"django",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"series":      "trusty",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "wordpress",
		},
		GUIArgs: []interface{}{"wordpress", "", ""},
		Args: map[string]interface{}{
			"charm": "wordpress",
		},
	}, {
		Id:     "deploy-3",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-2",
			Application: "wordpress",
		},
		GUIArgs: []interface{}{
			"$addCharm-2",
			"",
			"wordpress",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "wordpress",
			"charm":       "$addCharm-2",
		},
		Requires: []string{"addCharm-2"},
	}, {
		Id:     "addUnit-6",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
		},
		GUIArgs: []interface{}{"$deploy-3", nil},
		Args: map[string]interface{}{
			"application": "$deploy-3",
		},
		Requires: []string{"deploy-3"},
	}, {
		Id:     "addUnit-7",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
		},
		GUIArgs: []interface{}{"$deploy-3", nil},
		Args: map[string]interface{}{
			"application": "$deploy-3",
		},
		Requires: []string{"deploy-3", "addUnit-6"},
	}, {
		Id:     "addUnit-8",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
		},
		GUIArgs: []interface{}{"$deploy-3", nil},
		Args: map[string]interface{}{
			"application": "$deploy-3",
		},
		Requires: []string{"deploy-3", "addUnit-7"},
	}, {
		Id:     "addUnit-4",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addUnit-6",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addUnit-6"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addUnit-6",
		},
		Requires: []string{"deploy-1", "addUnit-6"},
	}, {
		Id:     "addUnit-5",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addUnit-7",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addUnit-7"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addUnit-7",
		},
		Requires: []string{"deploy-1", "addUnit-7", "addUnit-4"},
	}}

	s.assertParseDataWithDevices(c, content, expected)
}

func (s *changesSuite) TestUnitColocationWithOtherUnits(c *gc.C) {
	content := `
        applications:
            memcached:
                charm: cs:trusty/mem-47
                num_units: 3
                to: [1, new]
            django:
                charm: cs:trusty/django-42
                num_units: 5
                to:
                    - memcached/0
                    - lxc:memcached/1
                    - lxc:memcached/2
                    - kvm:ror
            ror:
                charm: cs:vivid/rails
                num_units: 2
                to:
                    - new
                    - 1
        machines:
            1:
                series: trusty
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:trusty/django-42",
			Series: "trusty",
		},
		GUIArgs: []interface{}{"cs:trusty/django-42", "trusty", ""},
		Args: map[string]interface{}{
			"charm":  "cs:trusty/django-42",
			"series": "trusty",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Series:      "trusty",
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"trusty",
			"django",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"series":      "trusty",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:trusty/mem-47",
			Series: "trusty",
		},
		GUIArgs: []interface{}{"cs:trusty/mem-47", "trusty", ""},
		Args: map[string]interface{}{
			"charm":  "cs:trusty/mem-47",
			"series": "trusty",
		},
	}, {
		Id:     "deploy-3",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-2",
			Application: "memcached",
			Series:      "trusty",
		},
		GUIArgs: []interface{}{
			"$addCharm-2",
			"trusty",
			"memcached",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "memcached",
			"charm":       "$addCharm-2",
			"series":      "trusty",
		},
		Requires: []string{"addCharm-2"},
	}, {
		Id:     "addCharm-4",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:vivid/rails",
			Series: "vivid",
		},
		GUIArgs: []interface{}{"cs:vivid/rails", "vivid", ""},
		Args: map[string]interface{}{
			"charm":  "cs:vivid/rails",
			"series": "vivid",
		},
	}, {
		Id:     "deploy-5",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-4",
			Application: "ror",
			Series:      "vivid",
		},
		GUIArgs: []interface{}{
			"$addCharm-4",
			"vivid",
			"ror",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "ror",
			"charm":       "$addCharm-4",
			"series":      "vivid",
		},
		Requires: []string{"addCharm-4"},
	}, {
		Id:     "addMachines-6",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Series: "trusty",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				Series:      "trusty",
				Constraints: "",
			},
		},
		Args: map[string]interface{}{
			"series": "trusty",
		},
	}, {
		Id:     "addUnit-12",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
			To:          "$addMachines-6",
		},
		GUIArgs: []interface{}{"$deploy-3", "$addMachines-6"},
		Args: map[string]interface{}{
			"application": "$deploy-3",
			"to":          "$addMachines-6",
		},
		Requires: []string{"deploy-3", "addMachines-6"},
	}, {
		Id:     "addMachines-17",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Series: "trusty",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				Series: "trusty",
			},
		},
		Args: map[string]interface{}{
			"series": "trusty",
		},
	}, {
		Id:     "addMachines-18",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Series: "trusty",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				Series: "trusty",
			},
		},
		Args: map[string]interface{}{
			"series": "trusty",
		},
	}, {
		Id:     "addMachines-19",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Series: "vivid",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				Series: "vivid",
			},
		},
		Args: map[string]interface{}{
			"series": "vivid",
		},
	}, {
		Id:     "addUnit-7",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addUnit-12",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addUnit-12"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addUnit-12",
		},
		Requires: []string{"deploy-1", "addUnit-12"},
	}, {
		Id:     "addUnit-13",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
			To:          "$addMachines-17",
		},
		GUIArgs: []interface{}{"$deploy-3", "$addMachines-17"},
		Args: map[string]interface{}{
			"application": "$deploy-3",
			"to":          "$addMachines-17",
		},
		Requires: []string{"deploy-3", "addMachines-17", "addUnit-12"},
	}, {
		Id:     "addUnit-14",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
			To:          "$addMachines-18",
		},
		GUIArgs: []interface{}{"$deploy-3", "$addMachines-18"},
		Args: map[string]interface{}{
			"application": "$deploy-3",
			"to":          "$addMachines-18",
		},
		Requires: []string{"deploy-3", "addMachines-18", "addUnit-13"},
	}, {
		Id:     "addUnit-15",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-5",
			To:          "$addMachines-19",
		},
		GUIArgs: []interface{}{"$deploy-5", "$addMachines-19"},
		Args: map[string]interface{}{
			"application": "$deploy-5",
			"to":          "$addMachines-19",
		},
		Requires: []string{"deploy-5", "addMachines-19"},
	}, {
		Id:     "addUnit-16",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-5",
			To:          "$addMachines-6",
		},
		GUIArgs: []interface{}{"$deploy-5", "$addMachines-6"},
		Args: map[string]interface{}{
			"application": "$deploy-5",
			"to":          "$addMachines-6",
		},
		Requires: []string{"deploy-5", "addMachines-6", "addUnit-15"},
	}, {
		Id:     "addMachines-20",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "lxc",
			Series:        "trusty",
			ParentId:      "$addUnit-13",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				ContainerType: "lxc",
				Series:        "trusty",
				ParentId:      "$addUnit-13",
			},
		},
		Args: map[string]interface{}{
			"container-type": "lxc",
			"parent-id":      "$addUnit-13",
			"series":         "trusty",
		},
		Requires: []string{"addUnit-13"},
	}, {
		Id:     "addMachines-21",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "lxc",
			Series:        "trusty",
			ParentId:      "$addUnit-14",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				ContainerType: "lxc",
				Series:        "trusty",
				ParentId:      "$addUnit-14",
			},
		},
		Args: map[string]interface{}{
			"container-type": "lxc",
			"parent-id":      "$addUnit-14",
			"series":         "trusty",
		},
		Requires: []string{"addUnit-14"},
	}, {
		Id:     "addMachines-22",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "kvm",
			Series:        "trusty",
			ParentId:      "$addUnit-15",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				ContainerType: "kvm",
				Series:        "trusty",
				ParentId:      "$addUnit-15",
			},
		},
		Args: map[string]interface{}{
			"container-type": "kvm",
			"parent-id":      "$addUnit-15",
			"series":         "trusty",
		},
		Requires: []string{"addUnit-15"},
	}, {
		Id:     "addMachines-23",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "kvm",
			Series:        "trusty",
			ParentId:      "$addUnit-16",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				ContainerType: "kvm",
				Series:        "trusty",
				ParentId:      "$addUnit-16",
			},
		},
		Args: map[string]interface{}{
			"container-type": "kvm",
			"parent-id":      "$addUnit-16",
			"series":         "trusty",
		},
		Requires: []string{"addUnit-16"},
	}, {
		Id:     "addUnit-8",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-20",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addMachines-20"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-20",
		},
		Requires: []string{"deploy-1", "addMachines-20", "addUnit-7"},
	}, {
		Id:     "addUnit-9",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-21",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addMachines-21"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-21",
		},
		Requires: []string{"deploy-1", "addMachines-21", "addUnit-8"},
	}, {
		Id:     "addUnit-10",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-22",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addMachines-22"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-22",
		},
		Requires: []string{"deploy-1", "addMachines-22", "addUnit-9"},
	}, {
		Id:     "addUnit-11",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-23",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addMachines-23"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-23",
		},
		Requires: []string{"deploy-1", "addMachines-23", "addUnit-10"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestUnitPlacedToMachines(c *gc.C) {
	content := `
        applications:
            django:
                charm: cs:trusty/django-42
                num_units: 5
                to:
                    - new
                    - 4
                    - kvm:8
                    - lxc:new
        machines:
            4:
                constraints: "cpu-cores=4"
            8:
                constraints: "cpu-cores=8"
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:trusty/django-42",
			Series: "trusty",
		},
		GUIArgs: []interface{}{"cs:trusty/django-42", "trusty", ""},
		Args: map[string]interface{}{
			"charm":  "cs:trusty/django-42",
			"series": "trusty",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Series:      "trusty",
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"trusty",
			"django",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"series":      "trusty",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addMachines-2",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Constraints: "cpu-cores=4",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				Constraints: "cpu-cores=4",
			},
		},
		Args: map[string]interface{}{
			"constraints": "cpu-cores=4",
		},
	}, {
		Id:     "addMachines-3",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Constraints: "cpu-cores=8",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				Constraints: "cpu-cores=8",
			},
		},
		Args: map[string]interface{}{
			"constraints": "cpu-cores=8",
		},
	}, {
		Id:     "addMachines-9",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Series: "trusty",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				Series: "trusty",
			},
		},
		Args: map[string]interface{}{
			"series": "trusty",
		},
	}, {
		Id:     "addMachines-10",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "kvm",
			Series:        "trusty",
			ParentId:      "$addMachines-3",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				ContainerType: "kvm",
				Series:        "trusty",
				ParentId:      "$addMachines-3",
			},
		},
		Args: map[string]interface{}{
			"container-type": "kvm",
			"parent-id":      "$addMachines-3",
			"series":         "trusty",
		},
		Requires: []string{"addMachines-3"},
	}, {
		Id:     "addMachines-11",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "lxc",
			Series:        "trusty",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				ContainerType: "lxc",
				Series:        "trusty",
			},
		},
		Args: map[string]interface{}{
			"container-type": "lxc",
			"series":         "trusty",
		},
	}, {
		Id:     "addMachines-12",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "lxc",
			Series:        "trusty",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				ContainerType: "lxc",
				Series:        "trusty",
			},
		},
		Args: map[string]interface{}{
			"container-type": "lxc",
			"series":         "trusty",
		},
	}, {
		Id:     "addUnit-4",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-9",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addMachines-9"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-9",
		},
		Requires: []string{"deploy-1", "addMachines-9"},
	}, {
		Id:     "addUnit-5",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-2",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addMachines-2"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-2",
		},
		Requires: []string{"deploy-1", "addMachines-2", "addUnit-4"},
	}, {
		Id:     "addUnit-6",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-10",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addMachines-10"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-10",
		},
		Requires: []string{"deploy-1", "addMachines-10", "addUnit-5"},
	}, {
		Id:     "addUnit-7",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-11",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addMachines-11"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-11",
		},
		Requires: []string{"deploy-1", "addMachines-11", "addUnit-6"},
	}, {
		Id:     "addUnit-8",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-12",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addMachines-12"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-12",
		},
		Requires: []string{"deploy-1", "addMachines-12", "addUnit-7"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestUnitPlacedToNewMachineWithConstraints(c *gc.C) {
	content := `
        applications:
            django:
                charm: cs:trusty/django-42
                num_units: 1
                to:
                    - new
                constraints: "cpu-cores=4"
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:trusty/django-42",
			Series: "trusty",
		},
		GUIArgs: []interface{}{"cs:trusty/django-42", "trusty", ""},
		Args: map[string]interface{}{
			"charm":  "cs:trusty/django-42",
			"series": "trusty",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Series:      "trusty",
			Constraints: "cpu-cores=4",
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"trusty",
			"django",
			map[string]interface{}{},
			"cpu-cores=4",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"constraints": "cpu-cores=4",
			"series":      "trusty",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addMachines-3",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Constraints: "cpu-cores=4",
			Series:      "trusty",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				Series:      "trusty",
				Constraints: "cpu-cores=4",
			},
		},
		Args: map[string]interface{}{
			"constraints": "cpu-cores=4",
			"series":      "trusty",
		},
	}, {
		Id:     "addUnit-2",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-3",
		},
		GUIArgs: []interface{}{"$deploy-1", "$addMachines-3"},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-3",
		},
		Requires: []string{"deploy-1", "addMachines-3"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestApplicationWithStorage(c *gc.C) {
	content := `
        applications:
            django:
                charm: cs:trusty/django-42
                num_units: 2
                storage:
                    osd-devices: 3,30G
                    tmpfs: tmpfs,1G
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:trusty/django-42",
			Series: "trusty",
		},
		GUIArgs: []interface{}{"cs:trusty/django-42", "trusty", ""},
		Args: map[string]interface{}{
			"charm":  "cs:trusty/django-42",
			"series": "trusty",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Series:      "trusty",
			Storage: map[string]string{
				"osd-devices": "3,30G",
				"tmpfs":       "tmpfs,1G",
			},
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"trusty",
			"django",
			map[string]interface{}{},
			"",
			map[string]string{
				"osd-devices": "3,30G",
				"tmpfs":       "tmpfs,1G",
			},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"series":      "trusty",
			"storage": map[string]interface{}{
				"osd-devices": "3,30G",
				"tmpfs":       "tmpfs,1G",
			},
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addUnit-2",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
		},
		GUIArgs: []interface{}{"$deploy-1", nil},
		Args: map[string]interface{}{
			"application": "$deploy-1",
		},
		Requires: []string{"deploy-1"},
	}, {
		Id:     "addUnit-3",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
		},
		GUIArgs: []interface{}{"$deploy-1", nil},
		Args: map[string]interface{}{
			"application": "$deploy-1",
		},
		Requires: []string{"deploy-1", "addUnit-2"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestApplicationWithDevices(c *gc.C) {
	content := `
        applications:
            django:
                charm: cs:trusty/django-42
                num_units: 2
                devices:
                    description: a nvidia gpu device
                    type: nvidia.com/gpu
                    countmin: 1
                    countmax: 2
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:trusty/django-42",
			Series: "trusty",
		},
		GUIArgs: []interface{}{"cs:trusty/django-42", "trusty", ""},
		Args: map[string]interface{}{
			"charm":  "cs:trusty/django-42",
			"series": "trusty",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Series:      "trusty",
			Devices: map[string]string{
				"description": "a nvidia gpu device",
				"type":        "nvidia.com/gpu",
				"countmin":    "1",
				"countmax":    "2",
			},
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"trusty",
			"django",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{
				"description": "a nvidia gpu device",
				"type":        "nvidia.com/gpu",
				"countmin":    "1",
				"countmax":    "2",
			},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"devices": map[string]interface{}{
				"countmax":    "2",
				"countmin":    "1",
				"description": "a nvidia gpu device",
				"type":        "nvidia.com/gpu",
			},
			"series": "trusty",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addUnit-2",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
		},
		GUIArgs: []interface{}{"$deploy-1", nil},
		Args: map[string]interface{}{
			"application": "$deploy-1",
		},
		Requires: []string{"deploy-1"},
	}, {
		Id:     "addUnit-3",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
		},
		GUIArgs: []interface{}{"$deploy-1", nil},
		Args: map[string]interface{}{
			"application": "$deploy-1",
		},
		Requires: []string{"deploy-1", "addUnit-2"},
	}}

	s.assertParseDataWithDevices(c, content, expected)
}

func (s *changesSuite) TestApplicationWithEndpointBindings(c *gc.C) {
	content := `
        applications:
            django:
                charm: django
                bindings:
                    foo: bar
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "django",
		},
		GUIArgs: []interface{}{"django", "", ""},
		Args: map[string]interface{}{
			"charm": "django",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:            "$addCharm-0",
			Application:      "django",
			EndpointBindings: map[string]string{"foo": "bar"},
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"",
			"django",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{"foo": "bar"},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"endpoint-bindings": map[string]interface{}{
				"foo": "bar",
			},
		},
		Requires: []string{"addCharm-0"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestApplicationWithExposeParams(c *gc.C) {
	content := `
applications:
    django:
      charm: django
--- #overlay
applications:
    django:
      exposed-endpoints:	
        "":
          expose-to-cidrs:
            - 0.0.0.0/0
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "django",
		},
		GUIArgs: []interface{}{"django", "", ""},
		Args: map[string]interface{}{
			"charm": "django",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"",
			"django",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "expose-2",
		Method: "expose",
		Params: bundlechanges.ExposeParams{
			Application: "$deploy-1",
			ExposedEndpoints: map[string]*bundlechanges.ExposedEndpointParams{
				"": {
					ExposeToCIDRs: []string{"0.0.0.0/0"},
				},
			},
		},
		GUIArgs: []interface{}{"$deploy-1", map[string]*bundlechanges.ExposedEndpointParams{
			"": {
				ExposeToCIDRs: []string{"0.0.0.0/0"},
			},
		}},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"exposed-endpoints": map[string]interface{}{
				"": map[string]interface{}{
					"expose-to-cidrs": []interface{}{"0.0.0.0/0"},
				},
			},
		},
		Requires: []string{"deploy-1"},
	}}

	s.assertParseData(c, content, expected)
}
func (s *changesSuite) TestApplicationWithNonDefaultSeriesAndPlacements(c *gc.C) {
	content := `
series: trusty
applications:
    gui3:
        charm: cs:precise/juju-gui
        num_units: 2
        to:
            - new
            - lxc:1
machines:
    1:
   `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  "cs:precise/juju-gui",
			Series: "precise",
		},
		GUIArgs: []interface{}{"cs:precise/juju-gui", "precise", ""},
		Args: map[string]interface{}{
			"charm":  "cs:precise/juju-gui",
			"series": "precise",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "gui3",
			Series:      "precise",
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			"precise",
			"gui3",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Args: map[string]interface{}{
			"application": "gui3",
			"charm":       "$addCharm-0",
			"series":      "precise",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addMachines-2",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Series: "trusty",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				Series: "trusty",
			},
		},
		Args: map[string]interface{}{
			"series": "trusty",
		},
	}, {
		Id:     "addMachines-5",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Series: "precise",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				Series: "precise",
			},
		},
		Args: map[string]interface{}{
			"series": "precise",
		},
	}, {
		Id:       "addMachines-6",
		Method:   "addMachines",
		Requires: []string{"addMachines-2"},
		Params: bundlechanges.AddMachineParams{
			ContainerType: "lxc",
			ParentId:      "$addMachines-2",
			Series:        "precise",
		},
		GUIArgs: []interface{}{
			bundlechanges.AddMachineOptions{
				ContainerType: "lxc",
				ParentId:      "$addMachines-2",
				Series:        "precise",
			},
		},
		Args: map[string]interface{}{
			"container-type": "lxc",
			"parent-id":      "$addMachines-2",
			"series":         "precise",
		},
	}, {
		Id:       "addUnit-3",
		Method:   "addUnit",
		Requires: []string{"deploy-1", "addMachines-5"},
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-5",
		},
		GUIArgs: []interface{}{
			"$deploy-1",
			"$addMachines-5",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-5",
		},
	}, {
		Id:       "addUnit-4",
		Method:   "addUnit",
		Requires: []string{"deploy-1", "addMachines-6", "addUnit-3"},
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-6",
		},
		GUIArgs: []interface{}{
			"$deploy-1",
			"$addMachines-6",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-6",
		},
	}}

	s.assertParseData(c, content, expected)
}

func copyParams(value interface{}) interface{} {
	source := reflect.ValueOf(value).Elem().FieldByName("Params")
	target := reflect.New(source.Type()).Elem()

	for i := 0; i < source.NumField(); i++ {
		// Only copy public fields of the type.
		if targetField := target.Field(i); targetField.CanSet() {
			targetField.Set(source.Field(i))
		}
	}

	return target.Interface()
}

func (s *changesSuite) assertParseData(c *gc.C, content string, expected []record) {
	s.assertParseDataWithModel(c, nil, content, expected)
}

func (s *changesSuite) assertParseDataWithModel(c *gc.C, model *bundlechanges.Model, content string, expected []record) {
	// Retrieve and validate the bundle data merging any overlays in the bundle contents.
	bundleSrc, err := charm.StreamBundleDataSource(strings.NewReader(content), "./")
	c.Assert(err, jc.ErrorIsNil)
	data, err := charm.ReadAndMergeBundleData(bundleSrc)
	c.Assert(err, jc.ErrorIsNil)
	err = data.Verify(nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the changes, and convert them to a sequence of records.
	changes, err := bundlechanges.FromData(bundlechanges.ChangesConfig{
		Model:  model,
		Bundle: data,
		Logger: loggo.GetLogger("bundlechanges"),
	})
	c.Assert(err, jc.ErrorIsNil)
	records := make([]record, len(changes))
	for i, change := range changes {
		args, err := change.Args()
		c.Assert(err, jc.ErrorIsNil)
		r := record{
			Id:       change.Id(),
			Requires: change.Requires(),
			Method:   change.Method(),
			GUIArgs:  change.GUIArgs(),
			Args:     args,
			Params:   copyParams(change),
		}
		records[i] = r
		c.Log(change.Description())
	}

	// Output the records for debugging.
	b, err := json.MarshalIndent(records, "", "  ")
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("obtained records: %s", b)

	// Check that the obtained records are what we expect.
	c.Check(records, jc.DeepEquals, expected)
}

func (s *changesSuite) assertParseDataWithDevices(c *gc.C, content string, expected []record) {
	// Retrieve and validate the bundle data.
	data, err := charm.ReadBundleData(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	err = data.Verify(nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the changes, and convert them to a sequence of records.
	changes, err := bundlechanges.FromData(bundlechanges.ChangesConfig{
		Bundle: data,
		Logger: loggo.GetLogger("bundlechanges"),
	})
	c.Assert(err, jc.ErrorIsNil)
	records := make([]record, len(changes))
	for i, change := range changes {
		var guiArgs []interface{}
		switch change := change.(type) {
		case *bundlechanges.AddApplicationChange:
			guiArgs = change.GUIArgsWithDevices()
		default:
			guiArgs = change.GUIArgs()
		}
		args, err := change.Args()
		c.Assert(err, jc.ErrorIsNil)
		r := record{
			Id:       change.Id(),
			Requires: change.Requires(),
			Method:   change.Method(),
			GUIArgs:  guiArgs,
			Args:     args,
			Params:   copyParams(change),
		}
		records[i] = r
		c.Log(change.Description())
	}

	// Output the records for debugging.
	b, err := json.MarshalIndent(records, "", "  ")
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("obtained records: %s", b)

	// Check that the obtained records are what we expect.
	c.Check(records, jc.DeepEquals, expected)
}

func (s *changesSuite) assertLocalBundleChanges(c *gc.C, charmDir, bundleContent, series string) {
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  charmDir,
			Series: series,
		},
		GUIArgs: []interface{}{charmDir, series, ""},
		Args: map[string]interface{}{
			"charm":  charmDir,
			"series": series,
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Series:      series,
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			series,
			"django",
			map[string]interface{}{}, // options.
			"",                       // constraints.
			map[string]string{},      // storage.
			map[string]string{},      // endpoint bindings.
			map[string]int{},         // resources.
			0,                        // num_units.
			"",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"series":      series,
		},
		Requires: []string{"addCharm-0"},
	}}
	s.assertParseData(c, bundleContent, expected)
}

func (s *changesSuite) assertLocalBundleChangesWithDevices(c *gc.C, charmDir, bundleContent, series string) {
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:  charmDir,
			Series: series,
		},
		GUIArgs: []interface{}{charmDir, series, ""},
		Args: map[string]interface{}{
			"charm":  charmDir,
			"series": series,
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Series:      series,
		},
		GUIArgs: []interface{}{
			"$addCharm-0",
			series,
			"django",
			map[string]interface{}{}, // options.
			"",                       // constraints.
			map[string]string{},      // storage.
			map[string]string{},      // devices.
			map[string]string{},      // endpoint bindings.
			map[string]int{},         // resources.
			0,                        // num_units.
			"",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"series":      series,
		},
		Requires: []string{"addCharm-0"},
	}}
	s.assertParseDataWithDevices(c, bundleContent, expected)
}

func (s *changesSuite) TestLocalCharmWithExplicitSeries(c *gc.C) {
	charmDir := c.MkDir()
	bundleContent := fmt.Sprintf(`
        applications:
            django:
                charm: %s
                series: xenial
    `, charmDir)
	s.assertLocalBundleChanges(c, charmDir, bundleContent, "xenial")
	s.assertLocalBundleChangesWithDevices(c, charmDir, bundleContent, "xenial")
}

func (s *changesSuite) TestLocalCharmWithSeriesFromCharm(c *gc.C) {
	charmDir := c.MkDir()
	bundleContent := fmt.Sprintf(`
        applications:
            django:
                charm: %s
    `, charmDir)
	charmMeta := `
name: multi-series
summary: That's a dummy charm with multi-series.
description: A dummy charm.
series:
    - precise
    - trusty
`[1:]
	err := ioutil.WriteFile(filepath.Join(charmDir, "metadata.yaml"), []byte(charmMeta), 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.assertLocalBundleChanges(c, charmDir, bundleContent, "precise")
	s.assertLocalBundleChangesWithDevices(c, charmDir, bundleContent, "precise")
}

func (s *changesSuite) TestLocalCharmWithSeriesFromBundle(c *gc.C) {
	charmDir := c.MkDir()
	bundleContent := fmt.Sprintf(`
        series: bionic
        applications:
            django:
                charm: %s
    `, charmDir)
	charmMeta := `
name: multi-series
summary: That's a dummy charm with multi-series.
description: A dummy charm.
series:
    - precise
    - trusty
`[1:]
	err := ioutil.WriteFile(filepath.Join(charmDir, "metadata.yaml"), []byte(charmMeta), 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.assertLocalBundleChanges(c, charmDir, bundleContent, "bionic")
	s.assertLocalBundleChangesWithDevices(c, charmDir, bundleContent, "bionic")
}

func (s *changesSuite) TestSimpleBundleEmptyModel(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        expose: true
                        num_units: 1
                        options:
                            key-1: value-1
                            key-2: value-2
                        annotations:
                            gui-x: "10"
                            gui-y: "50"
            `
	expectedChanges := []string{
		"upload charm django from charm-store",
		"deploy application django from charm-store",
		"expose all endpoints of django and allow access from CIDRs 0.0.0.0/0 and ::/0",
		"set annotations for django",
		"add unit django/0 to new machine 0",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestKubernetesBundleEmptyModel(c *gc.C) {
	bundleContent := `
                bundle: kubernetes
                applications:
                    django:
                        charm: cs:django-4
                        expose: yes
                        num_units: 1
                        options:
                            key-1: value-1
                            key-2: value-2
                        annotations:
                            gui-x: "10"
                            gui-y: "50"
                    mariadb:
                        charm: cs:mariadb-5
                        num_units: 2
            `
	expectedChanges := []string{
		"upload charm django from charm-store for series kubernetes",
		"deploy application django from charm-store with 1 unit on kubernetes",
		"expose all endpoints of django and allow access from CIDRs 0.0.0.0/0 and ::/0",
		"set annotations for django",
		"upload charm mariadb from charm-store for series kubernetes",
		"deploy application mariadb from charm-store with 2 units on kubernetes",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestCharmInUseByAnotherApplication(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        num_units: 1
                        expose: yes
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"other-app": {
				Charm: "cs:django-4",
			},
		},
	}
	expectedChanges := []string{
		"deploy application django from charm-store",
		"expose all endpoints of django and allow access from CIDRs 0.0.0.0/0 and ::/0",
		"add unit django/0 to new machine 0",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestExposeOverlayParameters(c *gc.C) {
	bundleContent := `
bundle: kubernetes
applications:
    django:
      charm: cs:django-4
      num_units: 2
--- #overlay
applications:
    django:
      exposed-endpoints:	
        "":
          expose-to-cidrs:
            - 0.0.0.0/0
        www:
          expose-to-cidrs:
            - 13.37.0.0/16
            - 192.168.0.0/16
        admin:
          expose-to-cidrs:
            - 13.37.0.0/16
            - 192.168.0.0/16
        dmz:
          expose-to-spaces:
            - public
          expose-to-cidrs:
            - 13.37.0.0/16
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm:   "cs:django-4",
				Scale:   3,
				Exposed: true,
				Series:  "kubernetes",
			},
		},
	}
	expectedChanges := []string{
		"expose all endpoints of django and allow access from CIDR 0.0.0.0/0",
		"override expose settings for endpoints admin,www of django and allow access from CIDRs 13.37.0.0/16,192.168.0.0/16",
		"override expose settings for endpoint dmz of django and allow access from space public and CIDR 13.37.0.0/16",
		"scale django to 2 units",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestExposeOverlayParametersForNonCurrentlyExposedApp(c *gc.C) {
	// Here, we expose a single endpoint for an application that is NOT
	// currently exposed. The change description should be slightly
	// different to indicate to the operator that the application is marked
	// as "exposed".
	bundleContent := `
bundle: kubernetes
applications:
    django:
      charm: cs:django-4
      num_units: 2
--- #overlay
applications:
    django:
      exposed-endpoints:	
        www:
          expose-to-cidrs:
            - 13.37.0.0/16
            - 192.168.0.0/16
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm:   "cs:django-4",
				Scale:   3,
				Exposed: false,
				Series:  "kubernetes",
			},
		},
	}
	expectedChanges := []string{
		"override expose settings for endpoint www of django and allow access from CIDRs 13.37.0.0/16,192.168.0.0/16",
		"scale django to 2 units",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestExposeOverlayParametersWithOnlyWildcardEntry(c *gc.C) {
	bundleContent := `
bundle: kubernetes
applications:
    django:
      charm: cs:django-4
      num_units: 2
--- #overlay
applications:
    django:
      exposed-endpoints:	
        "":
          expose-to-cidrs:
            - 13.37.0.0/16
            - 192.168.0.0/16
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm:   "cs:django-4",
				Scale:   3,
				Exposed: true,
				Series:  "kubernetes",
			},
		},
	}
	expectedChanges := []string{
		"expose all endpoints of django and allow access from CIDRs 13.37.0.0/16,192.168.0.0/16",
		"scale django to 2 units",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestCharmUpgrade(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-6
                        num_units: 1
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm: "cs:django-4",
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
				},
			},
		},
	}
	expectedChanges := []string{
		"upload charm django from charm-store",
		"upgrade django from charm-store using charm django",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestCharmUpgradeWithChannel(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-6
                        channel: stable
                        num_units: 1
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm: "cs:django-4",
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
				},
			},
		},
	}
	expectedChanges := []string{
		"upload charm django from charm-store from channel stable",
		"upgrade django from charm-store using charm django from channel stable",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestCharmUpgradeWithExistingChannel(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-6
                        channel: stable
                        num_units: 1
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm:   "cs:django-4",
				Channel: "edge",
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
				},
			},
		},
	}
	expectedChanges := []string{
		"upload charm django from charm-store from channel stable",
		"upgrade django from charm-store using charm django from channel stable",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestCharmUpgradeWithCharmhubCharmAndExistingChannel(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        channel: stable
                        num_units: 1
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm:    "ch:django",
				Channel:  "stable",
				Revision: 1,
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
				},
			},
		},
	}
	expectedChanges := []string{
		"upgrade django from charm-hub using charm django from channel stable",
	}
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(string, string, string, string) (string, int, error) {
		return "stable", 42, nil
	})
}

func (s *changesSuite) TestAppExistsWithLessUnits(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        num_units: 2
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm: "cs:django-4",
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			// We don't actually look at the content of the machines
			// for this test, just the keys.
			"0": nil,
		},
	}
	expectedChanges := []string{
		"add unit django/1 to new machine 1",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestAppExistsWithDifferentScale(c *gc.C) {
	bundleContent := `
                bundle: kubernetes
                applications:
                    django:
                        charm: cs:django-4
                        num_units: 2
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm:  "cs:django-4",
				Scale:  3,
				Series: "kubernetes",
			},
		},
	}
	expectedChanges := []string{
		"scale django to 2 units",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestAppExistsMissingSeriesWithDifferentScale(c *gc.C) {
	bundleContent := `
                bundle: kubernetes
                applications:
                    django:
                        charm: cs:django-4
                        num_units: 2
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm: "cs:django-4",
				Scale: 3,
			},
		},
	}
	expectedChanges := []string{
		"upload charm django from charm-store for series kubernetes",
		"scale django to 2 units",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestNewMachineNumberHigherUnitHigher(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        num_units: 2
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm: "cs:django-4",
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			// We don't actually look at the content of the machines
			// for this test, just the keys.
			"0": nil,
		},
		Sequence: map[string]int{
			"application-django": 2,
			"machine":            3,
		},
	}
	expectedChanges := []string{
		"add unit django/2 to new machine 3",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestAppWithDifferentConstraints(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        constraints: cpu-cores=4 cpu-power=42
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm: "cs:django-4",
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
				},
			},
		},
		ConstraintsEqual: func(string, string) bool {
			return false
		},
		Machines: map[string]*bundlechanges.Machine{
			// We don't actually look at the content of the machines
			// for this test, just the keys.
			"0": nil,
		},
	}
	expectedChanges := []string{
		`set constraints for django to "cpu-cores=4 cpu-power=42"`,
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestAppsWithArchConstraints(c *gc.C) {
	bundleContent := `
                applications:
                    django-1:
                        charm: cs:django-4
                        constraints: arch=amd64 cpu-cores=4 cpu-power=42
                    django-2:
                        charm: cs:django-4
                        constraints: arch=s390x cpu-cores=4 cpu-power=42
            `
	expectedChanges := []string{
		"upload charm django from charm-store with architecture=amd64",
		"deploy application django-1 from charm-store using django",
		"upload charm django from charm-store with architecture=s390x",
		"deploy application django-2 from charm-store using django",
	}
	s.checkBundleWithConstraintsParser(c, bundleContent, expectedChanges, constraintParser)
}

func (s *changesSuite) TestExistingAppsWithArchConstraints(c *gc.C) {
	bundleContent := `
                applications:
                    django-1:
                        charm: cs:django-4
                        constraints: arch=amd64 cpu-cores=4 cpu-power=42
                    django-2:
                        charm: cs:django-4
                        constraints: arch=s390x cpu-cores=4 cpu-power=42
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django-1": {
				Charm: "cs:django-4",
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
				},
				Constraints: "arch=amd64",
			},
		},
		ConstraintsEqual: func(string, string) bool {
			return false
		},
		Machines: map[string]*bundlechanges.Machine{
			// We don't actually look at the content of the machines
			// for this test, just the keys.
			"0": nil,
		},
	}
	expectedChanges := []string{
		`set constraints for django-1 to "arch=amd64 cpu-cores=4 cpu-power=42"`,
		"upload charm django from charm-store with architecture=s390x",
		"deploy application django-2 from charm-store using django",
	}
	s.checkBundleExistingModelWithConstraintsParser(c, bundleContent, existingModel, expectedChanges, constraintParser)
}

func (s *changesSuite) TestExistingAppsWithoutArchConstraints(c *gc.C) {
	bundleContent := `
                applications:
                    django-1:
                        charm: cs:django-4
                        constraints: arch=amd64 cpu-cores=4 cpu-power=42
                    django-2:
                        charm: cs:django-4
                        constraints: arch=s390x cpu-cores=4 cpu-power=42
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django-1": {
				Charm: "cs:django-4",
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
				},
				Constraints: "",
			},
		},
		ConstraintsEqual: func(string, string) bool {
			return false
		},
		Machines: map[string]*bundlechanges.Machine{
			// We don't actually look at the content of the machines
			// for this test, just the keys.
			"0": nil,
		},
	}
	expectedChanges := []string{
		"upload charm django from charm-store with architecture=amd64",
		`set constraints for django-1 to "arch=amd64 cpu-cores=4 cpu-power=42"`,
		"upload charm django from charm-store with architecture=s390x",
		"deploy application django-2 from charm-store using django",
	}
	s.checkBundleExistingModelWithConstraintsParser(c, bundleContent, existingModel, expectedChanges, constraintParser)
}

func (s *changesSuite) TestAppsWithSeriesAndArchConstraints(c *gc.C) {
	bundleContent := `
                applications:
                    django-1:
                        charm: cs:django-4
                        constraints: arch=amd64 cpu-cores=4 cpu-power=42
                        series: bionic
                    django-2:
                        charm: cs:django-4
                        constraints: arch=s390x cpu-cores=4 cpu-power=42
                        series: bionic
                    django-3:
                        charm: cs:django-4
                        constraints: arch=s390x cpu-cores=4 cpu-power=42
                        series: focal
            `
	expectedChanges := []string{
		"upload charm django from charm-store for series bionic with architecture=amd64",
		"deploy application django-1 from charm-store on bionic using django",
		"upload charm django from charm-store for series bionic with architecture=s390x",
		"deploy application django-2 from charm-store on bionic using django",
		"upload charm django from charm-store for series focal with architecture=s390x",
		"deploy application django-3 from charm-store on focal using django",
	}
	s.checkBundleWithConstraintsParser(c, bundleContent, expectedChanges, constraintParser)
}

func (s *changesSuite) TestAppWithArchConstraints(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        constraints: arch=amd64 cpu-cores=4 cpu-power=42
            `
	expectedChanges := []string{
		"upload charm django from charm-store with architecture=amd64",
		"deploy application django from charm-store",
	}
	s.checkBundleWithConstraintsParser(c, bundleContent, expectedChanges, constraintParser)
}

func (s *changesSuite) TestAppWithArchConstraintsWithError(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        constraints: arch=amd64 cpu-cores=4 cpu-power=42
            `
	s.checkBundleWithConstraintsParserError(c, bundleContent, "bad", constraintParserWithError(errors.Errorf("bad")))
}

func (s *changesSuite) TestAppWithArchConstraintsWithNoParser(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        constraints: arch=amd64 cpu-cores=4 cpu-power=42
            `
	expectedChanges := []string{
		"upload charm django from charm-store",
		"deploy application django from charm-store",
	}
	s.checkBundleWithConstraintsParser(c, bundleContent, expectedChanges, nil)
}

func (s *changesSuite) TestAppExistsWithEnoughUnits(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        num_units: 2
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm: "cs:django-4",
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
					{"django/1", "1"},
					{"django/2", "2"},
				},
			},
		},
	}
	expectedChanges := []string{}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestAppExistsWithChangedOptionsAndAnnotations(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        num_units: 1
                        options:
                            key-1: value-1
                            key-2: value-2
                        annotations:
                            gui-x: "10"
                            gui-y: "50"
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm: "cs:django-4",
				Options: map[string]interface{}{
					"key-1": "value-1",
					"key-2": "value-4",
					"key-3": "value-5",
				},
				Annotations: map[string]string{
					"gui-x": "10",
					"gui-y": "40",
				},
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
				},
			},
		},
	}
	expectedChanges := []string{
		"set application options for django",
		"set annotations for django",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestNewMachineAnnotationsAndPlacement(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        exposed: true
                        num_units: 1
                        to: [1]
                machines:
                    1:
                        annotations:
                            foo: "10"
                            bar: "50"
            `
	expectedChanges := []string{
		"upload charm django from charm-store",
		"deploy application django from charm-store",
		"add new machine 0 (bundle machine 1)",
		"set annotations for new machine 0",
		"add unit django/0 to new machine 0",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestFinalPlacementNotReusedIfSpecifiesMachine(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        num_units: 2
                        to: [1]
                machines:
                    1:
            `
	expectedChanges := []string{
		"upload charm django from charm-store",
		"deploy application django from charm-store",
		"add new machine 0 (bundle machine 1)",
		"add unit django/0 to new machine 0",
		// NOTE: new machine, not put on $1.
		"add unit django/1 to new machine 1",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestFinalPlacementNotReusedIfSpecifiesUnit(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        num_units: 1
                    nginx:
                        charm: cs:nginx
                        num_units: 2
                        to: ["django/0"]
            `
	expectedChanges := []string{
		"upload charm django from charm-store",
		"deploy application django from charm-store",
		"upload charm nginx from charm-store",
		"deploy application nginx from charm-store",
		"add unit django/0 to new machine 0",
		"add unit nginx/0 to new machine 0 to satisfy [django/0]",
		// NOTE: new machine, not put on $0.
		"add unit nginx/1 to new machine 1",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestUnitPlaceNextToOtherNewUnitOnExistingMachine(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        num_units: 1
                        to: [1]
                    nginx:
                        charm: cs:nginx
                        num_units: 1
                        to: ["django/0"]
                machines:
                    1:
            `
	existingModel := &bundlechanges.Model{
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
		},
		MachineMap: map[string]string{"1": "0"},
	}
	expectedChanges := []string{
		"upload charm django from charm-store",
		"deploy application django from charm-store",
		"upload charm nginx from charm-store",
		"deploy application nginx from charm-store",
		"add unit django/0 to existing machine 0",
		"add unit nginx/0 to existing machine 0 to satisfy [django/0]",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestApplicationPlacementNotEnoughUnits(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        num_units: 3
                    nginx:
                        charm: cs:nginx
                        num_units: 5
                        to: [django]
            `
	expectedChanges := []string{
		"upload charm django from charm-store",
		"deploy application django from charm-store",
		"upload charm nginx from charm-store",
		"deploy application nginx from charm-store",
		"add unit django/0 to new machine 0",
		"add unit django/1 to new machine 1",
		"add unit django/2 to new machine 2",
		"add unit nginx/0 to new machine 0 to satisfy [django]",
		"add unit nginx/1 to new machine 1 to satisfy [django]",
		"add unit nginx/2 to new machine 2 to satisfy [django]",
		"add unit nginx/3 to new machine 3",
		"add unit nginx/4 to new machine 4",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestApplicationPlacementSomeExisting(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        num_units: 5
                    nginx:
                        charm: cs:nginx
                        num_units: 5
                        to: [django]
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm: "cs:django-4",
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
					{"django/1", "1"},
					{"django/3", "3"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			// We don't actually look at the content of the machines
			// for this test, just the keys.
			"0": nil, "1": nil, "3": nil,
		},
	}
	expectedChanges := []string{
		"upload charm nginx from charm-store",
		"deploy application nginx from charm-store",
		"add unit django/4 to new machine 4",
		"add unit django/5 to new machine 5",
		"add unit nginx/0 to existing machine 0 to satisfy [django]",
		"add unit nginx/1 to existing machine 1 to satisfy [django]",
		"add unit nginx/2 to existing machine 3 to satisfy [django]",
		"add unit nginx/3 to new machine 4 to satisfy [django]",
		"add unit nginx/4 to new machine 5 to satisfy [django]",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestApplicationPlacementSomeColocated(c *gc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: cs:django-4
                        num_units: 5
                    nginx:
                        charm: cs:nginx
                        num_units: 5
                        to: [django]
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm: "cs:django-4",
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
					{"django/1", "1"},
					{"django/3", "3"},
				},
			},
			"nginx": {
				Charm: "cs:nginx",
				Units: []bundlechanges.Unit{
					{"nginx/0", "0"},
					{"nginx/1", "1"},
					{"nginx/2", "4"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			// We don't actually look at the content of the machines
			// for this test, just the keys.
			"0": nil, "1": nil, "3": nil, "4": nil,
		},
	}
	expectedChanges := []string{
		"add unit django/4 to new machine 5",
		"add unit django/5 to new machine 6",
		"add unit nginx/3 to existing machine 3 to satisfy [django]",
		"add unit nginx/4 to new machine 5 to satisfy [django]",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestWeirdUnitDeployedNoExistingModel(c *gc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: cs:mysql
                        num_units: 3
                        # The first placement directive here is skipped because
                        # the existing model already has one unit.
                        to: [new, "lxd:0", "lxd:new"]
                    keystone:
                        charm: cs:keystone
                        num_units: 3
                        to: ["lxd:mysql"]
                machines:
                    0:
            `
	expectedChanges := []string{
		"upload charm keystone from charm-store",
		"deploy application keystone from charm-store",
		"upload charm mysql from charm-store",
		"deploy application mysql from charm-store",
		"add new machine 0",
		"add new machine 1",
		"add lxd container 0/lxd/0 on new machine 0",
		"add lxd container 2/lxd/0 on new machine 2",
		"add unit mysql/0 to new machine 1",
		"add unit mysql/1 to 0/lxd/0",
		"add unit mysql/2 to 2/lxd/0",
		"add lxd container 1/lxd/0 on new machine 1",
		"add lxd container 0/lxd/1 on new machine 0",
		"add lxd container 2/lxd/1 on new machine 2",
		"add unit keystone/0 to 1/lxd/0 to satisfy [lxd:mysql]",
		"add unit keystone/1 to 0/lxd/1 to satisfy [lxd:mysql]",
		"add unit keystone/2 to 2/lxd/1 to satisfy [lxd:mysql]",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestUnitDeployedDefinedMachine(c *gc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: cs:mysql
                        num_units: 3
                        to: [new, "lxd:0", "lxd:new"]
                    keystone:
                        charm: cs:keystone
                        num_units: 3
                        to: ["lxd:mysql"]
                machines:
                    0:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"mysql": {
				Charm: "cs:mysql",
				Units: []bundlechanges.Unit{
					{"mysql/0", "0/lxd/0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0":       {ID: "0"},
			"0/lxd/0": {ID: "0/lxd/0"},
		},
	}
	expectedChanges := []string{
		"upload charm keystone from charm-store",
		"deploy application keystone from charm-store",
		"add unit keystone/0 to 0/lxd/1 to satisfy [lxd:mysql]",
		"add new machine 1",
		"add lxd container 2/lxd/0 on new machine 2",
		"add unit mysql/1 to new machine 1",
		"add unit mysql/2 to 2/lxd/0",
		"add lxd container 1/lxd/0 on new machine 1",
		"add lxd container 2/lxd/1 on new machine 2",
		"add unit keystone/1 to 1/lxd/0 to satisfy [lxd:mysql]",
		"add unit keystone/2 to 2/lxd/1 to satisfy [lxd:mysql]",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestLXDContainerSequence(c *gc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: cs:mysql
                        num_units: 1
                    keystone:
                        charm: cs:keystone
                        num_units: 1
                        to: ["lxd:mysql"]
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"mysql": {
				Charm: "cs:mysql",
				Units: []bundlechanges.Unit{
					{"mysql/0", "0/lxd/0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			// We don't actually look at the content of the machines
			// for this test, just the keys.
			"0": nil, "0/lxd/0": nil,
		},
		Sequence: map[string]int{
			"application-mysql": 1,
			"machine":           1,
			"machine-0/lxd":     2,
		},
	}
	expectedChanges := []string{
		"upload charm keystone from charm-store",
		"deploy application keystone from charm-store",
		"add unit keystone/0 to 0/lxd/2 to satisfy [lxd:mysql]",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestMachineMapToExistingMachineSomeDeployed(c *gc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: cs:mysql
                        num_units: 3
                        # The first placement directive here is skipped because
                        # the existing model already has one unit.
                        to: [new, "lxd:0", "lxd:new"]
                    keystone:
                        charm: cs:keystone
                        num_units: 3
                        to: ["lxd:mysql"]
                machines:
                    0:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"mysql": {
				Charm: "cs:mysql",
				Units: []bundlechanges.Unit{
					{"mysql/0", "0/lxd/0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0":       {ID: "0"},
			"0/lxd/0": {ID: "0/lxd/0"},
			"2":       {ID: "2"},
			"2/lxd/0": {ID: "2/lxd/0"},
		},
		MachineMap: map[string]string{
			"0": "2", // 0 in bundle is machine 2 in existing.
		},
	}
	expectedChanges := []string{
		"upload charm keystone from charm-store",
		"deploy application keystone from charm-store",
		// First unit of keystone goes in a container next to the existing mysql.
		"add unit keystone/0 to 0/lxd/1 to satisfy [lxd:mysql]",
		// Two more units of mysql are needed, and the "lxd:0" is unsatisfied
		// because machine 0 has been mapped to machine 2, and mysql isn't on machine 2.
		// Due to this, the placements directives are popped off as needed,
		// First one is "new", second is "lxd:0", and since 0 is mapped to 2, the lxd
		// is created on machine 2.
		"add new machine 3",
		"add unit mysql/1 to new machine 3",
		"add unit mysql/2 to 2/lxd/1",
		// Next, units of keystone go next to the new mysql units.
		"add lxd container 3/lxd/0 on new machine 3",
		"add lxd container 2/lxd/2 on existing machine 2",
		"add unit keystone/1 to 3/lxd/0 to satisfy [lxd:mysql]",
		"add unit keystone/2 to 2/lxd/2 to satisfy [lxd:mysql]",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestSettingAnnotationsForExistingMachine(c *gc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: cs:mysql
                        num_units: 1
                        to: ["0"]
                machines:
                    0:
                        annotations:
                            key: value
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"mysql": {
				Charm: "cs:mysql",
				Units: []bundlechanges.Unit{
					{"mysql/0", "0/lxd/0"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0":       {ID: "0"},
			"0/lxd/0": {ID: "0/lxd/0"},
			"2":       {ID: "2"},
		},
		MachineMap: map[string]string{
			"0": "2", // 0 in bundle is machine 2 in existing.
		},
	}
	expectedChanges := []string{
		"set annotations for existing machine 2",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestSiblingContainers(c *gc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: cs:mysql
                        num_units: 3
                        to: ["lxd:new"]
                    keystone:
                        charm: cs:keystone
                        num_units: 3
                        to: ["lxd:mysql"]
            `
	expectedChanges := []string{
		"upload charm keystone from charm-store",
		"deploy application keystone from charm-store",
		"upload charm mysql from charm-store",
		"deploy application mysql from charm-store",
		"add lxd container 0/lxd/0 on new machine 0",
		"add lxd container 1/lxd/0 on new machine 1",
		"add lxd container 2/lxd/0 on new machine 2",
		"add unit mysql/0 to 0/lxd/0",
		"add unit mysql/1 to 1/lxd/0",
		"add unit mysql/2 to 2/lxd/0",
		"add lxd container 0/lxd/1 on new machine 0",
		"add lxd container 1/lxd/1 on new machine 1",
		"add lxd container 2/lxd/1 on new machine 2",
		"add unit keystone/0 to 0/lxd/1 to satisfy [lxd:mysql]",
		"add unit keystone/1 to 1/lxd/1 to satisfy [lxd:mysql]",
		"add unit keystone/2 to 2/lxd/1 to satisfy [lxd:mysql]",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestSiblingContainersSomeDeployed(c *gc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: cs:mysql
                        num_units: 3
                        to: ["lxd:new"]
                    keystone:
                        charm: cs:keystone
                        num_units: 4
                        to: ["lxd:mysql"]
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"mysql": {
				Charm: "cs:mysql",
				Units: []bundlechanges.Unit{
					{"mysql/0", "0/lxd/0"},
					{"mysql/1", "1/lxd/0"},
					{"mysql/2", "2/lxd/0"},
				},
			},
			"keystone": {
				Charm: "cs:keystone",
				Units: []bundlechanges.Unit{
					{"keystone/0", "0/lxd/1"},
					{"keystone/2", "2/lxd/1"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0":       {ID: "0"},
			"0/lxd/0": {ID: "0/lxd/0"},
			"0/lxd/1": {ID: "0/lxd/1"},
			"1":       {ID: "1"},
			"1/lxd/0": {ID: "1/lxd/0"},
			"2":       {ID: "2"},
			"2/lxd/0": {ID: "2/lxd/0"},
			"2/lxd/1": {ID: "2/lxd/1"},
		},
		Sequence: map[string]int{
			"machine":              3,
			"application-keystone": 3,
			"machine-1/lxd":        2,
		},
	}
	expectedChanges := []string{
		"add unit keystone/3 to 1/lxd/2 to satisfy [lxd:mysql]",
		// TODO: this should really be 3/lxd/0 as fallback should
		// be "lxd:new", not "new"
		"add unit keystone/4 to new machine 3",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestColocationIntoAContainerUsingUnitPlacement(c *gc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: cs:mysql
                        num_units: 3
                        to: ["lxd:new"]
                    keystone:
                        charm: cs:keystone
                        num_units: 3
                        to: [mysql/0, mysql/1, mysql/2]
            `
	expectedChanges := []string{
		"upload charm keystone from charm-store",
		"deploy application keystone from charm-store",
		"upload charm mysql from charm-store",
		"deploy application mysql from charm-store",
		"add lxd container 0/lxd/0 on new machine 0",
		"add lxd container 1/lxd/0 on new machine 1",
		"add lxd container 2/lxd/0 on new machine 2",
		"add unit mysql/0 to 0/lxd/0",
		"add unit mysql/1 to 1/lxd/0",
		"add unit mysql/2 to 2/lxd/0",
		"add unit keystone/0 to 0/lxd/0 to satisfy [mysql/0]",
		"add unit keystone/1 to 1/lxd/0 to satisfy [mysql/1]",
		"add unit keystone/2 to 2/lxd/0 to satisfy [mysql/2]",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestColocationIntoAContainerUsingAppPlacement(c *gc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: cs:mysql
                        num_units: 3
                        to: ["lxd:new"]
                    keystone:
                        charm: cs:keystone
                        num_units: 3
                        to: ["mysql"]
            `
	expectedChanges := []string{
		"upload charm keystone from charm-store",
		"deploy application keystone from charm-store",
		"upload charm mysql from charm-store",
		"deploy application mysql from charm-store",
		"add lxd container 0/lxd/0 on new machine 0",
		"add lxd container 1/lxd/0 on new machine 1",
		"add lxd container 2/lxd/0 on new machine 2",
		"add unit mysql/0 to 0/lxd/0",
		"add unit mysql/1 to 1/lxd/0",
		"add unit mysql/2 to 2/lxd/0",
		"add unit keystone/0 to 0/lxd/0 to satisfy [mysql]",
		"add unit keystone/1 to 1/lxd/0 to satisfy [mysql]",
		"add unit keystone/2 to 2/lxd/0 to satisfy [mysql]",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestPlacementDescriptionsForUnitPlacement(c *gc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: cs:mysql
                        num_units: 3
                    keystone:
                        charm: cs:keystone
                        num_units: 3
                        to: ["lxd:mysql/0", "lxd:mysql/1", "lxd:mysql/2"]
            `
	expectedChanges := []string{
		"upload charm keystone from charm-store",
		"deploy application keystone from charm-store",
		"upload charm mysql from charm-store",
		"deploy application mysql from charm-store",
		"add unit mysql/0 to new machine 0",
		"add unit mysql/1 to new machine 1",
		"add unit mysql/2 to new machine 2",
		"add lxd container 0/lxd/0 on new machine 0",
		"add lxd container 1/lxd/0 on new machine 1",
		"add lxd container 2/lxd/0 on new machine 2",
		"add unit keystone/0 to 0/lxd/0 to satisfy [lxd:mysql/0]",
		"add unit keystone/1 to 1/lxd/0 to satisfy [lxd:mysql/1]",
		"add unit keystone/2 to 2/lxd/0 to satisfy [lxd:mysql/2]",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestMostAppOptions(c *gc.C) {
	bundleContent := `
                applications:
                    mediawiki:
                        charm: cs:precise/mediawiki-10
                        num_units: 1
                        expose: true
                        options:
                            debug: false
                        annotations:
                            gui-x: "609"
                            gui-y: "-15"
                        resources:
                            data: 3
                    mysql:
                        charm: cs:precise/mysql-28
                        num_units: 1
                        resources:
                          data: "./resources/data.tar"
                series: trusty
                relations:
                    - - mediawiki:db
                      - mysql:db
            `
	expectedChanges := []string{
		"upload charm mediawiki from charm-store for series precise",
		"deploy application mediawiki from charm-store on precise",
		"expose all endpoints of mediawiki and allow access from CIDRs 0.0.0.0/0 and ::/0",
		"set annotations for mediawiki",
		"upload charm mysql from charm-store for series precise",
		"deploy application mysql from charm-store on precise",
		"add relation mediawiki:db - mysql:db",
		"add unit mediawiki/0 to new machine 0",
		"add unit mysql/0 to new machine 1",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestUnitOrdering(c *gc.C) {
	bundleContent := `
                applications:
                    memcached:
                        charm: cs:xenial/mem-47
                        num_units: 3
                        to: [1, 2, 3]
                    django:
                        charm: cs:xenial/django-42
                        num_units: 4
                        to:
                            - 1
                            - lxd:memcached
                    ror:
                        charm: cs:rails
                        num_units: 3
                        to:
                            - 1
                            - kvm:3
                machines:
                    1:
                    2:
                    3:
            `
	expectedChanges := []string{
		"upload charm django from charm-store for series xenial",
		"deploy application django from charm-store on xenial",
		"upload charm mem from charm-store for series xenial",
		"deploy application memcached from charm-store on xenial using mem",
		"upload charm rails from charm-store",
		"deploy application ror from charm-store using rails",
		"add new machine 0 (bundle machine 1)",
		"add new machine 1 (bundle machine 2)",
		"add new machine 2 (bundle machine 3)",
		"add unit django/0 to new machine 0",
		"add unit memcached/0 to new machine 0",
		"add unit memcached/1 to new machine 1",
		"add unit memcached/2 to new machine 2",
		"add unit ror/0 to new machine 0",
		"add kvm container 2/kvm/0 on new machine 2",
		"add lxd container 0/lxd/0 on new machine 0",
		"add lxd container 1/lxd/0 on new machine 1",
		"add lxd container 2/lxd/0 on new machine 2",
		"add unit django/1 to 0/lxd/0 to satisfy [lxd:memcached]",
		"add unit django/2 to 1/lxd/0 to satisfy [lxd:memcached]",
		"add unit django/3 to 2/lxd/0 to satisfy [lxd:memcached]",
		"add unit ror/1 to 2/kvm/0",
		"add unit ror/2 to new machine 3",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestMachineNaturalSorting(c *gc.C) {
	bundleContent := `
                applications:
                    ubu:
                        charm: cs:ubuntu
                        num_units: 13
                        to: [0,1,2,3,4,5,6,7,8,9,10,11,12]
                machines:
                    0:
                    1:
                    2:
                    3:
                    4:
                    5:
                    6:
                    7:
                    8:
                    9:
                    10:
                    11:
                    12:
            `
	expectedChanges := []string{
		"upload charm ubuntu from charm-store",
		"deploy application ubu from charm-store using ubuntu",
		"add new machine 0",
		"add new machine 1",
		"add new machine 2",
		"add new machine 3",
		"add new machine 4",
		"add new machine 5",
		"add new machine 6",
		"add new machine 7",
		"add new machine 8",
		"add new machine 9",
		"add new machine 10",
		"add new machine 11",
		"add new machine 12",
		"add unit ubu/0 to new machine 0",
		"add unit ubu/1 to new machine 1",
		"add unit ubu/2 to new machine 2",
		"add unit ubu/3 to new machine 3",
		"add unit ubu/4 to new machine 4",
		"add unit ubu/5 to new machine 5",
		"add unit ubu/6 to new machine 6",
		"add unit ubu/7 to new machine 7",
		"add unit ubu/8 to new machine 8",
		"add unit ubu/9 to new machine 9",
		"add unit ubu/10 to new machine 10",
		"add unit ubu/11 to new machine 11",
		"add unit ubu/12 to new machine 12",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestAddUnitToExistingApp(c *gc.C) {
	bundleContent := `
                applications:
                    mediawiki:
                        charm: cs:precise/mediawiki-10
                        num_units: 2
                    mysql:
                        charm: cs:precise/mysql-28
                        num_units: 1
                series: trusty
                relations:
                    - - mediawiki:db
                      - mysql:db
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"mediawiki": {
				Charm: "cs:precise/mediawiki-10",
				Units: []bundlechanges.Unit{
					{"mediawiki/0", "1"},
				},
				Series: "precise",
			},
			"mysql": {
				Charm: "cs:precise/mysql-28",
				Units: []bundlechanges.Unit{
					{"mysql/0", "0"},
				},
				Series: "precise",
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
			"1": {ID: "1"},
		},
		Relations: []bundlechanges.Relation{
			{
				App1:      "mediawiki",
				Endpoint1: "db",
				App2:      "mysql",
				Endpoint2: "db",
			},
		},
	}
	expectedChanges := []string{
		"add unit mediawiki/1 to new machine 2",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestPlacementCycle(c *gc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: cs:mysql
                        num_units: 3
                        to: [new, "lxd:0", "lxd:keystone/2"]
                    keystone:
                        charm: cs:keystone
                        num_units: 3
                        to: ["lxd:mysql"]
                machines:
                    0:
            `
	s.checkBundleError(c, bundleContent, "cycle in placement directives for: keystone, mysql")
}

func (s *changesSuite) TestPlacementCycleSameApp(c *gc.C) {
	bundleContent := `
                applications:
                    problem:
                        charm: cs:problem
                        num_units: 2
                        to: ["lxd:new", "lxd:problem/0"]
            `
	s.checkBundleError(c, bundleContent, `cycle in placement directives for: problem`)
}

func (s *changesSuite) TestAddMissingUnitToNotLastPlacement(c *gc.C) {
	bundleContent := `
                applications:
                    foo:
                        charm: cs:foo
                        num_units: 3
                        to: [0,1,2]
                machines:
                   0:
                   1:
                   2:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"foo": {
				Charm: "cs:foo",
				Units: []bundlechanges.Unit{
					{"foo/1", "1"},
					{"foo/2", "2"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
			"1": {ID: "1"},
			"2": {ID: "2"},
		},
	}
	expectedChanges := []string{
		"add new machine 3 (bundle machine 0)",
		"add unit foo/3 to new machine 3",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestAddMissingUnitToNotLastPlacementExisting(c *gc.C) {
	bundleContent := `
                applications:
                    foo:
                        charm: cs:foo
                        num_units: 3
                        to: [0,1,2]
                machines:
                   0:
                   1:
                   2:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"foo": {
				Charm: "cs:foo",
				Units: []bundlechanges.Unit{
					{"foo/1", "1"},
					{"foo/2", "2"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
			"1": {ID: "1"},
			"2": {ID: "2"},
		},
		MachineMap: map[string]string{
			// map existing machines.
			"0": "0",
			"1": "1",
			"2": "2",
		},
	}
	expectedChanges := []string{
		"add unit foo/3 to existing machine 0",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestFromJujuMassiveUnitColocation(c *gc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: cs:xenial/mem-47
                num_units: 3
                to: [1, 2, 3]
            django:
                charm: cs:xenial/django-42
                num_units: 4
                to:
                    - 1
                    - lxd:memcached
            node:
                charm: cs:xenial/django-42
                num_units: 1
                to:
                    - lxd:memcached
        machines:
            1:
            2:
            3:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Name:  "django",
				Charm: "cs:xenial/django-42",
				Units: []bundlechanges.Unit{
					{Name: "django/2", Machine: "1/lxd/0"},
					{Name: "django/3", Machine: "2/lxd/0"},
					{Name: "django/0", Machine: "0"},
					{Name: "django/1", Machine: "0/lxd/0"},
				},
				Series: "xenial",
			},
			"memcached": {
				Name:  "memcached",
				Charm: "cs:xenial/mem-47",
				Units: []bundlechanges.Unit{
					{Name: "memcached/0", Machine: "0"},
					{Name: "memcached/1", Machine: "1"},
					{Name: "memcached/2", Machine: "2"},
				},
				Series: "xenial",
			},
			"ror": {
				Name:  "ror",
				Charm: "cs:xenial/rails-0",
				Units: []bundlechanges.Unit{
					{Name: "ror/0", Machine: "0"},
					{Name: "ror/1", Machine: "2/kvm/0"},
					{Name: "ror/2", Machine: "3"},
				},
				Series: "xenial",
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"0": {ID: "0"},
			"1": {ID: "1"},
			"2": {ID: "2"},
			"3": {ID: "3"},
		},
	}
	expectedChanges := []string{
		"deploy application node from charm-store on xenial using django",
		"add unit node/0 to 0/lxd/0 to satisfy [lxd:memcached]",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestInconsistentMappingError(c *gc.C) {
	// https://bugs.launchpad.net/juju/+bug/1773357 This bug occurs
	// when the model machine map is pre-set incorrectly, and the
	// applications all have enough units, but the mapping omits some
	// machines that host those units. FromData includes changes to
	// create machines but then doesn't put any units on them. It
	// should return an error indicating the inconsistency instead.
	bundleContent := `
        applications:
            memcached:
                charm: cs:xenial/mem-47
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"memcached": {
				Name:  "memcached",
				Charm: "cs:xenial/mem-47",
				Units: []bundlechanges.Unit{
					{Name: "memcached/1", Machine: "1"},
					{Name: "memcached/2", Machine: "2"},
					{Name: "memcached/3", Machine: "3"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"1": {ID: "1"},
			"2": {ID: "2"},
			"3": {ID: "3"},
		},
		MachineMap: map[string]string{
			// using --map-machines=existing
			"1": "1",
			"2": "2",
			"3": "3",
		},
	}
	s.checkBundleImpl(c, bundleContent, existingModel, nil, `bundle and machine mapping are inconsistent: need an explicit entry mapping bundle machine "0", perhaps to one of model machines \["2", "3"\] - the target should host \[memcached\]`, nil, nil)
}

func (s *changesSuite) TestConsistentMapping(c *gc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: cs:xenial/mem-47
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"memcached": {
				Name:  "memcached",
				Charm: "cs:xenial/mem-47",
				Units: []bundlechanges.Unit{
					{Name: "memcached/1", Machine: "1"},
					{Name: "memcached/2", Machine: "2"},
					{Name: "memcached/3", Machine: "3"},
				},
				Series: "xenial",
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"1": {ID: "1"},
			"2": {ID: "2"},
			"3": {ID: "3"},
		},
		MachineMap: map[string]string{
			// using --map-machines=existing
			"1": "1",
			"2": "2",
			"3": "3",
			// Plus an explicit mapping.
			"0": "3",
		},
	}
	// Now that we have a consistent mapping, no changes are needed.
	s.checkBundleExistingModel(c, bundleContent, existingModel, nil)
}

func (s *changesSuite) TestContainerHosts(c *gc.C) {
	// If we have a bundle that needs to create a container, we don't
	// treat the machine hosting the container as not having
	// dependants.
	bundleContent := `
        applications:
            memcached:
                charm: cs:xenial/mem-47
                num_units: 2
                to: [1, "lxd:2"]
        machines:
            1:
            2:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"memcached": {
				Name:  "memcached",
				Charm: "cs:xenial/mem-47",
				Units: []bundlechanges.Unit{
					{Name: "memcached/1", Machine: "1"},
				},
				Series: "xenial",
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"1": {ID: "1"},
		},
		MachineMap: map[string]string{
			// using --map-machines=existing
			"1": "1",
		},
	}
	expectedChanges := []string{
		"add new machine 2",
		"add lxd container 2/lxd/0 on new machine 2",
		"add unit memcached/2 to 2/lxd/0",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestSingleTarget(c *gc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: cs:xenial/mem-47
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"memcached": {
				Name:  "memcached",
				Charm: "cs:xenial/mem-47",
				Units: []bundlechanges.Unit{
					{Name: "memcached/1", Machine: "1"},
					{Name: "memcached/2", Machine: "2"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"1": {ID: "1"},
			"2": {ID: "2"},
		},
		MachineMap: map[string]string{
			// using --map-machines=existing
			"1": "1",
			"2": "2",
		},
	}
	s.checkBundleImpl(c, bundleContent, existingModel, nil, `bundle and machine mapping are inconsistent: need an explicit entry mapping bundle machine "0", perhaps to unreferenced model machine "2" - the target should host \[memcached\]`, nil, nil)
}

func (s *changesSuite) TestMultipleApplications(c *gc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: cs:xenial/mem-47
                num_units: 2
                to: [0, 1]
            prometheus:
                charm: cs:xenial/prom-22
                num_units: 1
                to: [0]
        machines:
            0:
            1:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"memcached": {
				Name:  "memcached",
				Charm: "cs:xenial/mem-47",
				Units: []bundlechanges.Unit{
					{Name: "memcached/1", Machine: "1"},
					{Name: "memcached/2", Machine: "2"},
				},
			},
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prom-22",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/1", Machine: "2"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"1": {ID: "1"},
			"2": {ID: "2"},
		},
		MachineMap: map[string]string{
			// using --map-machines=existing
			"1": "1",
			"2": "2",
		},
	}
	s.checkBundleImpl(c, bundleContent, existingModel, nil, `bundle and machine mapping are inconsistent: need an explicit entry mapping bundle machine "0", perhaps to unreferenced model machine "2" - the target should host \[memcached, prometheus\]`, nil, nil)
}

func (s *changesSuite) TestNoApplications(c *gc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: cs:xenial/mem-47
                num_units: 2
                to: ["lxd:0", 1]
            prometheus:
                charm: cs:xenial/prom-22
                num_units: 1
                to: [memcached/0]
        machines:
            0:
            1:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"memcached": {
				Name:  "memcached",
				Charm: "cs:xenial/mem-47",
				Units: []bundlechanges.Unit{
					{Name: "memcached/1", Machine: "1"},
					{Name: "memcached/2", Machine: "2"},
				},
			},
			"prometheus": {
				Name:  "prometheus",
				Charm: "cs:xenial/prom-22",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/1", Machine: "2"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"1": {ID: "1"},
			"2": {ID: "2"},
		},
		MachineMap: map[string]string{
			// using --map-machines=existing
			"1": "1",
			"2": "2",
		},
	}
	// In this case we can't find any applications for bundle machine
	// 0 because the applications don't refer to it with simple
	// placement..
	s.checkBundleImpl(c, bundleContent, existingModel, nil, `bundle and machine mapping are inconsistent: need an explicit entry mapping bundle machine "0", perhaps to unreferenced model machine "2"`, nil, nil)
}

func (s *changesSuite) TestNoPossibleTargets(c *gc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: cs:xenial/mem-47
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"memcached": {
				Name:  "memcached",
				Charm: "cs:xenial/mem-47",
				Units: []bundlechanges.Unit{
					{Name: "memcached/1", Machine: "1"},
					{Name: "memcached/2", Machine: "1"},
				},
			},
		},
		Machines: map[string]*bundlechanges.Machine{
			"1": {ID: "1"},
		},
		MachineMap: map[string]string{
			// using --map-machines=existing
			"1": "1",
		},
	}
	// There *are* two units, but they're both on machine one.
	s.checkBundleImpl(c, bundleContent, existingModel, nil, `bundle and machine mapping are inconsistent: need an explicit entry mapping bundle machine "0" - the target should host \[memcached\]`, nil, nil)
}

func (s *changesSuite) checkBundle(c *gc.C, bundleContent string, expectedChanges []string) {
	s.checkBundleImpl(c, bundleContent, nil, expectedChanges, "", nil, nil)
}

func (s *changesSuite) checkBundleExistingModel(c *gc.C, bundleContent string, existingModel *bundlechanges.Model, expectedChanges []string) {
	s.checkBundleImpl(c, bundleContent, existingModel, expectedChanges, "", nil, nil)
}

func (s *changesSuite) checkBundleError(c *gc.C, bundleContent string, errMatch string) {
	s.checkBundleImpl(c, bundleContent, nil, nil, errMatch, nil, nil)
}

func (s *changesSuite) checkBundleWithConstraintsParser(c *gc.C, bundleContent string, expectedChanges []string, parserFn bundlechanges.ConstraintGetter) {
	s.checkBundleImpl(c, bundleContent, nil, expectedChanges, "", parserFn, nil)
}

func (s *changesSuite) checkBundleExistingModelWithConstraintsParser(c *gc.C, bundleContent string, existingModel *bundlechanges.Model, expectedChanges []string, parserFn bundlechanges.ConstraintGetter) {
	s.checkBundleImpl(c, bundleContent, existingModel, expectedChanges, "", parserFn, nil)
}

func (s *changesSuite) checkBundleWithConstraintsParserError(c *gc.C, bundleContent, errMatch string, parserFn bundlechanges.ConstraintGetter) {
	s.checkBundleImpl(c, bundleContent, nil, nil, errMatch, parserFn, nil)
}

func (s *changesSuite) checkBundleExistingModelWithRevisionParser(c *gc.C, bundleContent string, existingModel *bundlechanges.Model, expectedChanges []string, charmResolverFn bundlechanges.CharmResolver) {
	s.checkBundleImpl(c, bundleContent, existingModel, expectedChanges, "", nil, charmResolverFn)
}

func (s *changesSuite) checkBundleImpl(c *gc.C,
	bundleContent string,
	existingModel *bundlechanges.Model,
	expectedChanges []string,
	errMatch string,
	parserFn bundlechanges.ConstraintGetter,
	charmResolverFn bundlechanges.CharmResolver,
) {
	// Retrieve and validate the bundle data merging any overlays in the bundle contents.
	bundleSrc, err := charm.StreamBundleDataSource(strings.NewReader(bundleContent), "./")
	c.Assert(err, jc.ErrorIsNil)
	data, err := charm.ReadAndMergeBundleData(bundleSrc)
	c.Assert(err, jc.ErrorIsNil)
	err = data.Verify(nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the changes, and convert them to a sequence of records.
	changes, err := bundlechanges.FromData(bundlechanges.ChangesConfig{
		Bundle:           data,
		Model:            existingModel,
		Logger:           loggo.GetLogger("bundlechanges"),
		ConstraintGetter: parserFn,
		CharmResolver:    charmResolverFn,
	})
	if errMatch != "" {
		c.Assert(err, gc.ErrorMatches, errMatch)
	} else {
		c.Assert(err, jc.ErrorIsNil)
		var obtained []string
		for _, change := range changes {
			c.Log(change.Description())
			//c.Logf("  %s %v", change.Method(), change.GUIArgs())

			for _, descr := range change.Description() {
				obtained = append(obtained, descr)
			}
		}
		c.Check(obtained, jc.DeepEquals, expectedChanges)
	}
}

type archConstraint struct {
	arch string
	err  error
}

func (c *archConstraint) Arch() (string, error) {
	return c.arch, c.err
}

func constraintParser(s string) bundlechanges.ArchConstraint {
	parts := strings.Split(s, " ")
	for _, part := range parts {
		keyValue := strings.Split(part, "=")
		if len(keyValue) == 2 && keyValue[0] == "arch" {
			return &archConstraint{arch: keyValue[1]}
		}
	}
	return &archConstraint{}
}

func constraintParserWithError(err error) bundlechanges.ConstraintGetter {
	return func(string) bundlechanges.ArchConstraint {
		return &archConstraint{err: err}
	}
}
