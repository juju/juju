// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state/api/keyupdater"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/utils/ssh"
	"launchpad.net/juju-core/worker"
)

// The user name used to ssh into Juju nodes.
// Override for testing.
var SSHUser = "ubuntu"

var logger = loggo.GetLogger("juju.worker.authenticationworker")

type keyupdaterWorker struct {
	st   *keyupdater.State
	tomb tomb.Tomb
	tag  string
	// jujuKeys are the most recently retrieved keys from state.
	jujuKeys set.Strings
	// nonJujuKeys are those added externally to auth keys file
	// such keys do not have comments with the Juju: prefix.
	nonJujuKeys []string
}

var _ worker.NotifyWatchHandler = (*keyupdaterWorker)(nil)

// NewWorker returns a worker that keeps track of
// the machine's authorised ssh keys and ensures the
// ~/.ssh/authorized_keys file is up to date.
func NewWorker(st *keyupdater.State, agentConfig agent.Config) worker.Worker {
	kw := &keyupdaterWorker{st: st, tag: agentConfig.Tag()}
	return worker.NewNotifyWorker(kw)
}

// SetUp is defined on the worker.NotifyWatchHandler interface.
func (kw *keyupdaterWorker) SetUp() (watcher.NotifyWatcher, error) {
	// Record the keys Juju knows about.
	jujuKeys, err := kw.st.AuthorisedKeys(kw.tag)
	if err != nil {
		return nil, errors.LoggedErrorf(logger, "reading Juju ssh keys for %q: %v", kw.tag, err)
	}
	kw.jujuKeys = set.NewStrings(jujuKeys...)

	// Read the keys currently in ~/.ssh/authorised_keys.
	sshKeys, err := ssh.ListKeys(SSHUser, ssh.FullKeys)
	if err != nil {
		return nil, errors.LoggedErrorf(logger, "reading ssh authorized keys for %q: %v", kw.tag, err)
	}
	// Record any keys not added by Juju.
	for _, key := range sshKeys {
		_, comment, err := ssh.KeyFingerprint(key)
		// Also record keys which we cannot parse.
		if err != nil || !strings.HasPrefix(comment, ssh.JujuCommentPrefix) {
			kw.nonJujuKeys = append(kw.nonJujuKeys, key)
		}
	}
	// Write out the ssh authorised keys file to match the current state of the world.
	if err := kw.writeSSHKeys(jujuKeys); err != nil {
		return nil, errors.LoggedErrorf(logger, "adding current Juju keys to ssh authorised keys: %v", err)
	}

	w, err := kw.st.WatchAuthorisedKeys(kw.tag)
	if err != nil {
		return nil, errors.LoggedErrorf(logger, "starting key updater worker: %v", err)
	}
	logger.Infof("%q key updater worker started", kw.tag)
	return w, nil
}

// writeSSHKeys writes out a new ~/.ssh/authorised_keys file, retaining any non Juju keys
// and adding the specified set of Juju keys.
func (kw *keyupdaterWorker) writeSSHKeys(jujuKeys []string) error {
	allKeys := kw.nonJujuKeys
	// Ensure any Juju keys have the required prefix in their comment.
	for i, key := range jujuKeys {
		jujuKeys[i] = ssh.EnsureJujuComment(key)
	}
	allKeys = append(allKeys, jujuKeys...)
	return ssh.ReplaceKeys(SSHUser, allKeys...)
}

// Handle is defined on the worker.NotifyWatchHandler interface.
func (kw *keyupdaterWorker) Handle() error {
	// Read the keys that Juju has.
	newKeys, err := kw.st.AuthorisedKeys(kw.tag)
	if err != nil {
		return errors.LoggedErrorf(logger, "reading Juju ssh keys for %q: %v", kw.tag, err)
	}
	// Figure out if any keys have been added or deleted.
	newJujuKeys := set.NewStrings(newKeys...)
	deleted := kw.jujuKeys.Difference(newJujuKeys)
	added := newJujuKeys.Difference(kw.jujuKeys)
	if added.Size() > 0 || deleted.Size() > 0 {
		logger.Debugf("adding ssh keys to authorised keys: %v", added)
		logger.Debugf("deleting ssh keys from authorised keys: %v", deleted)
		if err = kw.writeSSHKeys(newKeys); err != nil {
			return errors.LoggedErrorf(logger, "updating ssh keys: %v", err)
		}
	}
	kw.jujuKeys = newJujuKeys
	return nil
}

// TearDown is defined on the worker.NotifyWatchHandler interface.
func (kw *keyupdaterWorker) TearDown() error {
	// Nothing to do here.
	return nil
}
