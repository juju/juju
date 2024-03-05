// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"
	"reflect"
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/hook"
)

// SecretsClient is used by the secrets tracker to access the Juju model.
type SecretsClient interface {
	api.SecretsClient
	SecretMetadata() ([]coresecrets.SecretOwnerMetadata, error)
}

// Secrets generates storage hooks in response to changes to
// storage Secrets, and provides access to information about
// storage Secrets to hooks.
type Secrets struct {
	client  SecretsClient
	unitTag names.UnitTag
	logger  Logger

	secretsState *State
	stateOps     *stateOps

	mu sync.Mutex
}

// NewSecrets returns a new secrets tracker.
func NewSecrets(
	client SecretsClient,
	tag names.UnitTag,
	rw UnitStateReadWriter,
	logger Logger,
) (SecretStateTracker, error) {
	s := &Secrets{
		client:   client,
		unitTag:  tag,
		logger:   logger,
		stateOps: NewStateOps(rw),
	}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

// init processes initialises the tracker based on the unit
// state read from the controller.
func (s *Secrets) init() error {
	existingSecretsState, err := s.stateOps.Read()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Annotate(err, "reading secrets state")
	}
	s.secretsState = existingSecretsState

	changed := false
	if len(s.secretsState.ConsumedSecretInfo) > 0 {
		uris := set.NewStrings()
		for uri := range s.secretsState.ConsumedSecretInfo {
			uris.Add(uri)
		}
		info, err := s.client.GetConsumerSecretsRevisionInfo(s.unitTag.Id(), uris.SortedValues())
		if err != nil {
			return errors.Annotate(err, "getting consumed secret info")
		}
		updated := make(map[string]int)
		for u, v := range info {
			updated[u] = v.Revision
		}
		changed = !reflect.DeepEqual(updated, s.secretsState.ConsumedSecretInfo)
		if changed {
			s.secretsState.ConsumedSecretInfo = updated
		}
	}
	metadata, err := s.client.SecretMetadata()
	if err != nil {
		return errors.Annotate(err, "reading secret metadata")
	}
	owned := set.NewStrings()
	for _, md := range metadata {
		owned.Add(md.Metadata.URI.String())
	}
	for uri := range s.secretsState.SecretObsoleteRevisions {
		if !owned.Contains(uri) {
			changed = true
			delete(s.secretsState.SecretObsoleteRevisions, uri)
		}
	}
	if !changed {
		return nil
	}

	return s.stateOps.Write(s.secretsState)
}

// ConsumedSecretRevision implements SecretStateTracker.
func (s *Secrets) ConsumedSecretRevision(uri string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.secretsState.ConsumedSecretInfo[uri]
}

// SecretObsoleteRevisions implements SecretStateTracker.
func (s *Secrets) SecretObsoleteRevisions(uri string) []int {
	s.mu.Lock()
	defer s.mu.Unlock()

	revs := s.secretsState.SecretObsoleteRevisions[uri]
	result := make([]int, len(revs))
	copy(result, revs)
	return result
}

// PrepareHook implements SecretStateTracker.
func (s *Secrets) PrepareHook(_ context.Context, hi hook.Info) error {
	if !hi.Kind.IsSecret() {
		return errors.Errorf("not a secret hook: %#v", hi)
	}

	return nil
}

// CommitHook implements SecretStateTracker.
func (s *Secrets) CommitHook(_ context.Context, hi hook.Info) error {
	if !hi.Kind.IsSecret() {
		return errors.Errorf("not a secret hook: %#v", hi)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.secretsState.UpdateStateForHook(hi)
	if err := s.stateOps.Write(s.secretsState); err != nil {
		return err
	}
	return nil
}

// SecretsRemoved implements SecretStateTracker.
func (s *Secrets) SecretsRemoved(uris []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, uri := range uris {
		delete(s.secretsState.ConsumedSecretInfo, uri)
		delete(s.secretsState.SecretObsoleteRevisions, uri)
	}
	if err := s.stateOps.Write(s.secretsState); err != nil {
		return err
	}
	return nil
}

// Report provides information for the engine report.
func (s *Secrets) Report() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make(map[string]interface{})
	obsolete := make(map[string][]int)
	for u, v := range s.secretsState.SecretObsoleteRevisions {
		rCopy := make([]int, len(v))
		copy(rCopy, v)
		obsolete[u] = rCopy
	}
	result["obsolete-revisions"] = obsolete

	consumed := make(map[string]int)
	for u, v := range s.secretsState.ConsumedSecretInfo {
		consumed[u] = v
	}
	result["consumed-revisions"] = consumed

	return result
}
