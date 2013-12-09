// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	stdssh "code.google.com/p/go.crypto/ssh"

	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/api/keyupdater"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/utils/ssh"
	"launchpad.net/juju-core/worker"
)

var logger = loggo.GetLogger("juju.worker.authenticationworker")

type keyupdaterWorker struct {
	st       *keyupdater.State
	tomb     tomb.Tomb
	tag      string
	jujuKeys set.Strings
}

// NewWorker returns a worker that keeps track of
// the machine's authorised ssh keys and ensures the
// ~/.ssh/authorized_keys file is up to date.
func NewWorker(st *keyupdater.State, agentConfig agent.Config) worker.Worker {
	cw := &keyupdaterWorker{st: st, tag: agentConfig.Tag()}
	return worker.NewNotifyWorker(cw)
}

func (cw *keyupdaterWorker) SetUp() (watcher.NotifyWatcher, error) {
	// Read the keys Juju knows about
	jujuKeys, err := cw.st.AuthorisedKeys(cw.tag)
	if err != nil {
		return nil, log.LoggedErrorf(logger, "reading Juju ssh keys for %q: %v", cw.tag, err)
	}
	cw.jujuKeys = set.NewStrings(jujuKeys...)

	// Read the keys currently in ~/.ssh/authorised_keys
	sshKeys, err := ssh.ListKeys(ssh.FullKeys)
	if err != nil {
		return nil, log.LoggedErrorf(logger, "reading ssh authorized keys for %q: %v", cw.tag, err)
	}
	// Find what's in Juju but not stored locally and update the local keys.
	missing := cw.jujuKeys.Difference(set.NewStrings(sshKeys...))
	if err := ssh.AddKeys(missing.Values()...); err != nil {
		return nil, log.LoggedErrorf(logger, "adding missing Juju keys to ssh authorised keys: %v", err)
	}

	w, err := cw.st.WatchAuthorisedKeys(cw.tag)
	if err != nil {
		return nil, log.LoggedErrorf(logger, "starting key updater worker: %v", err)
	}
	logger.Infof("%q key updater worker started", cw.tag)
	return w, nil
}

func (cw *keyupdaterWorker) Handle() error {
	// Read the keys that Juju has.
	newKeys, err := cw.st.AuthorisedKeys(cw.tag)
	if err != nil {
		return log.LoggedErrorf(logger, "reading Juju ssh keys for %q: %v", cw.tag, err)
	}
	// Figure out what needs to be added and deleted.
	newJujuKeys := set.NewStrings(newKeys...)
	toDelete := cw.jujuKeys.Difference(newJujuKeys)

	toAdd := newJujuKeys.Difference(cw.jujuKeys)
	if toAdd.Size() > 0 {
		keysToAdd := toAdd.Values()
		logger.Debugf("adding ssh keys to authorised keys: %v", keysToAdd)
		if err = ssh.AddKeys(keysToAdd...); err != nil {
			return log.LoggedErrorf(logger, "adding new ssh keys: %v", err)
		}
	}
	if toDelete.Size() > 0 {
		keysToDelete := toDelete.Values()
		logger.Debugf("deleting ssh keys from authorised keys: %v", keysToDelete)
		var deleteComments []string
		for _, key := range keysToDelete {
			_, comment, _, _, ok := stdssh.ParseAuthorizedKey([]byte(key))
			if !ok || comment == "" {
				logger.Debugf("keeping unrecognised existing ssh key %q: %v", key, err)
				continue
			}
			deleteComments = append(deleteComments, comment)
		}
		if err = ssh.DeleteKeys(deleteComments...); err != nil {
			return log.LoggedErrorf(logger, "deleting old ssh keys: %v", err)
		}
	}
	cw.jujuKeys = newJujuKeys
	return nil
}

func (cw *keyupdaterWorker) TearDown() error {
	// Nothing to do here.
	return nil
}
