// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	"fmt"

	"github.com/juju/description"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/client/bundle"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
)

type bundleSuite struct {
	coretesting.BaseSuite
	auth     *apiservertesting.FakeAuthorizer
	facade   *bundle.APIv2
	apiv1    *bundle.APIv1
	st       *mockState
	modelTag names.ModelTag
}

var _ = gc.Suite(&bundleSuite{})

func (s *bundleSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.auth = &apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("read"),
	}

	s.st = newMockState()
	s.modelTag = names.NewModelTag("some-uuid")

	s.apiv1 = s.makeAPIv1(c)
	s.facade = s.makeAPI(c)
}

func (s *bundleSuite) makeAPI(c *gc.C) *bundle.APIv2 {
	api, err := bundle.NewBundleAPI(
		s.st,
		s.auth,
		s.modelTag,
	)
	c.Assert(err, jc.ErrorIsNil)
	return &bundle.APIv2{api}
}

func (s *bundleSuite) makeAPIv1(c *gc.C) *bundle.APIv1 {
	api := s.makeAPI(c)
	return &bundle.APIv1{api}
}

func (s *bundleSuite) TestGetChangesBundleContentError(c *gc.C) {
	args := params.BundleChangesParams{
		BundleDataYAML: ":",
	}
	r, err := s.facade.GetChanges(args)
	c.Assert(err, gc.ErrorMatches, `cannot read bundle YAML: cannot unmarshal bundle data: yaml: did not find expected key`)
	c.Assert(r, gc.DeepEquals, params.BundleChangesResults{})
}

func (s *bundleSuite) TestGetChangesBundleVerificationErrors(c *gc.C) {
	args := params.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    to: [1]
                haproxy:
                    charm: 42
                    num_units: -1
        `,
	}
	r, err := s.facade.GetChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`placement "1" refers to a machine not defined in this bundle`,
		`too many units specified in unit placement for application "django"`,
		`invalid charm URL in application "haproxy": cannot parse URL "42": name "42" not valid`,
		`negative number of units specified on application "haproxy"`,
	})
}

func (s *bundleSuite) TestGetChangesBundleConstraintsError(c *gc.C) {
	args := params.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    num_units: 1
                    constraints: bad=wolf
        `,
	}
	r, err := s.facade.GetChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`invalid constraints "bad=wolf" in application "django": unknown constraint "bad"`,
	})
}

func (s *bundleSuite) TestGetChangesBundleStorageError(c *gc.C) {
	args := params.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    num_units: 1
                    storage:
                        bad: 0,100M
        `,
	}
	r, err := s.facade.GetChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`invalid storage "bad" in application "django": cannot parse count: count must be greater than zero, got "0"`,
	})
}

func (s *bundleSuite) TestGetChangesBundleDevicesError(c *gc.C) {
	args := params.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    num_units: 1
                    devices:
                        bad-gpu: -1,nvidia.com/gpu
        `,
	}
	r, err := s.facade.GetChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`invalid device "bad-gpu" in application "django": count must be greater than zero, got "-1"`,
	})
}

func (s *bundleSuite) TestGetChangesSuccessV2(c *gc.C) {
	args := params.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    options:
                        debug: true
                    storage:
                        tmpfs: tmpfs,1G
                    devices:
                        bitcoinminer: 2,nvidia.com/gpu
                haproxy:
                    charm: cs:trusty/haproxy-42
            relations:
                - - django:web
                  - haproxy:web
        `,
	}
	r, err := s.facade.GetChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, jc.DeepEquals, []*params.BundleChange{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Args:   []interface{}{"django", ""},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Args: []interface{}{
			"$addCharm-0",
			"",
			"django",
			map[string]interface{}{"debug": true},
			"",
			map[string]string{"tmpfs": "tmpfs,1G"},
			map[string]string{"bitcoinminer": "2,nvidia.com/gpu"},
			map[string]string{},
			map[string]int{},
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Args:   []interface{}{"cs:trusty/haproxy-42", "trusty"},
	}, {
		Id:     "deploy-3",
		Method: "deploy",
		Args: []interface{}{
			"$addCharm-2",
			"trusty",
			"haproxy",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]string{},
			map[string]int{},
		},
		Requires: []string{"addCharm-2"},
	}, {
		Id:       "addRelation-4",
		Method:   "addRelation",
		Args:     []interface{}{"$deploy-1:web", "$deploy-3:web"},
		Requires: []string{"deploy-1", "deploy-3"},
	}})
	c.Assert(r.Errors, gc.IsNil)
}

func (s *bundleSuite) TestGetChangesSuccessV1(c *gc.C) {
	args := params.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    options:
                        debug: true
                    storage:
                        tmpfs: tmpfs,1G
                    devices:
                        bitcoinminer: 2,nvidia.com/gpu
                haproxy:
                    charm: cs:trusty/haproxy-42
            relations:
                - - django:web
                  - haproxy:web
        `,
	}
	r, err := s.apiv1.GetChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, jc.DeepEquals, []*params.BundleChange{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Args:   []interface{}{"django", ""},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Args: []interface{}{
			"$addCharm-0",
			"",
			"django",
			map[string]interface{}{"debug": true},
			"",
			map[string]string{"tmpfs": "tmpfs,1G"},
			map[string]string{},
			map[string]int{},
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Args:   []interface{}{"cs:trusty/haproxy-42", "trusty"},
	}, {
		Id:     "deploy-3",
		Method: "deploy",
		Args: []interface{}{
			"$addCharm-2",
			"trusty",
			"haproxy",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]int{},
		},
		Requires: []string{"addCharm-2"},
	}, {
		Id:       "addRelation-4",
		Method:   "addRelation",
		Args:     []interface{}{"$deploy-1:web", "$deploy-3:web"},
		Requires: []string{"deploy-1", "deploy-3"},
	}})
	c.Assert(r.Errors, gc.IsNil)
}

func (s *bundleSuite) TestGetChangesBundleEndpointBindingsSuccess(c *gc.C) {
	args := params.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    num_units: 1
                    bindings:
                        url: public
        `,
	}
	r, err := s.facade.GetChanges(args)
	c.Assert(err, jc.ErrorIsNil)

	for _, change := range r.Changes {
		if change.Method == "deploy" {
			c.Assert(change, jc.DeepEquals, &params.BundleChange{
				Id:     "deploy-1",
				Method: "deploy",
				Args: []interface{}{
					"$addCharm-0",
					"",
					"django",
					map[string]interface{}{},
					"",
					map[string]string{},
					map[string]string{},
					map[string]string{"url": "public"},
					map[string]int{},
				},
				Requires: []string{"addCharm-0"},
			})
		}
	}
}

func (s *bundleSuite) TestExportBundleFailNoApplication(c *gc.C) {
	s.st.model = description.NewModel(description.ModelArgs{Owner: names.NewUserTag("magic"),
		Config: map[string]interface{}{
			"name": "awesome",
			"uuid": "some-uuid",
		},
		CloudRegion: "some-region"})
	s.st.model.SetStatus(description.StatusArgs{Value: "available"})

	result, err := s.facade.ExportBundle()
	c.Assert(err, gc.NotNil)
	c.Assert(result, gc.Equals, params.StringResult{})
	c.Check(err, gc.ErrorMatches, "nothing to export as there are no applications")
	s.st.CheckCall(c, 0, "ExportPartial", s.st.GetExportConfig())
}

func (s *bundleSuite) minimalApplicationArgs(modelType string) description.ApplicationArgs {
	result := description.ApplicationArgs{
		Tag:                  names.NewApplicationTag("ubuntu"),
		Series:               "trusty",
		Type:                 modelType,
		CharmURL:             "cs:trusty/ubuntu",
		Channel:              "stable",
		CharmModifiedVersion: 1,
		CharmConfig: map[string]interface{}{
			"key": "value",
		},
		Leader: "ubuntu/0",
		LeadershipSettings: map[string]interface{}{
			"leader": true,
		},
		MetricsCredentials: []byte("sekrit"),
	}
	if modelType == description.CAAS {
		result.PasswordHash = "some-hash"
		result.PodSpec = "some-spec"
		result.CloudService = &description.CloudServiceArgs{
			ProviderId: "some-provider",
			Addresses: []description.AddressArgs{
				{Value: "10.0.0.1", Type: "special"},
				{Value: "10.0.0.2", Type: "other"},
			},
		}
	}
	return result
}

func minimalUnitArgs(modelType string) description.UnitArgs {
	result := description.UnitArgs{
		Tag:          names.NewUnitTag("ubuntu/0"),
		Type:         modelType,
		Machine:      names.NewMachineTag("0"),
		PasswordHash: "secure-hash",
	}
	if modelType == description.CAAS {
		result.CloudContainer = &description.CloudContainerArgs{
			ProviderId: "some-provider",
			Address:    description.AddressArgs{Value: "10.0.0.1", Type: "special"},
			Ports:      []string{"80", "443"},
		}
	}
	return result
}

func minimalStatusArgs() description.StatusArgs {
	return description.StatusArgs{
		Value: "running",
	}
}

func (s *bundleSuite) TestExportBundleWithApplication(c *gc.C) {
	s.st.model = description.NewModel(description.ModelArgs{Owner: names.NewUserTag("magic"),
		Config: map[string]interface{}{
			"name": "awesome",
			"uuid": "some-uuid",
		},
		CloudRegion: "some-region"})

	app := s.st.model.AddApplication(s.minimalApplicationArgs(description.IAAS))
	app.SetStatus(minimalStatusArgs())

	u := app.AddUnit(minimalUnitArgs(app.Type()))
	u.SetAgentStatus(minimalStatusArgs())

	s.st.model.SetStatus(description.StatusArgs{Value: "available"})

	result, err := s.facade.ExportBundle()
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.StringResult{nil, `
series: trusty
applications:
  ubuntu:
    charm: cs:trusty/ubuntu
    num_units: 1
    to:
    - "0"
    options:
      key: value
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
	s.st.CheckCall(c, 0, "ExportPartial", s.st.GetExportConfig())
}

func (s *bundleSuite) addApplicationToModel(model description.Model, name string, numUnits int) description.Application {
	application := model.AddApplication(description.ApplicationArgs{
		Tag:                names.NewApplicationTag(name),
		CharmURL:           "cs:" + name,
		Series:             "xenial",
		CharmConfig:        map[string]interface{}{},
		LeadershipSettings: map[string]interface{}{},
	})
	application.SetStatus(minimalStatusArgs())
	for i := 0; i < numUnits; i++ {
		machine := model.AddMachine(description.MachineArgs{
			Id:     names.NewMachineTag(fmt.Sprint(i)),
			Series: "xenial",
		})
		unit := application.AddUnit(description.UnitArgs{
			Tag:     names.NewUnitTag(fmt.Sprintf("%s/%d", name, i)),
			Machine: machine.Tag(),
		})
		unit.SetAgentStatus(minimalStatusArgs())
	}

	return application
}

func (s *bundleSuite) setEndpointSettings(ep description.Endpoint, units ...string) {
	for _, unit := range units {
		ep.SetUnitSettings(unit, map[string]interface{}{
			"key": "value",
		})
	}
}

func (s *bundleSuite) newModel(app1 string, app2 string) description.Model {
	s.st.model = description.NewModel(description.ModelArgs{Owner: names.NewUserTag("magic"),
		Config: map[string]interface{}{
			"name": "awesome",
			"uuid": "some-uuid",
		},
		CloudRegion: "some-region"})

	s.addApplicationToModel(s.st.model, app1, 2)
	s.addApplicationToModel(s.st.model, app2, 1)

	// Add a relation between wordpress and mysql.
	rel := s.st.model.AddRelation(description.RelationArgs{
		Id:  42,
		Key: "special key",
	})
	rel.SetStatus(minimalStatusArgs())

	app1Endpoint := rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: app1,
		Name:            "db",
		// Ignoring other aspects of endpoints.
	})
	s.setEndpointSettings(app1Endpoint, app1+"/0", app1+"/1")

	app2Endpoint := rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "mysql",
		Name:            "mysql",
		// Ignoring other aspects of endpoints.
	})
	s.setEndpointSettings(app2Endpoint, app2+"/0")

	return s.st.model
}

func (s *bundleSuite) TestExportBundleModelWithSettingsRelations(c *gc.C) {
	model := s.newModel("wordpress", "mysql")
	model.SetStatus(description.StatusArgs{Value: "available"})

	result, err := s.facade.ExportBundle()
	c.Assert(err, jc.ErrorIsNil)

	output := `
series: xenial
applications:
  mysql:
    charm: cs:mysql
    num_units: 1
    to:
    - "0"
  wordpress:
    charm: cs:wordpress
    num_units: 2
    to:
    - "0"
    - "1"
machines:
  "0": {}
  "1": {}
relations:
- - wordpress:db
  - mysql:mysql
`[1:]
	expectedResult := params.StringResult{nil, output}

	c.Assert(result, gc.Equals, expectedResult)
	s.st.CheckCall(c, 0, "ExportPartial", s.st.GetExportConfig())
}

func (s *bundleSuite) addSubordinateEndpoints(
	c *gc.C,
	rel description.Relation, app string,
) (description.Endpoint, description.Endpoint) {
	appEndpoint := rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: app,
		Name:            "logging",
		Scope:           "container",
		// Ignoring other aspects of endpoints.
	})
	loggingEndpoint := rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "logging",
		Name:            "logging",
		Scope:           "container",
		// Ignoring other aspects of endpoints.
	})
	return appEndpoint, loggingEndpoint
}

func (s *bundleSuite) TestExportBundleModelRelationsWithSubordinates(c *gc.C) {
	model := s.newModel("wordpress", "mysql")
	model.SetStatus(description.StatusArgs{Value: "available"})

	// Add a subordinate relations between logging and both wordpress and mysql.
	rel := model.AddRelation(description.RelationArgs{
		Id:  43,
		Key: "some key",
	})
	wordpressEndpoint, loggingEndpoint := s.addSubordinateEndpoints(c, rel, "wordpress")
	s.setEndpointSettings(wordpressEndpoint, "wordpress/0", "wordpress/1")
	s.setEndpointSettings(loggingEndpoint, "logging/0", "logging/1")

	rel = model.AddRelation(description.RelationArgs{
		Id:  44,
		Key: "other key",
	})
	mysqlEndpoint, loggingEndpoint := s.addSubordinateEndpoints(c, rel, "mysql")
	s.setEndpointSettings(mysqlEndpoint, "mysql/0")
	s.setEndpointSettings(loggingEndpoint, "logging/2")

	result, err := s.facade.ExportBundle()
	c.Assert(err, jc.ErrorIsNil)

	expectedResult := params.StringResult{nil, `
series: xenial
applications:
  mysql:
    charm: cs:mysql
    num_units: 1
    to:
    - "0"
  wordpress:
    charm: cs:wordpress
    num_units: 2
    to:
    - "0"
    - "1"
machines:
  "0": {}
  "1": {}
relations:
- - wordpress:db
  - mysql:mysql
- - wordpress:logging
  - logging:logging
- - mysql:logging
  - logging:logging
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
	s.st.CheckCall(c, 0, "ExportPartial", s.st.GetExportConfig())
}

func (s *bundleSuite) TestExportBundleSubordinateApplication(c *gc.C) {
	s.st.model = description.NewModel(description.ModelArgs{Owner: names.NewUserTag("magic"),
		Config: map[string]interface{}{
			"name": "awesome",
			"uuid": "some-uuid",
		},
		CloudRegion: "some-region"})

	application := s.st.model.AddApplication(description.ApplicationArgs{
		Tag:                  names.NewApplicationTag("magic"),
		Series:               "zesty",
		Subordinate:          true,
		CharmURL:             "cs:zesty/magic",
		Channel:              "stable",
		CharmModifiedVersion: 1,
		ForceCharm:           true,
		Exposed:              true,
		EndpointBindings: map[string]string{
			"rel-name": "some-space",
		},
		ApplicationConfig: map[string]interface{}{
			"config key": "config value",
		},
		CharmConfig: map[string]interface{}{
			"key": "value",
		},
		Leader: "magic/1",
		LeadershipSettings: map[string]interface{}{
			"leader": true,
		},
		MetricsCredentials: []byte("sekrit"),
		PasswordHash:       "passwordhash",
		PodSpec:            "podspec",
	})
	application.SetStatus(minimalStatusArgs())

	result, err := s.facade.ExportBundle()
	c.Assert(err, jc.ErrorIsNil)

	expectedResult := params.StringResult{nil, `
series: zesty
applications:
  magic:
    charm: cs:zesty/magic
    expose: true
    options:
      key: value
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
	s.st.CheckCall(c, 0, "ExportPartial", s.st.GetExportConfig())
}

func (s *bundleSuite) TestExportBundleSubordinateApplicationAndMachine(c *gc.C) {
	s.st.model = description.NewModel(description.ModelArgs{Owner: names.NewUserTag("magic"),
		Config: map[string]interface{}{
			"name": "awesome",
			"uuid": "some-uuid",
		},
		CloudRegion: "some-region"})

	application := s.st.model.AddApplication(description.ApplicationArgs{
		Tag:         names.NewApplicationTag("magic"),
		Series:      "zesty",
		Subordinate: true,
		CharmURL:    "cs:zesty/magic",
		Channel:     "stable",
		Exposed:     true,
		CharmConfig: map[string]interface{}{
			"key": "value",
		},
	})
	application.SetStatus(minimalStatusArgs())

	s.addMinimalMachineWithConstraints(s.st.model, "0")

	result, err := s.facade.ExportBundle()
	c.Assert(err, jc.ErrorIsNil)

	expectedResult := params.StringResult{nil, `
series: zesty
applications:
  magic:
    charm: cs:zesty/magic
    expose: true
    options:
      key: value
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
	s.st.CheckCall(c, 0, "ExportPartial", s.st.GetExportConfig())
}

func (s *bundleSuite) addMinimalMachineWithConstraints(model description.Model, id string) {
	m := model.AddMachine(description.MachineArgs{
		Id:           names.NewMachineTag(id),
		Nonce:        "a-nonce",
		PasswordHash: "some-hash",
		Series:       "xenial",
		Jobs:         []string{"host-units"},
	})
	args := description.ConstraintsArgs{
		Architecture: "amd64",
		Memory:       8 * 1024,
		RootDisk:     40 * 1024,
	}
	m.SetConstraints(args)
	m.SetStatus(minimalStatusArgs())
}

func (s *bundleSuite) TestExportBundleModelWithConstraints(c *gc.C) {
	model := s.newModel("mediawiki", "mysql")

	s.addMinimalMachineWithConstraints(model, "0")
	s.addMinimalMachineWithConstraints(model, "1")

	model.SetStatus(description.StatusArgs{Value: "available"})

	result, err := s.facade.ExportBundle()
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.StringResult{nil, `
series: xenial
applications:
  mediawiki:
    charm: cs:mediawiki
    num_units: 2
    to:
    - "0"
    - "1"
  mysql:
    charm: cs:mysql
    num_units: 1
    to:
    - "0"
machines:
  "0":
    constraints: arch=amd64 mem=8192 root-disk=40960
  "1":
    constraints: arch=amd64 mem=8192 root-disk=40960
relations:
- - mediawiki:db
  - mysql:mysql
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)

	s.st.CheckCall(c, 0, "ExportPartial", s.st.GetExportConfig())
}

func (s *bundleSuite) addMinimalMachinewithHardwareConstraints(model description.Model, id string) {
	m := model.AddMachine(description.MachineArgs{
		Id:           names.NewMachineTag(id),
		Nonce:        "a-nonce",
		PasswordHash: "some-hash",
		Series:       "xenial",
		Jobs:         []string{"host-units"},
	})
	args := description.ConstraintsArgs{
		Architecture: "amd64",
		Memory:       8 * 1024,
		RootDisk:     40 * 1024,
	}
	m.SetConstraints(args)
	instanceArgs := description.CloudInstanceArgs{
		Architecture: "amd64",
		Memory:       4 * 1024,
		RootDisk:     16 * 1024,
	}
	m.SetInstance(instanceArgs)
	m.SetStatus(minimalStatusArgs())
}

func (s *bundleSuite) TestExportBundleModelWithHardwareConstraints(c *gc.C) {
	model := s.newModel("mediawiki", "mysql")

	s.addMinimalMachinewithHardwareConstraints(model, "0")
	s.addMinimalMachinewithHardwareConstraints(model, "1")

	model.SetStatus(description.StatusArgs{Value: "available"})

	result, err := s.facade.ExportBundle()
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.StringResult{nil, `
series: xenial
applications:
  mediawiki:
    charm: cs:mediawiki
    num_units: 2
    to:
    - "0"
    - "1"
  mysql:
    charm: cs:mysql
    num_units: 1
    to:
    - "0"
machines:
  "0":
    constraints: arch=amd64 mem=4096 root-disk=16384
  "1":
    constraints: arch=amd64 mem=4096 root-disk=16384
relations:
- - mediawiki:db
  - mysql:mysql
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)

	s.st.CheckCall(c, 0, "ExportPartial", s.st.GetExportConfig())
}

func (s *bundleSuite) addMinimalMachineWithAnnotations(model description.Model, id string) {
	m := model.AddMachine(description.MachineArgs{
		Id:           names.NewMachineTag(id),
		Nonce:        "a-nonce",
		PasswordHash: "some-hash",
		Series:       "xenial",
		Jobs:         []string{"host-units"},
	})
	m.SetAnnotations(map[string]string{
		"string":  "value",
		"another": "one",
	})
	m.SetStatus(minimalStatusArgs())
}

func (s *bundleSuite) TestExportBundleModelWithAnnotations(c *gc.C) {
	model := s.newModel("wordpress", "mysql")

	s.addMinimalMachineWithAnnotations(model, "0")
	s.addMinimalMachineWithAnnotations(model, "1")

	model.SetStatus(description.StatusArgs{Value: "available"})

	result, err := s.facade.ExportBundle()
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.StringResult{nil, `
series: xenial
applications:
  mysql:
    charm: cs:mysql
    num_units: 1
    to:
    - "0"
  wordpress:
    charm: cs:wordpress
    num_units: 2
    to:
    - "0"
    - "1"
machines:
  "0":
    annotations:
      another: one
      string: value
  "1":
    annotations:
      another: one
      string: value
relations:
- - wordpress:db
  - mysql:mysql
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
	s.st.CheckCall(c, 0, "ExportPartial", s.st.GetExportConfig())
}

func (s *bundleSuite) TestExportBundleWithContainers(c *gc.C) {
	s.st.model = description.NewModel(description.ModelArgs{Owner: names.NewUserTag("magic"),
		Config: map[string]interface{}{
			"name": "awesome",
			"uuid": "some-uuid",
		},
		CloudRegion: "some-region"})

	application0 := s.st.model.AddApplication(description.ApplicationArgs{
		Tag:      names.NewApplicationTag("wordpress"),
		CharmURL: "cs:wordpress",
		Series:   "xenial",
	})
	application0.SetStatus(minimalStatusArgs())

	m0 := s.st.model.AddMachine(description.MachineArgs{
		Id:     names.NewMachineTag("0"),
		Series: "xenial",
	})
	args := description.ConstraintsArgs{
		Architecture: "amd64",
		Memory:       8 * 1024,
		RootDisk:     40 * 1024,
	}
	m0.SetConstraints(args)
	ut0 := application0.AddUnit(description.UnitArgs{
		Tag:     names.NewUnitTag("wordpress/0"),
		Machine: names.NewMachineTag("0"),
	})
	ut0.SetAgentStatus(minimalStatusArgs())

	application1 := s.st.model.AddApplication(description.ApplicationArgs{
		Tag:      names.NewApplicationTag("mysql"),
		CharmURL: "cs:mysql",
		Series:   "xenial",
	})
	application1.SetStatus(minimalStatusArgs())

	m1 := s.st.model.AddMachine(description.MachineArgs{
		Id:     names.NewMachineTag("1"),
		Series: "xenial",
	})
	args = description.ConstraintsArgs{
		Architecture: "amd64",
		Memory:       8 * 1024,
		RootDisk:     40 * 1024,
	}
	m1.SetConstraints(args)

	ut := application1.AddUnit(description.UnitArgs{
		Tag:     names.NewUnitTag("mysql/1"),
		Machine: names.NewMachineTag("1/lxd/0"),
	})
	ut.SetAgentStatus(minimalStatusArgs())

	result, err := s.facade.ExportBundle()
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.StringResult{nil, `
series: xenial
applications:
  mysql:
    charm: cs:mysql
    num_units: 1
    to:
    - lxd:1
  wordpress:
    charm: cs:wordpress
    num_units: 1
    to:
    - "0"
machines:
  "0":
    constraints: arch=amd64 mem=8192 root-disk=40960
  "1":
    constraints: arch=amd64 mem=8192 root-disk=40960
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
	s.st.CheckCall(c, 0, "ExportPartial", s.st.GetExportConfig())
}

func (s *bundleSuite) TestMixedSeries(c *gc.C) {
	s.st.model = description.NewModel(description.ModelArgs{Owner: names.NewUserTag("magic"),
		Config: map[string]interface{}{
			"name":           "awesome",
			"uuid":           "some-uuid",
			"default-series": "xenial",
		},
		CloudRegion: "some-region"})

	application := s.st.model.AddApplication(description.ApplicationArgs{
		Tag:      names.NewApplicationTag("magic"),
		Series:   "xenial",
		CharmURL: "cs:xenial/magic",
	})
	application.AddUnit(description.UnitArgs{
		Tag:     names.NewUnitTag("magic/0"),
		Machine: names.NewMachineTag("0"),
	})
	s.st.model.AddMachine(description.MachineArgs{
		Id:     names.NewMachineTag("0"),
		Series: "xenial",
	})

	application = s.st.model.AddApplication(description.ApplicationArgs{
		Tag:      names.NewApplicationTag("mojo"),
		Series:   "trusty",
		CharmURL: "cs:mojo",
	})
	application.AddUnit(description.UnitArgs{
		Tag:     names.NewUnitTag("mojo/0"),
		Machine: names.NewMachineTag("1"),
	})
	s.st.model.AddMachine(description.MachineArgs{
		Id:     names.NewMachineTag("1"),
		Series: "trusty",
	})

	result, err := s.facade.ExportBundle()
	c.Assert(err, jc.ErrorIsNil)

	expectedResult := params.StringResult{nil, `
series: xenial
applications:
  magic:
    charm: cs:xenial/magic
    num_units: 1
    to:
    - "0"
  mojo:
    charm: cs:mojo
    series: trusty
    num_units: 1
    to:
    - "1"
machines:
  "0": {}
  "1":
    series: trusty
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
	s.st.CheckCall(c, 0, "ExportPartial", s.st.GetExportConfig())
}

func (s *bundleSuite) TestMixedSeriesNoDefaultSeries(c *gc.C) {
	s.st.model = description.NewModel(description.ModelArgs{Owner: names.NewUserTag("magic"),
		Config: map[string]interface{}{
			"name":           "awesome",
			"uuid":           "some-uuid",
			"default-series": "bionic",
		},
		CloudRegion: "some-region"})

	application := s.st.model.AddApplication(description.ApplicationArgs{
		Tag:      names.NewApplicationTag("magic"),
		Series:   "xenial",
		CharmURL: "cs:xenial/magic",
	})
	application.AddUnit(description.UnitArgs{
		Tag:     names.NewUnitTag("magic/0"),
		Machine: names.NewMachineTag("0"),
	})
	s.st.model.AddMachine(description.MachineArgs{
		Id:     names.NewMachineTag("0"),
		Series: "xenial",
	})

	application = s.st.model.AddApplication(description.ApplicationArgs{
		Tag:      names.NewApplicationTag("mojo"),
		Series:   "trusty",
		CharmURL: "cs:mojo",
	})
	application.AddUnit(description.UnitArgs{
		Tag:     names.NewUnitTag("mojo/0"),
		Machine: names.NewMachineTag("1"),
	})
	s.st.model.AddMachine(description.MachineArgs{
		Id:     names.NewMachineTag("1"),
		Series: "trusty",
	})

	result, err := s.facade.ExportBundle()
	c.Assert(err, jc.ErrorIsNil)

	expectedResult := params.StringResult{nil, `
applications:
  magic:
    charm: cs:xenial/magic
    series: xenial
    num_units: 1
    to:
    - "0"
  mojo:
    charm: cs:mojo
    series: trusty
    num_units: 1
    to:
    - "1"
machines:
  "0":
    series: xenial
  "1":
    series: trusty
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
	s.st.CheckCall(c, 0, "ExportPartial", s.st.GetExportConfig())
}
