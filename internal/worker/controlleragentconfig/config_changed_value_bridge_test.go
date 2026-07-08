// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlleragentconfig

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/utils/v4/voyeur"
	dependencytesting "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"
)

type configChangedValueBridgeSuite struct{}

func TestConfigChangedValueBridgeSuite(t *stdtesting.T) {
	tc.Run(t, &configChangedValueBridgeSuite{})
}

func (s *configChangedValueBridgeSuite) TestBridgeSetsVoyeurValueOnReload(c *tc.C) {
	configChanged := voyeur.NewValue(false)
	watcher := configChanged.Watch()
	defer watcher.Close()

	configWatcher := newStubConfigWatcher()
	w := NewConfigChangedValueBridge(configWatcher, configChanged)
	defer workertest.CleanKill(c, w)

	configWatcher.dispatch()
	c.Assert(watcher.Next(), tc.IsTrue)
}

func (s *configChangedValueBridgeSuite) TestBridgeUnsubscribesOnStop(c *tc.C) {
	configWatcher := newStubConfigWatcher()
	w := NewConfigChangedValueBridge(configWatcher, voyeur.NewValue(false))

	workertest.CleanKill(c, w)
	select {
	case <-configWatcher.unsubscribed:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for config watcher unsubscribe")
	}
}

func (s *configChangedValueBridgeSuite) TestManifoldInputs(c *tc.C) {
	manifold := ConfigChangedValueBridgeManifold(ConfigChangedValueBridgeManifoldConfig{
		ControllerAgentConfigName: "controller-agent-config",
		ConfigChangedValue:        voyeur.NewValue(false),
	})
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"controller-agent-config"})
}

func (s *configChangedValueBridgeSuite) TestManifoldStart(c *tc.C) {
	configWatcher := newStubConfigWatcher()
	configChanged := voyeur.NewValue(false)
	watcher := configChanged.Watch()
	defer watcher.Close()

	manifold := ConfigChangedValueBridgeManifold(ConfigChangedValueBridgeManifoldConfig{
		ControllerAgentConfigName: "controller-agent-config",
		ConfigChangedValue:        configChanged,
	})
	w, err := manifold.Start(c.Context(), dependencytesting.StubGetter(map[string]any{
		"controller-agent-config": ConfigWatcher(configWatcher),
	}))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	configWatcher.dispatch()
	c.Assert(watcher.Next(), tc.IsTrue)
}

type stubConfigWatcher struct {
	changes      chan struct{}
	done         chan struct{}
	unsubscribed chan struct{}
}

func newStubConfigWatcher() *stubConfigWatcher {
	return &stubConfigWatcher{
		changes:      make(chan struct{}, 1),
		done:         make(chan struct{}),
		unsubscribed: make(chan struct{}),
	}
}

func (w *stubConfigWatcher) Changes() <-chan struct{} {
	return w.changes
}

func (w *stubConfigWatcher) Done() <-chan struct{} {
	return w.done
}

func (w *stubConfigWatcher) Unsubscribe() {
	select {
	case <-w.unsubscribed:
	default:
		close(w.unsubscribed)
	}
	select {
	case <-w.done:
	default:
		close(w.done)
	}
}

func (w *stubConfigWatcher) dispatch() {
	w.changes <- struct{}{}
}
