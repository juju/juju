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

// UpgradeReadyError is returned by an Upgrader to report that
// an upgrade is ready to be performed and a restart is due.
type UpgradeReadyError struct {
	AgentName string
	OldTools  *state.Tools
	NewTools  *state.Tools
	DataDir   string
}

var logger = loggo.GetLogger("juju.upgrader")

type Upgrader struct {
	tomb    tomb.Tomb
	st *upgrader.State
	dataDir string
	tag string
}

// NewUpgrader returns a new upgrader worker. It watches changes to the
// current version and tries to download the tools for any new version.
// If an upgrade is needed, the worker will exit with an
// UpgradeReadyError holding details of the requested upgrade. The tools
// will have been downloaded and unpacked.
func NewUpgrader(st *upgrader.State, dataDir, tag string) *Upgrader {
	u := &Upgrader{
		st: st,
		dataDir: dataDir,
		tag: tag,
	}
	go func() {
		defer u.tomb.Done()
		u.tomb.Kill(u.loop())
	}()
	return u
}

// Kill implements worker.Worker.Kill.
func (u *UpgradeWorker) Kill() {
	u.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (u *UpgradeWorker) Wait() error {
	return u.tomb.Wait()
}

func (u *UpgradeWorker) Stop() error {
	u.Kill(nil)
	return u.Wait()
}

func (u *UpgradeWorker) loop() error {
	currentTools, err := agent.ReadTools(u.dataDir, version.Current)
	if err != nil {
		// Don't abort everything because we can't find the tools directory.
		// The problem should sort itself out as we will immediately
		// download some more tools and upgrade.
		log.Warningf("upgrader cannot read current tools: %v", err)
		currentTools = &state.Tools{
			Binary: version.Current,
		}
	}
	err = u.st.SetTools(currentTools)
	if err != nil {
		return err
	}
	versionWatcher, err := u.st.WatchAPIVersion(u.tag)
	if err != nil {
		return err
	}
	changes := versionWatcher.Changes()
	if _, ok := <-changes; !ok {
		return watcher.MustErr(versionWatcher)
	}
	wantTools, err := st.Tools(u.tag)
	if err != nil {
		return err
	}
	var retry <-chan time.Time
	for {
		if wantTools.Number != currentTools.Number {
			// The worker cannot be stopped while we're downloading
			// the tools - this means that even if the API is going down
			// repeatedly (causing the agent to be stopped), as long
			// as we have got as far as this, we will still be able to
			// upgrade the agent.
			err := fetchTools(wantTools)
			if err != nil {
				if err, ok := err.(*UpgradeReadyError); ok {
					// fill in information that fetchTools doesn't have.
					err.OldTools = currentTools
					err.AgentName = u.tag
					err.DataDir = u.dataDir
					return err
				}
				logger.Errorf("failed to fetch tools: %v", err)
				retry = time.After(retryDelay)
				continue
			}
		}
		select {
		case <-retry:
		case <-changes:
		case <-u.tomb.Dying():
			return nil
		}
	}
}

func fetchTools(tools *agent.Tools) error {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad http response: %v", resp.Status)
	}
	err = agent.UnpackTools(u.dataDir, tools, resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot unpack tools: %v", err)
	}
	return &UpgradeReadyError{
		NewTools: tools,
	}
}
