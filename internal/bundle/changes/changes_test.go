// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bundlechanges_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"

	corebase "github.com/juju/juju/core/base"
	bundlechanges "github.com/juju/juju/internal/bundle/changes"
	"github.com/juju/juju/internal/charm"
	charmtesting "github.com/juju/juju/internal/charm/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type changesSuite struct {
	jujutesting.IsolationSuite
}

var _ = tc.Suite(&changesSuite{})

func (s *changesSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	err := loggo.ConfigureLoggers("bundlechanges=trace")
	c.Assert(err, tc.ErrorIsNil)
}

// record holds expected information about the contents of a change value.
type record struct {
	Id       string
	Requires []string
	Method   string
	Params   interface{}
	Args     map[string]interface{}
}

func (s *changesSuite) TestMinimalBundle(c *tc.C) {
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
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
		},
		Requires: []string{"addCharm-0"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestMinimalBundleWithRevision(c *tc.C) {
	content := `
        applications:
            django:
                charm: django
                revision: 42
                channel: candidate
   `
	rev := 42
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:    "django",
			Revision: &rev,
			Channel:  "candidate",
		},
		Args: map[string]interface{}{
			"channel":  "candidate",
			"charm":    "django",
			"revision": float64(42),
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Channel:     "candidate",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"channel":     "candidate",
		},
		Requires: []string{"addCharm-0"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestMinimalBundleWithChannels(c *tc.C) {
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
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"channel":     "edge",
		},
		Requires: []string{"addCharm-0"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestBundleURLAnnotationSet(c *tc.C) {
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
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "setAnnotations-2",
		Method: "setAnnotations",
		Params: bundlechanges.SetAnnotationsParams{
			Id:         "$deploy-1",
			EntityType: "application",
			Annotations: map[string]string{
				"bundleURL": "ch:bundle/blog",
			},
		},
		Requires: []string{"deploy-1"},
	}}

	data, err := charm.ReadBundleData(strings.NewReader(content))
	c.Assert(err, tc.ErrorIsNil)
	err = data.Verify(nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	// Retrieve the changes, and convert them to a sequence of records.
	changes, err := bundlechanges.FromData(context.Background(),
		bundlechanges.ChangesConfig{
			Bundle:    data,
			BundleURL: "ch:bundle/blog",
			Logger:    loggertesting.WrapCheckLog(c),
		})
	c.Assert(err, tc.ErrorIsNil)
	records := make([]record, len(changes))
	for i, change := range changes {
		r := record{
			Id:       change.Id(),
			Requires: change.Requires(),
			Method:   change.Method(),
			Params:   copyParams(change),
		}
		records[i] = r
	}
	c.Check(records, tc.DeepEquals, expected)
}

func (s *changesSuite) TestMinimalBundleWithDevices(c *tc.C) {
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
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
		},
		Requires: []string{"addCharm-0"},
	}}

	s.assertParseDataWithDevices(c, content, expected)
}

var twentySix = 26

func (s *changesSuite) TestMinimalBundleWithOffer(c *tc.C) {
	content := `
saas:
  keystone:
    url: production:admin/info.keystone
applications:
  apache2:
    charm: "apache2"
    revision: 26
    channel: "stable"
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
				Charm:    "apache2",
				Revision: &twentySix,
				Channel:  "stable",
			},
			Args: map[string]interface{}{
				"channel":  "stable",
				"charm":    "apache2",
				"revision": float64(26),
			},
		},
		{
			Id:     "deploy-1",
			Method: "deploy",
			Params: bundlechanges.AddApplicationParams{
				Charm:       "$addCharm-0",
				Application: "apache2",
				Channel:     "stable",
			},
			Args: map[string]interface{}{
				"application": "apache2",
				"channel":     "stable",
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

func (s *changesSuite) TestMinimalBundleWithOfferAndPreDeployedApp(c *tc.C) {
	content := `
applications:
  apache2:
    charm: "ch:apache2"
    revision: 26
    channel: stable
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
				Name:     "apache-2",
				Charm:    "ch:apache2",
				Revision: 26,
				Channel:  "stable",
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

func (s *changesSuite) TestMinimalBundleWithOfferACL(c *tc.C) {
	content := `
applications:
  apache2:
    charm: "ch:apache2"
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
				Charm: "ch:apache2",
			},
			Args: map[string]interface{}{
				"charm": "ch:apache2",
			},
		},
		{
			Id:     "deploy-1",
			Method: "deploy",
			Params: bundlechanges.AddApplicationParams{
				Charm:       "$addCharm-0",
				Application: "apache2",
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

func (s *changesSuite) TestMinimalBundleWithOfferUpdate(c *tc.C) {
	content := `
applications:
  apache2:
    charm: "ch:apache2"
    revision: 26
    channel: stable
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
				Name:     "apache2",
				Charm:    "ch:apache2",
				Revision: 26,
				Channel:  "stable",
				Offers:   []string{"offer1"},
			},
		},
	}
	s.assertParseDataWithModel(c, curModel, content, expected)
}

func (s *changesSuite) TestMinimalBundleWithOfferAndRelations(c *tc.C) {
	content := `
saas:
  mysql:
    url: production:admin/info.mysql
applications:
  apache2:
    charm: "ch:apache2"
relations:
- - apache2:db
  - mysql:db
   `
	expected := []record{
		{
			Id:     "addCharm-0",
			Method: "addCharm",
			Params: bundlechanges.AddCharmParams{
				Charm: "ch:apache2",
			},
			Args: map[string]interface{}{
				"charm": "ch:apache2",
			},
		},
		{
			Id:     "deploy-1",
			Method: "deploy",
			Params: bundlechanges.AddApplicationParams{
				Charm:       "$addCharm-0",
				Application: "apache2",
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
			Args: map[string]interface{}{
				"endpoint1": "$deploy-1:db",
				"endpoint2": "$consumeOffer-2:db",
			},
			Requires: []string{"consumeOffer-2", "deploy-1"},
		},
	}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestSimpleBundle(c *tc.C) {
	content := `
        applications:
            mediawiki:
                charm: ch:mediawiki
                base: ubuntu@20.04
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
                charm: ch:mysql
                base: ubuntu@20.04
                num_units: 1
                resources:
                  data: "./resources/data.tar"
        default-base: ubuntu@22.04
        relations:
            - - mediawiki:db
              - mysql:db
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "ch:mediawiki",
			Base:  "ubuntu@20.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "ch:mediawiki",
			"base":  "ubuntu@20.04/stable",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "mediawiki",
			Base:        "ubuntu@20.04/stable",
			Options:     map[string]interface{}{"debug": false},
			Resources:   map[string]int{"data": 3},
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
			"base": "ubuntu@20.04/stable",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "expose-2",
		Method: "expose",
		Params: bundlechanges.ExposeParams{
			Application: "$deploy-1",
		},
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
			Charm: "ch:mysql",
			Base:  "ubuntu@20.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "ch:mysql",
			"base":  "ubuntu@20.04/stable",
		},
	}, {
		Id:     "deploy-5",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:          "$addCharm-4",
			Application:    "mysql",
			Base:           "ubuntu@20.04/stable",
			LocalResources: map[string]string{"data": "./resources/data.tar"},
		},
		Args: map[string]interface{}{
			"application": "mysql",
			"charm":       "$addCharm-4",
			"local-resources": map[string]interface{}{
				"data": "./resources/data.tar",
			},
			"base": "ubuntu@20.04/stable",
		},
		Requires: []string{"addCharm-4"},
	}, {
		Id:     "addRelation-6",
		Method: "addRelation",
		Params: bundlechanges.AddRelationParams{
			Endpoint1: "$deploy-1:db",
			Endpoint2: "$deploy-5:db",
		},
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
		Args: map[string]interface{}{
			"application": "$deploy-5",
		},
		Requires: []string{"deploy-5"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestSimpleBundleWithDevices(c *tc.C) {
	content := `
        applications:
            mediawiki:
                charm: ch:mediawiki
                base: ubuntu@20.04
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
                charm: ch:mysql
                base: ubuntu@20.04
                num_units: 1
                resources:
                  data: "./resources/data.tar"
        default-base: ubuntu@22.04
        relations:
            - - mediawiki:db
              - mysql:db
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "ch:mediawiki",
			Base:  "ubuntu@20.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "ch:mediawiki",
			"base":  "ubuntu@20.04/stable",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "mediawiki",
			Base:        "ubuntu@20.04/stable",
			Options:     map[string]interface{}{"debug": false},
			Resources:   map[string]int{"data": 3},
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
			"base": "ubuntu@20.04/stable",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "expose-2",
		Method: "expose",
		Params: bundlechanges.ExposeParams{
			Application: "$deploy-1",
		},
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
			Charm: "ch:mysql",
			Base:  "ubuntu@20.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "ch:mysql",
			"base":  "ubuntu@20.04/stable",
		},
	}, {
		Id:     "deploy-5",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:          "$addCharm-4",
			Application:    "mysql",
			Base:           "ubuntu@20.04/stable",
			LocalResources: map[string]string{"data": "./resources/data.tar"},
		},
		Args: map[string]interface{}{
			"application": "mysql",
			"charm":       "$addCharm-4",
			"local-resources": map[string]interface{}{
				"data": "./resources/data.tar",
			},
			"base": "ubuntu@20.04/stable",
		},
		Requires: []string{"addCharm-4"},
	}, {
		Id:     "addRelation-6",
		Method: "addRelation",
		Params: bundlechanges.AddRelationParams{
			Endpoint1: "$deploy-1:db",
			Endpoint2: "$deploy-5:db",
		},
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
		Args: map[string]interface{}{
			"application": "$deploy-5",
		},
		Requires: []string{"deploy-5"},
	}}

	s.assertParseDataWithDevices(c, content, expected)
}

func (s *changesSuite) TestKubernetesBundle(c *tc.C) {
	content := `
        bundle: kubernetes
        applications:
            mediawiki:
                charm: ch:mediawiki-k8s
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
                charm: ch:mysql-k8s
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
			Charm: "ch:mediawiki-k8s",
		},
		Args: map[string]interface{}{
			"charm": "ch:mediawiki-k8s",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "mediawiki",
			NumUnits:    1,
			Options:     map[string]interface{}{"debug": false},
			Resources:   map[string]int{"data": 3},
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
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "expose-2",
		Method: "expose",
		Params: bundlechanges.ExposeParams{
			Application: "$deploy-1",
		},
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
			Charm: "ch:mysql-k8s",
		},
		Args: map[string]interface{}{
			"charm": "ch:mysql-k8s",
		},
	}, {
		Id:     "deploy-5",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:          "$addCharm-4",
			Application:    "mysql",
			NumUnits:       2,
			LocalResources: map[string]string{"data": "./resources/data.tar"},
		},
		Args: map[string]interface{}{
			"application": "mysql",
			"charm":       "$addCharm-4",
			"local-resources": map[string]interface{}{
				"data": "./resources/data.tar",
			},
			"num-units": float64(2),
		},
		Requires: []string{"addCharm-4"},
	}, {
		Id:     "addRelation-6",
		Method: "addRelation",
		Params: bundlechanges.AddRelationParams{
			Endpoint1: "$deploy-1:db",
			Endpoint2: "$deploy-5:db",
		},
		Args: map[string]interface{}{
			"endpoint1": "$deploy-1:db",
			"endpoint2": "$deploy-5:db",
		},
		Requires: []string{"deploy-1", "deploy-5"},
	}}

	s.assertParseDataWithDevices(c, content, expected)
}

func (s *changesSuite) TestSameCharmReused(c *tc.C) {
	content := `
        applications:
            mediawiki:
                charm: ch:mediawiki
                base: ubuntu@20.04
                num_units: 1
            otherwiki:
                charm: ch:mediawiki
                base: ubuntu@20.04
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "ch:mediawiki",
			Base:  "ubuntu@20.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "ch:mediawiki",
			"base":  "ubuntu@20.04/stable",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "mediawiki",
			Base:        "ubuntu@20.04/stable",
		},
		Args: map[string]interface{}{
			"application": "mediawiki",
			"charm":       "$addCharm-0",
			"base":        "ubuntu@20.04/stable",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "deploy-2",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "otherwiki",
			Base:        "ubuntu@20.04/stable",
		},
		Args: map[string]interface{}{
			"application": "otherwiki",
			"charm":       "$addCharm-0",
			"base":        "ubuntu@20.04/stable",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addUnit-3",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
		},
		Requires: []string{"deploy-1"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestMachinesAndUnitsPlacementWithBindings(c *tc.C) {
	content := `
        applications:
            django:
                charm: ch:django
                base: ubuntu@22.04
                num_units: 2
                bindings:
                    "": foo
                    http: bar
                to:
                    - 1
                    - lxc:2
                constraints: spaces=baz cpu-cores=4 cpu-power=42
            haproxy:
                charm: ch:haproxy
                base: ubuntu@22.04
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
                base: ubuntu@22.04
            2:
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "ch:django",
			Base:  "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "ch:django",
			"base":  "ubuntu@22.04/stable",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:            "$addCharm-0",
			Application:      "django",
			Base:             "ubuntu@22.04/stable",
			Constraints:      "spaces=baz cpu-cores=4 cpu-power=42",
			EndpointBindings: map[string]string{"": "foo", "http": "bar"},
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"constraints": "spaces=baz cpu-cores=4 cpu-power=42",
			"endpoint-bindings": map[string]interface{}{
				"":     "foo",
				"http": "bar",
			},
			"base": "ubuntu@22.04/stable",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "ch:haproxy",
			Base:  "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "ch:haproxy",
			"base":  "ubuntu@22.04/stable",
		},
	}, {
		Id:     "deploy-3",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-2",
			Application: "haproxy",
			Base:        "ubuntu@22.04/stable",
			Options:     map[string]interface{}{"bad": "wolf", "number": 42.47},
		},
		Args: map[string]interface{}{
			"application": "haproxy",
			"charm":       "$addCharm-2",
			"options": map[string]interface{}{
				"bad":    "wolf",
				"number": 42.47,
			},
			"base": "ubuntu@22.04/stable",
		},
		Requires: []string{"addCharm-2"},
	}, {
		Id:     "expose-4",
		Method: "expose",
		Params: bundlechanges.ExposeParams{
			Application: "$deploy-3",
		},
		Args: map[string]interface{}{
			"application": "$deploy-3",
		},
		Requires: []string{"deploy-3"},
	}, {
		Id:     "addMachines-5",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Base: "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"base": "ubuntu@22.04/stable",
		},
	}, {
		Id:       "addMachines-6",
		Method:   "addMachines",
		Params:   bundlechanges.AddMachineParams{},
		Args:     map[string]interface{}{},
		Requires: []string{"addMachines-5"},
	}, {
		Id:     "addUnit-7",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-5",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-5",
		},
		Requires: []string{"addMachines-5", "deploy-1"},
	}, {
		Id:     "addMachines-11",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "lxc",
			Base:          "ubuntu@22.04/stable",
			ParentId:      "$addMachines-6",
			Constraints:   "spaces=bar,baz,foo cpu-cores=4 cpu-power=42",
		},
		Args: map[string]interface{}{
			"constraints":    "spaces=bar,baz,foo cpu-cores=4 cpu-power=42",
			"container-type": "lxc",
			"parent-id":      "$addMachines-6",
			"base":           "ubuntu@22.04/stable",
		},
		Requires: []string{"addMachines-5", "addMachines-6"},
	}, {
		Id:     "addMachines-12",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "lxc",
			Base:          "ubuntu@22.04/stable",
			ParentId:      "$addUnit-7",
		},
		Args: map[string]interface{}{
			"container-type": "lxc",
			"parent-id":      "$addUnit-7",
			"base":           "ubuntu@22.04/stable",
		},
		Requires: []string{"addMachines-11", "addMachines-5", "addMachines-6", "addUnit-7"},
	}, {
		Id:     "addMachines-13",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Base: "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"base": "ubuntu@22.04/stable",
		},
		Requires: []string{"addMachines-11", "addMachines-12", "addMachines-5", "addMachines-6"},
	}, {
		Id:     "addUnit-8",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-11",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-11",
		},
		Requires: []string{"addMachines-11", "addUnit-7", "deploy-1"},
	}, {
		Id:     "addUnit-9",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
			To:          "$addMachines-12",
		},
		Args: map[string]interface{}{
			"application": "$deploy-3",
			"to":          "$addMachines-12",
		},
		Requires: []string{"addMachines-12", "deploy-3"},
	}, {
		Id:     "addUnit-10",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
			To:          "$addMachines-13",
		},
		Args: map[string]interface{}{
			"application": "$deploy-3",
			"to":          "$addMachines-13",
		},
		Requires: []string{"addMachines-13", "addUnit-9", "deploy-3"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestMachinesWithConstraintsAndAnnotations(c *tc.C) {
	content := `
        applications:
            django:
                charm: ch:django
                base: ubuntu@22.04
                num_units: 2
                to:
                    - 1
                    - new
        machines:
            1:
                constraints: "cpu-cores=4 image-id=ubuntu-bf2"
                annotations:
                    foo: bar
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "ch:django",
			Base:  "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "ch:django",
			"base":  "ubuntu@22.04/stable",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Base:        "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"base":        "ubuntu@22.04/stable",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addMachines-2",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Constraints: "cpu-cores=4 image-id=ubuntu-bf2",
		},
		Args: map[string]interface{}{
			"constraints": "cpu-cores=4 image-id=ubuntu-bf2",
		},
	}, {
		Id:     "setAnnotations-3",
		Method: "setAnnotations",
		Params: bundlechanges.SetAnnotationsParams{
			Id:          "$addMachines-2",
			EntityType:  bundlechanges.MachineType,
			Annotations: map[string]string{"foo": "bar"},
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
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-2",
		},
		Requires: []string{"addMachines-2", "deploy-1"},
	}, {
		Id:     "addMachines-6",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Base: "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"base": "ubuntu@22.04/stable",
		},
		Requires: []string{"addMachines-2"},
	}, {
		Id:     "addUnit-5",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-6",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-6",
		},
		Requires: []string{"addMachines-6", "addUnit-4", "deploy-1"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestEndpointWithoutRelationName(c *tc.C) {
	content := `
        applications:
            mediawiki:
                charm: ch:mediawiki
                base: ubuntu@20.04
            mysql:
                charm: mysql
                base: ubuntu@20.04
                constraints: mem=42G
        relations:
            - - mediawiki:db
              - mysql
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "ch:mediawiki",
			Base:  "ubuntu@20.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "ch:mediawiki",
			"base":  "ubuntu@20.04/stable",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "mediawiki",
			Base:        "ubuntu@20.04/stable",
		},
		Args: map[string]interface{}{
			"application": "mediawiki",
			"charm":       "$addCharm-0",
			"base":        "ubuntu@20.04/stable",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "mysql",
			Base:  "ubuntu@20.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "mysql",
			"base":  "ubuntu@20.04/stable",
		},
	}, {
		Id:     "deploy-3",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-2",
			Application: "mysql",
			Base:        "ubuntu@20.04/stable",
			Constraints: "mem=42G",
		},
		Args: map[string]interface{}{
			"application": "mysql",
			"charm":       "$addCharm-2",
			"constraints": "mem=42G",
			"base":        "ubuntu@20.04/stable",
		},
		Requires: []string{"addCharm-2"},
	}, {
		Id:     "addRelation-4",
		Method: "addRelation",
		Params: bundlechanges.AddRelationParams{
			Endpoint1: "$deploy-1:db",
			Endpoint2: "$deploy-3",
		},
		Args: map[string]interface{}{
			"endpoint1": "$deploy-1:db",
			"endpoint2": "$deploy-3",
		},
		Requires: []string{"deploy-1", "deploy-3"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestUnitPlacedInApplication(c *tc.C) {
	content := `
        applications:
            wordpress:
                charm: wordpress
                num_units: 3
            django:
                charm: ch:django
                base: ubuntu@22.04
                num_units: 2
                to: [wordpress]
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "ch:django",
			Base:  "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "ch:django",
			"base":  "ubuntu@22.04/stable",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Base:        "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"base":        "ubuntu@22.04/stable",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "wordpress",
		},
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
		Args: map[string]interface{}{
			"application": "$deploy-3",
		},
		Requires: []string{"addUnit-6", "deploy-3"},
	}, {
		Id:     "addUnit-8",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
		},
		Args: map[string]interface{}{
			"application": "$deploy-3",
		},
		Requires: []string{"addUnit-7", "deploy-3"},
	}, {
		Id:     "addUnit-4",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addUnit-6",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addUnit-6",
		},
		Requires: []string{"addUnit-6", "deploy-1"},
	}, {
		Id:     "addUnit-5",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addUnit-7",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addUnit-7",
		},
		Requires: []string{"addUnit-4", "addUnit-7", "deploy-1"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestUnitPlacedInApplicationWithDevices(c *tc.C) {
	content := `
        applications:
            wordpress:
                charm: wordpress
                num_units: 3
            django:
                charm: ch:django
                base: ubuntu@22.04
                num_units: 2
                to: [wordpress]
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "ch:django",
			Base:  "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "ch:django",
			"base":  "ubuntu@22.04/stable",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Base:        "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"base":        "ubuntu@22.04/stable",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "wordpress",
		},
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
		Args: map[string]interface{}{
			"application": "$deploy-3",
		},
		Requires: []string{"addUnit-6", "deploy-3"},
	}, {
		Id:     "addUnit-8",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
		},
		Args: map[string]interface{}{
			"application": "$deploy-3",
		},
		Requires: []string{"addUnit-7", "deploy-3"},
	}, {
		Id:     "addUnit-4",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addUnit-6",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addUnit-6",
		},
		Requires: []string{"addUnit-6", "deploy-1"},
	}, {
		Id:     "addUnit-5",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addUnit-7",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addUnit-7",
		},
		Requires: []string{"addUnit-4", "addUnit-7", "deploy-1"},
	}}

	s.assertParseDataWithDevices(c, content, expected)
}

func (s *changesSuite) TestUnitColocationWithOtherUnits(c *tc.C) {
	content := `
        applications:
            memcached:
                charm: ch:mem
                base: ubuntu@22.04
                num_units: 3
                to: [1, new]
            django:
                charm: ch:django
                base: ubuntu@22.04
                num_units: 5
                to:
                    - memcached/0
                    - lxc:memcached/1
                    - lxc:memcached/2
                    - kvm:ror
            ror:
                charm: ch:rails
                num_units: 2
                to:
                    - new
                    - 1
        machines:
            1:
                base: ubuntu@22.04
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "ch:django",
			Base:  "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "ch:django",
			"base":  "ubuntu@22.04/stable",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Base:        "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"base":        "ubuntu@22.04/stable",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "ch:mem",
			Base:  "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "ch:mem",
			"base":  "ubuntu@22.04/stable",
		},
	}, {
		Id:     "deploy-3",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-2",
			Application: "memcached",
			Base:        "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"application": "memcached",
			"charm":       "$addCharm-2",
			"base":        "ubuntu@22.04/stable",
		},
		Requires: []string{"addCharm-2"},
	}, {
		Id:     "addCharm-4",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: "ch:rails",
		},
		Args: map[string]interface{}{
			"charm": "ch:rails",
		},
	}, {
		Id:     "deploy-5",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-4",
			Application: "ror",
		},
		Args: map[string]interface{}{
			"application": "ror",
			"charm":       "$addCharm-4",
		},
		Requires: []string{"addCharm-4"},
	}, {
		Id:     "addMachines-6",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Base: "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"base": "ubuntu@22.04/stable",
		},
	}, {
		Id:     "addUnit-12",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
			To:          "$addMachines-6",
		},
		Args: map[string]interface{}{
			"application": "$deploy-3",
			"to":          "$addMachines-6",
		},
		Requires: []string{"addMachines-6", "deploy-3"},
	}, {
		Id:     "addMachines-17",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Base: "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"base": "ubuntu@22.04/stable",
		},
		Requires: []string{"addMachines-6"},
	}, {
		Id:     "addMachines-18",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Base: "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"base": "ubuntu@22.04/stable",
		},
		Requires: []string{"addMachines-17", "addMachines-6"},
	}, {
		Id:       "addMachines-19",
		Method:   "addMachines",
		Params:   bundlechanges.AddMachineParams{},
		Args:     map[string]interface{}{},
		Requires: []string{"addMachines-17", "addMachines-18", "addMachines-6"},
	}, {
		Id:     "addUnit-7",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addUnit-12",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addUnit-12",
		},
		Requires: []string{"addUnit-12", "deploy-1"},
	}, {
		Id:     "addUnit-13",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
			To:          "$addMachines-17",
		},
		Args: map[string]interface{}{
			"application": "$deploy-3",
			"to":          "$addMachines-17",
		},
		Requires: []string{"addMachines-17", "addUnit-12", "deploy-3"},
	}, {
		Id:     "addUnit-14",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-3",
			To:          "$addMachines-18",
		},
		Args: map[string]interface{}{
			"application": "$deploy-3",
			"to":          "$addMachines-18",
		},
		Requires: []string{"addMachines-18", "addUnit-13", "deploy-3"},
	}, {
		Id:     "addUnit-15",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-5",
			To:          "$addMachines-19",
		},
		Args: map[string]interface{}{
			"application": "$deploy-5",
			"to":          "$addMachines-19",
		},
		Requires: []string{"addMachines-19", "deploy-5"},
	}, {
		Id:     "addUnit-16",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-5",
			To:          "$addMachines-6",
		},
		Args: map[string]interface{}{
			"application": "$deploy-5",
			"to":          "$addMachines-6",
		},
		Requires: []string{"addMachines-6", "addUnit-15", "deploy-5"},
	}, {
		Id:     "addMachines-20",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "lxc",
			Base:          "ubuntu@22.04/stable",
			ParentId:      "$addUnit-13",
		},
		Args: map[string]interface{}{
			"container-type": "lxc",
			"parent-id":      "$addUnit-13",
			"base":           "ubuntu@22.04/stable",
		},
		Requires: []string{"addMachines-17", "addMachines-18", "addMachines-19", "addMachines-6", "addUnit-13"},
	}, {
		Id:     "addMachines-21",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "lxc",
			Base:          "ubuntu@22.04/stable",
			ParentId:      "$addUnit-14",
		},
		Args: map[string]interface{}{
			"container-type": "lxc",
			"parent-id":      "$addUnit-14",
			"base":           "ubuntu@22.04/stable",
		},
		Requires: []string{"addMachines-17", "addMachines-18", "addMachines-19", "addMachines-20", "addMachines-6", "addUnit-14"},
	}, {
		Id:     "addMachines-22",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "kvm",
			Base:          "ubuntu@22.04/stable",
			ParentId:      "$addUnit-15",
		},
		Args: map[string]interface{}{
			"container-type": "kvm",
			"parent-id":      "$addUnit-15",
			"base":           "ubuntu@22.04/stable",
		},
		Requires: []string{"addMachines-17", "addMachines-18", "addMachines-19", "addMachines-20", "addMachines-21", "addMachines-6", "addUnit-15"},
	}, {
		Id:     "addMachines-23",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "kvm",
			Base:          "ubuntu@22.04/stable",
			ParentId:      "$addUnit-16",
		},
		Args: map[string]interface{}{
			"container-type": "kvm",
			"parent-id":      "$addUnit-16",
			"base":           "ubuntu@22.04/stable",
		},
		Requires: []string{"addMachines-17", "addMachines-18", "addMachines-19", "addMachines-20", "addMachines-21", "addMachines-22", "addMachines-6", "addUnit-16"},
	}, {
		Id:     "addUnit-8",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-20",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-20",
		},
		Requires: []string{"addMachines-20", "addUnit-7", "deploy-1"},
	}, {
		Id:     "addUnit-9",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-21",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-21",
		},
		Requires: []string{"addMachines-21", "addUnit-8", "deploy-1"},
	}, {
		Id:     "addUnit-10",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-22",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-22",
		},
		Requires: []string{"addMachines-22", "addUnit-9", "deploy-1"},
	}, {
		Id:     "addUnit-11",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-23",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-23",
		},
		Requires: []string{"addMachines-23", "addUnit-10", "deploy-1"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestUnitPlacedToMachines(c *tc.C) {
	content := `
        applications:
            django:
                charm: ch:django
                base: ubuntu@22.04
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
			Charm: "ch:django",
			Base:  "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "ch:django",
			"base":  "ubuntu@22.04/stable",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Base:        "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"base":        "ubuntu@22.04/stable",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addMachines-2",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Constraints: "cpu-cores=4",
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
		Args: map[string]interface{}{
			"constraints": "cpu-cores=8",
		},
		Requires: []string{"addMachines-2"},
	}, {
		Id:     "addMachines-9",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Base: "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"base": "ubuntu@22.04/stable",
		},
		Requires: []string{"addMachines-2", "addMachines-3"},
	}, {
		Id:     "addMachines-10",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "kvm",
			Base:          "ubuntu@22.04/stable",
			ParentId:      "$addMachines-3",
		},
		Args: map[string]interface{}{
			"container-type": "kvm",
			"parent-id":      "$addMachines-3",
			"base":           "ubuntu@22.04/stable",
		},
		Requires: []string{"addMachines-2", "addMachines-3", "addMachines-9"},
	}, {
		Id:     "addMachines-11",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "lxc",
			Base:          "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"container-type": "lxc",
			"base":           "ubuntu@22.04/stable",
		},
		Requires: []string{"addMachines-10", "addMachines-2", "addMachines-3", "addMachines-9"},
	}, {
		Id:     "addMachines-12",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			ContainerType: "lxc",
			Base:          "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"container-type": "lxc",
			"base":           "ubuntu@22.04/stable",
		},
		Requires: []string{"addMachines-10", "addMachines-11", "addMachines-2", "addMachines-3", "addMachines-9"},
	}, {
		Id:     "addUnit-4",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-9",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-9",
		},
		Requires: []string{"addMachines-9", "deploy-1"},
	}, {
		Id:     "addUnit-5",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-2",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-2",
		},
		Requires: []string{"addMachines-2", "addUnit-4", "deploy-1"},
	}, {
		Id:     "addUnit-6",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-10",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-10",
		},
		Requires: []string{"addMachines-10", "addUnit-5", "deploy-1"},
	}, {
		Id:     "addUnit-7",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-11",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-11",
		},
		Requires: []string{"addMachines-11", "addUnit-6", "deploy-1"},
	}, {
		Id:     "addUnit-8",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-12",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-12",
		},
		Requires: []string{"addMachines-12", "addUnit-7", "deploy-1"},
	}}

	s.assertParseData(c, content, expected)
}

var fortytwo = 42

func (s *changesSuite) TestUnitPlacedToNewMachineWithConstraints(c *tc.C) {
	content := `
        applications:
            django:
                charm: django
                channel: stable
                revision: 42
                base: ubuntu@22.04
                num_units: 1
                to:
                    - new
                constraints: "cpu-cores=4 image-id=ubuntu-bf2"
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:    "django",
			Revision: &fortytwo,
			Channel:  "stable",
			Base:     "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"charm":    "django",
			"revision": float64(42),
			"channel":  "stable",
			"base":     "ubuntu@22.04/stable",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Base:        "ubuntu@22.04/stable",
			Channel:     "stable",
			Constraints: "cpu-cores=4 image-id=ubuntu-bf2",
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"channel":     "stable",
			"constraints": "cpu-cores=4 image-id=ubuntu-bf2",
			"base":        "ubuntu@22.04/stable",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addMachines-3",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Constraints: "cpu-cores=4 image-id=ubuntu-bf2",
			Base:        "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"constraints": "cpu-cores=4 image-id=ubuntu-bf2",
			"base":        "ubuntu@22.04/stable",
		},
	}, {
		Id:     "addUnit-2",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-3",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-3",
		},
		Requires: []string{"addMachines-3", "deploy-1"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestApplicationWithStorage(c *tc.C) {
	content := `
        applications:
            django:
                charm: django
                channel: stable
                revision: 42
                base: ubuntu@22.04
                num_units: 2
                storage:
                    osd-devices: 3,30G
                    tmpfs: tmpfs,1G
        `
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm:    "django",
			Revision: &fortytwo,
			Channel:  "stable",
			Base:     "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"charm":    "django",
			"base":     "ubuntu@22.04/stable",
			"revision": float64(42),
			"channel":  "stable",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Base:        "ubuntu@22.04/stable",
			Channel:     "stable",
			Storage: map[string]string{
				"osd-devices": "3,30G",
				"tmpfs":       "tmpfs,1G",
			},
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"channel":     "stable",
			"base":        "ubuntu@22.04/stable",
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
		Args: map[string]interface{}{
			"application": "$deploy-1",
		},
		Requires: []string{"addUnit-2", "deploy-1"},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestApplicationWithDevices(c *tc.C) {
	content := `
        applications:
            django:
                charm: ch:django
                revision: 42
                channel: stable
                base: ubuntu@22.04
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
			Charm:    "ch:django",
			Revision: &fortytwo,
			Channel:  "stable",
			Base:     "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"charm":    "ch:django",
			"channel":  "stable",
			"revision": float64(42),
			"base":     "ubuntu@22.04/stable",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Base:        "ubuntu@22.04/stable",
			Channel:     "stable",
			Devices: map[string]string{
				"description": "a nvidia gpu device",
				"type":        "nvidia.com/gpu",
				"countmin":    "1",
				"countmax":    "2",
			},
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"channel":     "stable",
			"devices": map[string]interface{}{
				"countmax":    "2",
				"countmin":    "1",
				"description": "a nvidia gpu device",
				"type":        "nvidia.com/gpu",
			},
			"base": "ubuntu@22.04/stable",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addUnit-2",
		Method: "addUnit",
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
		},
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
		Args: map[string]interface{}{
			"application": "$deploy-1",
		},
		Requires: []string{"addUnit-2", "deploy-1"},
	}}

	s.assertParseDataWithDevices(c, content, expected)
}

func (s *changesSuite) TestApplicationWithEndpointBindings(c *tc.C) {
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

func (s *changesSuite) TestApplicationWithExposeParams(c *tc.C) {
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

func (s *changesSuite) TestApplicationWithNonDefaultBaseAndPlacements(c *tc.C) {
	content := `
default-base: ubuntu@22.04
applications:
    gui3:
        charm: ch:juju-gui
        base: ubuntu@20.04
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
			Charm: "ch:juju-gui",
			Base:  "ubuntu@20.04/stable",
		},
		Args: map[string]interface{}{
			"charm": "ch:juju-gui",
			"base":  "ubuntu@20.04/stable",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "gui3",
			Base:        "ubuntu@20.04/stable",
		},
		Args: map[string]interface{}{
			"application": "gui3",
			"charm":       "$addCharm-0",
			"base":        "ubuntu@20.04/stable",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addMachines-2",
		Method: "addMachines",
		Params: bundlechanges.AddMachineParams{
			Base: "ubuntu@22.04/stable",
		},
		Args: map[string]interface{}{
			"base": "ubuntu@22.04/stable",
		},
	}, {
		Id:       "addMachines-5",
		Method:   "addMachines",
		Requires: []string{"addMachines-2"},
		Params: bundlechanges.AddMachineParams{
			Base: "ubuntu@20.04/stable",
		},
		Args: map[string]interface{}{
			"base": "ubuntu@20.04/stable",
		},
	}, {
		Id:       "addMachines-6",
		Method:   "addMachines",
		Requires: []string{"addMachines-2", "addMachines-5"},
		Params: bundlechanges.AddMachineParams{
			ContainerType: "lxc",
			ParentId:      "$addMachines-2",
			Base:          "ubuntu@20.04/stable",
		},
		Args: map[string]interface{}{
			"container-type": "lxc",
			"parent-id":      "$addMachines-2",
			"base":           "ubuntu@20.04/stable",
		},
	}, {
		Id:       "addUnit-3",
		Method:   "addUnit",
		Requires: []string{"addMachines-5", "deploy-1"},
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-5",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-5",
		},
	}, {
		Id:       "addUnit-4",
		Method:   "addUnit",
		Requires: []string{"addMachines-6", "addUnit-3", "deploy-1"},
		Params: bundlechanges.AddUnitParams{
			Application: "$deploy-1",
			To:          "$addMachines-6",
		},
		Args: map[string]interface{}{
			"application": "$deploy-1",
			"to":          "$addMachines-6",
		},
	}}

	s.assertParseData(c, content, expected)
}

func (s *changesSuite) TestAddMachineParamsMachine(c *tc.C) {
	param := bundlechanges.NewAddMachineParamsMachine("42")
	c.Assert(param.Machine(), tc.Equals, "42")
}

func (s *changesSuite) TestAddMachineParamsContainer(c *tc.C) {
	param := bundlechanges.NewAddMachineParamsContainer("42", "42/lxd/0")
	c.Assert(param.Machine(), tc.Equals, "42/lxd/0")
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

func (s *changesSuite) assertParseData(c *tc.C, content string, expected []record) {
	s.assertParseDataWithModel(c, nil, content, expected)
}

func (s *changesSuite) assertParseDataWithModel(c *tc.C, model *bundlechanges.Model, content string, expected []record) {
	// Retrieve and validate the bundle data merging any overlays in the bundle contents.
	bundleSrc, err := charm.StreamBundleDataSource(strings.NewReader(content), "./")
	c.Assert(err, tc.ErrorIsNil)
	data, err := charm.ReadAndMergeBundleData(bundleSrc)
	c.Assert(err, tc.ErrorIsNil)
	err = data.Verify(nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	// Retrieve the changes, and convert them to a sequence of records.
	changes, err := bundlechanges.FromData(context.Background(), bundlechanges.ChangesConfig{
		Model:  model,
		Bundle: data,
		Logger: loggertesting.WrapCheckLog(c),
		CharmResolver: func(ctx context.Context, charm string, _ corebase.Base, channel, _ string, rev int) (string, int, error) {
			if charm == "ch:apache2" {
				return "stable", 26, nil
			}
			return "stable", -1, nil
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	records := make([]record, len(changes))
	for i, change := range changes {
		args, err := change.Args()
		c.Assert(err, tc.ErrorIsNil)
		requires := change.Requires()
		sort.Sort(sort.StringSlice(requires))
		r := record{
			Id:       change.Id(),
			Requires: requires,
			Method:   change.Method(),
			Args:     args,
			Params:   copyParams(change),
		}
		records[i] = r
		c.Log(change.Description())
	}

	// Output the records for debugging.
	b, err := json.MarshalIndent(records, "", "  ")
	c.Assert(err, tc.ErrorIsNil)
	c.Logf("obtained records: %s", b)

	// Check that the obtained records are what we expect.
	c.Check(records, tc.DeepEquals, expected)
}

func (s *changesSuite) assertParseDataWithDevices(c *tc.C, content string, expected []record) {
	// Retrieve and validate the bundle data.
	data, err := charm.ReadBundleData(strings.NewReader(content))
	c.Assert(err, tc.ErrorIsNil)
	err = data.Verify(nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	// Retrieve the changes, and convert them to a sequence of records.
	changes, err := bundlechanges.FromData(context.Background(), bundlechanges.ChangesConfig{
		Bundle: data,
		Logger: loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	records := make([]record, len(changes))
	for i, change := range changes {
		args, err := change.Args()
		c.Assert(err, tc.ErrorIsNil)
		r := record{
			Id:       change.Id(),
			Requires: change.Requires(),
			Method:   change.Method(),
			Args:     args,
			Params:   copyParams(change),
		}
		records[i] = r
		c.Log(change.Description())
	}

	// Output the records for debugging.
	b, err := json.MarshalIndent(records, "", "  ")
	c.Assert(err, tc.ErrorIsNil)
	c.Logf("obtained records: %s", b)

	// Check that the obtained records are what we expect.
	c.Check(records, tc.DeepEquals, expected)
}

func (s *changesSuite) assertLocalBundleChanges(c *tc.C, charmDir, bundleContent, base string) {
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: charmDir,
			Base:  base,
		},
		Args: map[string]interface{}{
			"charm": charmDir,
			"base":  base,
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Base:        base,
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"base":        base,
		},
		Requires: []string{"addCharm-0"},
	}}
	s.assertParseData(c, bundleContent, expected)
}

func (s *changesSuite) assertLocalBundleChangesWithDevices(c *tc.C, charmDir, bundleContent, base string) {
	expected := []record{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Params: bundlechanges.AddCharmParams{
			Charm: charmDir,
			Base:  base,
		},
		Args: map[string]interface{}{
			"charm": charmDir,
			"base":  base,
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Params: bundlechanges.AddApplicationParams{
			Charm:       "$addCharm-0",
			Application: "django",
			Base:        base,
		},
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"base":        base,
		},
		Requires: []string{"addCharm-0"},
	}}
	s.assertParseDataWithDevices(c, bundleContent, expected)
}

func (s *changesSuite) TestLocalCharmWithExplicitBase(c *tc.C) {
	charmDir := c.MkDir()
	bundleContent := fmt.Sprintf(`
        applications:
            django:
                charm: %s
                base: ubuntu@16.04
    `, charmDir)
	charmMeta := `
name: multi-series
summary: That's a dummy charm with multi-series.
description: A dummy charm.
series:
    - jammy
    - focal
    - bionic
`[1:]
	err := os.WriteFile(filepath.Join(charmDir, "metadata.yaml"), []byte(charmMeta), 0644)
	c.Assert(err, tc.ErrorIsNil)
	s.assertLocalBundleChanges(c, charmDir, bundleContent, "ubuntu@16.04/stable")
	s.assertLocalBundleChangesWithDevices(c, charmDir, bundleContent, "ubuntu@16.04/stable")
}

func (s *changesSuite) TestLocalCharmWithSeriesFromCharm(c *tc.C) {
	tmpDir := filepath.Join(c.MkDir(), "multiseries")
	charmPath := filepath.Join(tmpDir, "multiseries.charm")
	err := os.Mkdir(tmpDir, 0700)
	c.Assert(err, tc.ErrorIsNil)
	bundleContent := fmt.Sprintf(`
        applications:
            django:
                charm: %s
    `, charmPath)
	charmMeta := `
name: multi-series
summary: That's a dummy charm with multi-series.
description: A dummy charm.
`[1:]
	charmManifest := `
bases:
- name: ubuntu
  channel: "22.04"  
- name: ubuntu
  channel: "20.04"  
- name: ubuntu
  channel: "18.04"  
`[1:]
	err = os.WriteFile(filepath.Join(tmpDir, "metadata.yaml"), []byte(charmMeta), 0644)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(tmpDir, "manifest.yaml"), []byte(charmManifest), 0644)
	c.Assert(err, tc.ErrorIsNil)

	charmDir, err := charmtesting.ReadCharmDir(tmpDir)
	c.Assert(err, tc.ErrorIsNil)
	err = charmDir.ArchiveToPath(charmPath)
	c.Assert(err, tc.ErrorIsNil)

	s.assertLocalBundleChanges(c, charmPath, bundleContent, "ubuntu@22.04/stable")
	s.assertLocalBundleChangesWithDevices(c, charmPath, bundleContent, "ubuntu@22.04/stable")
}

func (s *changesSuite) TestLocalCharmWithBaseFromBundle(c *tc.C) {
	tmpDir := c.MkDir()
	charmPath := filepath.Join(tmpDir, "django.charm")
	bundleContent := fmt.Sprintf(`
        default-base: ubuntu@20.04
        applications:
            django:
                charm: %s
    `, charmPath)
	charmMeta := `
name: multi-series
summary: That's a dummy charm with multi-series.
description: A dummy charm.
`[1:]
	charmManifest := `
bases:
- name: ubuntu
  channel: "22.04"  
- name: ubuntu
  channel: "20.04"  
- name: ubuntu
  channel: "18.04"  
`[1:]
	err := os.WriteFile(filepath.Join(tmpDir, "metadata.yaml"), []byte(charmMeta), 0644)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(tmpDir, "manifest.yaml"), []byte(charmManifest), 0644)
	c.Assert(err, tc.ErrorIsNil)

	charmDir, err := charmtesting.ReadCharmDir(tmpDir)
	c.Assert(err, tc.ErrorIsNil)
	err = charmDir.ArchiveToPath(charmPath)
	c.Assert(err, tc.ErrorIsNil)

	s.assertLocalBundleChanges(c, charmPath, bundleContent, "ubuntu@20.04/stable")
	s.assertLocalBundleChangesWithDevices(c, charmPath, bundleContent, "ubuntu@20.04/stable")
}

func (s *changesSuite) TestSimpleBundleEmptyModel(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
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
		"upload charm django from charm-hub",
		"deploy application django from charm-hub",
		"expose all endpoints of django and allow access from CIDRs 0.0.0.0/0 and ::/0",
		"set annotations for django",
		"add unit django/0 to new machine 0",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestKubernetesBundleEmptyModel(c *tc.C) {
	bundleContent := `
                bundle: kubernetes
                applications:
                    django:
                        charm: ch:django
                        expose: yes
                        num_units: 1
                        options:
                            key-1: value-1
                            key-2: value-2
                        annotations:
                            gui-x: "10"
                            gui-y: "50"
                    mariadb:
                        charm: ch:mariadb
                        num_units: 2
            `
	expectedChanges := []string{
		"upload charm django from charm-hub",
		"deploy application django from charm-hub with 1 unit",
		"expose all endpoints of django and allow access from CIDRs 0.0.0.0/0 and ::/0",
		"set annotations for django", "upload charm mariadb from charm-hub",
		"deploy application mariadb from charm-hub with 2 units",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestCharmInUseByAnotherApplication(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        revision: 4
                        channel: stable
                        num_units: 1
                        expose: yes
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"other-app": {
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "stable",
			},
		},
	}
	expectedChanges := []string{
		"deploy application django from charm-hub with stable",
		"expose all endpoints of django and allow access from CIDRs 0.0.0.0/0 and ::/0",
		"add unit django/0 to new machine 0",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestExposeOverlayParameters(c *tc.C) {
	bundleContent := `
bundle: kubernetes
applications:
    django:
      charm: ch:django
      revision: 4
      channel: stable
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
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "stable",
				Scale:    3,
				Exposed:  true,
			},
		},
	}
	expectedChanges := []string{
		"expose all endpoints of django and allow access from CIDR 0.0.0.0/0",
		"override expose settings for endpoints admin,www of django and allow access from CIDRs 13.37.0.0/16,192.168.0.0/16",
		"override expose settings for endpoint dmz of django and allow access from space public and CIDR 13.37.0.0/16",
		"scale django to 2 units",
	}
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", 4, nil
	})
}

func (s *changesSuite) TestExposeOverlayParametersForNonCurrentlyExposedApp(c *tc.C) {
	// Here, we expose a single endpoint for an application that is NOT
	// currently exposed. The change description should be slightly
	// different to indicate to the operator that the application is marked
	// as "exposed".
	bundleContent := `
bundle: kubernetes
applications:
    django:
      charm: ch:django
      revision: 4
      channel: stable
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
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "stable",
				Scale:    3,
				Exposed:  false,
			},
		},
	}
	expectedChanges := []string{
		"override expose settings for endpoint www of django and allow access from CIDRs 13.37.0.0/16,192.168.0.0/16",
		"scale django to 2 units",
	}
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", 4, nil
	})
}

func (s *changesSuite) TestExposeOverlayParametersWithOnlyWildcardEntry(c *tc.C) {
	bundleContent := `
bundle: kubernetes
applications:
    django:
      charm: ch:django
      revision: 4
      channel: stable
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
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "stable",
				Scale:    3,
				Exposed:  true,
			},
		},
	}
	expectedChanges := []string{
		"expose all endpoints of django and allow access from CIDRs 13.37.0.0/16,192.168.0.0/16",
		"scale django to 2 units",
	}
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", 4, nil
	})
}

func (s *changesSuite) TestCharmUpgrade(c *tc.C) {
	c.Skip("TODO: Fix bug in charm upgrade with charm-hub")
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        revision: 6
                        channel: stable
                        num_units: 1
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "stable",
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
				},
			},
		},
	}
	expectedChanges := []string{
		"upload charm django from charm-hub with revision 6",
		"upgrade django from charm-hub using charm django",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestCharmUpgradeWithExistingChannel(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        revision: 6
                        channel: stable
                        num_units: 1
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "edge",
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
				},
			},
		},
	}
	expectedChanges := []string{
		"upload charm django from charm-hub from channel stable",
		"upgrade django from charm-hub using charm django from channel stable",
	}
	s.checkBundleImpl(c, bundleContent, existingModel, expectedChanges, ".*upgrades not supported across channels.*edge.*stable.*", nil, nil)
}

func (s *changesSuite) TestCharmUpgradeWithCharmhubCharmAndExistingChannel(c *tc.C) {
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
		"upload charm django from charm-hub from channel stable",
		"upgrade django from charm-hub using charm django from channel stable",
	}
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", 42, nil
	})
}

func (s *changesSuite) TestAppExistsWithLessUnits(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        revision: 4
                        channel: stable
                        num_units: 2
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "stable",
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
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", 4, nil
	})
}

func (s *changesSuite) TestAppExistsWithDifferentScale(c *tc.C) {
	// Note: In a non UT environment the deployer code would setup
	// correctly for bundles changes and set the application series
	// to kubernetes.  The UT environment does not, set the application
	// series in the bundleContent to compensate.
	bundleContent := `
                bundle: kubernetes
                applications:
                    django:
                        charm: ch:django
                        revision: 4
                        channel: stable
                        num_units: 2
                        base: ubuntu@20.04
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "stable",
				Scale:    3,
				Base:     corebase.MakeDefaultBase("ubuntu", "20.04"),
			},
		},
	}
	expectedChanges := []string{
		"scale django to 2 units",
	}
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", 4, nil
	})
}

func (s *changesSuite) TestNewMachineNumberHigherUnitHigher(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        revision: 4
                        channel: stable
                        num_units: 2
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "stable",
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
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", 4, nil
	})
}

func (s *changesSuite) TestAppWithDifferentConstraints(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        revision: 4
                        channel: stable
                        constraints: cpu-cores=4 cpu-power=42 image-id=ubuntu-bf2
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "stable",
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
		`set constraints for django to "cpu-cores=4 cpu-power=42 image-id=ubuntu-bf2"`,
	}
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", 4, nil
	})
}

func (s *changesSuite) TestAppsWithArchConstraints(c *tc.C) {
	bundleContent := `
                applications:
                    django-one:
                        charm: ch:django
                        constraints: arch=amd64 cpu-cores=4 cpu-power=42 image-id=ubuntu-bf2
                    django-two:
                        charm: ch:django
                        constraints: arch=s390x cpu-cores=4 cpu-power=42 image-id=ubuntu-bf2
            `
	expectedChanges := []string{
		"upload charm django from charm-hub with architecture=amd64",
		"deploy application django-one from charm-hub using django",
		"upload charm django from charm-hub with architecture=s390x",
		"deploy application django-two from charm-hub using django",
	}
	s.checkBundleWithConstraintsParser(c, bundleContent, expectedChanges, constraintParser)
}

func (s *changesSuite) TestExistingAppsWithArchConstraints(c *tc.C) {
	bundleContent := `
                applications:
                    django-one:
                        charm: ch:django
                        revision: 4
                        channel: stable
                        constraints: arch=amd64 cpu-cores=4 cpu-power=42 image-id=ubuntu-bf2
                    django-two:
                        charm: ch:django
                        revision: 4
                        channel: stable
                        constraints: arch=s390x cpu-cores=4 cpu-power=42 image-id=ubuntu-bf2
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django-one": {
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "stable",
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
		"set constraints for django-one to \"arch=amd64 cpu-cores=4 cpu-power=42 image-id=ubuntu-bf2\"",
		"upload charm django from charm-hub with revision 4 with architecture=s390x",
		"deploy application django-two from charm-hub with stable using django",
	}
	s.checkBundleImpl(c, bundleContent, existingModel, expectedChanges, "", constraintParser, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", 4, nil
	})
}

func (s *changesSuite) TestExistingAppsWithoutArchConstraints(c *tc.C) {
	bundleContent := `
                applications:
                    django-one:
                        charm: ch:django
                        revision: 4
                        channel: stable
                        constraints: arch=amd64 cpu-cores=4 cpu-power=42 image-id=ubuntu-bf2
                    django-two:
                        charm: ch:django
                        revision: 4
                        channel: stable
                        constraints: arch=s390x cpu-cores=4 cpu-power=42 image-id=ubuntu-bf2
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django-one": {
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "stable",
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
		"upload charm django from charm-hub with revision 4 with architecture=amd64",
		"set constraints for django-one to \"arch=amd64 cpu-cores=4 cpu-power=42 image-id=ubuntu-bf2\"",
		"upload charm django from charm-hub with revision 4 with architecture=s390x",
		"deploy application django-two from charm-hub with stable using django",
	}
	s.checkBundleImpl(c, bundleContent, existingModel, expectedChanges, "", constraintParser, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", 4, nil
	})
}

func (s *changesSuite) TestAppsWithSeriesAndArchConstraints(c *tc.C) {
	bundleContent := `
                applications:
                    django-one:
                        charm: ch:django
                        constraints: arch=amd64 cpu-cores=4 cpu-power=42 image-id=ubuntu-bf2
                        base: ubuntu@18.04
                    django-two:
                        charm: ch:django
                        constraints: arch=s390x cpu-cores=4 cpu-power=42 image-id=ubuntu-bf2
                        base: ubuntu@18.04
                    django-three:
                        charm: ch:django
                        constraints: arch=s390x cpu-cores=4 cpu-power=42 image-id=ubuntu-bf2
                        base: ubuntu@20.04/candidate
            `
	expectedChanges := []string{
		"upload charm django from charm-hub for base ubuntu@18.04/stable with architecture=amd64",
		"deploy application django-one from charm-hub on ubuntu@18.04/stable using django",
		"upload charm django from charm-hub for base ubuntu@20.04/candidate with architecture=s390x",
		"deploy application django-three from charm-hub on ubuntu@20.04/candidate using django",
		"upload charm django from charm-hub for base ubuntu@18.04/stable with architecture=s390x",
		"deploy application django-two from charm-hub on ubuntu@18.04/stable using django",
	}
	s.checkBundleWithConstraintsParser(c, bundleContent, expectedChanges, constraintParser)
}

func (s *changesSuite) TestAppWithArchConstraints(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        constraints: arch=amd64 cpu-cores=4 cpu-power=42 image-id=ubuntu-bf2
            `
	expectedChanges := []string{
		"upload charm django from charm-hub with architecture=amd64",
		"deploy application django from charm-hub",
	}
	s.checkBundleWithConstraintsParser(c, bundleContent, expectedChanges, constraintParser)
}

func (s *changesSuite) TestAppWithArchConstraintsWithError(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        constraints: arch=amd64 cpu-cores=4 cpu-power=42
            `
	s.checkBundleWithConstraintsParserError(c, bundleContent, "bad", constraintParserWithError(errors.Errorf("bad")))
}

func (s *changesSuite) TestAppWithArchConstraintsWithNoParser(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        constraints: arch=amd64 cpu-cores=4 cpu-power=42
            `
	expectedChanges := []string{
		"upload charm django from charm-hub",
		"deploy application django from charm-hub",
	}
	s.checkBundleWithConstraintsParser(c, bundleContent, expectedChanges, nil)
}

func (s *changesSuite) TestAppExistsWithEnoughUnits(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        revision: 4
                        channel: stable
                        num_units: 2
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "stable",
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
					{"django/1", "1"},
					{"django/2", "2"},
				},
			},
		},
	}
	expectedChanges := []string{}
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", 4, nil
	})
}

func (s *changesSuite) TestAppExistsWithChangedOptionsAndAnnotations(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        revision: 4
                        channel: stable
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
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "stable",
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
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", 4, nil
	})
}

func (s *changesSuite) TestNewMachineAnnotationsAndPlacement(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
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
		"upload charm django from charm-hub",
		"deploy application django from charm-hub",
		"add new machine 0 (bundle machine 1)",
		"set annotations for new machine 0",
		"add unit django/0 to new machine 0",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestFinalPlacementNotReusedIfSpecifiesMachine(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        num_units: 2
                        to: [1]
                machines:
                    1:
            `
	expectedChanges := []string{
		"upload charm django from charm-hub",
		"deploy application django from charm-hub",
		"add new machine 0 (bundle machine 1)",
		"add unit django/0 to new machine 0",
		// NOTE: new machine, not put on $1.
		"add unit django/1 to new machine 1",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestFinalPlacementNotReusedIfSpecifiesUnit(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        num_units: 1
                    nginx:
                        charm: ch:nginx
                        num_units: 2
                        to: ["django/0"]
            `
	expectedChanges := []string{
		"upload charm django from charm-hub",
		"deploy application django from charm-hub",
		"upload charm nginx from charm-hub",
		"deploy application nginx from charm-hub",
		"add unit django/0 to new machine 0",
		"add unit nginx/0 to new machine 0 to satisfy [django/0]",
		"add unit nginx/1 to new machine 1",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestUnitPlaceNextToOtherNewUnitOnExistingMachine(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        num_units: 1
                        to: [1]
                    nginx:
                        charm: ch:nginx
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
		"upload charm django from charm-hub",
		"deploy application django from charm-hub",
		"upload charm nginx from charm-hub",
		"deploy application nginx from charm-hub",
		"add unit django/0 to existing machine 0",
		"add unit nginx/0 to existing machine 0 to satisfy [django/0]",
	}
	s.checkBundleExistingModel(c, bundleContent, existingModel, expectedChanges)
}

func (s *changesSuite) TestApplicationPlacementNotEnoughUnits(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        num_units: 3
                    nginx:
                        charm: ch:nginx
                        num_units: 5
                        to: [django]
            `
	expectedChanges := []string{
		"upload charm django from charm-hub",
		"deploy application django from charm-hub",
		"upload charm nginx from charm-hub",
		"deploy application nginx from charm-hub",
		"add unit django/0 to new machine 0",
		"add unit django/1 to new machine 1",
		"add unit django/2 to new machine 2",
		"add unit nginx/0 to new machine 0 to satisfy [django]",
		"add unit nginx/1 to new machine 1 to satisfy [django]",
		"add unit nginx/2 to new machine 2 to satisfy [django]",
		"add unit nginx/3 to new machine 3", "add unit nginx/4 to new machine 4",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestApplicationPlacementSomeExisting(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        revision: 4
                        channel: stable
                        num_units: 5
                    nginx:
                        charm: ch:nginx
                        num_units: 5
                        to: [django]
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "stable",
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
		"upload charm nginx from charm-hub",
		"deploy application nginx from charm-hub",
		"add unit django/4 to new machine 4",
		"add unit django/5 to new machine 5",
		"add unit nginx/0 to existing machine 0 to satisfy [django]",
		"add unit nginx/1 to existing machine 1 to satisfy [django]",
		"add unit nginx/2 to existing machine 3 to satisfy [django]",
		"add unit nginx/3 to new machine 4 to satisfy [django]",
		"add unit nginx/4 to new machine 5 to satisfy [django]",
	}
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(_ context.Context, charm string, _ corebase.Base, channel, _ string, rev int) (string, int, error) {
		if charm == "ch:django" {
			return "stable", 4, nil
		}
		return "stable", -1, nil
	})
}

func (s *changesSuite) TestApplicationPlacementSomeColocated(c *tc.C) {
	bundleContent := `
                applications:
                    django:
                        charm: ch:django
                        revision: 4
                        channel: stable
                        num_units: 5
                    nginx:
                        charm: ch:nginx
                        revision: 76
                        channel: stable
                        num_units: 5
                        to: [django]
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"django": {
				Charm:    "ch:django",
				Revision: 4,
				Channel:  "stable",
				Units: []bundlechanges.Unit{
					{"django/0", "0"},
					{"django/1", "1"},
					{"django/3", "3"},
				},
			},
			"nginx": {
				Charm:    "ch:nginx",
				Revision: 76,
				Channel:  "stable",
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
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(ctx context.Context, charm string, _ corebase.Base, channel, _ string, rev int) (string, int, error) {
		if charm == "ch:django" {
			return "stable", 4, nil
		}
		if charm == "ch:nginx" {
			return "stable", 76, nil
		}
		return "stable", -1, nil
	})
}

func (s *changesSuite) TestWeirdUnitDeployedNoExistingModel(c *tc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: mysql
                        num_units: 3
                        # The first placement directive here is skipped because
                        # the existing model already has one unit.
                        to: [new, "lxd:0", "lxd:new"]
                    keystone:
                        charm: keystone
                        num_units: 3
                        to: ["lxd:mysql"]
                machines:
                    0:
            `
	expectedChanges := []string{
		"upload charm keystone from charm-hub",
		"deploy application keystone from charm-hub",
		"upload charm mysql from charm-hub",
		"deploy application mysql from charm-hub",
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

func (s *changesSuite) TestUnitDeployedDefinedMachine(c *tc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: ch:mysql
                        revision: 4
                        channel: stable
                        num_units: 3
                        to: [new, "lxd:0", "lxd:new"]
                    keystone:
                        charm: ch:keystone
                        num_units: 3
                        to: ["lxd:mysql"]
                machines:
                    0:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"mysql": {
				Charm:    "ch:mysql",
				Revision: 4,
				Channel:  "stable",
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
		"upload charm keystone from charm-hub",
		"deploy application keystone from charm-hub",
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
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", -1, nil
	})
}

func (s *changesSuite) TestLXDContainerSequence(c *tc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: ch:mysql
                        revision: 4
                        channel: stable
                        num_units: 1
                    keystone:
                        charm: ch:keystone
                        num_units: 1
                        to: ["lxd:mysql"]
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"mysql": {
				Charm:    "ch:mysql",
				Revision: 4,
				Channel:  "stable",
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
		"upload charm keystone from charm-hub",
		"deploy application keystone from charm-hub",
		"add unit keystone/0 to 0/lxd/2 to satisfy [lxd:mysql]",
	}
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", -1, nil
	})
}

func (s *changesSuite) TestMachineMapToExistingMachineSomeDeployed(c *tc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: ch:mysql
                        revision: 32
                        channel: stable
                        num_units: 3
                        # The first placement directive here is skipped because
                        # the existing model already has one unit.
                        to: [new, "lxd:0", "lxd:new"]
                    keystone:
                        charm: ch:keystone
                        num_units: 3
                        to: ["lxd:mysql"]
                machines:
                    0:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"mysql": {
				Charm:    "ch:mysql",
				Revision: 32,
				Channel:  "stable",
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
		"upload charm keystone from charm-hub",
		"deploy application keystone from charm-hub",
		// First unit of keystone goes in a container next to the existing mysql.
		"add unit keystone/0 to 0/lxd/1 to satisfy [lxd:mysql]",
		"add new machine 3",
		// Two more units of mysql are needed, and the "lxd:0" is unsatisfied
		// because machine 0 has been mapped to machine 2, and mysql isn't on machine 2.
		// Due to this, the placements directives are popped off as needed,
		// First one is "new", second is "lxd:0", and since 0 is mapped to 2, the lxd
		// is created on machine 2.
		"add unit mysql/1 to new machine 3",
		"add unit mysql/2 to 2/lxd/1",
		"add lxd container 3/lxd/0 on new machine 3",
		"add lxd container 2/lxd/2 on existing machine 2",
		// Next, units of keystone go next to the new mysql units.
		"add unit keystone/1 to 3/lxd/0 to satisfy [lxd:mysql]",
		"add unit keystone/2 to 2/lxd/2 to satisfy [lxd:mysql]",
	}
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(ctx context.Context, charm string, _ corebase.Base, channel, _ string, rev int) (string, int, error) {
		if charm == "ch:mysql" {
			return "stable", 32, nil
		}
		return "stable", -1, nil
	})
}

func (s *changesSuite) TestSettingAnnotationsForExistingMachine(c *tc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: ch:mysql
                        revision: 42
                        channel: stable
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
				Charm:    "ch:mysql",
				Revision: 42,
				Channel:  "stable",
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
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", -1, nil
	})
}

func (s *changesSuite) TestSiblingContainers(c *tc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: ch:mysql
                        num_units: 3
                        to: ["lxd:new"]
                    keystone:
                        charm: ch:keystone
                        num_units: 3
                        to: ["lxd:mysql"]
            `
	expectedChanges := []string{
		"upload charm keystone from charm-hub",
		"deploy application keystone from charm-hub",
		"upload charm mysql from charm-hub",
		"deploy application mysql from charm-hub",
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

func (s *changesSuite) TestSiblingContainersSomeDeployed(c *tc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: ch:mysql
                        revision: 32
                        channel: stable
                        num_units: 3
                        to: ["lxd:new"]
                    keystone:
                        charm: ch:keystone
                        revision: 47
                        channel: stable
                        num_units: 4
                        to: ["lxd:mysql"]
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"mysql": {
				Charm:    "ch:mysql",
				Revision: 32,
				Channel:  "stable",
				Units: []bundlechanges.Unit{
					{"mysql/0", "0/lxd/0"},
					{"mysql/1", "1/lxd/0"},
					{"mysql/2", "2/lxd/0"},
				},
			},
			"keystone": {
				Charm:    "ch:keystone",
				Revision: 47,
				Channel:  "stable",
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
		"add unit keystone/4 to new machine 3",
	}
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(ctx context.Context, charm string, _ corebase.Base, channel, _ string, rev int) (string, int, error) {
		if charm == "ch:mysql" {
			return "stable", 32, nil
		}
		if charm == "ch:keystone" {
			return "stable", 47, nil
		}
		return "stable", -1, nil
	})
}

func (s *changesSuite) TestColocationIntoAContainerUsingUnitPlacement(c *tc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: ch:mysql
                        num_units: 3
                        to: ["lxd:new"]
                    keystone:
                        charm: ch:keystone
                        num_units: 3
                        to: [mysql/0, mysql/1, mysql/2]
            `
	expectedChanges := []string{
		"upload charm keystone from charm-hub",
		"deploy application keystone from charm-hub",
		"upload charm mysql from charm-hub",
		"deploy application mysql from charm-hub",
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

func (s *changesSuite) TestColocationIntoAContainerUsingAppPlacement(c *tc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: ch:mysql
                        num_units: 3
                        to: ["lxd:new"]
                    keystone:
                        charm: ch:keystone
                        num_units: 3
                        to: ["mysql"]
            `
	expectedChanges := []string{
		"upload charm keystone from charm-hub",
		"deploy application keystone from charm-hub",
		"upload charm mysql from charm-hub",
		"deploy application mysql from charm-hub",
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

func (s *changesSuite) TestPlacementDescriptionsForUnitPlacement(c *tc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: ch:mysql
                        num_units: 3
                    keystone:
                        charm: ch:keystone
                        num_units: 3
                        to: ["lxd:mysql/0", "lxd:mysql/1", "lxd:mysql/2"]
            `
	expectedChanges := []string{
		"upload charm keystone from charm-hub",
		"deploy application keystone from charm-hub",
		"upload charm mysql from charm-hub",
		"deploy application mysql from charm-hub",
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

func (s *changesSuite) TestMostAppOptions(c *tc.C) {
	bundleContent := `
                applications:
                    mediawiki:
                        charm: ch:mediawiki
                        base: ubuntu@20.04
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
                        charm: ch:mysql
                        base: ubuntu@20.04
                        num_units: 1
                        resources:
                          data: "./resources/data.tar"
                default-base: ubuntu@18.04
                relations:
                    - - mediawiki:db
                      - mysql:db
            `
	expectedChanges := []string{
		"upload charm mediawiki from charm-hub for base ubuntu@20.04/stable",
		"deploy application mediawiki from charm-hub on ubuntu@20.04/stable",
		"expose all endpoints of mediawiki and allow access from CIDRs 0.0.0.0/0 and ::/0",
		"set annotations for mediawiki",
		"upload charm mysql from charm-hub for base ubuntu@20.04/stable",
		"deploy application mysql from charm-hub on ubuntu@20.04/stable",
		"add relation mediawiki:db - mysql:db",
		"add unit mediawiki/0 to new machine 0",
		"add unit mysql/0 to new machine 1",
	}
	s.checkBundle(c, bundleContent, expectedChanges)
}

func (s *changesSuite) TestUnitOrdering(c *tc.C) {
	bundleContent := `
                applications:
                    memcached:
                        charm: ch:mem
                        base: ubuntu@16.04
                        num_units: 3
                        to: [1, 2, 3]
                    django:
                        charm: ch:django
                        revision: 42
                        channel: stable
                        base: ubuntu@16.04
                        num_units: 4
                        to:
                            - 1
                            - lxd:memcached
                    ror:
                        charm: ch:rails
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
		"upload charm django from charm-hub for base ubuntu@16.04/stable with revision 42",
		"deploy application django from charm-hub on ubuntu@16.04/stable with stable",
		"upload charm mem from charm-hub for base ubuntu@16.04/stable",
		"deploy application memcached from charm-hub on ubuntu@16.04/stable using mem",
		"upload charm rails from charm-hub",
		"deploy application ror from charm-hub using rails",
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

func (s *changesSuite) TestMachineAddedInNumericalOrder(c *tc.C) {
	bundleContent := `
               applications:
                   ubu:
                       charm: ubuntu
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
		"upload charm ubuntu from charm-hub",
		"deploy application ubu from charm-hub using ubuntu",
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

func (s *changesSuite) TestAddUnitToExistingApp(c *tc.C) {
	bundleContent := `
                applications:
                    mediawiki:
                        charm: ch:mediawiki
                        revision: 10
                        channel: stable
                        base: ubuntu@20.04
                        num_units: 2
                    mysql:
                        charm: ch:mysql
                        revision: 28
                        channel: stable
                        base: ubuntu@20.04
                        num_units: 1
                default-base: ubuntu@22.04
                relations:
                    - - mediawiki:db
                      - mysql:db
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"mediawiki": {
				Charm:    "ch:mediawiki",
				Revision: 10,
				Channel:  "stable",
				Units: []bundlechanges.Unit{
					{"mediawiki/0", "1"},
				},
				Base: corebase.MakeDefaultBase("ubuntu", "20.04"),
			},
			"mysql": {
				Charm:    "ch:mysql",
				Revision: 28,
				Channel:  "stable",
				Units: []bundlechanges.Unit{
					{"mysql/0", "0"},
				},
				Base: corebase.MakeDefaultBase("ubuntu", "20.04"),
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
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(ctx context.Context, charm string, _ corebase.Base, channel, _ string, rev int) (string, int, error) {
		if charm == "ch:mediawiki" {
			return "stable", 10, nil
		}
		if charm == "ch:mysql" {
			return "stable", 28, nil
		}
		return "stable", -1, nil
	})
}

func (s *changesSuite) TestPlacementCycle(c *tc.C) {
	bundleContent := `
                applications:
                    mysql:
                        charm: ch:mysql
                        num_units: 3
                        to: [new, "lxd:0", "lxd:keystone/2"]
                    keystone:
                        charm: ch:keystone
                        num_units: 3
                        to: ["lxd:mysql"]
                machines:
                    0:
            `
	s.checkBundleError(c, bundleContent, "cycle in placement directives for: keystone, mysql")
}

func (s *changesSuite) TestPlacementCycleSameApp(c *tc.C) {
	bundleContent := `
                applications:
                    problem:
                        charm: ch:problem
                        num_units: 2
                        to: ["lxd:new", "lxd:problem/0"]
            `
	s.checkBundleError(c, bundleContent, `cycle in placement directives for: problem`)
}

func (s *changesSuite) TestAddMissingUnitToNotLastPlacement(c *tc.C) {
	bundleContent := `
                applications:
                    foo:
                        charm: ch:foo
                        revision: 5
                        channel: stable
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
				Charm:    "ch:foo",
				Revision: 5,
				Channel:  "stable",
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
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", 5, nil
	})
}

func (s *changesSuite) TestAddMissingUnitToNotLastPlacementExisting(c *tc.C) {
	bundleContent := `
                applications:
                    foo:
                        charm: ch:foo
                        revision: 5
                        channel: stable
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
				Charm:    "ch:foo",
				Revision: 5,
				Channel:  "stable",
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
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(context.Context, string, corebase.Base, string, string, int) (string, int, error) {
		return "stable", 5, nil
	})
}

func (s *changesSuite) TestFromJujuMassiveUnitColocation(c *tc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: ch:mem
                base: ubuntu@16.04
                revision: 47
                channel: stable
                num_units: 3
                to: [1, 2, 3]
            django:
                charm: ch:django
                base: ubuntu@16.04
                revision: 42
                channel: stable
                num_units: 4
                to:
                    - 1
                    - lxd:memcached
            node:
                charm: ch:django
                base: ubuntu@16.04
                revision: 42
                channel: stable
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
				Name:     "django",
				Charm:    "ch:django",
				Revision: 42,
				Channel:  "stable",
				Units: []bundlechanges.Unit{
					{Name: "django/2", Machine: "1/lxd/0"},
					{Name: "django/3", Machine: "2/lxd/0"},
					{Name: "django/0", Machine: "0"},
					{Name: "django/1", Machine: "0/lxd/0"},
				},
				Base: corebase.MakeDefaultBase("ubuntu", "16.04"),
			},
			"memcached": {
				Name:     "memcached",
				Charm:    "ch:mem",
				Channel:  "stable",
				Revision: 47,
				Units: []bundlechanges.Unit{
					{Name: "memcached/0", Machine: "0"},
					{Name: "memcached/1", Machine: "1"},
					{Name: "memcached/2", Machine: "2"},
				},
				Base: corebase.MakeDefaultBase("ubuntu", "16.04"),
			},
			"ror": {
				Name:     "ror",
				Charm:    "ch:rails",
				Revision: 0,
				Channel:  "stable",
				Units: []bundlechanges.Unit{
					{Name: "ror/0", Machine: "0"},
					{Name: "ror/1", Machine: "2/kvm/0"},
					{Name: "ror/2", Machine: "3"},
				},
				Base: corebase.MakeDefaultBase("ubuntu", "16.04"),
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
		"deploy application node from charm-hub on ubuntu@16.04/stable with stable using django",
		"add unit node/0 to 0/lxd/0 to satisfy [lxd:memcached]",
	}
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(ctx context.Context, charm string, _ corebase.Base, channel, _ string, rev int) (string, int, error) {
		if charm == "ch:django" {
			return "stable", 42, nil
		}
		if charm == "ch:mem" {
			return "stable", 47, nil
		}
		return "stable", -1, nil
	})
}

func (s *changesSuite) TestInconsistentMappingError(c *tc.C) {
	// https://bugs.launchpad.net/juju/+bug/1773357 This bug occurs
	// when the model machine map is pre-set incorrectly, and the
	// applications all have enough units, but the mapping omits some
	// machines that host those units. FromData includes changes to
	// create machines but then doesn't put any units on them. It
	// should return an error indicating the inconsistency instead.
	bundleContent := `
        applications:
            memcached:
                charm: ch:mem
                base: ubuntu@16.04
                revision: 47
                channel: stable
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"memcached": {
				Name:     "memcached",
				Charm:    "ch:mem",
				Base:     corebase.MakeDefaultBase("ubuntu", "16.04"),
				Revision: 47,
				Channel:  "stable",
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
	s.checkBundleImpl(c, bundleContent, existingModel, nil, `bundle and machine mapping are inconsistent: need an explicit entry mapping bundle machine "0", perhaps to one of model machines \["2", "3"\] - the target should host \[memcached\]`, nil, func(ctx context.Context, charm string, _ corebase.Base, channel, _ string, rev int) (string, int, error) {
		if charm == "ch:mem" {
			return "stable", 47, nil
		}
		return channel, rev, nil
	})
}

func (s *changesSuite) TestConsistentMapping(c *tc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: ch:mem
                base: ubuntu@16.04
                revision: 47
                channel: stable
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"memcached": {
				Name:     "memcached",
				Charm:    "ch:mem",
				Revision: 47,
				Channel:  "stable",
				Units: []bundlechanges.Unit{
					{Name: "memcached/1", Machine: "1"},
					{Name: "memcached/2", Machine: "2"},
					{Name: "memcached/3", Machine: "3"},
				},
				Base: corebase.MakeDefaultBase("ubuntu", "16.04"),
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
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, nil, func(ctx context.Context, charm string, _ corebase.Base, channel, _ string, rev int) (string, int, error) {
		if charm == "ch:mem" {
			return "stable", 47, nil
		}
		return "stable", -1, nil
	})
}

func (s *changesSuite) TestContainerHosts(c *tc.C) {
	// If we have a bundle that needs to create a container, we don't
	// treat the machine hosting the container as not having
	// dependants.
	bundleContent := `
        applications:
            memcached:
                charm: ch:mem
                base: ubuntu@16.04
                revision: 47
                channel: stable
                num_units: 2
                to: [1, "lxd:2"]
        machines:
            1:
            2:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"memcached": {
				Name:     "memcached",
				Charm:    "ch:mem",
				Revision: 47,
				Channel:  "stable",
				Units: []bundlechanges.Unit{
					{Name: "memcached/1", Machine: "1"},
				},
				Base: corebase.MakeDefaultBase("ubuntu", "16.04"),
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
	s.checkBundleExistingModelWithRevisionParser(c, bundleContent, existingModel, expectedChanges, func(ctx context.Context, charm string, _ corebase.Base, channel, _ string, rev int) (string, int, error) {
		if charm == "ch:mem" {
			return "stable", 47, nil
		}
		return "stable", -1, nil
	})
}

func (s *changesSuite) TestSingleTarget(c *tc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: ch:mem
                base: ubuntu@16.04
                revision: 47
                channel: stable
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"memcached": {
				Name:     "memcached",
				Charm:    "ch:mem",
				Revision: 47,
				Channel:  "stable",
				Base:     corebase.MakeDefaultBase("ubuntu", "16.04"),
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
	s.checkBundleImpl(c, bundleContent, existingModel, nil, `bundle and machine mapping are inconsistent: need an explicit entry mapping bundle machine "0", perhaps to unreferenced model machine "2" - the target should host \[memcached\]`, nil, func(ctx context.Context, charm string, _ corebase.Base, channel, _ string, rev int) (string, int, error) {
		if charm == "ch:mem" {
			return "stable", 47, nil
		}
		return channel, rev, nil
	})
}

func (s *changesSuite) TestMultipleApplications(c *tc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: ch:mem
                base: ubuntu@16.04
                revision: 47
                channel: stable
                num_units: 2
                to: [0, 1]
            prometheus:
                charm: ch:prom
                base: ubuntu@16.04
                revision: 22
                channel: stable
                num_units: 1
                to: [0]
        machines:
            0:
            1:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"memcached": {
				Name:     "memcached",
				Charm:    "ch:mem",
				Revision: 47,
				Channel:  "stable",
				Units: []bundlechanges.Unit{
					{Name: "memcached/1", Machine: "1"},
					{Name: "memcached/2", Machine: "2"},
				},
				Base: corebase.MakeDefaultBase("ubuntu", "16.04"),
			},
			"prometheus": {
				Name:    "prometheus",
				Charm:   "ch:prom",
				Channel: "stable",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/1", Machine: "2"},
				},
				Base: corebase.MakeDefaultBase("ubuntu", "16.04"),
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
	s.checkBundleImpl(c, bundleContent, existingModel, nil, `bundle and machine mapping are inconsistent: need an explicit entry mapping bundle machine "0", perhaps to unreferenced model machine "2" - the target should host \[memcached, prometheus\]`, nil, func(ctx context.Context, charm string, _ corebase.Base, channel, _ string, rev int) (string, int, error) {
		if charm == "ch:mem" {
			return "stable", 47, nil
		}
		if charm == "ch:prom" {
			return "stable", 22, nil
		}
		return channel, rev, nil
	})
}

func (s *changesSuite) TestNoApplications(c *tc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: ch:mem
                base: ubuntu@16.04
                revision: 47
                channel: stable
                num_units: 2
                to: ["lxd:0", 1]
            prometheus:
                charm: ch:prom
                base: ubuntu@16.04
                revision: 22
                channel: stable
                num_units: 1
                to: [memcached/0]
        machines:
            0:
            1:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"memcached": {
				Name:     "memcached",
				Charm:    "ch:mem",
				Revision: 47,
				Channel:  "stable",
				Units: []bundlechanges.Unit{
					{Name: "memcached/1", Machine: "1"},
					{Name: "memcached/2", Machine: "2"},
				},
				Base: corebase.MakeDefaultBase("ubuntu", "16.04"),
			},
			"prometheus": {
				Name:     "prometheus",
				Charm:    "ch:prom",
				Revision: 22,
				Channel:  "stable",
				Units: []bundlechanges.Unit{
					{Name: "prometheus/1", Machine: "2"},
				},
				Base: corebase.MakeDefaultBase("ubuntu", "16.04"),
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
	s.checkBundleImpl(c, bundleContent, existingModel, nil, `bundle and machine mapping are inconsistent: need an explicit entry mapping bundle machine "0", perhaps to unreferenced model machine "2"`, nil, func(ctx context.Context, charm string, _ corebase.Base, channel, _ string, rev int) (string, int, error) {
		if charm == "ch:mem" {
			return "stable", 47, nil
		}
		if charm == "ch:prom" {
			return "stable", 22, nil
		}
		return channel, rev, nil
	})
}

func (s *changesSuite) TestNoPossibleTargets(c *tc.C) {
	bundleContent := `
        applications:
            memcached:
                charm: ch:mem
                base: ubuntu@16.04
                revision: 47
                channel: stable
                num_units: 2
                to: [0, 1]
        machines:
            0:
            1:
            `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"memcached": {
				Name:     "memcached",
				Charm:    "ch:mem",
				Revision: 47,
				Channel:  "stable",
				Units: []bundlechanges.Unit{
					{Name: "memcached/1", Machine: "1"},
					{Name: "memcached/2", Machine: "1"},
				},
				Base: corebase.MakeDefaultBase("ubuntu", "16.04"),
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
	s.checkBundleImpl(c, bundleContent, existingModel, nil, `bundle and machine mapping are inconsistent: need an explicit entry mapping bundle machine "0" - the target should host \[memcached\]`, nil, func(ctx context.Context, charm string, _ corebase.Base, channel, _ string, rev int) (string, int, error) {
		if charm == "ch:mem" {
			return "stable", 47, nil
		}
		return channel, rev, nil
	})
}

func (s *changesSuite) TestRedeploymentOfBundleWithLocalCharms(c *tc.C) {
	bundleContent := `
        applications:
          haproxy:
            charm: "local:haproxy-0"
        `
	existingModel := &bundlechanges.Model{
		Applications: map[string]*bundlechanges.Application{
			"haproxy": {
				Name:     "haproxy",
				Charm:    "local:haproxy-0",
				Revision: 42,
				// NOTE: local charms are not associated with a
				// channel as this information is not available
				// at deploy time.
			},
		},
	}

	// No changes expected.
	s.checkBundleExistingModel(c, bundleContent, existingModel, nil)
}

func (s *changesSuite) checkBundle(c *tc.C, bundleContent string, expectedChanges []string) {
	s.checkBundleImpl(c, bundleContent, nil, expectedChanges, "", nil, nil)
}

func (s *changesSuite) checkBundleExistingModel(c *tc.C, bundleContent string, existingModel *bundlechanges.Model, expectedChanges []string) {
	s.checkBundleImpl(c, bundleContent, existingModel, expectedChanges, "", nil, nil)
}

func (s *changesSuite) checkBundleError(c *tc.C, bundleContent string, errMatch string) {
	s.checkBundleImpl(c, bundleContent, nil, nil, errMatch, nil, nil)
}

func (s *changesSuite) checkBundleWithConstraintsParser(c *tc.C, bundleContent string, expectedChanges []string, parserFn bundlechanges.ConstraintGetter) {
	s.checkBundleImpl(c, bundleContent, nil, expectedChanges, "", parserFn, nil)
}

func (s *changesSuite) checkBundleWithConstraintsParserError(c *tc.C, bundleContent, errMatch string, parserFn bundlechanges.ConstraintGetter) {
	s.checkBundleImpl(c, bundleContent, nil, nil, errMatch, parserFn, nil)
}

func (s *changesSuite) checkBundleExistingModelWithRevisionParser(c *tc.C, bundleContent string, existingModel *bundlechanges.Model, expectedChanges []string, charmResolverFn bundlechanges.CharmResolver) {
	s.checkBundleImpl(c, bundleContent, existingModel, expectedChanges, "", nil, charmResolverFn)
}

func (s *changesSuite) checkBundleImpl(c *tc.C,
	bundleContent string,
	existingModel *bundlechanges.Model,
	expectedChanges []string,
	errMatch string,
	parserFn bundlechanges.ConstraintGetter,
	charmResolverFn bundlechanges.CharmResolver,
) {
	// Retrieve and validate the bundle data merging any overlays in the bundle contents.
	bundleSrc, err := charm.StreamBundleDataSource(strings.NewReader(bundleContent), "./")
	c.Assert(err, tc.ErrorIsNil)
	data, err := charm.ReadAndMergeBundleData(bundleSrc)
	c.Assert(err, tc.ErrorIsNil)
	err = data.Verify(nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	// Retrieve the changes, and convert them to a sequence of records.
	changes, err := bundlechanges.FromData(context.Background(), bundlechanges.ChangesConfig{
		Bundle:           data,
		Model:            existingModel,
		Logger:           loggertesting.WrapCheckLog(c),
		ConstraintGetter: parserFn,
		CharmResolver:    charmResolverFn,
	})
	if errMatch != "" {
		c.Assert(err, tc.ErrorMatches, errMatch)
	} else {
		c.Assert(err, tc.ErrorIsNil)
		var obtained []string
		for _, change := range changes {
			c.Log(change.Description())

			for _, descr := range change.Description() {
				obtained = append(obtained, descr)
			}
		}
		c.Check(obtained, tc.DeepEquals, expectedChanges)
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
