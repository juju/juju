// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

type diffSuite struct {
	testhelpers.IsolationSuite
	apiRoot     *mockAPIRoot
	charmHub    *mockCharmHub
	modelClient *mockModelClient
	dir         string
}

var _ = tc.Suite(&diffSuite{})

func (s *diffSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.apiRoot = &mockAPIRoot{
		responses: makeAPIResponses(),
	}
	s.charmHub = &mockCharmHub{}
	s.modelClient = &mockModelClient{
		constraints: constraints.MustParse("arch=amd64"),
	}
	s.dir = c.MkDir()
}

func (s *diffSuite) runDiffBundle(c *tc.C, args ...string) (*cmd.Context, error) {
	return s.runDiffBundleWithCharmAdaptor(c, func(base.APICallCloser, *charm.URL) (application.BundleResolver, error) {
		return s.charmHub, nil
	}, func(ctx context.Context) (application.ModelConstraintsClient, error) {
		return s.modelClient, nil
	}, args...)
}

func (s *diffSuite) runDiffBundleWithCharmAdaptor(c *tc.C,
	charmAdataperFn func(base.APICallCloser, *charm.URL) (application.BundleResolver, error),
	modelConsFn func(ctx context.Context) (application.ModelConstraintsClient, error),
	args ...string,
) (*cmd.Context, error) {
	store := jujuclienttesting.MinimalStore()
	store.Models["enz"] = &jujuclient.ControllerModels{
		CurrentModel: "golden/horse",
		Models: map[string]jujuclient.ModelDetails{"golden/horse": {
			ModelType: model.IAAS,
		}},
	}
	command := application.NewDiffBundleCommandForTest(s.apiRoot, charmAdataperFn, modelConsFn, store)
	return cmdtesting.RunCommandInDir(c, command, args, s.dir)
}

func (s *diffSuite) TestNoArgs(c *tc.C) {
	_, err := s.runDiffBundle(c)
	c.Assert(err, tc.ErrorMatches, "no bundle specified")
}

func (s *diffSuite) TestTooManyArgs(c *tc.C) {
	_, err := s.runDiffBundle(c, "bundle", "somethingelse")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["somethingelse"\]`)
}

func (s *diffSuite) TestVerifiesBundle(c *tc.C) {
	_, err := s.runDiffBundle(c, s.writeLocalBundle(c, invalidBundle))
	c.Assert(err, tc.ErrorMatches, "(?s)the provided bundle has the following errors:.*")
}

func (s *diffSuite) TestNotABundle(c *tc.C) {
	s.charmHub.url = &charm.URL{
		Schema:   "ch",
		Name:     "prometheus",
		Revision: 23,
	}
	s.apiRoot.responses["ModelConfig.ModelGet"] = params.ModelConfigResults{
		Config: map[string]params.ConfigValue{
			"uuid":           {Value: testing.ModelTag.Id()},
			"type":           {Value: "iaas"},
			"name":           {Value: "horse"},
			"default-base":   {Value: "ubuntu@16.04/stable"},
			"secret-backend": {Value: "auto"},
		},
	}
	s.charmHub.stub.SetErrors(nil, errors.NotValidf("not a bundle"))
	_, err := s.runDiffBundle(c, "prometheus")
	c.Logf("%s", errors.ErrorStack(err))
	// Fails because the series that comes back from the charm store
	// is xenial rather than "bundle" (and there's no local bundle).
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *diffSuite) TestLocalBundle(c *tc.C) {
	ctx, err := s.runDiffBundle(c, s.writeLocalBundle(c, testCharmHubBundle))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
applications:
  grafana:
    missing: bundle
  prometheus:
    options:
      ontology:
        bundle: anselm
        model: kant
    constraints:
      bundle: cores=4
      model: cores=3
machines:
  "1":
    missing: bundle
`[1:])
}

func (s *diffSuite) TestLocalBundleInvalidYaml(c *tc.C) {
	_, err := s.runDiffBundle(c, s.writeLocalBundle(c, invalidYaml))
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err, tc.ErrorMatches, `.*cannot unmarshal bundle contents.*`[1:])
}

func (s *diffSuite) TestIncludeAnnotations(c *tc.C) {
	ctx, err := s.runDiffBundle(c, "--annotations", s.writeLocalBundle(c, testCharmHubBundle))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
applications:
  grafana:
    missing: bundle
  prometheus:
    options:
      ontology:
        bundle: anselm
        model: kant
    annotations:
      aspect:
        bundle: west
        model: north
    constraints:
      bundle: cores=4
      model: cores=3
machines:
  "1":
    missing: bundle
`[1:])
}

func (s *diffSuite) TestHandlesIncludes(c *tc.C) {
	s.writeFile(c, "include.yaml", "hume")
	ctx, err := s.runDiffBundle(c, s.writeLocalBundle(c, withInclude))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
applications:
  grafana:
    missing: bundle
  prometheus:
    options:
      ontology:
        bundle: hume
        model: kant
    constraints:
      bundle: cores=4
      model: cores=3
machines:
  "1":
    missing: bundle
`[1:])
}

func (s *diffSuite) TestHandlesOverlays(c *tc.C) {
	path1 := s.writeFile(c, "overlay1.yaml", overlay1)
	path2 := s.writeFile(c, "overlay2.yaml", overlay2)
	ctx, err := s.runDiffBundle(c,
		"--overlay", path1,
		"--overlay", path2,
		s.writeLocalBundle(c, testCharmHubBundle))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
applications:
  grafana:
    missing: bundle
  prometheus:
    options:
      admin-user:
        bundle: lovecraft
        model: null
      ontology:
        bundle: anselm
        model: kant
    constraints:
      bundle: cores=4
      model: cores=3
  telegraf:
    missing: model
machines:
  "1":
    missing: bundle
relations:
  bundle-additions:
  - - prometheus:juju-info
    - telegraf:info
`[1:])
}

func (s *diffSuite) TestCharmSeriesBundle(c *tc.C) {
	bundleData, err := charm.ReadBundleData(strings.NewReader(withSeries))
	c.Assert(err, tc.ErrorIsNil)
	s.charmHub.url = &charm.URL{
		Schema: "ch",
		Name:   "my-bundle",
	}
	s.charmHub.bundle = &mockBundle{data: bundleData}

	ctx, err := s.runDiffBundle(c, "my-bundle")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
{}
`[1:])
}

func (s *diffSuite) TestBundleNotFound(c *tc.C) {
	s.charmHub.stub.SetErrors(errors.NotFoundf(`cannot resolve URL "ch:my-bundle": charm or bundle`))
	_, err := s.runDiffBundle(c, "ch:my-bundle")
	c.Assert(err, tc.ErrorMatches, `cannot resolve URL "ch:my-bundle": charm or bundle not found`)
}

func (s *diffSuite) TestMachineMap(c *tc.C) {
	ctx, err := s.runDiffBundle(c,
		"--map-machines", "0=1",
		s.writeLocalBundle(c, testCharmHubBundle))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
applications:
  grafana:
    missing: bundle
  prometheus:
    options:
      ontology:
        bundle: anselm
        model: kant
    constraints:
      bundle: cores=4
      model: cores=3
machines:
  "0":
    missing: bundle
  "1":
    base:
      bundle: ubuntu@16.04/stable
      model: ubuntu@18.04/stable
`[1:])
}

func (s *diffSuite) TestCharmHubBundle(c *tc.C) {
	bundleData, err := charm.ReadBundleData(strings.NewReader(testCharmHubBundle))
	c.Assert(err, tc.ErrorIsNil)
	s.charmHub.url = &charm.URL{
		Schema: "ch",
		Name:   "my-bundle",
	}
	s.charmHub.bundle = &mockBundle{data: bundleData}

	ctx, err := s.runDiffBundle(c, "my-bundle")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
applications:
  grafana:
    missing: bundle
  prometheus:
    options:
      ontology:
        bundle: anselm
        model: kant
    constraints:
      bundle: cores=4
      model: cores=3
machines:
  "1":
    missing: bundle
`[1:])
}

func (s *diffSuite) TestRelationsWithMissingEndpoints(c *tc.C) {
	rels := []params.RelationStatus{
		{
			Endpoints: []params.EndpointStatus{
				{ApplicationName: "prometheus", Name: relation.JujuInfo},
				{ApplicationName: "grafana", Name: relation.JujuInfo},
			},
		},
	}
	s.apiRoot = &mockAPIRoot{
		responses: makeAPIResponsesWithRelations(rels),
	}

	ctx, err := s.runDiffBundle(c, s.writeLocalBundle(c, withMissingRelationEndpoints))
	c.Assert(err, tc.ErrorIsNil)

	// Note: the logger output is not captured so only the relevant diff
	// output is checked here.
	exp := `
relations:
  bundle-additions:
  - - 'grafana:'
    - prometheus:juju-info
  model-additions:
  - - grafana:juju-info
    - prometheus:juju-info`

	c.Assert(strings.Contains(cmdtesting.Stdout(ctx), exp[1:]), tc.IsTrue)
}

func (s *diffSuite) TestExposedEndpoints(c *tc.C) {
	specs := []struct {
		descr                 string
		modelExposedEndpoints map[string]params.ExposedEndpoint
		bundle                string
		expDiff               string
	}{
		{
			descr: "2.8-compatible bundle with expose:true",
			modelExposedEndpoints: map[string]params.ExposedEndpoint{
				"": {
					ExposeToSpaces: []string{"alpha"},
					ExposeToCIDRs:  []string{"10.0.0.0/24"},
				},
			},
			bundle: `
applications:
  prometheus:
    charm: 'prometheus'
    revision: 7
    channel: stable
    num_units: 1
    base: ubuntu@16.04/stable
    expose: true
    to:
      - 0
machines:
  '0':
    base: ubuntu@16.04/stable
`[1:],
			expDiff: `
applications:
  prometheus:
    exposed_endpoints:
      "":
        bundle:
          expose_to_cidrs:
          - 0.0.0.0/0
          - ::/0
        model:
          expose_to_spaces:
          - alpha
          expose_to_cidrs:
          - 10.0.0.0/24
`[1:],
		},
		{
			descr: "2.9-compatible bundle with expose settings in overlay section",
			modelExposedEndpoints: map[string]params.ExposedEndpoint{
				"website": {
					ExposeToSpaces: []string{"alpha"},
					ExposeToCIDRs:  []string{"10.0.0.0/24"},
				},
			},
			bundle: `
applications:
  prometheus:
    charm: 'prometheus'
    revision: 7
    channel: stable
    num_units: 1
    base: ubuntu@16.04/stable
    to:
      - 0
machines:
  '0':
    base: ubuntu@16.04/stable
---
applications:
  prometheus:
    exposed-endpoints:
      "":
        expose-to-cidrs:
        - 0.0.0.0/0
      website:
        expose-to-cidrs:
        - 40.0.0.0/24
`[1:],
			expDiff: `
applications:
  prometheus:
    exposed_endpoints:
      "":
        bundle:
          expose_to_cidrs:
          - 0.0.0.0/0
        model: null
      website:
        bundle:
          expose_to_cidrs:
          - 40.0.0.0/24
        model:
          expose_to_spaces:
          - alpha
          expose_to_cidrs:
          - 10.0.0.0/24
`[1:],
		},
	}

	for i, spec := range specs {
		c.Logf("test %d: %s", i, spec.descr)

		s.apiRoot = &mockAPIRoot{
			responses: makeAPIResponsesWithExposedEndpoints(spec.modelExposedEndpoints),
		}

		ctx, err := s.runDiffBundle(c, s.writeLocalBundle(c, spec.bundle))
		c.Assert(err, tc.ErrorIsNil)

		c.Log(cmdtesting.Stdout(ctx))
		c.Assert(cmdtesting.Stdout(ctx), tc.Equals, spec.expDiff)
	}
}

func (s *diffSuite) writeLocalBundle(c *tc.C, content string) string {
	return s.writeFile(c, "bundle.yaml", content)
}

func (s *diffSuite) writeFile(c *tc.C, name, content string) string {
	path := filepath.Join(s.dir, name)
	err := os.WriteFile(path, []byte(content), 0666)
	c.Assert(err, tc.ErrorIsNil)
	return path
}

func makeAPIResponses() map[string]interface{} {
	return makeAPIResponsesWithRelations(nil)
}

func makeAPIResponsesWithRelations(relations []params.RelationStatus) map[string]interface{} {
	var cores uint64 = 3
	return map[string]interface{}{
		"ModelConfig.ModelGet": params.ModelConfigResults{
			Config: map[string]params.ConfigValue{
				"uuid":           {Value: testing.ModelTag.Id()},
				"type":           {Value: "iaas"},
				"name":           {Value: "horse"},
				"default-base":   {Value: "ubuntu@16.04/stable"},
				"secret-backend": {Value: "auto"},
			},
		},
		"Client.FullStatus": params.FullStatus{
			Applications: map[string]params.ApplicationStatus{
				"prometheus": {
					Charm:        "ch:prometheus2-47",
					CharmChannel: "stable",
					Base:         params.Base{Name: "ubuntu", Channel: "16.04"},
					Life:         "alive",
					Units: map[string]params.UnitStatus{
						"prometheus/0": {Machine: "0"},
					},
				},
				"grafana": {
					Charm:        "ch:grafana-19",
					CharmChannel: "stable",
					Base:         params.Base{Name: "ubuntu", Channel: "18.04"},
					Life:         "alive",
					Units: map[string]params.UnitStatus{
						"grafana/0": {Machine: "1"},
					},
				},
			},
			Relations: relations,
			Machines: map[string]params.MachineStatus{
				"0": {Base: params.Base{Name: "ubuntu", Channel: "16.04"}},
				"1": {Base: params.Base{Name: "ubuntu", Channel: "18.04"}},
			},
		},
		"Annotations.Get": params.AnnotationsGetResults{
			Results: []params.AnnotationsGetResult{{
				EntityTag: "application-prometheus",
				Annotations: map[string]string{
					"aspect": "north",
				},
			}},
		},
		"ModelConfig.Sequences": params.ModelSequencesResult{},
		"Application.CharmConfig": params.ApplicationGetConfigResults{
			// Included twice since we can't predict which app will be
			// requested first.
			Results: []params.ConfigResult{{
				Config: map[string]interface{}{"ontology": map[string]interface{}{
					"value":  "kant",
					"source": "user",
				}},
			}, {
				Config: map[string]interface{}{"ontology": map[string]interface{}{
					"value":  "kant",
					"source": "user",
				}},
			}},
		},
		"Application.GetConstraints": params.ApplicationGetConstraintsResults{
			Results: []params.ApplicationConstraint{{
				Constraints: constraints.Value{CpuCores: &cores},
			}, {
				Constraints: constraints.Value{CpuCores: &cores},
			}},
		},
	}
}

func makeAPIResponsesWithExposedEndpoints(exposedEndpoints map[string]params.ExposedEndpoint) map[string]interface{} {
	return map[string]interface{}{
		"ModelConfig.ModelGet": params.ModelConfigResults{
			Config: map[string]params.ConfigValue{
				"uuid":           {Value: testing.ModelTag.Id()},
				"type":           {Value: "iaas"},
				"name":           {Value: "horse"},
				"default-base":   {Value: "ubuntu@16.04/stable"},
				"secret-backend": {Value: "auto"},
			},
		},
		"Client.FullStatus": params.FullStatus{
			Applications: map[string]params.ApplicationStatus{
				"prometheus": {
					Charm:        "ch:prometheus-7",
					CharmChannel: "stable",
					Base:         params.Base{Name: "ubuntu", Channel: "16.04"},
					Life:         "alive",
					Units: map[string]params.UnitStatus{
						"prometheus/0": {Machine: "0"},
					},
					ExposedEndpoints: exposedEndpoints,
				},
			},
			Machines: map[string]params.MachineStatus{
				"0": {Base: params.Base{Name: "ubuntu", Channel: "16.04"}},
			},
		},
		"Annotations.Get": params.AnnotationsGetResults{
			Results: []params.AnnotationsGetResult{{
				EntityTag: "application-prometheus",
			}},
		},
		"ModelConfig.Sequences":      params.ModelSequencesResult{},
		"Application.CharmConfig":    params.ApplicationGetConfigResults{},
		"Application.GetConstraints": params.ApplicationGetConstraintsResults{},
	}
}

type mockModelClient struct {
	stub        testhelpers.Stub
	constraints constraints.Value
}

func (s *mockModelClient) GetModelConstraints(ctx context.Context) (constraints.Value, error) {
	s.stub.AddCall("GetModelConstraints")
	return s.constraints, nil
}

func (s *mockModelClient) Close() error {
	s.stub.AddCall("Close")
	return nil
}

type mockCharmHub struct {
	stub   testhelpers.Stub
	url    *charm.URL
	origin commoncharm.Origin
	bundle *mockBundle
}

func (s *mockCharmHub) ResolveBundleURL(ctx context.Context, url *charm.URL, preferredOrigin commoncharm.Origin) (*charm.URL, commoncharm.Origin, error) {
	s.stub.AddCall("ResolveBundleURL", url, preferredOrigin)
	return s.url, s.origin, s.stub.NextErr()
}

func (s *mockCharmHub) GetBundle(_ context.Context, url *charm.URL, _ commoncharm.Origin, path string) (charm.Bundle, error) {
	s.stub.AddCall("GetBundle", url, path)
	return s.bundle, s.stub.NextErr()
}

type mockBundle struct {
	data *charm.BundleData
}

func (b *mockBundle) Data() *charm.BundleData { return b.data }
func (b *mockBundle) BundleBytes() []byte     { return []byte{} }
func (b *mockBundle) ReadMe() string          { return "" }
func (b *mockBundle) ContainsOverlays() bool  { return false }

type mockAPIRoot struct {
	base.APICallCloser

	stub      testhelpers.Stub
	responses map[string]interface{}
}

func (r *mockAPIRoot) BestFacadeVersion(name string) int {
	r.stub.AddCall("BestFacadeVersion", name)
	return 42
}

func (r *mockAPIRoot) APICall(ctx context.Context, objType string, version int, id, request string, params, response interface{}) error {
	call := objType + "." + request
	r.stub.AddCall(call, version, params)
	value := r.responses[call]
	rv := reflect.ValueOf(response)
	if value == nil {
		panic(fmt.Sprintf("nil response for %s call", call))
	}
	if reflect.TypeOf(value).AssignableTo(rv.Type().Elem()) {
		rv.Elem().Set(reflect.ValueOf(value))
	} else {
		panic(fmt.Sprintf("%s: can't assign value %v to %T", call, value, response))
	}
	return r.stub.NextErr()
}

func (r *mockAPIRoot) Close() error {
	r.stub.AddCall("Close")
	return r.stub.NextErr()
}

const (
	testCharmHubBundle = `
applications:
  prometheus:
    charm: 'prometheus2'
    revision: 47
    channel: stable
    num_units: 1
    base: ubuntu@16.04/stable
    options:
      ontology: anselm
    annotations:
      aspect: west
    constraints: 'cores=4'
    to:
      - 0
machines:
  '0':
    base: ubuntu@16.04/stable
`
	withInclude = `
applications:
  prometheus:
    charm: 'prometheus2'
    revision: 47
    channel: stable
    num_units: 1
    base: ubuntu@16.04/stable
    options:
      ontology: include-file://include.yaml
    annotations:
      aspect: west
    constraints: 'cores=4'
    to:
      - 0
machines:
  '0':
    base: ubuntu@16.04/stable
`
	invalidBundle = `
machines:
  0:
`
	invalidYaml = `
applications:
  prometheus:
    options:
      admin-user: lovecraft
va
`
	overlay1 = `
applications:
  prometheus:
    options:
      admin-user: lovecraft
`

	overlay2 = `
applications:
  telegraf:
    charm: 'ch:telegraf'
relations:
- - telegraf:info
  - prometheus:juju-info
`

	withMissingRelationEndpoints = `
default-base: ubuntu@16.04/stable
applications:
  prometheus:
    charm: 'ch:prometheus2'
    num_units: 1
    base: ubuntu@16.04/stable
    options:
      ontology: anselm
    annotations:
      aspect: west
    constraints: 'cores=4'
  grafana:
    charm: 'ch:grafana'
    num_units: 1
    base: ubuntu@18.04/stable
relations:
- - prometheus:juju-info
  - grafana
`
	withSeries = `
default-base: ubuntu@18.04/stable
applications:
  prometheus:
    charm: 'prometheus2'
    revision: 47
    channel: stable
    num_units: 1
    base: ubuntu@16.04/stable
    constraints: 'cores=3'
    options:
      ontology: kant
    to:
      - 0
  grafana:
    charm: 'grafana'
    revision: 19
    channel: stable
    num_units: 1
    constraints: 'cores=3'
    options:
      ontology: kant
    to:
      - 1
machines:
  "0":
    base: ubuntu@16.04/stable
  "1": {}
relations:
bundle-additions:
- - prometheus:juju-info
  - telegraf:info
`
)
