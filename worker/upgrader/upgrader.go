// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"fmt"

	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/upgrader"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker"
)

var logger = loggo.GetLogger("juju.upgrader")

type UpgradeWorker struct {
	tomb    tomb.Tomb
	handler *upgradeHandler
}

// Kill the loop with no-error
func (uw *UpgradeWorker) Kill() {
	uw.tomb.Kill(nil)
}

// Stop kils the worker and waits for it to exit
func (uw *UpgradeWorker) Stop() error {
	uw.tomb.Kill(nil)
	return uw.tomb.Wait()
}

// Wait for the looping to finish
func (uw *UpgradeWorker) Wait() error {
	return uw.tomb.Wait()
}

// String returns a nice description of this worker, taken from the underlying handler
func (uw *UpgradeWorker) String() string {
	return uw.handler.String()
}

// TearDown the handler, but ensure any error is propagated
func handlerTearDown(handler worker.WatchHandler, t *tomb.Tomb) {
	if err := handler.TearDown(); err != nil {
		t.Kill(err)
	}
}

func (uw *UpgradeWorker) loop() error {
	var w api.NotifyWatcher
	var err error
	defer handlerTearDown(uw.handler, &uw.tomb)
	if w, err = uw.handler.SetUp(); err != nil {
		if w != nil {
			// We don't bother to propogate an error, because we
			// already have an error
			w.Stop()
		}
		return err
	}
	defer watcher.Stop(w, &uw.tomb)
	for {
		select {
		case <-uw.tomb.Dying():
			return tomb.ErrDying
		case _, ok := <-w.Changes():
			if !ok {
				return watcher.MustErr(w)
			}
			if err := uw.handler.Handle(); err != nil {
				return err
			}
		case <-uw.handler.DownloadChannel():
			continue
		}
	}
	panic("unreachable")
}

func NewUpgrader(st *api.State, agentTag string, toolManager agent.ToolsManager) worker.NotifyWorker {
	uw := &UpgradeWorker{
		handler: &upgradeHandler{
			apiState:    st,
			agentTag:    agentTag,
			toolManager: toolManager,
		},
	}
	go func() {
		defer uw.tomb.Done()
		uw.tomb.Kill(uw.loop())
	}()
	return uw
}

type upgradeHandler struct {
	apiState    *api.State
	apiUpgrader *upgrader.Upgrader
	agentTag    string
	toolManager agent.ToolsManager
}

func (u *upgradeHandler) String() string {
	return fmt.Sprintf("upgrader for %q", u.agentTag)
}

func (u *upgradeHandler) SetUp() (api.NotifyWatcher, error) {
	// First thing we do is alert the API of our current tools
	u.apiUpgrader = u.apiState.Upgrader()
	cur := version.Current
	err := u.apiUpgrader.SetTools(params.AgentTools{
		Tag:    u.agentTag,
		Major:  cur.Major,
		Minor:  cur.Minor,
		Patch:  cur.Patch,
		Build:  cur.Build,
		Arch:   cur.Arch,
		Series: cur.Series,
		URL:    "",
	})
	if err != nil {
		return nil, err
	}
	return u.apiUpgrader.WatchAPIVersion(u.agentTag)
}

func (u *upgradeHandler) TearDown() error {
	u.apiUpgrader = nil
	u.apiState = nil
	return nil
}

func (u *upgradeHandler) Handle() error {
	_, err := u.apiUpgrader.Tools(u.agentTag)
	if err != nil {
		return err
	}
	return nil
}

// DownloadChannel will signal to indicate the download has completed
func (u *upgradeHandler) DownloadChannel() chan struct{} {
	return nil
}

// DownloadHandle should be called when download completes
func (u *upgradeHandler) DownloadHandle() error {
	return nil
}
