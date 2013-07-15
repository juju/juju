// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"time"

	"launchpad.net/tomb"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/downloader"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/version"
)

var upgraderKillDelay = 5 * time.Minute

// An Upgrader observes the version information for an agent in the
// environment state, and handles the downloading and unpacking of
// new versions of the juju tools when necessary.
//
// When a new version is available Wait and Stop return UpgradeReadyError.
type Upgrader struct {
	tomb       tomb.Tomb
	st         *state.State
	agentState AgentState
	dataDir    string
}

// UpgradeReadyError is returned by an Upgrader to report that
// an upgrade is ready to be performed and a restart is due.
type UpgradeReadyError struct {
	AgentName string
	OldTools  *state.Tools
	NewTools  *state.Tools
	DataDir   string
}

func (e *UpgradeReadyError) Error() string {
	return "must restart: an agent upgrade is available"
}

// ChangeAgentTools does the actual agent upgrade.
func (e *UpgradeReadyError) ChangeAgentTools() error {
	tools, err := agent.ChangeAgentTools(e.DataDir, e.AgentName, e.NewTools.Binary)
	if err != nil {
		return err
	}
	log.Infof("upgrader upgraded from %v to %v (%q)", e.OldTools.Binary, tools.Binary, tools.URL)
	return nil
}

// NewUpgrader returns a new Upgrader watching the given agent.
func NewUpgrader(st *state.State, agentState AgentState, dataDir string) *Upgrader {
	u := &Upgrader{
		st:         st,
		agentState: agentState,
		dataDir:    dataDir,
	}
	go func() {
		defer u.tomb.Done()
		u.tomb.Kill(u.run())
	}()
	return u
}

func (u *Upgrader) String() string {
	return "upgrader"
}

func (u *Upgrader) Kill() {
	u.tomb.Kill(nil)
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
	err = u.agentState.SetAgentTools(currentTools)
	if err != nil {
		return err
	}

	// TODO(fwereade): this whole package should be ignorant of environs,
	// so it shouldn't be watching environ config (and it shouldn't be
	// looking in storage): we should be able to find out what to download
	// from state, exactly as we do for charms.
	w := u.st.WatchEnvironConfig()
	defer watcher.Stop(w, &u.tomb)

	// Rather than using worker.WaitForEnviron, invalid environments are
	// managed explicitly so that all configuration changes are observed
	// by the loop below.
	var environ environs.Environ

	// TODO(rog) retry downloads when they fail.
	var (
		download      *downloader.Download
		downloadTools *state.Tools
		downloadDone  <-chan downloader.Status
	)
	// If we're killed early on (probably as a result of some other
	// task dying) we allow ourselves some time to try to connect to
	// the state and download a new version. We return to normal
	// undelayed behaviour when:
	// 1) We find there's no upgrade to do.
	// 2) A download fails.
	tomb := delayedTomb(&u.tomb, upgraderKillDelay)
	noDelay := func() {
		if tomb != &u.tomb {
			tomb.Kill(nil)
			tomb = &u.tomb
		}
	}
	for {
		// We wait for the tools to change while we're downloading
		// so that if something goes wrong (for instance a bad URL
		// hangs up) another change to the proposed tools can
		// potentially fix things.
		select {
		case cfg, ok := <-w.Changes():
			if !ok {
				return watcher.MustErr(w)
			}
			var err error
			if environ == nil {
				environ, err = environs.New(cfg)
				if err != nil {
					log.Errorf("upgrader loaded invalid initial environment configuration: %v", err)
					break
				}
			} else {
				err = environ.SetConfig(cfg)
				if err != nil {
					log.Warningf("upgrader loaded invalid environment configuration: %v", err)
					// continue on, because the version number is still significant.
				}
			}
			proposed, ok := cfg.AgentVersion()
			if !ok {
				// This shouldn't be possible; but if it happens it's no reason
				// to kill this task. Just wait for the config to change again.
				continue
			}
			if download != nil {
				// There's a download in progress, stop it if we need to.
				if downloadTools.Number == proposed {
					// We are already downloading the requested tools.
					break
				}
				// Tools changed. We need to stop and restart.
				download.Stop()
				download, downloadTools, downloadDone = nil, nil, nil
			}
			// TODO: major version upgrades.
			if proposed.Major != version.Current.Major {
				log.Errorf("major version upgrades are not supported yet")
				noDelay()
				break
			}
			if proposed == version.Current.Number {
				noDelay()
				break
			}
			required := version.Binary{
				Number: proposed,
				Series: version.Current.Series,
				Arch:   version.Current.Arch,
			}
			if tools, err := agent.ReadTools(u.dataDir, required); err == nil {
				// The exact tools have already been downloaded, so use them.
				return u.upgradeReady(currentTools, tools)
			}
			tools, err := environs.FindExactTools(environ, required)
			if err != nil {
				log.Errorf("upgrader error finding tools for %v: %v", required, err)
				if !errors.IsNotFoundError(err) {
					return err
				}
				noDelay()
				// TODO(rog): poll until tools become available.
				break
			}
			log.Infof("upgrader downloading %q", tools.URL)
			download = downloader.New(tools.URL, "")
			downloadTools = tools
			downloadDone = download.Done()
		case status := <-downloadDone:
			tools := downloadTools
			download, downloadTools, downloadDone = nil, nil, nil
			if status.Err != nil {
				log.Errorf("upgrader download of %v failed: %v", tools.Binary, status.Err)
				noDelay()
				break
			}
			err := agent.UnpackTools(u.dataDir, tools, status.File)
			status.File.Close()
			if err := os.Remove(status.File.Name()); err != nil {
				log.Warningf("upgrader cannot remove temporary download file: %v", err)
			}
			if err != nil {
				log.Errorf("upgrader cannot unpack %v tools: %v", tools.Binary, err)
				noDelay()
				break
			}
			return u.upgradeReady(currentTools, tools)
		case <-tomb.Dying():
			if download != nil {
				return fmt.Errorf("upgrader aborted download of %q", downloadTools.URL)
			}
			return nil
		}
	}
	panic("not reached")
}

func (u *Upgrader) upgradeReady(old, new *state.Tools) *UpgradeReadyError {
	return &UpgradeReadyError{
		AgentName: u.agentState.Tag(),
		OldTools:  old,
		DataDir:   u.dataDir,
		NewTools:  new,
	}
}

// delayedTomb returns a tomb that starts dying a given duration
// after t starts dying.
func delayedTomb(t *tomb.Tomb, d time.Duration) *tomb.Tomb {
	var delayed tomb.Tomb
	go func() {
		select {
		case <-t.Dying():
			time.Sleep(d)
			delayed.Kill(nil)
		case <-delayed.Dying():
			return
		}
	}()
	return &delayed
}
