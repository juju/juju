// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

import (
	"gopkg.in/tomb.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
)

type mockExternalControllers struct {
	state.ExternalControllers
	controllers []*mockExternalController
	watcher     *mockStringsWatcher
}

func (m *mockExternalControllers) Watch() state.StringsWatcher {
	return m.watcher
}

func (m *mockExternalControllers) Controller(uuid string) (state.ExternalController, error) {
	for _, c := range m.controllers {
		if c.id == uuid {
			return c, nil
		}
	}
	return nil, errors.NotFoundf("external controller %q", uuid)
}

func (m *mockExternalControllers) Save(info crossmodel.ControllerInfo, _ ...string) (state.ExternalController, error) {
	for _, c := range m.controllers {
		if c.id == info.ControllerTag.Id() {
			c.info = info
			return c, nil
		}
	}
	c := &mockExternalController{
		id:   info.ControllerTag.Id(),
		info: info,
	}
	m.controllers = append(m.controllers, c)
	return c, nil
}

type mockExternalController struct {
	id   string
	info crossmodel.ControllerInfo
}

func (c *mockExternalController) Id() string {
	return c.id
}

func (c *mockExternalController) ControllerInfo() crossmodel.ControllerInfo {
	return c.info
}

type mockStringsWatcher struct {
	tomb    tomb.Tomb
	changes chan []string
}

func newMockStringsWatcher() *mockStringsWatcher {
	w := &mockStringsWatcher{changes: make(chan []string, 1)}
	go w.loop()
	return w
}

func (w *mockStringsWatcher) loop() {
	defer w.tomb.Done()
	<-w.tomb.Dying()
}

func (w *mockStringsWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *mockStringsWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *mockStringsWatcher) Kill() {
	w.tomb.Kill(nil)
}

func (w *mockStringsWatcher) Err() error {
	return w.tomb.Err()
}

func (w *mockStringsWatcher) Changes() <-chan []string {
	return w.changes
}
