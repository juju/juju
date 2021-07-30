// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/worker/caasfirewaller"
)

type fakeAPICaller struct {
	base.APICaller
}

type fakeBroker struct {
	caas.Broker
}

type fakeClient struct {
	caasfirewaller.Client
}

type mockServiceExposer struct {
	testing.Stub
	exposed   chan<- struct{}
	unexposed chan<- struct{}
}

func (m *mockServiceExposer) ExposeService(appName string, resourceTags map[string]string, config application.ConfigAttributes) error {
	m.MethodCall(m, "ExposeService", appName, resourceTags, config)
	m.exposed <- struct{}{}
	return m.NextErr()
}

func (m *mockServiceExposer) UnexposeService(appName string) error {
	m.MethodCall(m, "UnexposeService", appName)
	m.unexposed <- struct{}{}
	return m.NextErr()
}

type mockApplicationGetter struct {
	testing.Stub
	allWatcher *watchertest.MockStringsWatcher
	appWatcher *watchertest.MockNotifyWatcher
	exposed    bool
}

func (m *mockApplicationGetter) WatchApplications() (watcher.StringsWatcher, error) {
	m.MethodCall(m, "WatchApplications")
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.allWatcher, nil
}

func (m *mockApplicationGetter) WatchApplication(appName string) (watcher.NotifyWatcher, error) {
	m.MethodCall(m, "WatchApplication", appName)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.appWatcher, nil
}

func (m *mockApplicationGetter) IsExposed(appName string) (bool, error) {
	m.MethodCall(m, "IsExposed", appName)
	if err := m.NextErr(); err != nil {
		return false, err
	}
	return m.exposed, nil
}

func (a *mockApplicationGetter) ApplicationConfig(appName string) (application.ConfigAttributes, error) {
	a.MethodCall(a, "ApplicationConfig", appName)
	return application.ConfigAttributes{"juju-external-hostname": "exthost"}, a.NextErr()
}

type mockLifeGetter struct {
	testing.Stub
	life life.Value
}

func (m *mockLifeGetter) Life(entityName string) (life.Value, error) {
	m.MethodCall(m, "Life", entityName)
	if err := m.NextErr(); err != nil {
		return "", err
	}
	return m.life, nil
}

type mockCharmGetter struct {
	testing.Stub
	charmInfo *charms.CharmInfo
}

func (m *mockCharmGetter) ApplicationCharmInfo(appName string) (*charms.CharmInfo, error) {
	m.MethodCall(m, "ApplicationCharmInfo", appName)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	if m.charmInfo == nil {
		return nil, errors.NotFoundf("application %q", appName)
	}
	return m.charmInfo, nil
}
