// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/description/v9"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/resource"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type bundleSuite struct {
	coretesting.BaseSuite
	auth                  *apiservertesting.FakeAuthorizer
	facade                *APIv8
	modelTag              names.ModelTag
	modelMigrationFactory *MockModelMigrationFactory
	modelExporter         *MockModelExporter
	networkService        *MockNetworkService
	applicationService    *MockApplicationService
	store                 *MockObjectStore
}

var _ = gc.Suite(&bundleSuite{})

func (s *bundleSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.networkService = NewMockNetworkService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.store = NewMockObjectStore(ctrl)
	s.modelMigrationFactory = NewMockModelMigrationFactory(ctrl)
	s.modelExporter = NewMockModelExporter(ctrl)

	// If this expect fails, change have been made in mock configuration or in
	// setup test.
	s.modelMigrationFactory.EXPECT().ModelExporter(gomock.Any(), model.UUID(s.modelTag.Id()),
		nil).Return(s.modelExporter, nil).AnyTimes()

	return ctrl
}

func (s *bundleSuite) SetUpTest(c *gc.C) {

	s.BaseSuite.SetUpTest(c)
	s.auth = &apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("read"),
	}

	s.modelTag = names.NewModelTag("some-uuid")
}

func (s *bundleSuite) makeAPI(c *gc.C) *APIv8 {
	api, err := NewBundleAPI(
		getModelExporter(
			s.modelMigrationFactory,
			model.UUID(s.modelTag.Id()),
			s.store, nil), // legacy Exporter is meant to disappear
		s.auth,
		s.modelTag,
		s.networkService,
		s.applicationService,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	return &APIv8{api}
}

func (s *bundleSuite) expectGetAllSpaces(c *gc.C, spaces ...network.SpaceInfo) {
	defaultSpaces := network.SpaceInfos{
		{
			ID:   network.AlphaSpaceId,
			Name: network.AlphaSpaceName,
		},
		{
			ID:   "1",
			Name: "vlan2",
		},
	}
	defaultSpaces = append(defaultSpaces, spaces...)
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).
		Return(defaultSpaces, nil)
}

func (s *bundleSuite) TestGetChangesMapArgsBundleContentError(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	args := params.BundleChangesParams{
		BundleDataYAML: ":",
	}
	r, err := s.facade.GetChangesMapArgs(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, `cannot read bundle YAML: malformed bundle: bundle is empty not valid`)
	c.Assert(r, gc.DeepEquals, params.BundleChangesMapArgsResults{})
}

func (s *bundleSuite) TestGetChangesMapArgsBundleVerificationErrors(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

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
	r, err := s.facade.GetChangesMapArgs(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`placement "1" refers to a machine not defined in this bundle`,
		`too many units specified in unit placement for application "django"`,
		`invalid charm URL in application "haproxy": cannot parse name and/or revision in URL "42": name "42" not valid`,
		`negative number of units specified on application "haproxy"`,
	})
}

func (s *bundleSuite) TestGetChangesMapArgsBundleConstraintsError(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	args := params.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    num_units: 1
                    constraints: bad=wolf
        `,
	}
	r, err := s.facade.GetChangesMapArgs(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`invalid constraints "bad=wolf" in application "django": unknown constraint "bad"`,
	})
}

func (s *bundleSuite) TestGetChangesMapArgsBundleStorageError(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

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
	r, err := s.facade.GetChangesMapArgs(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`invalid storage "bad" in application "django": cannot parse count: count must be greater than zero, got "0"`,
	})
}

func (s *bundleSuite) TestGetChangesMapArgsBundleDevicesError(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

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
	r, err := s.facade.GetChangesMapArgs(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`invalid device "bad-gpu" in application "django": count must be greater than zero, got "-1"`,
	})
}

func (s *bundleSuite) TestGetChangesMapArgsSuccess(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

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
                    charm: ch:haproxy
                    revision: 42
                    channel: stable
                    base: ubuntu@22.04/stable
            relations:
                - - django:web
                  - haproxy:web
        `,
	}
	r, err := s.facade.GetChangesMapArgs(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(r.Changes, jc.DeepEquals, []*params.BundleChangesMapArgs{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Args: map[string]interface{}{
			"charm": "django",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"devices": map[string]interface{}{
				"bitcoinminer": "2,nvidia.com/gpu",
			},
			"options": map[string]interface{}{
				"debug": true,
			},
			"storage": map[string]interface{}{
				"tmpfs": "tmpfs,1G",
			},
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Args: map[string]interface{}{
			"channel":  "stable",
			"charm":    "ch:haproxy",
			"revision": float64(42),
			"base":     "ubuntu@22.04/stable",
		},
	}, {
		Id:     "deploy-3",
		Method: "deploy",
		Args: map[string]interface{}{
			"channel":     "stable",
			"application": "haproxy",
			"charm":       "$addCharm-2",
			"base":        "ubuntu@22.04/stable",
		},
		Requires: []string{"addCharm-2"},
	}, {
		Id:     "addRelation-4",
		Method: "addRelation",
		Args: map[string]interface{}{
			"endpoint1": "$deploy-1:web",
			"endpoint2": "$deploy-3:web",
		},
		Requires: []string{"deploy-1", "deploy-3"},
	}}, gc.Commentf("\nobtained: %s\n", pretty.Sprint(r.Changes)))
	for _, err := range r.Errors {
		c.Assert(err, gc.Equals, "")
	}
}

func (s *bundleSuite) TestGetChangesMapArgsSuccessCharmHubRevision(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	args := params.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    revision: 76
                    channel: candidate
        `,
	}
	r, err := s.facade.GetChangesMapArgs(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(r.Changes, jc.DeepEquals, []*params.BundleChangesMapArgs{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Args: map[string]interface{}{
			"revision": float64(76),
			"channel":  "candidate",
			"charm":    "django",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Args: map[string]interface{}{
			"application": "django",
			"channel":     "candidate",
			"charm":       "$addCharm-0",
		},
		Requires: []string{"addCharm-0"},
	}})
	for _, err := range r.Errors {
		c.Assert(err, gc.Equals, "")
	}
}

func (s *bundleSuite) TestGetChangesMapArgsKubernetes(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	args := params.BundleChangesParams{
		BundleDataYAML: `
            bundle: kubernetes
            applications:
                django:
                    charm: django
                    scale: 1
                    options:
                        debug: true
                    storage:
                        tmpfs: tmpfs,1G
                    devices:
                        bitcoinminer: 2,nvidia.com/gpu
                haproxy:
                    charm: ch:haproxy
                    revision: 42
                    channel: stable
            relations:
                - - django:web
                  - haproxy:web
        `,
	}
	r, err := s.facade.GetChangesMapArgs(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(r.Changes, jc.DeepEquals, []*params.BundleChangesMapArgs{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Args: map[string]interface{}{
			"charm": "django",
		},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Args: map[string]interface{}{
			"application": "django",
			"charm":       "$addCharm-0",
			"devices": map[string]interface{}{
				"bitcoinminer": "2,nvidia.com/gpu",
			},
			"num-units": float64(1),
			"options": map[string]interface{}{
				"debug": true,
			},
			"storage": map[string]interface{}{
				"tmpfs": "tmpfs,1G",
			},
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Args: map[string]interface{}{
			"channel":  "stable",
			"charm":    "ch:haproxy",
			"revision": float64(42),
		},
	}, {
		Id:     "deploy-3",
		Method: "deploy",
		Args: map[string]interface{}{
			"channel":     "stable",
			"application": "haproxy",
			"charm":       "$addCharm-2",
		},
		Requires: []string{"addCharm-2"},
	}, {
		Id:     "addRelation-4",
		Method: "addRelation",
		Args: map[string]interface{}{
			"endpoint1": "$deploy-1:web",
			"endpoint2": "$deploy-3:web",
		},
		Requires: []string{"deploy-1", "deploy-3"},
	}}, gc.Commentf("\nobtained: %s\n", pretty.Sprint(r.Changes)))
	for _, err := range r.Errors {
		c.Assert(err, gc.Equals, "")
	}
}

func (s *bundleSuite) TestGetChangesMapArgsBundleEndpointBindingsSuccess(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

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
	r, err := s.facade.GetChangesMapArgs(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	for _, change := range r.Changes {
		if change.Method == "deploy" {
			c.Assert(change, jc.DeepEquals, &params.BundleChangesMapArgs{
				Id:     "deploy-1",
				Method: "deploy",
				Args: map[string]interface{}{
					"application": "django",
					"charm":       "$addCharm-0",
					"endpoint-bindings": map[string]interface{}{
						"url": "public",
					},
				},
				Requires: []string{"addCharm-0"},
			})
		}
	}
}

func (s *bundleSuite) TestExportBundleFailNoApplication(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)
	s.expectExportModel(description.NewModel(description.ModelArgs{Owner: "magic",
		Config:      coretesting.FakeConfig(),
		CloudRegion: "some-region"}))

	// Act.
	_, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Check(err, gc.ErrorMatches, "nothing to export as there are no applications")
}

func (s *bundleSuite) minimalApplicationArgs(modelType string) description.ApplicationArgs {
	return s.minimalApplicationArgsWithCharmConfig(modelType, map[string]interface{}{
		"key": "value",
	})
}

func (s *bundleSuite) minimalApplicationArgsWithCharmConfig(modelType string,
	charmConfig map[string]interface{}) description.ApplicationArgs {
	result := description.ApplicationArgs{
		Name:                 "ubuntu",
		Type:                 modelType,
		CharmURL:             "ch:ubuntu",
		Channel:              "stable",
		CharmModifiedVersion: 1,
		CharmConfig:          charmConfig,
		Leader:               "ubuntu/0",
		LeadershipSettings: map[string]interface{}{
			"leader": true,
		},
		MetricsCredentials: []byte("sekrit"),
		EndpointBindings:   map[string]string{"juju-info": "1", "another": "0"},
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
		Name:         "ubuntu/0",
		Type:         modelType,
		Machine:      "0",
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
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := description.NewModel(description.ModelArgs{Owner: "magic",
		Config:      coretesting.FakeConfig(),
		CloudRegion: "some-region"})

	app := model.AddApplication(s.minimalApplicationArgs(description.IAAS))
	app.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})
	app.SetStatus(minimalStatusArgs())

	u := app.AddUnit(minimalUnitArgs(app.Type()))
	u.SetAgentStatus(minimalStatusArgs())

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.StringResult{Result: `
default-base: ubuntu@20.04/stable
applications:
  ubuntu:
    charm: ubuntu
    channel: stable
    num_units: 1
    to:
    - "0"
    options:
      key: value
    bindings:
      another: alpha
      juju-info: vlan2
`[1:]}

	//  Assert
	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) TestExportBundleWithApplicationResources(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := description.NewModel(description.ModelArgs{Owner: "magic",
		Config:      coretesting.FakeConfig(),
		CloudRegion: "some-region"})

	app := model.AddApplication(s.minimalApplicationArgs(description.IAAS))
	app.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})
	app.SetStatus(minimalStatusArgs())

	res := app.AddResource(description.ResourceArgs{Name: "foo-file"})
	res.SetApplicationRevision(description.ResourceRevisionArgs{
		Revision: 42,
		Type:     "file",
		Origin:   resource.OriginStore.String(),
	})
	res2 := app.AddResource(description.ResourceArgs{Name: "bar-file"})
	res2.SetApplicationRevision(description.ResourceRevisionArgs{
		Revision: 0,
		Type:     "file",
		Origin:   resource.OriginUpload.String(),
	})

	u := app.AddUnit(minimalUnitArgs(app.Type()))
	u.SetAgentStatus(minimalStatusArgs())

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.StringResult{Result: `
default-base: ubuntu@20.04/stable
applications:
  ubuntu:
    charm: ubuntu
    channel: stable
    resources:
      foo-file: 42
    num_units: 1
    to:
    - "0"
    options:
      key: value
    bindings:
      another: alpha
      juju-info: vlan2
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) TestExportBundleWithApplicationStorage(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := description.NewModel(description.ModelArgs{Owner: "magic",
		Config:      coretesting.FakeConfig(),
		CloudRegion: "some-region"})

	args := s.minimalApplicationArgs(description.IAAS)
	// Add storage directives to the app
	args.StorageDirectives = map[string]description.StorageDirectiveArgs{
		"storage1": {
			Pool:  "pool1",
			Size:  1024,
			Count: 3,
		},
		"storage2": {
			Pool: "pool2",
			Size: 4096,
		},
		"storage3": {
			Size: 2048,
		},
	}
	app := model.AddApplication(args)
	app.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})
	app.SetStatus(minimalStatusArgs())

	u := app.AddUnit(minimalUnitArgs(app.Type()))
	u.SetAgentStatus(minimalStatusArgs())

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.StringResult{Result: `
default-base: ubuntu@20.04/stable
applications:
  ubuntu:
    charm: ubuntu
    channel: stable
    num_units: 1
    to:
    - "0"
    options:
      key: value
    storage:
      storage1: pool1,3,1024M
      storage2: pool2,4096M
      storage3: 2048M
    bindings:
      another: alpha
      juju-info: vlan2
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) TestExportBundleWithTrustedApplication(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := description.NewModel(description.ModelArgs{Owner: "magic",
		Config:      coretesting.FakeConfig(),
		CloudRegion: "some-region"})

	appArgs := s.minimalApplicationArgs(description.IAAS)
	appArgs.ApplicationConfig = map[string]interface{}{
		coreapplication.TrustConfigOptionName: true,
	}

	app := model.AddApplication(appArgs)
	app.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})
	app.SetStatus(minimalStatusArgs())

	u := app.AddUnit(minimalUnitArgs(app.Type()))
	u.SetAgentStatus(minimalStatusArgs())

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.StringResult{Result: `
default-base: ubuntu@20.04/stable
applications:
  ubuntu:
    charm: ubuntu
    channel: stable
    num_units: 1
    to:
    - "0"
    options:
      key: value
    bindings:
      another: alpha
      juju-info: vlan2
    trust: true
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) TestExportBundleWithApplicationOffers(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := description.NewModel(description.ModelArgs{Owner: "magic",
		Config:      coretesting.FakeConfig(),
		CloudRegion: "some-region"})

	app := model.AddApplication(s.minimalApplicationArgs(description.IAAS))
	app.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})
	app.SetStatus(minimalStatusArgs())
	u := app.AddUnit(minimalUnitArgs(app.Type()))
	u.SetAgentStatus(minimalStatusArgs())

	_ = app.AddOffer(description.ApplicationOfferArgs{
		OfferName: "my-offer",
		Endpoints: map[string]string{
			"endpoint-1": "endpoint-1",
			"endpoint-2": "endpoint-2",
		},
		ACL: map[string]string{
			"admin": "admin",
			"foo":   "consume",
		},
	})

	_ = app.AddOffer(description.ApplicationOfferArgs{
		OfferName: "my-other-offer",
		Endpoints: map[string]string{
			"endpoint-1": "endpoint-1",
			"endpoint-2": "endpoint-2",
		},
	})

	// Add second app without an offer
	app2Args := s.minimalApplicationArgs(description.IAAS)
	app2Args.Name = "foo"
	app2 := model.AddApplication(app2Args)
	app2.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})
	app2.SetStatus(minimalStatusArgs())

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Arrange.
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.StringResult{Result: `
default-base: ubuntu@20.04/stable
applications:
  foo:
    charm: ubuntu
    channel: stable
    options:
      key: value
    bindings:
      another: alpha
      juju-info: vlan2
  ubuntu:
    charm: ubuntu
    channel: stable
    num_units: 1
    to:
    - "0"
    options:
      key: value
    bindings:
      another: alpha
      juju-info: vlan2
--- # overlay.yaml
applications:
  ubuntu:
    offers:
      my-offer:
        endpoints:
        - endpoint-1
        - endpoint-2
        acl:
          admin: admin
          foo: consume
      my-other-offer:
        endpoints:
        - endpoint-1
        - endpoint-2
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) TestExportBundleWithApplicationCharmConfig(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := description.NewModel(description.ModelArgs{Owner: "magic",
		Config:      coretesting.FakeConfig(),
		CloudRegion: "some-region"})

	app := model.AddApplication(s.minimalApplicationArgsWithCharmConfig(description.IAAS, map[string]interface{}{
		"key": "value",
		"ssl_ca": `-----BEGIN RSA PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: DES-EDE3-CBC,EA657EE6FE842AD0

B8UX3J49NUiCPo0y/fhUJWdSzKjWicY13BJrsX5cv20gVjPLkkSo54BvUwhn8QgJ
D0n4TJ4TcpUtQCDhbXZNiNPNQOoahRi7AkViDSoGxZ/6HKaJR7NqDHaiikgQFcb5
6dMG8C30VjuTrbS4y5O/LqmYV9Gj6bkxuHGgxJaDN2og2RJCRJb16Ubl0HR2S129
dZA0UyQTtDkUwlhVBaQW0Rp0nwVTFZo8g5PXpvT+MsdGgs5Pop+lYq4gSCpuw3vV
T2UBOEQyYcok//9J3N+GRgfQbTidbgs8uGFGT4L8b8Ybji7Dhm/SkKpgVdH2wMuD
yxKSfIt/Gm7j1I2DnZtLrd+zUSJql2UiYMeQ7suHBHBKtFriv01zFeCzChQ17uiJ
NUo5IBzc+fagnN6aNaA5ImR07PHwMu4es4ORAEB+q/N3/0Eoe1U9h+ZBd6a7DM5V
5A/wizu2jM25tfTNwQfaSkYCZzpGmucVwMj7NO5Rbjw2/PCOo5IP8GNahXF/cxPQ
KqbIyT9nMPvKX8o9MR3Adzv5to5s9cPSyKSFfSZhJv+oXRdr8z9WaElIBJ0K0um/
ueteAX4s+U2F1Y7FAG1ivUnCOrzezvivuiPq7JS2PJh/jbdyu30xAu2gG/LZRWx+
m9zwCMUSxdrsBefl05WTzTjlpclwdjoG1sztRZ1d3/PoaMtjfLGePyEPbPFpPraq
//Dj/MNKy9sefqX4NcqRtMvGj/0VuFspXsl3B1fHRrM7BsB8IM0+BheS22+dXfOK
0EicO5ePw5Ji2vptC3ME+X3OztzJtfG/FN2CNvnLIKaR/N0v9Qt66U6aNoKPWD8l
2PW+b/BV0dz0YTo16sd6txcmcaUIo5ysZrCWLYGsoRLL1uNAlgRT8Qb2jO8liEhg
rfg5GH6RWpUL4mPo6cZ6GFAVPcdbPPSPMUSGscixeRo7oUqhnZrhvJ7L+I3TF7Pp
+aWbBpRkTvYiB3/BkgMUfZzozAv6WB2MQttHqjo7cQYgT9MklCJ80a92BkVtQsRa
hgDd6BUMA3ag4j0LivJflEmLd7prFVAcfyuN13UFku49soESUVnbPaWogyrGtc5w
CvbTazcsKiDEJqGuZhfT0ixCNPrgLj5dNJrWOmFdpZRqa/tgShkf91qCjtMq7AEi
eRQ/yOFb0sPjntORQoo0hdmXP3Y+5Ob4NqL2MNbCEf4uMOZ6OtsK4yCTfzVCMQve
pQeoosXle1nOjMrRotk6/QzbKh5rMApEjaNKekwcb47Qa8pnw2RFYZT9wsF0a7wd
wqINbgN2rnQ6Aea4If7O6Nh1Ftkrk2BW7g8N3OVwryQTFIKQ1W6Pgq6SeqoOVy0m
S6funfA+kqwEK+iXu2afvewDQkWY8+e5WkQc/5aGkbJ3K7pX8mnTgLdwGe8lZa+M
3VClT/XiHxU0ka8meio/4tq/SQUqlxiNWzgw7TJZfD1if0lAStEMAcBzZ96EQrre
HaXavAO1UPl2BczwaU2O2MDcXSE21Jw1cDCas2jc90X/iBEBM50xXTwAtNmJ84mg
UGNmDMvj8tUYI7+SvffHrTBwBPvcGeXa7XP4Au+GoJUN0jHspCeik/04KwanRCmu
-----END RSA PRIVATE KEY-----`,
	}))
	app.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})
	app.SetStatus(minimalStatusArgs())
	u := app.AddUnit(minimalUnitArgs(app.Type()))
	u.SetAgentStatus(minimalStatusArgs())

	_ = app.AddOffer(description.ApplicationOfferArgs{
		OfferName: "my-offer",
		Endpoints: map[string]string{
			"endpoint-1": "endpoint-1",
			"endpoint-2": "endpoint-2",
		},
		ACL: map[string]string{
			"admin": "admin",
			"foo":   "--- {}",
			"ssl_ca": `-----BEGIN RSA PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: DES-EDE3-CBC,EA657EE6FE842AD0

B8UX3J49NUiCPo0y/fhUJWdSzKjWicY13BJrsX5cv20gVjPLkkSo54BvUwhn8QgJ
D0n4TJ4TcpUtQCDhbXZNiNPNQOoahRi7AkViDSoGxZ/6HKaJR7NqDHaiikgQFcb5
6dMG8C30VjuTrbS4y5O/LqmYV9Gj6bkxuHGgxJaDN2og2RJCRJb16Ubl0HR2S129
dZA0UyQTtDkUwlhVBaQW0Rp0nwVTFZo8g5PXpvT+MsdGgs5Pop+lYq4gSCpuw3vV
T2UBOEQyYcok//9J3N+GRgfQbTidbgs8uGFGT4L8b8Ybji7Dhm/SkKpgVdH2wMuD
yxKSfIt/Gm7j1I2DnZtLrd+zUSJql2UiYMeQ7suHBHBKtFriv01zFeCzChQ17uiJ
NUo5IBzc+fagnN6aNaA5ImR07PHwMu4es4ORAEB+q/N3/0Eoe1U9h+ZBd6a7DM5V
5A/wizu2jM25tfTNwQfaSkYCZzpGmucVwMj7NO5Rbjw2/PCOo5IP8GNahXF/cxPQ
KqbIyT9nMPvKX8o9MR3Adzv5to5s9cPSyKSFfSZhJv+oXRdr8z9WaElIBJ0K0um/
ueteAX4s+U2F1Y7FAG1ivUnCOrzezvivuiPq7JS2PJh/jbdyu30xAu2gG/LZRWx+
m9zwCMUSxdrsBefl05WTzTjlpclwdjoG1sztRZ1d3/PoaMtjfLGePyEPbPFpPraq
//Dj/MNKy9sefqX4NcqRtMvGj/0VuFspXsl3B1fHRrM7BsB8IM0+BheS22+dXfOK
0EicO5ePw5Ji2vptC3ME+X3OztzJtfG/FN2CNvnLIKaR/N0v9Qt66U6aNoKPWD8l
2PW+b/BV0dz0YTo16sd6txcmcaUIo5ysZrCWLYGsoRLL1uNAlgRT8Qb2jO8liEhg
rfg5GH6RWpUL4mPo6cZ6GFAVPcdbPPSPMUSGscixeRo7oUqhnZrhvJ7L+I3TF7Pp
+aWbBpRkTvYiB3/BkgMUfZzozAv6WB2MQttHqjo7cQYgT9MklCJ80a92BkVtQsRa
hgDd6BUMA3ag4j0LivJflEmLd7prFVAcfyuN13UFku49soESUVnbPaWogyrGtc5w
CvbTazcsKiDEJqGuZhfT0ixCNPrgLj5dNJrWOmFdpZRqa/tgShkf91qCjtMq7AEi
eRQ/yOFb0sPjntORQoo0hdmXP3Y+5Ob4NqL2MNbCEf4uMOZ6OtsK4yCTfzVCMQve
pQeoosXle1nOjMrRotk6/QzbKh5rMApEjaNKekwcb47Qa8pnw2RFYZT9wsF0a7wd
wqINbgN2rnQ6Aea4If7O6Nh1Ftkrk2BW7g8N3OVwryQTFIKQ1W6Pgq6SeqoOVy0m
S6funfA+kqwEK+iXu2afvewDQkWY8+e5WkQc/5aGkbJ3K7pX8mnTgLdwGe8lZa+M
3VClT/XiHxU0ka8meio/4tq/SQUqlxiNWzgw7TJZfD1if0lAStEMAcBzZ96EQrre
HaXavAO1UPl2BczwaU2O2MDcXSE21Jw1cDCas2jc90X/iBEBM50xXTwAtNmJ84mg
UGNmDMvj8tUYI7+SvffHrTBwBPvcGeXa7XP4Au+GoJUN0jHspCeik/04KwanRCmu
-----END RSA PRIVATE KEY-----`,
		},
	})

	_ = app.AddOffer(description.ApplicationOfferArgs{
		OfferName: "my-other-offer",
		Endpoints: map[string]string{
			"endpoint-1": "endpoint-1",
			"endpoint-2": "endpoint-2",
		},
	})

	// Add second app without an offer
	app2Args := s.minimalApplicationArgs(description.IAAS)
	app2Args.Name = "foo"
	app2 := model.AddApplication(app2Args)
	app2.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})
	app2.SetStatus(minimalStatusArgs())

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.StringResult{Result: `
default-base: ubuntu@20.04/stable
applications:
  foo:
    charm: ubuntu
    channel: stable
    options:
      key: value
    bindings:
      another: alpha
      juju-info: vlan2
  ubuntu:
    charm: ubuntu
    channel: stable
    num_units: 1
    to:
    - "0"
    options:
      key: value
      ssl_ca: |-
        -----BEGIN RSA PRIVATE KEY-----
        Proc-Type: 4,ENCRYPTED
        DEK-Info: DES-EDE3-CBC,EA657EE6FE842AD0

        B8UX3J49NUiCPo0y/fhUJWdSzKjWicY13BJrsX5cv20gVjPLkkSo54BvUwhn8QgJ
        D0n4TJ4TcpUtQCDhbXZNiNPNQOoahRi7AkViDSoGxZ/6HKaJR7NqDHaiikgQFcb5
        6dMG8C30VjuTrbS4y5O/LqmYV9Gj6bkxuHGgxJaDN2og2RJCRJb16Ubl0HR2S129
        dZA0UyQTtDkUwlhVBaQW0Rp0nwVTFZo8g5PXpvT+MsdGgs5Pop+lYq4gSCpuw3vV
        T2UBOEQyYcok//9J3N+GRgfQbTidbgs8uGFGT4L8b8Ybji7Dhm/SkKpgVdH2wMuD
        yxKSfIt/Gm7j1I2DnZtLrd+zUSJql2UiYMeQ7suHBHBKtFriv01zFeCzChQ17uiJ
        NUo5IBzc+fagnN6aNaA5ImR07PHwMu4es4ORAEB+q/N3/0Eoe1U9h+ZBd6a7DM5V
        5A/wizu2jM25tfTNwQfaSkYCZzpGmucVwMj7NO5Rbjw2/PCOo5IP8GNahXF/cxPQ
        KqbIyT9nMPvKX8o9MR3Adzv5to5s9cPSyKSFfSZhJv+oXRdr8z9WaElIBJ0K0um/
        ueteAX4s+U2F1Y7FAG1ivUnCOrzezvivuiPq7JS2PJh/jbdyu30xAu2gG/LZRWx+
        m9zwCMUSxdrsBefl05WTzTjlpclwdjoG1sztRZ1d3/PoaMtjfLGePyEPbPFpPraq
        //Dj/MNKy9sefqX4NcqRtMvGj/0VuFspXsl3B1fHRrM7BsB8IM0+BheS22+dXfOK
        0EicO5ePw5Ji2vptC3ME+X3OztzJtfG/FN2CNvnLIKaR/N0v9Qt66U6aNoKPWD8l
        2PW+b/BV0dz0YTo16sd6txcmcaUIo5ysZrCWLYGsoRLL1uNAlgRT8Qb2jO8liEhg
        rfg5GH6RWpUL4mPo6cZ6GFAVPcdbPPSPMUSGscixeRo7oUqhnZrhvJ7L+I3TF7Pp
        +aWbBpRkTvYiB3/BkgMUfZzozAv6WB2MQttHqjo7cQYgT9MklCJ80a92BkVtQsRa
        hgDd6BUMA3ag4j0LivJflEmLd7prFVAcfyuN13UFku49soESUVnbPaWogyrGtc5w
        CvbTazcsKiDEJqGuZhfT0ixCNPrgLj5dNJrWOmFdpZRqa/tgShkf91qCjtMq7AEi
        eRQ/yOFb0sPjntORQoo0hdmXP3Y+5Ob4NqL2MNbCEf4uMOZ6OtsK4yCTfzVCMQve
        pQeoosXle1nOjMrRotk6/QzbKh5rMApEjaNKekwcb47Qa8pnw2RFYZT9wsF0a7wd
        wqINbgN2rnQ6Aea4If7O6Nh1Ftkrk2BW7g8N3OVwryQTFIKQ1W6Pgq6SeqoOVy0m
        S6funfA+kqwEK+iXu2afvewDQkWY8+e5WkQc/5aGkbJ3K7pX8mnTgLdwGe8lZa+M
        3VClT/XiHxU0ka8meio/4tq/SQUqlxiNWzgw7TJZfD1if0lAStEMAcBzZ96EQrre
        HaXavAO1UPl2BczwaU2O2MDcXSE21Jw1cDCas2jc90X/iBEBM50xXTwAtNmJ84mg
        UGNmDMvj8tUYI7+SvffHrTBwBPvcGeXa7XP4Au+GoJUN0jHspCeik/04KwanRCmu
        -----END RSA PRIVATE KEY-----
    bindings:
      another: alpha
      juju-info: vlan2
--- # overlay.yaml
applications:
  ubuntu:
    offers:
      my-offer:
        endpoints:
        - endpoint-1
        - endpoint-2
        acl:
          admin: admin
          foo: '--- {}'
          ssl_ca: |-
            -----BEGIN RSA PRIVATE KEY-----
            Proc-Type: 4,ENCRYPTED
            DEK-Info: DES-EDE3-CBC,EA657EE6FE842AD0

            B8UX3J49NUiCPo0y/fhUJWdSzKjWicY13BJrsX5cv20gVjPLkkSo54BvUwhn8QgJ
            D0n4TJ4TcpUtQCDhbXZNiNPNQOoahRi7AkViDSoGxZ/6HKaJR7NqDHaiikgQFcb5
            6dMG8C30VjuTrbS4y5O/LqmYV9Gj6bkxuHGgxJaDN2og2RJCRJb16Ubl0HR2S129
            dZA0UyQTtDkUwlhVBaQW0Rp0nwVTFZo8g5PXpvT+MsdGgs5Pop+lYq4gSCpuw3vV
            T2UBOEQyYcok//9J3N+GRgfQbTidbgs8uGFGT4L8b8Ybji7Dhm/SkKpgVdH2wMuD
            yxKSfIt/Gm7j1I2DnZtLrd+zUSJql2UiYMeQ7suHBHBKtFriv01zFeCzChQ17uiJ
            NUo5IBzc+fagnN6aNaA5ImR07PHwMu4es4ORAEB+q/N3/0Eoe1U9h+ZBd6a7DM5V
            5A/wizu2jM25tfTNwQfaSkYCZzpGmucVwMj7NO5Rbjw2/PCOo5IP8GNahXF/cxPQ
            KqbIyT9nMPvKX8o9MR3Adzv5to5s9cPSyKSFfSZhJv+oXRdr8z9WaElIBJ0K0um/
            ueteAX4s+U2F1Y7FAG1ivUnCOrzezvivuiPq7JS2PJh/jbdyu30xAu2gG/LZRWx+
            m9zwCMUSxdrsBefl05WTzTjlpclwdjoG1sztRZ1d3/PoaMtjfLGePyEPbPFpPraq
            //Dj/MNKy9sefqX4NcqRtMvGj/0VuFspXsl3B1fHRrM7BsB8IM0+BheS22+dXfOK
            0EicO5ePw5Ji2vptC3ME+X3OztzJtfG/FN2CNvnLIKaR/N0v9Qt66U6aNoKPWD8l
            2PW+b/BV0dz0YTo16sd6txcmcaUIo5ysZrCWLYGsoRLL1uNAlgRT8Qb2jO8liEhg
            rfg5GH6RWpUL4mPo6cZ6GFAVPcdbPPSPMUSGscixeRo7oUqhnZrhvJ7L+I3TF7Pp
            +aWbBpRkTvYiB3/BkgMUfZzozAv6WB2MQttHqjo7cQYgT9MklCJ80a92BkVtQsRa
            hgDd6BUMA3ag4j0LivJflEmLd7prFVAcfyuN13UFku49soESUVnbPaWogyrGtc5w
            CvbTazcsKiDEJqGuZhfT0ixCNPrgLj5dNJrWOmFdpZRqa/tgShkf91qCjtMq7AEi
            eRQ/yOFb0sPjntORQoo0hdmXP3Y+5Ob4NqL2MNbCEf4uMOZ6OtsK4yCTfzVCMQve
            pQeoosXle1nOjMrRotk6/QzbKh5rMApEjaNKekwcb47Qa8pnw2RFYZT9wsF0a7wd
            wqINbgN2rnQ6Aea4If7O6Nh1Ftkrk2BW7g8N3OVwryQTFIKQ1W6Pgq6SeqoOVy0m
            S6funfA+kqwEK+iXu2afvewDQkWY8+e5WkQc/5aGkbJ3K7pX8mnTgLdwGe8lZa+M
            3VClT/XiHxU0ka8meio/4tq/SQUqlxiNWzgw7TJZfD1if0lAStEMAcBzZ96EQrre
            HaXavAO1UPl2BczwaU2O2MDcXSE21Jw1cDCas2jc90X/iBEBM50xXTwAtNmJ84mg
            UGNmDMvj8tUYI7+SvffHrTBwBPvcGeXa7XP4Au+GoJUN0jHspCeik/04KwanRCmu
            -----END RSA PRIVATE KEY-----
      my-other-offer:
        endpoints:
        - endpoint-1
        - endpoint-2
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) TestExportBundleWithSaas(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := description.NewModel(description.ModelArgs{Owner: "magic",
		Config:      coretesting.FakeConfig(),
		CloudRegion: "some-region"})

	app := model.AddApplication(s.minimalApplicationArgs(description.IAAS))
	app.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})
	app.SetStatus(minimalStatusArgs())

	remoteApp := model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name: "awesome",
		URL:  "test:admin/default.awesome",
	})
	remoteApp.SetStatus(minimalStatusArgs())

	u := app.AddUnit(minimalUnitArgs(app.Type()))
	u.SetAgentStatus(minimalStatusArgs())

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.StringResult{Result: `
default-base: ubuntu@20.04/stable
saas:
  awesome:
    url: test:admin/default.awesome
applications:
  ubuntu:
    charm: ubuntu
    channel: stable
    num_units: 1
    to:
    - "0"
    options:
      key: value
    bindings:
      another: alpha
      juju-info: vlan2
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) addApplicationToModel(model description.Model, name string, numUnits int) string {
	var charmURL string
	var channel string

	if strings.HasPrefix(name, "ch:") {
		charmURL = name
		name = name[3:]
		channel = "stable"
	} else if strings.HasPrefix(name, "local:") {
		charmURL = name
		curl := charm.MustParseURL(name)
		name = curl.Name
	} else {
		charmURL = "ch:" + name
	}
	application := model.AddApplication(description.ApplicationArgs{
		Name:               name,
		CharmURL:           charmURL,
		Channel:            channel,
		CharmConfig:        map[string]interface{}{},
		LeadershipSettings: map[string]interface{}{},
	})
	application.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})
	application.SetStatus(minimalStatusArgs())
	for i := 0; i < numUnits; i++ {
		machine := model.AddMachine(description.MachineArgs{
			Id:   fmt.Sprint(i),
			Base: "ubuntu@20.04",
		})
		unit := application.AddUnit(description.UnitArgs{
			Name:    fmt.Sprintf("%s/%d", name, i),
			Machine: machine.Id(),
		})
		unit.SetAgentStatus(minimalStatusArgs())
	}

	return name
}

func (s *bundleSuite) setEndpointSettings(ep description.Endpoint, units ...string) {
	for _, unit := range units {
		ep.SetUnitSettings(unit, map[string]interface{}{
			"key": "value",
		})
	}
}

func (s *bundleSuite) newModel(modelType string, app1 string, app2 string) description.Model {
	model := description.NewModel(description.ModelArgs{
		Type:        modelType,
		Owner:       "magic",
		Config:      coretesting.FakeConfig(),
		CloudRegion: "some-region"})

	appName1 := s.addApplicationToModel(model, app1, 2)
	appName2 := s.addApplicationToModel(model, app2, 1)

	// Add a relation between wordpress and mysql.
	rel := model.AddRelation(description.RelationArgs{
		Id:  42,
		Key: "special key",
	})
	rel.SetStatus(minimalStatusArgs())

	app1Endpoint := rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: appName1,
		Name:            "db",
		// Ignoring other aspects of endpoints.
	})
	s.setEndpointSettings(app1Endpoint, appName1+"/0", appName1+"/1")

	app2Endpoint := rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "mysql",
		Name:            "mysql",
		// Ignoring other aspects of endpoints.
	})
	s.setEndpointSettings(app2Endpoint, appName2+"/0")

	return model
}

func (s *bundleSuite) TestExportBundleModelWithSettingsRelations(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)
	model := s.newModel("iaas", "wordpress", "mysql")

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)

	output := `
default-base: ubuntu@20.04/stable
applications:
  mysql:
    charm: mysql
    num_units: 1
    to:
    - "0"
  wordpress:
    charm: wordpress
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
	expectedResult := params.StringResult{Result: output}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) TestExportBundleModelWithCharmDefaults(c *gc.C) {
	// Arrange.
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	s.facade = s.makeAPI(c)

	model := s.newModel("iaas", "wordpress", "mysql")
	app := model.AddApplication(description.ApplicationArgs{
		Name:     "mariadb",
		CharmURL: "ch:mariadb",
		CharmConfig: map[string]interface{}{
			"mem": 200,
			"foo": "baz",
		},
	})
	app.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	wordpressCharm := NewMockCharm(ctrl)
	s.applicationService.EXPECT().GetCharm(gomock.Any(), applicationcharm.CharmLocator{
		Name:     "wordpress",
		Source:   applicationcharm.CharmHubSource,
		Revision: -1,
	}).Return(wordpressCharm, applicationcharm.CharmLocator{}, true, nil)
	wordpressCharm.EXPECT().Config().Return(&charm.Config{
		Options: map[string]charm.Option{
			"foo": {Default: "bar"},
		},
	})

	mysqlCharm := NewMockCharm(ctrl)
	s.applicationService.EXPECT().GetCharm(gomock.Any(), applicationcharm.CharmLocator{
		Name:     "mysql",
		Source:   applicationcharm.CharmHubSource,
		Revision: -1,
	}).Return(mysqlCharm, applicationcharm.CharmLocator{}, true, nil)
	mysqlCharm.EXPECT().Config().Return(&charm.Config{
		Options: map[string]charm.Option{
			"foo": {Default: "bar"},
		},
	})

	mariadbCharm := NewMockCharm(ctrl)
	s.applicationService.EXPECT().GetCharm(gomock.Any(), applicationcharm.CharmLocator{
		Name:     "mariadb",
		Source:   applicationcharm.CharmHubSource,
		Revision: -1,
	}).Return(mariadbCharm, applicationcharm.CharmLocator{}, true, nil)
	mariadbCharm.EXPECT().Config().Return(&charm.Config{})

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{IncludeCharmDefaults: true})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)

	output := `
default-base: ubuntu@20.04/stable
applications:
  mariadb:
    charm: mariadb
    options:
      foo: baz
      mem: 200
  mysql:
    charm: mysql
    num_units: 1
    to:
    - "0"
    options:
      foo: bar
  wordpress:
    charm: wordpress
    num_units: 2
    to:
    - "0"
    - "1"
    options:
      foo: bar
machines:
  "0": {}
  "1": {}
relations:
- - wordpress:db
  - mysql:mysql
`[1:]
	expectedResult := params.StringResult{Result: output}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) addSubordinateEndpoints(rel description.Relation, app string) (description.Endpoint, description.Endpoint) {
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
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := s.newModel("iaas", "wordpress", "mysql")

	// Add a subordinate relations between logging and both wordpress and mysql.
	rel := model.AddRelation(description.RelationArgs{
		Id:  43,
		Key: "some key",
	})
	wordpressEndpoint, loggingEndpoint := s.addSubordinateEndpoints(rel, "wordpress")
	s.setEndpointSettings(wordpressEndpoint, "wordpress/0", "wordpress/1")
	s.setEndpointSettings(loggingEndpoint, "logging/0", "logging/1")

	rel = model.AddRelation(description.RelationArgs{
		Id:  44,
		Key: "other key",
	})
	mysqlEndpoint, loggingEndpoint := s.addSubordinateEndpoints(rel, "mysql")
	s.setEndpointSettings(mysqlEndpoint, "mysql/0")
	s.setEndpointSettings(loggingEndpoint, "logging/2")

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)

	expectedResult := params.StringResult{Result: `
default-base: ubuntu@20.04/stable
applications:
  mysql:
    charm: mysql
    num_units: 1
    to:
    - "0"
  wordpress:
    charm: wordpress
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
}

func (s *bundleSuite) TestExportBundleSubordinateApplication(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := description.NewModel(description.ModelArgs{Owner: "magic",
		Config:      coretesting.FakeConfig(),
		CloudRegion: "some-region"})

	application := model.AddApplication(description.ApplicationArgs{
		Name:                 "magic",
		Subordinate:          true,
		CharmURL:             "ch:magic",
		Channel:              "stable",
		CharmModifiedVersion: 1,
		ForceCharm:           true,
		Exposed:              true,
		EndpointBindings: map[string]string{
			"rel-name": "2",
			"magic":    "0",
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
	})
	application.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/18.04/stable"})
	application.SetStatus(minimalStatusArgs())

	s.expectGetAllSpaces(c, network.SpaceInfo{
		ID:   "2",
		Name: "some-space",
	})
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)

	expectedResult := params.StringResult{Result: `
default-base: ubuntu@18.04/stable
applications:
  magic:
    charm: magic
    channel: stable
    expose: true
    options:
      key: value
    bindings:
      magic: alpha
      rel-name: some-space
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) setupExportBundleEndpointBindingsPrinted(all, oneOff string) description.Model {
	model := description.NewModel(description.ModelArgs{Owner: "magic",
		Config:      coretesting.FakeConfig(),
		CloudRegion: "some-region"})

	args := s.minimalApplicationArgs(description.IAAS)
	args.EndpointBindings = map[string]string{
		"rel-name": all,
		"another":  all,
	}
	app := model.AddApplication(args)
	app.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})

	app = model.AddApplication(description.ApplicationArgs{
		Name:                 "magic",
		Subordinate:          true,
		CharmURL:             "ch:magic",
		Channel:              "stable",
		CharmModifiedVersion: 1,
		ForceCharm:           true,
		Exposed:              true,
		EndpointBindings: map[string]string{
			"rel-name": all,
			"another":  oneOff,
		},
		ApplicationConfig: map[string]interface{}{
			"config key": "config value",
		},
	})
	app.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/18.04/stable"})
	return model
}

func (s *bundleSuite) TestExportBundleNoEndpointBindingsPrinted(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := s.setupExportBundleEndpointBindingsPrinted("0", "0")

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)

	expectedResult := params.StringResult{Result: `
applications:
  magic:
    charm: magic
    channel: stable
    base: ubuntu@18.04/stable
    expose: true
  ubuntu:
    charm: ubuntu
    channel: stable
    base: ubuntu@20.04/stable
    options:
      key: value
`[1:]}
	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) TestExportBundleEndpointBindingsPrinted(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := s.setupExportBundleEndpointBindingsPrinted("0", "1")

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Arrange.
	c.Assert(err, jc.ErrorIsNil)

	expectedResult := params.StringResult{Result: `
applications:
  magic:
    charm: magic
    channel: stable
    base: ubuntu@18.04/stable
    expose: true
    bindings:
      another: vlan2
      rel-name: alpha
  ubuntu:
    charm: ubuntu
    channel: stable
    base: ubuntu@20.04/stable
    options:
      key: value
    bindings:
      another: alpha
      rel-name: alpha
`[1:]}
	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) TestExportBundleSubordinateApplicationAndMachine(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := description.NewModel(description.ModelArgs{Owner: "magic",
		Config:      coretesting.FakeConfig(),
		CloudRegion: "some-region"})

	application := model.AddApplication(description.ApplicationArgs{
		Name:        "magic",
		Subordinate: true,
		CharmURL:    "ch:amd64/zesty/magic",
		Channel:     "stable",
		Exposed:     true,
		CharmConfig: map[string]interface{}{
			"key": "value",
		},
	})
	application.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/17.04/stable"})
	application.SetStatus(minimalStatusArgs())

	s.addMinimalMachineWithConstraints(model, "0")

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Arrange.
	c.Assert(err, jc.ErrorIsNil)

	expectedResult := params.StringResult{Result: `
default-base: ubuntu@17.04/stable
applications:
  magic:
    charm: magic
    channel: stable
    expose: true
    options:
      key: value
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) addMinimalMachineWithConstraints(model description.Model, id string) {
	m := model.AddMachine(description.MachineArgs{
		Id:           id,
		Nonce:        "a-nonce",
		PasswordHash: "some-hash",
		Base:         "ubuntu@20.04",
		Jobs:         []string{"host-units"},
	})
	args := description.ConstraintsArgs{
		Architecture:   "amd64",
		Memory:         8 * 1024,
		RootDisk:       40 * 1024,
		CpuCores:       2,
		CpuPower:       100,
		InstanceType:   "big-inst",
		Tags:           []string{"foo", "bar"},
		VirtType:       "pv",
		Container:      "kvm",
		Spaces:         []string{"internal"},
		Zones:          []string{"zone-a"},
		RootDiskSource: "source-of-all-evil",
	}
	m.SetConstraints(args)
	m.SetStatus(minimalStatusArgs())
}

func (s *bundleSuite) TestExportBundleModelWithConstraints(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := s.newModel("iaas", "mediawiki", "mysql")

	s.addMinimalMachineWithConstraints(model, "0")
	s.addMinimalMachineWithConstraints(model, "1")

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.StringResult{Result: `
default-base: ubuntu@20.04/stable
applications:
  mediawiki:
    charm: mediawiki
    num_units: 2
    to:
    - "0"
    - "1"
  mysql:
    charm: mysql
    num_units: 1
    to:
    - "0"
machines:
  "0":
    constraints: arch=amd64 cpu-cores=2 cpu-power=100 mem=8192 root-disk=40960 instance-type=big-inst
      container=kvm virt-type=pv tags=foo,bar spaces=internal zones=zone-a root-disk-source=source-of-all-evil
  "1":
    constraints: arch=amd64 cpu-cores=2 cpu-power=100 mem=8192 root-disk=40960 instance-type=big-inst
      container=kvm virt-type=pv tags=foo,bar spaces=internal zones=zone-a root-disk-source=source-of-all-evil
relations:
- - mediawiki:db
  - mysql:mysql
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) addMinimalMachineWithAnnotations(model description.Model, id string) {
	m := model.AddMachine(description.MachineArgs{
		Id:           id,
		Nonce:        "a-nonce",
		PasswordHash: "some-hash",
		Base:         "ubuntu@20.04",
		Jobs:         []string{"host-units"},
	})
	m.SetAnnotations(map[string]string{
		"string":  "value",
		"another": "one",
	})
	m.SetStatus(minimalStatusArgs())
}

func (s *bundleSuite) TestExportBundleModelWithAnnotations(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := s.newModel("iaas", "wordpress", "mysql")

	s.addMinimalMachineWithAnnotations(model, "0")
	s.addMinimalMachineWithAnnotations(model, "1")

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.StringResult{Result: `
default-base: ubuntu@20.04/stable
applications:
  mysql:
    charm: mysql
    num_units: 1
    to:
    - "0"
  wordpress:
    charm: wordpress
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
}

func (s *bundleSuite) TestExportBundleWithContainers(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := description.NewModel(description.ModelArgs{Owner: "magic",
		Config:      coretesting.FakeConfig(),
		CloudRegion: "some-region"})

	application0 := model.AddApplication(description.ApplicationArgs{
		Name:     "wordpress",
		CharmURL: "ch:wordpress",
	})
	application0.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})
	application0.SetStatus(minimalStatusArgs())

	m0 := model.AddMachine(description.MachineArgs{
		Id:   "0",
		Base: "ubuntu@20.04",
	})
	args := description.ConstraintsArgs{
		Architecture: "amd64",
		Memory:       8 * 1024,
		RootDisk:     40 * 1024,
	}
	m0.SetConstraints(args)
	ut0 := application0.AddUnit(description.UnitArgs{
		Name:    "wordpress/0",
		Machine: "0",
	})
	ut0.SetAgentStatus(minimalStatusArgs())

	application1 := model.AddApplication(description.ApplicationArgs{
		Name:     "mysql",
		CharmURL: "ch:mysql",
	})
	application1.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})
	application1.SetStatus(minimalStatusArgs())

	m1 := model.AddMachine(description.MachineArgs{
		Id:   "1",
		Base: "ubuntu@20.04",
	})
	args = description.ConstraintsArgs{
		Architecture: "amd64",
		Memory:       8 * 1024,
		RootDisk:     40 * 1024,
	}
	m1.SetConstraints(args)

	ut := application1.AddUnit(description.UnitArgs{
		Name:    "mysql/1",
		Machine: "1/lxd/0",
	})
	ut.SetAgentStatus(minimalStatusArgs())

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Arrange.
	c.Assert(err, jc.ErrorIsNil)
	expectedResult := params.StringResult{Result: `
default-base: ubuntu@20.04/stable
applications:
  mysql:
    charm: mysql
    num_units: 1
    to:
    - lxd:1
  wordpress:
    charm: wordpress
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
}

func (s *bundleSuite) TestMixedSeries(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := description.NewModel(description.ModelArgs{Owner: "magic",
		Config: coretesting.FakeConfig().Merge(map[string]interface{}{
			"default-base": "ubuntu@20.04",
		}),
		CloudRegion: "some-region"})

	application := model.AddApplication(description.ApplicationArgs{
		Name:     "magic",
		CharmURL: "ch:magic",
	})
	application.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})
	application.AddUnit(description.UnitArgs{
		Name:    "magic/0",
		Machine: "0",
	})
	model.AddMachine(description.MachineArgs{
		Id:   "0",
		Base: "ubuntu@20.04",
	})

	application = model.AddApplication(description.ApplicationArgs{
		Name:     "mojo",
		CharmURL: "ch:mojo",
	})
	application.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/22.04/stable"})
	application.AddUnit(description.UnitArgs{
		Name:    "mojo/0",
		Machine: "1",
	})
	model.AddMachine(description.MachineArgs{
		Id:   "1",
		Base: "ubuntu@22.04",
	})

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)

	expectedResult := params.StringResult{Result: `
default-base: ubuntu@20.04/stable
applications:
  magic:
    charm: magic
    num_units: 1
    to:
    - "0"
  mojo:
    charm: mojo
    base: ubuntu@22.04/stable
    num_units: 1
    to:
    - "1"
machines:
  "0": {}
  "1":
    base: ubuntu@22.04/stable
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) TestMixedSeriesNoDefaultSeries(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	model := description.NewModel(description.ModelArgs{Owner: "magic",
		Config: coretesting.FakeConfig().Merge(map[string]interface{}{
			"default-base": "ubuntu@20.04",
		}),
		CloudRegion: "some-region"})

	application := model.AddApplication(description.ApplicationArgs{
		Name:     "magic",
		CharmURL: "ch:magic",
	})

	// TODO(jack-w-shaw) this test is somewhat contrived since ubuntu@21.04 is not a supported base.
	// However, since this test requires at least 3 different bases, we need to do this until 24.04
	// is supported
	application.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/21.04/stable"})
	application.AddUnit(description.UnitArgs{
		Name:    "magic/0",
		Machine: "0",
	})
	model.AddMachine(description.MachineArgs{
		Id:   "0",
		Base: "ubuntu@21.04",
	})

	application = model.AddApplication(description.ApplicationArgs{
		Name:     "mojo",
		CharmURL: "ch:mojo",
	})
	application.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/22.04/stable"})
	application.AddUnit(description.UnitArgs{
		Name:    "mojo/0",
		Machine: "1",
	})
	model.AddMachine(description.MachineArgs{
		Id:   "1",
		Base: "ubuntu@22.04",
	})

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)

	expectedResult := params.StringResult{Result: `
applications:
  magic:
    charm: magic
    base: ubuntu@21.04/stable
    num_units: 1
    to:
    - "0"
  mojo:
    charm: mojo
    base: ubuntu@22.04/stable
    num_units: 1
    to:
    - "1"
machines:
  "0":
    base: ubuntu@21.04/stable
  "1":
    base: ubuntu@22.04/stable
`[1:]}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) TestExportKubernetesBundle(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)
	model := s.newModel("caas", "wordpress", "mysql")

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)

	output := `
bundle: kubernetes
applications:
  mysql:
    charm: mysql
    scale: 1
  wordpress:
    charm: wordpress
    scale: 2
relations:
- - wordpress:db
  - mysql:mysql
`[1:]
	expectedResult := params.StringResult{Result: output}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) TestExportCharmhubBundle(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)
	model := s.newModel("iaas", "ch:wordpress", "ch:mysql")

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)

	output := `
default-base: ubuntu@20.04/stable
applications:
  mysql:
    charm: mysql
    channel: stable
    num_units: 1
    to:
    - "0"
  wordpress:
    charm: wordpress
    channel: stable
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
	expectedResult := params.StringResult{Result: output}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) TestExportLocalBundle(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)
	model := s.newModel("iaas", "local:wordpress", "local:mysql")

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)

	output := `
default-base: ubuntu@20.04/stable
applications:
  mysql:
    charm: local:mysql
    num_units: 1
    to:
    - "0"
  wordpress:
    charm: local:wordpress
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
	expectedResult := params.StringResult{Result: output}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) TestExportLocalBundleWithSeries(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)
	model := s.newModel("iaas", "local:focal/wordpress", "local:mysql")

	s.expectGetAllSpaces(c)
	s.expectExportModel(model)

	// Act.
	result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

	// Assert.
	c.Assert(err, jc.ErrorIsNil)

	output := `
default-base: ubuntu@20.04/stable
applications:
  mysql:
    charm: local:mysql
    num_units: 1
    to:
    - "0"
  wordpress:
    charm: local:wordpress
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
	expectedResult := params.StringResult{Result: output}

	c.Assert(result, gc.Equals, expectedResult)
}

func (s *bundleSuite) TestExportBundleWithExposedEndpointSettings(c *gc.C) {
	// Arrange.
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	specs := []struct {
		descr            string
		exposed          bool
		exposedEndpoints map[string]description.ExposedEndpointArgs
		expBundle        string
	}{
		{
			descr:   "exposed application without exposed endpoint settings (upgraded 2.8 controller)",
			exposed: true,
			expBundle: `
default-base: ubuntu@20.04/stable
applications:
  magic:
    charm: magic
    channel: stable
    expose: true
    bindings:
      hat: some-space
      rabbit: alpha
`[1:],
		},
		{
			descr:   "exposed application where all endpoints are exposed to 0.0.0.0/0",
			exposed: true,
			// This is equivalent to having "expose:true" and skipping the exposed settings section from the overlay
			exposedEndpoints: map[string]description.ExposedEndpointArgs{
				"": {
					ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR},
				},
			},
			expBundle: `
default-base: ubuntu@20.04/stable
applications:
  magic:
    charm: magic
    channel: stable
    expose: true
    bindings:
      hat: some-space
      rabbit: alpha
`[1:],
		},
		{
			descr:   "exposed application with per-endpoint expose settings",
			exposed: true,
			exposedEndpoints: map[string]description.ExposedEndpointArgs{
				"rabbit": {
					ExposeToSpaceIDs: []string{"2"},
					ExposeToCIDRs:    []string{"10.0.0.0/24", "192.168.0.0/24"},
				},
				"hat": {
					ExposeToCIDRs: []string{"192.168.0.0/24"},
				},
			},
			// The exposed:true will be omitted and only the exposed-endpoints section (in the overlay) will be present
			expBundle: `
default-base: ubuntu@20.04/stable
applications:
  magic:
    charm: magic
    channel: stable
    bindings:
      hat: some-space
      rabbit: alpha
--- # overlay.yaml
applications:
  magic:
    exposed-endpoints:
      hat:
        expose-to-cidrs:
        - 192.168.0.0/24
      rabbit:
        expose-to-spaces:
        - some-space
        expose-to-cidrs:
        - 10.0.0.0/24
        - 192.168.0.0/24
`[1:],
		},
	}

	for i, spec := range specs {
		c.Logf("%d. %s", i, spec.descr)

		model := description.NewModel(description.ModelArgs{Owner: "magic",
			Config:      coretesting.FakeConfig(),
			CloudRegion: "some-region"},
		)

		application := model.AddApplication(description.ApplicationArgs{
			Name:                 "magic",
			CharmURL:             "ch:amd64/focal/magic",
			Channel:              "stable",
			CharmModifiedVersion: 1,
			ForceCharm:           true,
			Exposed:              spec.exposed,
			ExposedEndpoints:     spec.exposedEndpoints,
			EndpointBindings: map[string]string{
				"hat":    "2",
				"rabbit": "0",
			},
		})
		application.SetCharmOrigin(description.CharmOriginArgs{Platform: "amd64/ubuntu/20.04/stable"})
		application.SetStatus(minimalStatusArgs())

		s.expectGetAllSpaces(c, network.SpaceInfo{
			ID:   "2",
			Name: "some-space",
		})
		s.expectExportModel(model)

		// Act.
		result, err := s.facade.ExportBundle(context.Background(), params.ExportBundleParams{})

		// Assert.
		c.Assert(err, jc.ErrorIsNil)

		exp := params.StringResult{Result: spec.expBundle}
		c.Assert(result, gc.Equals, exp)
	}
}

func (s *bundleSuite) expectExportModel(model description.Model) *MockModelExporterExportModelPartialCall {
	// This is expected configuration while calling export partial model, for
	//  non regression.
	exportConfig := state.ExportConfig{
		SkipActions:              true,
		SkipCloudImageMetadata:   true,
		SkipCredentials:          true,
		SkipIPAddresses:          true,
		SkipSSHHostKeys:          true,
		SkipLinkLayerDevices:     true,
		SkipRelationData:         true,
		SkipMachineAgentBinaries: true,
		SkipUnitAgentBinaries:    true,
		SkipInstanceData:         true,
		SkipSecrets:              true,
	}
	return s.modelExporter.EXPECT().ExportModelPartial(gomock.Any(), exportConfig, s.store).Return(model, nil)
}
