// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	"context"
	"strings"
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4/ssh"
	"github.com/juju/worker/v5"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/watcher"
	internallogger "github.com/juju/juju/internal/logger"
)

// The user name used to ssh into Juju nodes.
// Override for testing.
var SSHUser = "ubuntu"

var logger = internallogger.GetLogger("juju.worker.authenticationworker")

// Client provides the key updater api client.
type Client interface {
	AuthorisedKeys(ctx context.Context, tag names.MachineTag) ([]string, error)
	WatchAuthorisedKeys(ctx context.Context, tag names.MachineTag) (watcher.NotifyWatcher, error)
}

type keyupdaterWorker struct {
	client Client
	tag    names.MachineTag
	// mu serialises all mutations to the authorized_keys file, so that model
	// key updates (Handle) do not race ephemeral key add/remove operations.
	mu *sync.Mutex
	// jujuKeys are the most recently retrieved keys from state.
	jujuKeys set.Strings
	// nonJujuKeys are those added externally to auth keys file
	// such keys do not have comments with the Juju: prefix.
	nonJujuKeys []string
}

// EphemeralKeysUpdater adds and removes ephemeral SSH keys from the machine's
// authorized_keys file. It is consumed by the sshsession worker, which injects
// an ephemeral key for the lifetime of a reverse SSH tunnel.
type EphemeralKeysUpdater interface {
	// AddEphemeralKey adds an ephemeral key to the authorized_keys file,
	// tagged with the supplied comment for later removal.
	AddEphemeralKey(key gossh.PublicKey, comment string) error
	// RemoveEphemeralKey removes a previously added ephemeral key.
	RemoveEphemeralKey(key gossh.PublicKey) error
}

// AuthWorker is a worker that keeps track of the machine's authorised ssh keys
// and ensures the ~/.ssh/authorized_keys file is up to date. It additionally
// supports adding and removing ephemeral keys, used by the sshsession worker
// for the lifetime of a reverse SSH tunnel.
type AuthWorker struct {
	worker.Worker

	// mu is shared with the inner keyupdaterWorker so that ephemeral key
	// add/remove operations are serialised against model key updates. Both
	// mutate the same authorized_keys file.
	mu *sync.Mutex
}

// NewWorker returns a worker that keeps track of
// the machine's authorised ssh keys and ensures the
// ~/.ssh/authorized_keys file is up to date.
func NewWorker(client Client, agentConfig agent.Config) (worker.Worker, error) {
	machineTag, ok := agentConfig.Tag().(names.MachineTag)
	if !ok {
		return nil, errors.NotValidf("machine tag %v", agentConfig.Tag())
	}
	// A single mutex is shared between the key updater handler and the
	// AuthWorker's ephemeral key methods to serialise all writes to the
	// authorized_keys file.
	mu := &sync.Mutex{}
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: &keyupdaterWorker{
			client: client,
			tag:    machineTag,
			mu:     mu,
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &AuthWorker{Worker: w, mu: mu}, nil
}

// AddEphemeralKey adds an ephemeral key to the authorized_keys file. The
// supplied comment is used to identify the key for later removal.
func (a *AuthWorker) AddEphemeralKey(key gossh.PublicKey, comment string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	keyWithComment := ensureJujuEphemeralComment(key, comment)
	if err := ssh.AddKeys(SSHUser, keyWithComment); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// RemoveEphemeralKey removes an ephemeral key from the authorized_keys file.
func (a *AuthWorker) RemoveEphemeralKey(ephemeralKey gossh.PublicKey) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	fingerprint := gossh.FingerprintLegacyMD5(ephemeralKey)
	if err := ssh.DeleteKeys(SSHUser, fingerprint); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// SetUp is defined on the worker.NotifyWatchHandler interface.
func (kw *keyupdaterWorker) SetUp(ctx context.Context) (watcher.NotifyWatcher, error) {
	// Record the keys Juju knows about.
	jujuKeys, err := kw.client.AuthorisedKeys(ctx, kw.tag)
	if err != nil {
		err = errors.Annotatef(err, "reading Juju ssh keys for %q", kw.tag)
		logger.Infof(ctx, err.Error())
		return nil, err
	}
	kw.jujuKeys = set.NewStrings(jujuKeys...)

	// Read the keys currently in ~/.ssh/authorised_keys.
	sshKeys, err := ssh.ListKeys(SSHUser, ssh.FullKeys)
	if err != nil {
		err = errors.Annotatef(err, "reading ssh authorized keys for %q", kw.tag)
		logger.Infof(ctx, err.Error())
		return nil, err
	}
	// Record any keys not added by Juju.
	for _, key := range sshKeys {
		_, comment, err := ssh.KeyFingerprint(key)
		// Also record keys which we cannot parse.
		if err != nil || !strings.HasPrefix(comment, ssh.JujuCommentPrefix) {
			kw.nonJujuKeys = append(kw.nonJujuKeys, key)
		}
	}
	// Write out the ssh authorised keys file to match the current state of the
	// world. On startup any dangling ephemeral keys are intentionally not
	// preserved, so a restart flushes them.
	if err := kw.writeSSHKeys(jujuKeys, false); err != nil {
		err = errors.Annotate(err, "adding current Juju keys to ssh authorised keys")
		logger.Infof(ctx, err.Error())
		return nil, err
	}

	w, err := kw.client.WatchAuthorisedKeys(ctx, kw.tag)
	if err != nil {
		err = errors.Annotate(err, "starting key updater worker")
		logger.Infof(ctx, err.Error())
		return nil, err
	}
	logger.Infof(ctx, "%q key updater worker started", kw.tag)
	return w, nil
}

// writeSSHKeys writes out a new ~/.ssh/authorised_keys file, retaining any non
// Juju keys and adding the specified set of Juju keys. When preserveEphemeral
// is true, any ephemeral keys currently present in the file are retained too,
// so that a model key update does not clobber an in-flight tunnel's key.
//
// It holds the shared mutex for the duration of the read-modify-write so that
// it cannot race the AuthWorker's ephemeral key add/remove operations, which
// mutate the same file.
func (kw *keyupdaterWorker) writeSSHKeys(jujuKeys []string, preserveEphemeral bool) error {
	kw.mu.Lock()
	defer kw.mu.Unlock()

	// Copy nonJujuKeys so appends below never mutate the shared slice across
	// calls.
	allKeys := append([]string(nil), kw.nonJujuKeys...)

	// Preserve any ephemeral keys currently in the file so that a model key
	// update does not drop them. They carry the Juju ephemeral comment prefix,
	// so they are neither recorded as non-Juju keys nor part of the model keys.
	if preserveEphemeral {
		currentKeys, err := ssh.ListKeys(SSHUser, ssh.FullKeys)
		if err != nil {
			return errors.Annotate(err, "reading current ssh authorised keys")
		}
		for _, key := range currentKeys {
			if _, comment, err := ssh.KeyFingerprint(key); err == nil &&
				strings.HasPrefix(comment, jujuEphemeralCommentPrefix) {
				allKeys = append(allKeys, key)
			}
		}
	}

	// Ensure any Juju keys have the required prefix in their comment.
	for i, key := range jujuKeys {
		jujuKeys[i] = ssh.EnsureJujuComment(key)
	}
	allKeys = append(allKeys, jujuKeys...)
	return ssh.ReplaceKeys(SSHUser, allKeys...)
}

// Handle is defined on the worker.NotifyWatchHandler interface.
func (kw *keyupdaterWorker) Handle(ctx context.Context) error {
	// Read the keys that Juju has.
	newKeys, err := kw.client.AuthorisedKeys(ctx, kw.tag)
	if err != nil {
		err = errors.Annotatef(err, "reading Juju ssh keys for %q", kw.tag)
		logger.Infof(ctx, err.Error())
		return err
	}
	// Figure out if any keys have been added or deleted.
	newJujuKeys := set.NewStrings(newKeys...)
	deleted := kw.jujuKeys.Difference(newJujuKeys)
	added := newJujuKeys.Difference(kw.jujuKeys)
	if added.Size() > 0 || deleted.Size() > 0 {
		logger.Infof(ctx, "adding ssh keys to authorised keys: %v", added)
		logger.Infof(ctx, "deleting ssh keys from authorised keys: %v", deleted)
		// Preserve any in-flight ephemeral keys across the model key update.
		if err = kw.writeSSHKeys(newKeys, true); err != nil {
			err = errors.Annotate(err, "updating ssh keys")
			logger.Infof(ctx, err.Error())
			return err
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
