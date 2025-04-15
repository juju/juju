// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v3/ssh"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/keyupdater"
	"github.com/juju/juju/core/watcher"
)

// The user name used to ssh into Juju nodes.
// Override for testing.
var SSHUser = "ubuntu"

var logger = loggo.GetLogger("juju.worker.authenticationworker")

type keyUpdater struct {
	st  *keyupdater.State
	tag names.MachineTag
	// jujuKeys are the most recently retrieved keys from state.
	jujuKeys set.Strings
}

// AuthWorker is a worker that keeps track of
// the machine's authorised ssh keys and ensures the
// ~/.ssh/authorized_keys file is up to date.
type AuthWorker struct {
	*watcher.NotifyWorker
}

// NewWorker returns a worker that keeps track of
// the machine's authorised ssh keys and ensures the
// ~/.ssh/authorized_keys file is up to date.
func NewWorker(st *keyupdater.State, agentConfig agent.Config) (worker.Worker, error) {
	machineTag, ok := agentConfig.Tag().(names.MachineTag)
	if !ok {
		return nil, errors.NotValidf("machine tag %v", agentConfig.Tag())
	}
	keyUpdater := &keyUpdater{
		st:  st,
		tag: machineTag,
	}
	err := keyUpdater.initializeAuthorizedKeys()
	if err != nil {
		return nil, errors.Trace(err)
	}
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: keyUpdater,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	return AuthWorker{
		NotifyWorker: w,
	}, nil
}

// initializeAuthorizedKeys reads the current ssh keys from the state
// and the current ssh keys from ~/.ssh/authorized_keys. It removes any
// ephemeral keys.
func (kw *keyUpdater) initializeAuthorizedKeys() error {
	// Record the keys Juju knows about.
	jujuKeys, err := kw.st.AuthorisedKeys(kw.tag)
	if err != nil {
		err = errors.Annotatef(err, "reading Juju ssh keys for %q", kw.tag)
		logger.Infof(err.Error())
		return err
	}
	kw.jujuKeys = set.NewStrings(ensureJujuCommentForKeys(jujuKeys)...)
	allKeys := kw.jujuKeys.Values()
	// Read the keys currently in ~/.ssh/authorised_keys.
	sshKeys, err := ssh.ListKeys(SSHUser, ssh.FullKeys)
	if err != nil {
		err = errors.Annotatef(err, "reading ssh authorized keys for %q", kw.tag)
		logger.Infof(err.Error())
		return err
	}
	// Preserve any non-juju keys, including malformed ones.
	for _, key := range sshKeys {
		_, comment, err := ssh.KeyFingerprint(key)
		if err != nil {
			logger.Warningf("preserved malformed ssh key %q: %v", key, err)
			allKeys = append(allKeys, key)
			continue
		}
		if strings.HasPrefix(comment, ssh.JujuCommentPrefix) {
			logger.Infof("filtered out juju ssh key %q", key)
			continue
		}
		if strings.HasPrefix(comment, jujuEphemeralCommentPrefix) {
			logger.Infof("filtered out juju ephemeral ssh key %q", key)
			continue
		}
		logger.Infof("preserved ssh key %q", key)
		allKeys = append(allKeys, key)
	}
	// Write out the ssh authorised keys file to match the current state of the world.
	if err := ssh.ReplaceKeys(SSHUser, allKeys...); err != nil {
		err = errors.Annotate(err, "adding current Juju keys to ssh authorised keys")
		logger.Infof(err.Error())
		return err
	}
	return nil
}

// SetUp is defined on the worker.NotifyWatchHandler interface.
func (kw *keyUpdater) SetUp() (watcher.NotifyWatcher, error) {
	w, err := kw.st.WatchAuthorisedKeys(kw.tag)
	if err != nil {
		err = errors.Annotate(err, "starting key updater worker")
		logger.Infof(err.Error())
		return nil, err
	}
	logger.Infof("%q key updater worker started", kw.tag)
	return w, nil
}

// Handle is defined on the worker.NotifyWatchHandler interface.
func (kw *keyUpdater) Handle(_ <-chan struct{}) error {
	// Read the keys that Juju has.
	newKeys, err := kw.st.AuthorisedKeys(kw.tag)
	if err != nil {
		err = errors.Annotatef(err, "reading Juju ssh keys for %q", kw.tag)
		logger.Infof(err.Error())
		return err
	}
	// Figure out if any keys have been added or deleted.
	newJujuKeys := set.NewStrings(ensureJujuCommentForKeys(newKeys)...)
	deleted := kw.jujuKeys.Difference(newJujuKeys)
	added := newJujuKeys.Difference(kw.jujuKeys)
	logger.Infof("adding ssh keys to authorised keys: %v", added)
	logger.Infof("deleting ssh keys from authorised keys: %v", deleted)
	err = ssh.AddKeys(SSHUser, added.Values()...)
	if err != nil {
		err = errors.Annotatef(err, "adding ssh keys to authorised keys: %v", added)
		logger.Infof(err.Error())
		return err
	}
	keyFingerprintsToDelete := []string{}
	for _, key := range deleted.Values() {
		fingerprint, _, err := ssh.KeyFingerprint(key)
		if err != nil {
			continue
		}
		keyFingerprintsToDelete = append(keyFingerprintsToDelete, fingerprint)
	}

	err = ssh.DeleteKeys(SSHUser, keyFingerprintsToDelete...)
	if err != nil {
		err = errors.Annotatef(err, "removing ssh keys from authorised keys: %v", deleted)
		return err
	}
	kw.jujuKeys = newJujuKeys
	return nil
}

// TearDown is defined on the worker.NotifyWatchHandler interface.
func (kw *keyUpdater) TearDown() error {
	// Nothing to do here.
	return nil
}

// AddEphemeralKey adds an ephemeral key to authorized_keys file.
func (a *AuthWorker) AddEphemeralKey(ephemeralKey string) error {
	ephemeralKey = ensureJujuEphemeralComment(ephemeralKey)
	err := ssh.AddKeys(SSHUser, ephemeralKey)
	if err != nil {
		return err
	}
	return nil
}

// RemoveEphemeralKey removes an ephemeral key from authorized_keys file.
func (a *AuthWorker) RemoveEphemeralKey(ephemeralKey string) error {
	fingerprint, _, err := ssh.KeyFingerprint(ephemeralKey)
	if err != nil {
		return errors.Annotatef(err, "removing ephemeral key %q", ephemeralKey)
	}
	err = ssh.DeleteKeys(SSHUser, fingerprint)
	if err != nil {
		return err
	}
	return nil
}
