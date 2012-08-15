package main
import (
	"fmt"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/tomb"
	"launchpad.net/juju-core/downloader"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

type Upgrader struct {
	tomb tomb.Tomb
	agentName string
	agentState AgentState
}

type UpgradedError struct {
	tools *state.Tools
}

func (e *UpgradedError) Error() string {
	return fmt.Sprintf("agent has been upgraded to %v (from %q)", e.tools.Binary, e.tools.URL)
}

type AgentState interface {
	SetAgentTools(tools *state.Tools) error
	WatchProposedAgentTools() *state.AgentToolsWatcher
}

// NewUpgrader watches the given agent state and attempts to upgrade the
// tools for the agent with the given name. If it is successful, Wait
// will return an UpgradedError describing the new tools.
func NewUpgrader(agentName string, as AgentState) *Upgrader {
	u := &Upgrader{
		agentName: agentName,
		agentState: as,
	}
	go func() {
		defer u.tomb.Done()
		if err := u.run(); err != nil {
			u.tomb.Kill(err)
		}
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
		log.Printf("cannot read tools directory: %v", err)
		currentTools = &state.Tools{
			Binary: version.Current,
		}
	}
	err = u.agentState.SetAgentTools(currentTools)
	if err != nil {
		return err
	}
	
	w := u.agentState.WatchProposedAgentTools()

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
		case tools := <-w.Changes():
			// If there's a download in progress, stop it if we need to.
			if download != nil {
				// If we are already downloading the requested tools,
				// continue to do so.
				if *tools == *downloadTools {
					break
				}
				download.Stop()
				download, downloadTools, downloadDone = nil, nil, nil
			}
			// There's no need to download anything if we're already running
			// the proposed version.
			if *tools == *currentTools {
				break
			}
			// It's possible the tools are already downloaded - attempt
			// an upgrade without downloading to check if that's the case.
			if tools, err := environs.UpgradeTools(u.agentName, tools.Binary); err == nil {
				return &upgradedError{tools}
			}
			download = downloader.New(tools.URL)
			downloadTools = tools
			downloadDone = download.Done()
		case status := <-downloadDone:
			tools := downloadTools
			download, downloadTools, downloadDone = nil, nil, nil
			if status.Err != nil {
				log.Printf("download %q failed: %v", tools.Binary, tools.URL)
				break
			}
			err := environs.UnpackTools(tools, status.File)
			status.File.Close()
			if err := os.Remove(status.File.Name()); err != nil {
				log.Printf("%s agent: cannot remove temporary download file: %v", err)
			}
			if err != nil {
				log.Printf("unpack error: %v", err)
				break
			}
			tools, err = environs.UpgradeTools(u.agentName, tools.Binary)
			if err != nil {
				log.Printf("upgrade error: %v", err)
				break
			}
			return &upgradedError{tools}
		case <-u.tomb.Dying():
			return nil
		}
	}
	panic("not reached")
}
