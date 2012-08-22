package main

import (
	"fmt"
	"launchpad.net/juju-core/downloader"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/version"
	"launchpad.net/tomb"
	"os"
)

// An Upgrader observes the version information for an agent in the
// environment state, and handles the downloading and unpacking of
// new versions of the juju tools when necessary.
//
// When a new version is available Wait and Stop return UpgradedError.
type Upgrader struct {
	tomb       tomb.Tomb
	agentName  string
	agentState AgentState
}

// UpgradedError is returned by an Upgrader to report that
// an upgrade has been performed and a restart is due.
type UpgradedError struct {
	*state.Tools
}

func (e *UpgradedError) Error() string {
	return fmt.Sprintf("must restart: agent has been upgraded to %v (from %q)", e.Binary, e.URL)
}

// The AgentState interface is implemented by state types
// that represent running agents.
type AgentState interface {
	// SetAgentTools sets the tools that the agent is currently running.
	SetAgentTools(tools *state.Tools) error

	// WatchProposedAgentTools watches the tools that the agent is
	// currently proposed to run.
	WatchProposedAgentTools() *state.AgentToolsWatcher
}

// NewUpgrader returns a new Upgrader watching the given agent.
func NewUpgrader(agentName string, agentState AgentState) *Upgrader {
	u := &Upgrader{
		agentName:  agentName,
		agentState: agentState,
	}
	go func() {
		defer u.tomb.Done()
		u.tomb.Kill(u.run())
	}()
	return u
}

func (u *Upgrader) Stop() error {
	u.tomb.Kill(nil)
	return u.tomb.Wait()
}

func (u *Upgrader) Wait() error {
	return u.tomb.Wait()
}

func (u *Upgrader) run() error {
	// Let the state know the version that is currently running.
	currentTools, err := environs.ReadTools(version.Current)
	if err != nil {
		// Don't abort everything because we can't find the tools directory.
		// The problem should sort itself out as we will immediately
		// download some more tools and upgrade.
		log.Printf("upgrader: cannot read current tools: %v", err)
		currentTools = &state.Tools{
			Binary: version.Current,
		}
	}
	err = u.agentState.SetAgentTools(currentTools)
	if err != nil {
		return err
	}

	w := u.agentState.WatchProposedAgentTools()
	defer watcher.Stop(w, &u.tomb)

	// TODO(rog) retry downloads when they fail.
	var (
		download      *downloader.Download
		downloadTools *state.Tools
		downloadDone  <-chan downloader.Status
	)
	for {
		// We wait for the tools to change while we're downloading
		// so that if something goes wrong (for instance a bad URL
		// hangs up) another change to the proposed tools can
		// potentially fix things.
		select {
		case tools, ok := <-w.Changes():
			if !ok {
				return watcher.MustErr(w)
			}
			if download != nil {
				// There's a download in progress, stop it if we need to.
				if *tools == *downloadTools {
					// We are already downloading the requested tools.
					break
				}
				// Tools changed. We need to stop and restart.
				download.Stop()
				download, downloadTools, downloadDone = nil, nil, nil
			}
			// Ignore the proposed tools if they haven't been set yet
			// or we're already running the proposed version.
			if tools.URL == "" || *tools == *currentTools {
				break
			}
			if tools, err := environs.ReadTools(tools.Binary); err == nil {
				// The tools have already been downloaded, so use them.
				return &UpgradedError{tools}
			}
			download = downloader.New(tools.URL, "")
			downloadTools = tools
			downloadDone = download.Done()
		case status := <-downloadDone:
			tools := downloadTools
			download, downloadTools, downloadDone = nil, nil, nil
			if status.Err != nil {
				log.Printf("upgrader: download of %v failed: %v", tools.Binary, status.Err)
				break
			}
			err := environs.UnpackTools(tools, status.File)
			status.File.Close()
			if err := os.Remove(status.File.Name()); err != nil {
				log.Printf("upgrader: cannot remove temporary download file: %v", u.agentName, err)
			}
			if err != nil {
				log.Printf("upgrader: cannot unpack %v tools: %v", tools.Binary, err)
				break
			}
			return &UpgradedError{tools}
		case <-u.tomb.Dying():
			return nil
		}
	}
	panic("not reached")
}
