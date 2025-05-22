// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/client/bundle"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type bundleSuite struct {
	coretesting.BaseSuite
	auth               *apiservertesting.FakeAuthorizer
	facade             *bundle.APIv8
	st                 *mockState
	store              *mockObjectStore
	networkService     *MockNetworkService
	applicationService *MockApplicationService
}

func TestBundleSuite(t *testing.T) {
	tc.Run(t, &bundleSuite{})
}

func (s *bundleSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.networkService = NewMockNetworkService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	return ctrl
}

func (s *bundleSuite) SetUpTest(c *tc.C) {

	s.BaseSuite.SetUpTest(c)
	s.auth = &apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("read"),
	}

	s.st = newMockState()
}

func (s *bundleSuite) makeAPI(c *tc.C) *bundle.APIv8 {
	api, err := bundle.NewBundleAPI(
		s.st,
		s.store,
		s.auth,
		s.networkService,
		s.applicationService,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)
	return &bundle.APIv8{api}
}

func (s *bundleSuite) TestGetChangesMapArgsBundleContentError(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.facade = s.makeAPI(c)

	args := params.BundleChangesParams{
		BundleDataYAML: ":",
	}
	r, err := s.facade.GetChangesMapArgs(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `cannot read bundle YAML: malformed bundle: bundle is empty not valid`)
	c.Assert(r, tc.DeepEquals, params.BundleChangesMapArgsResults{})
}

func (s *bundleSuite) TestGetChangesMapArgsBundleVerificationErrors(c *tc.C) {
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
	r, err := s.facade.GetChangesMapArgs(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Changes, tc.IsNil)
	c.Assert(r.Errors, tc.SameContents, []string{
		`placement "1" refers to a machine not defined in this bundle`,
		`too many units specified in unit placement for application "django"`,
		`invalid charm URL in application "haproxy": cannot parse name and/or revision in URL "42": name "42" not valid`,
		`negative number of units specified on application "haproxy"`,
	})
}

func (s *bundleSuite) TestGetChangesMapArgsBundleConstraintsError(c *tc.C) {
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
	r, err := s.facade.GetChangesMapArgs(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Changes, tc.IsNil)
	c.Assert(r.Errors, tc.SameContents, []string{
		`invalid constraints "bad=wolf" in application "django": unknown constraint "bad"`,
	})
}

func (s *bundleSuite) TestGetChangesMapArgsBundleStorageError(c *tc.C) {
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
	r, err := s.facade.GetChangesMapArgs(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Changes, tc.IsNil)
	c.Assert(r.Errors, tc.SameContents, []string{
		`invalid storage "bad" in application "django": cannot parse count: count must be greater than zero, got "0"`,
	})
}

func (s *bundleSuite) TestGetChangesMapArgsBundleDevicesError(c *tc.C) {
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
	r, err := s.facade.GetChangesMapArgs(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Changes, tc.IsNil)
	c.Assert(r.Errors, tc.SameContents, []string{
		`invalid device "bad-gpu" in application "django": count must be greater than zero, got "-1"`,
	})
}

func (s *bundleSuite) TestGetChangesMapArgsSuccess(c *tc.C) {
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
	r, err := s.facade.GetChangesMapArgs(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(r.Changes, tc.DeepEquals, []*params.BundleChangesMapArgs{{
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
	}}, tc.Commentf("\nobtained: %s\n", pretty.Sprint(r.Changes)))
	for _, err := range r.Errors {
		c.Assert(err, tc.Equals, "")
	}
}

func (s *bundleSuite) TestGetChangesMapArgsSuccessCharmHubRevision(c *tc.C) {
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
	r, err := s.facade.GetChangesMapArgs(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(r.Changes, tc.DeepEquals, []*params.BundleChangesMapArgs{{
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
		c.Assert(err, tc.Equals, "")
	}
}

func (s *bundleSuite) TestGetChangesMapArgsKubernetes(c *tc.C) {
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
	r, err := s.facade.GetChangesMapArgs(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(r.Changes, tc.DeepEquals, []*params.BundleChangesMapArgs{{
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
	}}, tc.Commentf("\nobtained: %s\n", pretty.Sprint(r.Changes)))
	for _, err := range r.Errors {
		c.Assert(err, tc.Equals, "")
	}
}

func (s *bundleSuite) TestGetChangesMapArgsBundleEndpointBindingsSuccess(c *tc.C) {
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
	r, err := s.facade.GetChangesMapArgs(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)

	for _, change := range r.Changes {
		if change.Method == "deploy" {
			c.Assert(change, tc.DeepEquals, &params.BundleChangesMapArgs{
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
