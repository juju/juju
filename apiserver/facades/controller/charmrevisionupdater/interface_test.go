// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater_test

import (
	"github.com/juju/charm/v12"
	"github.com/juju/charm/v12/resource"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater/mocks"
	"github.com/juju/juju/charmhub"
	charmmetrics "github.com/juju/juju/core/charm/metrics"
	"github.com/juju/juju/state"
)

func makeApplication(ctrl *gomock.Controller, schema, charmName, charmID, appID string, revision int) charmrevisionupdater.Application {
	app := mocks.NewMockApplication(ctrl)

	var source string
	switch schema {
	case "ch":
		source = "charm-hub"
		app.EXPECT().UnitCount().Return(2).AnyTimes()
	}

	curl := &charm.URL{
		Schema:   schema,
		Name:     charmName,
		Revision: revision,
	}
	str := curl.String()
	app.EXPECT().CharmURL().Return(&str, false).AnyTimes()
	app.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Source:   source,
		Type:     "charm",
		ID:       charmID,
		Revision: &revision,
		Channel: &state.Channel{
			Track: "latest",
			Risk:  "stable",
		},
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "20.04/stable",
		},
	}).AnyTimes()
	app.EXPECT().ApplicationTag().Return(names.ApplicationTag{Name: appID}).AnyTimes()

	return app
}

func makeResource(c *gc.C, name string, revision, size int, hexFingerprint string) resource.Resource {
	fingerprint, err := resource.ParseFingerprint(hexFingerprint)
	c.Assert(err, jc.ErrorIsNil)
	return resource.Resource{
		Meta: resource.Meta{
			Name: name,
			Type: resource.TypeFile,
		},
		Origin:      resource.OriginStore,
		Revision:    revision,
		Fingerprint: fingerprint,
		Size:        int64(size),
	}
}

// charmhubConfigMatcher matches only the charm IDs and revisions of a
// charmhub.RefreshMany config.
type charmhubConfigMatcher struct {
	expected []charmhubConfigExpected
}

type charmhubConfigExpected struct {
	id        string
	revision  int
	relMetric string
}

func (m charmhubConfigMatcher) Matches(x interface{}) bool {
	config, ok := x.(charmhub.RefreshConfig)
	if !ok {
		return false
	}
	request, err := config.Build()
	if err != nil {
		return false
	}
	if len(request.Context) != len(m.expected) || len(request.Actions) != len(m.expected) {
		return false
	}
	for i, context := range request.Context {
		if context.ID != m.expected[i].id {
			return false
		}
		if context.Revision != m.expected[i].revision {
			return false
		}
		r, ok := context.Metrics[string(charmmetrics.Relations)]
		if m.expected[i].relMetric != "" && !ok {
			return false
		}
		if m.expected[i].relMetric != "" && ok && r != m.expected[i].relMetric {
			return false
		}
		action := request.Actions[i]
		if *action.ID != m.expected[i].id {
			return false
		}
		_, ok = context.Metrics[string(charmmetrics.NumUnits)]
		if m.expected[i].relMetric != "" && !ok {
			return false
		}
	}
	return true
}

func (charmhubConfigMatcher) String() string {
	return "matches config"
}

// charmhubMetricsMatcher matches the controller and model parts of the metrics, then
// a value within each part.
type charmhubMetricsMatcher struct {
	c     *gc.C
	exist bool
}

func (m charmhubMetricsMatcher) Matches(x interface{}) bool {
	switch y := x.(type) {
	case map[charmmetrics.MetricKey]map[charmmetrics.MetricKey]string:
		if len(y) != 2 {
			if !m.exist {
				return true
			}
			return false
		}
		for k := range y {
			if k != charmmetrics.Controller && k != charmmetrics.Model {
				return false
			}
		}
		controller := y[charmmetrics.Controller]
		uuid, ok := controller[charmmetrics.UUID]
		if !ok {
			return false
		}
		m.c.Assert(uuid, gc.Equals, "controller-1")

		model := y[charmmetrics.Model]
		cloud, ok := model[charmmetrics.Cloud]
		if !ok {
			return false
		}
		m.c.Assert(cloud, gc.Equals, "cloud")
	default:
		return false
	}
	return true
}

func (charmhubMetricsMatcher) String() string {
	return "matches metrics"
}

type facadeContextShim struct {
	facade.Context // Make it fulfil the interface, but we only define a couple of methods
	state          *state.State
	authorizer     facade.Authorizer
}

func (s facadeContextShim) Auth() facade.Authorizer {
	return s.authorizer
}

func (s facadeContextShim) State() *state.State {
	return s.state
}
