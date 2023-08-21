// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"github.com/juju/charm/v11"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/caasoperator"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	_ "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

type mockState struct {
	testing.Stub
	common.APIAddressAccessor
	entities map[string]state.Entity
	app      mockApplication
	unit     mockUnit
	model    mockModel
}

func newMockState() *mockState {
	unitsChanges := make(chan []string, 1)
	appChanges := make(chan struct{}, 1)
	st := &mockState{
		entities: make(map[string]state.Entity),
		app: mockApplication{
			name: "gitlab",
			life: state.Alive,
			charm: mockCharm{
				url:    charm.MustParseURL("ch:gitlab-1"),
				sha256: "fake-sha256",
			},
			unitsChanges: unitsChanges,
			appChanges:   appChanges,
			unitsWatcher: statetesting.NewMockStringsWatcher(unitsChanges),
			watcher:      statetesting.NewMockNotifyWatcher(appChanges),
		},
		unit: mockUnit{
			life: state.Dying,
		},
	}
	st.entities[st.app.Tag().String()] = &st.app
	st.entities[st.unit.Tag().String()] = &st.unit
	return st
}

func (st *mockState) APIHostPortsForAgents() ([]network.SpaceHostPorts, error) {
	st.MethodCall(st, "APIHostPortsForAgents")
	return nil, nil
}

func (st *mockState) WatchAPIHostPortsForAgents() state.NotifyWatcher {
	st.MethodCall(st, "WatchAPIHostPortsForAgents")
	return apiservertesting.NewFakeNotifyWatcher()
}

func (st *mockState) ModelUUID() string {
	st.MethodCall(st, "ModelUUID")
	return coretesting.ModelTag.Id()
}

func (st *mockState) Application(id string) (caasoperator.Application, error) {
	st.MethodCall(st, "Application", id)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return &st.app, nil
}

func (st *mockState) ApplicationExists(id string) error {
	st.MethodCall(st, "ApplicationExists", id)
	if err := st.NextErr(); err != nil {
		return err
	}
	if st.app.name != id {
		return errors.NotFoundf("application %q", id)
	}
	return nil
}

func (st *mockState) Model() (caasoperator.Model, error) {
	st.MethodCall(st, "Model")
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return &st.model, nil
}

func (st *mockState) FindEntity(tag names.Tag) (state.Entity, error) {
	st.MethodCall(st, "FindEntity", tag)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	entity, ok := st.entities[tag.String()]
	if !ok {
		return nil, errors.NotFoundf("%s", names.ReadableString(tag))
	}
	return entity, nil
}

type mockModel struct {
	testing.Stub
	containers []state.CloudContainer
}

func (m *mockModel) SetPodSpec(token leadership.Token, tag names.ApplicationTag, spec *string) error {
	m.MethodCall(m, "SetPodSpec", token, tag, *spec)
	return m.NextErr()
}

func (st *mockModel) Name() string {
	return "some-model"
}

func (st *mockModel) UUID() string {
	return "deadbeef"
}

func (st *mockModel) Type() state.ModelType {
	return state.ModelTypeIAAS
}

func (st *mockModel) ModelConfig() (*config.Config, error) {
	cfg := coretesting.FakeConfig()
	attr := cfg.Merge(coretesting.Attrs{"type": "kubernetes"})
	return config.New(config.NoDefaults, attr)
}

func (st *mockModel) Containers(providerIds ...string) ([]state.CloudContainer, error) {
	st.MethodCall(st, "Containers", providerIds)
	return st.containers, st.NextErr()
}

type mockApplication struct {
	testing.Stub
	name         string
	life         state.Life
	charm        mockCharm
	forceUpgrade bool
	unitsChanges chan []string
	unitsWatcher *statetesting.MockStringsWatcher
	appChanges   chan struct{}
	watcher      *statetesting.MockNotifyWatcher
}

func (*mockApplication) Tag() names.Tag {
	return names.NewApplicationTag("gitlab")
}

func (a *mockApplication) Life() state.Life {
	a.MethodCall(a, "Life")
	return a.life
}

func (a *mockApplication) SetOperatorStatus(info status.StatusInfo) error {
	a.MethodCall(a, "SetOperatorStatus", info)
	return a.NextErr()
}

func (a *mockApplication) Charm() (caasoperator.Charm, bool, error) {
	a.MethodCall(a, "Charm")
	if err := a.NextErr(); err != nil {
		return nil, false, err
	}
	return &a.charm, a.forceUpgrade, nil
}

func (a *mockApplication) CharmModifiedVersion() int {
	a.MethodCall(a, "CharmModifiedVersion")
	return 666
}

func (a *mockApplication) WatchUnits() state.StringsWatcher {
	a.MethodCall(a, "WatchUnits")
	return a.unitsWatcher
}

func (a *mockApplication) Watch() state.NotifyWatcher {
	a.MethodCall(a, "Watch")
	return a.watcher
}

func (a *mockApplication) AgentTools() (*tools.Tools, error) {
	return nil, errors.NotImplementedf("AgentTools")
}

func (a *mockApplication) SetAgentVersion(vers version.Binary) error {
	a.MethodCall(a, "SetAgentVersion", vers)
	return nil
}

type mockUnit struct {
	testing.Stub
	life state.Life
}

func (*mockUnit) Tag() names.Tag {
	return names.NewUnitTag("gitlab/0")
}

func (u *mockUnit) Life() state.Life {
	u.MethodCall(u, "Life")
	return u.life
}

func (u *mockUnit) Remove() error {
	u.MethodCall(u, "Remove")
	return nil
}

func (u *mockUnit) EnsureDead() error {
	u.MethodCall(u, "EnsureDead")
	return nil
}

type mockCharm struct {
	url    *charm.URL
	sha256 string
}

func (ch *mockCharm) URL() *charm.URL {
	return ch.url
}

func (ch *mockCharm) String() string {
	return ch.url.String()
}

func (ch *mockCharm) BundleSha256() string {
	return ch.sha256
}

func (ch *mockCharm) Meta() *charm.Meta {
	return &charm.Meta{Deployment: &charm.Deployment{DeploymentMode: charm.ModeOperator}}
}

type mockBroker struct {
	testing.Stub
	watcher corewatcher.StringsWatcher
}

func (b *mockBroker) WatchContainerStart(appName string, containerName string) (corewatcher.StringsWatcher, error) {
	b.MethodCall(b, "WatchContainerStart", appName, containerName)
	return b.watcher, b.NextErr()
}

type mockCloudContainer struct {
	unit       string
	providerID string
}

func (cc *mockCloudContainer) Unit() string {
	return cc.unit
}

func (cc *mockCloudContainer) ProviderId() string {
	return cc.providerID
}

func (cc *mockCloudContainer) Address() *network.SpaceAddress {
	return nil
}

func (cc *mockCloudContainer) Ports() []string {
	return nil
}

type mockLeadershipRevoker struct {
	revoked set.Strings
}

func (s *mockLeadershipRevoker) RevokeLeadership(applicationId, unitId string) error {
	s.revoked.Add(unitId)
	return nil
}
