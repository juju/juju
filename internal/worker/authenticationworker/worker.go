// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	"context"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4/ssh"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"
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

// opType identifies the kind of ephemeral key operation carried by a request.
type opType int

const (
	addOp opType = iota
	removeOp
)

// ephemeralRequest is used to pass ephemeral key add/remove operations into the
// worker loop, so that all writes to the authorized_keys file are serialised
// through a single goroutine. The done channel is buffered so the loop never
// blocks replying to a caller that has stopped waiting.
type ephemeralRequest struct {
	op      opType
	key     gossh.PublicKey
	comment string
	done    chan error
}

// makeAddRequest creates a request to add the supplied ephemeral key, tagged
// with the given comment for later removal.
func makeAddRequest(key gossh.PublicKey, comment string) ephemeralRequest {
	return ephemeralRequest{
		op:      addOp,
		key:     key,
		comment: comment,
		done:    make(chan error, 1),
	}
}

// makeRemoveRequest creates a request to remove the supplied ephemeral key.
func makeRemoveRequest(key gossh.PublicKey) ephemeralRequest {
	return ephemeralRequest{
		op:   removeOp,
		key:  key,
		done: make(chan error, 1),
	}
}

// AuthWorker is a worker that keeps track of the machine's authorised ssh keys
// and ensures the ~/.ssh/authorized_keys file is up to date. It additionally
// supports adding and removing ephemeral keys, used by the sshsession worker
// for the lifetime of a reverse SSH tunnel.
//
// All mutations to the authorized_keys file - model key updates and ephemeral
// key add/remove operations - are serialised through the worker loop. Ephemeral
// key requests are enqueued onto the requests channel and applied by the loop
// goroutine, so they cannot race a model key update.
type AuthWorker struct {
	catacomb catacomb.Catacomb

	client Client
	tag    names.MachineTag

	// requests carries ephemeral key add/remove operations into the loop.
	requests chan ephemeralRequest

	// The following fields are owned by the loop goroutine and must only be
	// accessed from it.

	// jujuKeys are the most recently retrieved keys from state.
	jujuKeys set.Strings
	// nonJujuKeys are those added externally to the auth keys file; such keys
	// do not have comments with the Juju: prefix.
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
	w := &AuthWorker{
		client:   client,
		tag:      machineTag,
		requests: make(chan ephemeralRequest),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "authentication",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Kill is part of the worker.Worker interface.
func (a *AuthWorker) Kill() {
	a.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (a *AuthWorker) Wait() error {
	return a.catacomb.Wait()
}

// AddEphemeralKey adds an ephemeral key to the authorized_keys file. The
// supplied comment is used to identify the key for later removal. The write is
// performed by the worker loop, so it cannot race a model key update.
func (a *AuthWorker) AddEphemeralKey(key gossh.PublicKey, comment string) error {
	return a.enqueue(makeAddRequest(key, comment))
}

// RemoveEphemeralKey removes an ephemeral key from the authorized_keys file.
// The write is performed by the worker loop, so it cannot race a model key
// update.
func (a *AuthWorker) RemoveEphemeralKey(ephemeralKey gossh.PublicKey) error {
	return a.enqueue(makeRemoveRequest(ephemeralKey))
}

// enqueue sends an ephemeral key request into the worker loop and waits for it
// to be applied, returning any error. If the worker is dying it returns an
// error indicating so.
func (a *AuthWorker) enqueue(req ephemeralRequest) error {
	select {
	case a.requests <- req:
	case <-a.catacomb.Dying():
		return errors.Trace(a.catacomb.ErrDying())
	}

	select {
	case err := <-req.done:
		return errors.Trace(err)
	case <-a.catacomb.Dying():
		return errors.Trace(a.catacomb.ErrDying())
	}
}

func (a *AuthWorker) loop() error {
	ctx, cancel := a.scopedContext()
	defer cancel()

	// Perform the initial synchronisation of the authorized_keys file before
	// we start watching for changes. On startup any dangling ephemeral keys
	// are intentionally not preserved, so a restart flushes them.
	watch, err := a.setUp(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := a.catacomb.Add(watch); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-a.catacomb.Dying():
			return a.catacomb.ErrDying()
		case req := <-a.requests:
			// Ephemeral key requests are non-fatal: reply the result to the
			// caller and keep serving. A bad request must not bring down the
			// worker and, with it, the machine's key management.
			req.done <- a.handleEphemeralRequest(req)
		case _, ok := <-watch.Changes():
			if !ok {
				return errors.New("change channel closed")
			}
			if err := a.handleModelKeyUpdate(ctx); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// setUp records the keys Juju knows about and the keys not managed by Juju,
// writes out the authorized_keys file to match, and starts watching for
// authorised key changes.
func (a *AuthWorker) setUp(ctx context.Context) (watcher.NotifyWatcher, error) {
	// Record the keys Juju knows about.
	jujuKeys, err := a.client.AuthorisedKeys(ctx, a.tag)
	if err != nil {
		err = errors.Annotatef(err, "reading Juju ssh keys for %q", a.tag)
		logger.Infof(ctx, err.Error())
		return nil, err
	}
	a.jujuKeys = set.NewStrings(jujuKeys...)

	// Read the keys currently in ~/.ssh/authorised_keys.
	sshKeys, err := ssh.ListKeys(SSHUser, ssh.FullKeys)
	if err != nil {
		err = errors.Annotatef(err, "reading ssh authorized keys for %q", a.tag)
		logger.Infof(ctx, err.Error())
		return nil, err
	}
	// Record any keys not added by Juju.
	for _, key := range sshKeys {
		_, comment, err := ssh.KeyFingerprint(key)
		// Also record keys which we cannot parse.
		if err != nil || !strings.HasPrefix(comment, ssh.JujuCommentPrefix) {
			a.nonJujuKeys = append(a.nonJujuKeys, key)
		}
	}
	// Write out the ssh authorised keys file to match the current state of the
	// world. On startup any dangling ephemeral keys are intentionally not
	// preserved, so a restart flushes them.
	if err := a.writeSSHKeys(jujuKeys, false); err != nil {
		err = errors.Annotate(err, "adding current Juju keys to ssh authorised keys")
		logger.Infof(ctx, err.Error())
		return nil, err
	}

	w, err := a.client.WatchAuthorisedKeys(ctx, a.tag)
	if err != nil {
		err = errors.Annotate(err, "starting key updater worker")
		logger.Infof(ctx, err.Error())
		return nil, err
	}
	logger.Infof(ctx, "%q key updater worker started", a.tag)
	return w, nil
}

// handleEphemeralRequest applies a single ephemeral key add or remove
// operation. It runs on the loop goroutine, so it is serialised against model
// key updates and other ephemeral requests.
func (a *AuthWorker) handleEphemeralRequest(req ephemeralRequest) error {
	switch req.op {
	case addOp:
		keyWithComment := ensureJujuEphemeralComment(req.key, req.comment)
		if err := ssh.AddKeys(SSHUser, keyWithComment); err != nil {
			return errors.Trace(err)
		}
		return nil
	case removeOp:
		fingerprint := gossh.FingerprintLegacyMD5(req.key)
		if err := ssh.DeleteKeys(SSHUser, fingerprint); err != nil {
			return errors.Trace(err)
		}
		return nil
	default:
		return errors.Errorf("unknown op %v", req.op)
	}
}

// handleModelKeyUpdate reconciles the authorized_keys file with the latest set
// of keys reported by Juju. It runs on the loop goroutine.
func (a *AuthWorker) handleModelKeyUpdate(ctx context.Context) error {
	// Read the keys that Juju has.
	newKeys, err := a.client.AuthorisedKeys(ctx, a.tag)
	if err != nil {
		err = errors.Annotatef(err, "reading Juju ssh keys for %q", a.tag)
		logger.Infof(ctx, err.Error())
		return err
	}
	// Figure out if any keys have been added or deleted.
	newJujuKeys := set.NewStrings(newKeys...)
	deleted := a.jujuKeys.Difference(newJujuKeys)
	added := newJujuKeys.Difference(a.jujuKeys)
	if added.Size() > 0 || deleted.Size() > 0 {
		logger.Infof(ctx, "adding ssh keys to authorised keys: %v", added)
		logger.Infof(ctx, "deleting ssh keys from authorised keys: %v", deleted)
		// Preserve any in-flight ephemeral keys across the model key update.
		if err = a.writeSSHKeys(newKeys, true); err != nil {
			err = errors.Annotate(err, "updating ssh keys")
			logger.Infof(ctx, err.Error())
			return err
		}
	}
	a.jujuKeys = newJujuKeys
	return nil
}

// writeSSHKeys writes out a new ~/.ssh/authorised_keys file, retaining any non
// Juju keys and adding the specified set of Juju keys. When preserveEphemeral
// is true, any ephemeral keys currently present in the file are retained too,
// so that a model key update does not clobber an in-flight tunnel's key.
//
// It is only ever called from the loop goroutine, so it does not need its own
// locking: the loop serialises it against ephemeral key add/remove operations,
// which mutate the same file.
func (a *AuthWorker) writeSSHKeys(jujuKeys []string, preserveEphemeral bool) error {
	// Copy nonJujuKeys so appends below never mutate the shared slice across
	// calls.
	allKeys := append([]string(nil), a.nonJujuKeys...)

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

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (a *AuthWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return a.catacomb.Context(ctx), cancel
}
