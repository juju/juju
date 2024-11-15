// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	"context"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v4/ssh"
	"github.com/juju/worker/v4"

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
	// jujuKeys are the most recently retrieved keys from state.
	jujuKeys set.Strings
	// nonJujuKeys are those added externally to auth keys file
	// such keys do not have comments with the Juju: prefix.
	nonJujuKeys []string
}

// NewWorker returns a worker that keeps track of
// the machine's authorised ssh keys and ensures the
// ~/.ssh/authorized_keys file is up to date.
func NewWorker(client Client, agentConfig agent.Config) (worker.Worker, error) {
	machineTag, ok := agentConfig.Tag().(names.MachineTag)
	if !ok {
		return nil, errors.NotValidf("machine tag %v", agentConfig.Tag())
	}
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: &keyupdaterWorker{
			client: client,
			tag:    machineTag,
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
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
	// Write out the ssh authorised keys file to match the current state of the world.
	if err := kw.writeSSHKeys(jujuKeys); err != nil {
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
		if err = kw.writeSSHKeys(newKeys); err != nil {
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
