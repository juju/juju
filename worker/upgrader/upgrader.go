// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"fmt"
	"net/http"
	"time"

	"launchpad.net/loggo"
	"launchpad.net/tomb"

	agenttools "launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/state/api/upgrader"
	"launchpad.net/juju-core/state/watcher"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

// retryAfter returns a channel that receives a value
// when a failed download should be retried.
var retryAfter = func() <-chan time.Time {
	return time.After(5 * time.Second)
}

// UpgradeReadyError is returned by an Upgrader to report that
// an upgrade is ready to be performed and a restart is due.
type UpgradeReadyError struct {
	AgentName string
	OldTools  *coretools.Tools
	NewTools  *coretools.Tools
	DataDir   string
}

func (e *UpgradeReadyError) Error() string {
	return "must restart: an agent upgrade is available"
}

// ChangeAgentTools does the actual agent upgrade.
// It should be called just before an agent exits, so that
// it will restart running the new tools.
func (e *UpgradeReadyError) ChangeAgentTools() error {
	tools, err := agenttools.ChangeAgentTools(e.DataDir, e.AgentName, e.NewTools.Version)
	if err != nil {
		return err
	}
	logger.Infof("upgraded from %v to %v (%q)", e.OldTools.Version, tools.Version, tools.URL)
	return nil
}

var logger = loggo.GetLogger("juju.worker.upgrader")

// Upgrader represents a worker that watches the state for upgrade
// requests.
type Upgrader struct {
	tomb    tomb.Tomb
	st      *upgrader.State
	dataDir string
	tag     string
}

// New returns a new upgrader worker. It watches changes to the
// current version of the current agent (with the given tag)
// and tries to download the tools for any new version
// into the given data directory.
// If an upgrade is needed, the worker will exit with an
// UpgradeReadyError holding details of the requested upgrade. The tools
// will have been downloaded and unpacked.
func New(st *upgrader.State, tag, dataDir string) *Upgrader {
	u := &Upgrader{
		st:      st,
		dataDir: dataDir,
		tag:     tag,
	}
	go func() {
		defer u.tomb.Done()
		u.tomb.Kill(u.loop())
	}()
	return u
}

// Kill implements worker.Worker.Kill.
func (u *Upgrader) Kill() {
	u.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (u *Upgrader) Wait() error {
	return u.tomb.Wait()
}

// Stop stops the upgrader and returns any
// error it encountered when running.
func (u *Upgrader) Stop() error {
	u.Kill()
	return u.Wait()
}

func (u *Upgrader) loop() error {
	currentTools, err := agenttools.ReadTools(u.dataDir, version.Current)
	if err != nil {
		// Don't abort everything because we can't find the tools directory.
		// The problem should sort itself out as we will immediately
		// download some more tools and upgrade.
		logger.Warningf("cannot read current tools: %v", err)
		currentTools = &coretools.Tools{
			Version: version.Current,
		}
	}
	err = u.st.SetTools(u.tag, currentTools)
	if err != nil {
		return err
	}
	versionWatcher, err := u.st.WatchAPIVersion(u.tag)
	if err != nil {
		return err
	}
	changes := versionWatcher.Changes()
	defer watcher.Stop(versionWatcher, &u.tomb)
	var retry <-chan time.Time
	// We don't read on the dying channel until we have received the
	// initial event from the API version watcher, thus ensuring
	// that we attempt an upgrade even if other workers are dying
	// all around us.
	var dying <-chan struct{}
	var wantTools *coretools.Tools
	for {
		select {
		case _, ok := <-changes:
			if !ok {
				return watcher.MustErr(versionWatcher)
			}
			wantTools, err = u.st.Tools(u.tag)
			if err != nil {
				return err
			}
			logger.Infof("required tools: %v", wantTools.Version)
			dying = u.tomb.Dying()
		case <-retry:
		case <-dying:
			return nil
		}
		if wantTools.Version.Number != currentTools.Version.Number {
			logger.Infof("upgrade required from %v to %v", currentTools.Version, wantTools.Version)
			// The worker cannot be stopped while we're downloading
			// the tools - this means that even if the API is going down
			// repeatedly (causing the agent to be stopped), as long
			// as we have got as far as this, we will still be able to
			// upgrade the agent.
			err := u.fetchTools(wantTools)
			if err == nil {
				return &UpgradeReadyError{
					OldTools:  currentTools,
					NewTools:  wantTools,
					AgentName: u.tag,
					DataDir:   u.dataDir,
				}
			}
			logger.Errorf("failed to fetch tools from %q: %v", wantTools.URL, err)
			retry = retryAfter()
		}
	}
}

func (u *Upgrader) fetchTools(agentTools *coretools.Tools) error {
	logger.Infof("fetching tools from %q", agentTools.URL)
	resp, err := http.Get(agentTools.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad HTTP response: %v", resp.Status)
	}
	err = agenttools.UnpackTools(u.dataDir, agentTools, resp.Body)
	if err != nil {
		return fmt.Errorf("cannot unpack tools: %v", err)
	}
	return nil
}
