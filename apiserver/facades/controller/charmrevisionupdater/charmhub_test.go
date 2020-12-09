// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(benhoyt) - add test with some charm resources

package charmrevisionupdater_test

import (
	"context"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	statemocks "github.com/juju/juju/state/mocks"
	"github.com/juju/juju/testing"
)

type charmhubSuite struct{}

var _ = gc.Suite(&charmhubSuite{})

func newFakeCharmhubClient(st charmrevisionupdater.State, metadata map[string]string) (charmrevisionupdater.CharmhubRefreshClient, error) {
	charms := map[string]charmhubCharm{
		"charm-1": {
			id:       "charm-1",
			name:     "mysql",
			revision: 23,
			version:  "23",
		},
		"charm-2": {
			id:       "charm-2",
			name:     "postgresql",
			revision: 42,
			version:  "42",
		},
	}
	return &fakeCharmhubAPI{charms: charms, metadata: metadata}, nil
}

type charmhubCharm struct {
	id        string
	name      string
	revision  int
	resources []transport.ResourceRevision
	version   string
}

type fakeCharmhubAPI struct {
	charms   map[string]charmhubCharm
	metadata map[string]string
}

func (h *fakeCharmhubAPI) Refresh(_ context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error) {
	// Sanity check that metadata headers are present
	if h.metadata["model_uuid"] == "" {
		return nil, errors.Errorf("model metadata not present")
	}

	request, err := config.Build()
	if err != nil {
		return nil, err
	}
	responses := make([]transport.RefreshResponse, len(request.Context))
	for i, context := range request.Context {
		action := request.Actions[i]
		if action.Action != "refresh" {
			return nil, errors.Errorf("unexpected action %q", action.Action)
		}
		if *action.ID != context.ID {
			return nil, errors.Errorf("action ID %q doesn't match context ID %q", *action.ID, context.ID)
		}
		charm, ok := h.charms[context.ID]
		if !ok {
			return nil, errors.Errorf("charm ID %q not found", context.ID)
		}
		response := transport.RefreshResponse{
			Entity: transport.RefreshEntity{
				CreatedAt: time.Now(),
				ID:        context.ID,
				Name:      charm.name,
				Resources: charm.resources,
				Revision:  charm.revision,
				Version:   charm.version,
			},
			EffectiveChannel: context.TrackingChannel,
			ID:               context.ID,
			InstanceKey:      context.InstanceKey,
			Name:             charm.name,
			Result:           "refresh",
		}
		responses[i] = response
	}
	return responses, nil
}

func makeApplication(ctrl *gomock.Controller, schema, charmName, charmID, appID string, revision int) charmrevisionupdater.Application {
	source := "charm-hub"
	if schema == "cs" {
		source = "charm-store"
	}

	app := NewMockApplication(ctrl)
	app.EXPECT().CharmURL().Return(&charm.URL{
		Schema:   schema,
		Name:     charmName,
		Revision: revision,
	}, false).AnyTimes()
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
			Series:       "focal",
		},
	}).AnyTimes()
	app.EXPECT().Channel().Return(csparams.Channel("latest/stable")).AnyTimes()
	app.EXPECT().ApplicationTag().Return(names.ApplicationTag{Name: appID}).AnyTimes()

	return app
}

func makeModel(c *gc.C, ctrl *gomock.Controller) charmrevisionupdater.Model {
	model := NewMockModel(ctrl)
	model.EXPECT().CloudName().Return("testcloud").AnyTimes()
	model.EXPECT().CloudRegion().Return("juju-land").AnyTimes()
	uuid := testing.ModelTag.Id()
	cfg, err := config.New(true, map[string]interface{}{
		"charm-hub-url": "https://api.staging.snapcraft.io", // not actually used in tests
		"name":          "model",
		"type":          "type",
		"uuid":          uuid,
	})
	c.Assert(err, jc.ErrorIsNil)
	model.EXPECT().Config().Return(cfg, nil).AnyTimes()
	model.EXPECT().IsControllerModel().Return(false).AnyTimes()
	model.EXPECT().UUID().Return(uuid).AnyTimes()
	return model
}

func makeState(c *gc.C, ctrl *gomock.Controller, resources state.Resources) *MockState {
	state := NewMockState(ctrl)
	state.EXPECT().Cloud(gomock.Any()).Return(cloud.Cloud{Type: "cloud"}, nil).AnyTimes()
	state.EXPECT().ControllerUUID().Return("controller-1").AnyTimes()
	state.EXPECT().Model().Return(makeModel(c, ctrl), nil).AnyTimes()
	state.EXPECT().Resources().Return(resources, nil).AnyTimes()
	return state
}

func (s *charmhubSuite) TestUpdateRevisionsOutOfDate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	resources := statemocks.NewMockResources(ctrl)
	resources.EXPECT().SetCharmStoreResources(gomock.Any(), gomock.Len(0), gomock.Any()).Return(nil).AnyTimes()

	state := makeState(c, ctrl, resources)

	state.EXPECT().AllApplications().Return([]charmrevisionupdater.Application{
		makeApplication(ctrl, "ch", "mysql", "charm-1", "app-1", 22),
		makeApplication(ctrl, "ch", "postgresql", "charm-2", "app-2", 41),
	}, nil).AnyTimes()

	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("ch:mysql-23")).Return(nil)
	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("ch:postgresql-42")).Return(nil)

	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPIState(state, nil, newFakeCharmhubClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *charmhubSuite) TestUpdateRevisionsUpToDate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	resources := statemocks.NewMockResources(ctrl)
	resources.EXPECT().SetCharmStoreResources(gomock.Any(), gomock.Len(0), gomock.Any()).Return(nil).AnyTimes()

	state := makeState(c, ctrl, resources)

	state.EXPECT().AllApplications().Return([]charmrevisionupdater.Application{
		makeApplication(ctrl, "ch", "postgresql", "charm-2", "app-2", 42),
	}, nil).AnyTimes()

	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("ch:postgresql-42")).Return(nil)

	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPIState(state, nil, newFakeCharmhubClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}
