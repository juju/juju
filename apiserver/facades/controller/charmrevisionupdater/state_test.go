// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater_test

import (
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/charm/v8/resource"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/collections/set"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

type mockState struct {
	resources         state.Resources
	charmPlaceholders set.Strings
	applications      []charmrevisionupdater.Application
}

func newMockState(resources state.Resources) *mockState {
	return &mockState{
		resources:         resources,
		charmPlaceholders: set.NewStrings(),
	}
}

func (m *mockState) addApplication(schema, charmName, charmID, appID string, revision int) {
	source := "charm-hub"
	if schema == "cs" {
		source = "charm-store"
	}
	a := &mockApplication{
		curl: &charm.URL{
			Schema:   schema,
			Name:     charmName,
			Revision: revision,
		},
		origin: &state.CharmOrigin{
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
		},
		channel: "latest/stable",
		id:      appID,
	}
	m.applications = append(m.applications, a)
}

func (m *mockState) AddCharmPlaceholder(curl *charm.URL) error {
	m.charmPlaceholders.Add(curl.String())
	return nil
}

func (m *mockState) AllApplications() ([]charmrevisionupdater.Application, error) {
	return m.applications, nil
}

func (m *mockState) Charm(_ *charm.URL) (*state.Charm, error) {
	panic("not implemented") // not required for tests
}

func (m *mockState) Cloud(_ string) (cloud.Cloud, error) {
	return cloud.Cloud{Type: "cloud"}, nil
}

func (m *mockState) ControllerConfig() (controller.Config, error) {
	panic("not implemented") // not required for tests
}

func (m *mockState) ControllerUUID() string {
	return "controller-1"
}

func (m *mockState) Model() (charmrevisionupdater.Model, error) {
	return &mockModel{}, nil
}

func (m *mockState) Resources() (state.Resources, error) {
	return m.resources, nil
}

type mockModel struct{}

func (m *mockModel) CloudName() string {
	return "testcloud"
}

func (m *mockModel) CloudRegion() string {
	return "juju-land"
}

func (m *mockModel) Config() (*config.Config, error) {
	return config.New(false, map[string]interface{}{
		"charm-hub-url": "https://api.staging.snapcraft.io", // not actually used in tests
	})
}

func (m *mockModel) IsControllerModel() bool {
	return false
}

func (m *mockModel) UUID() string {
	return "model-1"
}

type mockResources struct {
	state.Resources
	resources map[string][]resource.Resource
}

func newMockResources() *mockResources {
	return &mockResources{
		resources: make(map[string][]resource.Resource),
	}
}

func (m *mockResources) SetCharmStoreResources(applicationID string, info []resource.Resource, _ time.Time) error {
	m.resources[applicationID] = info
	return nil
}

type mockApplication struct {
	curl    *charm.URL
	origin  *state.CharmOrigin
	channel string
	id      string
}

func (m *mockApplication) CharmURL() (curl *charm.URL, force bool) {
	return m.curl, false
}

func (m *mockApplication) CharmOrigin() *state.CharmOrigin {
	return m.origin
}

func (m *mockApplication) Channel() csparams.Channel {
	return csparams.Channel(m.channel)
}

func (m *mockApplication) ApplicationTag() names.ApplicationTag {
	return names.ApplicationTag{Name: m.id}
}
