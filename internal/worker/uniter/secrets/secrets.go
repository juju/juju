// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"reflect"
	"slices"
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
)

// SecretsClient is used by the secrets tracker to access the Juju model.
type SecretsClient interface {
	remotestate.SecretsClient
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
	if err != nil && !errors.IsNotFound(err) {
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

	return s.stateOps.Write(s.secretsState)
}

// ConsumedSecretRevision implements SecretStateTracker.
func (s *Secrets) ConsumedSecretRevision(uri string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.secretsState.ConsumedSecretInfo[uri]
}

// TrimSecretObsoleteRevisions implements SecretStateTracker.
func (s *Secrets) TrimSecretObsoleteRevisions(known map[string][]int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, trackedRevs := range s.secretsState.SecretObsoleteRevisions {
		revs, ok := known[id]
		if !ok {
			delete(s.secretsState.SecretObsoleteRevisions, id)
			continue
		}
		slices.Sort(revs)
		trackedRevs = slices.DeleteFunc(trackedRevs, func(rev int) bool {
			_, found := slices.BinarySearch(revs, rev)
			return !found
		})
		s.secretsState.SecretObsoleteRevisions[id] = trackedRevs
	}

	// No need to write as this will be picked up the next secret state write.
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
func (s *Secrets) PrepareHook(hi hook.Info) error {
	if !hi.Kind.IsSecret() {
		return errors.Errorf("not a secret hook: %#v", hi)
	}

	return nil
}

// CommitHook implements SecretStateTracker.
func (s *Secrets) CommitHook(hi hook.Info) error {
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
func (s *Secrets) SecretsRemoved(deletedRevisions map[string][]int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for uri, revs := range deletedRevisions {
		if len(revs) == 0 {
			delete(s.secretsState.ConsumedSecretInfo, uri)
			delete(s.secretsState.SecretObsoleteRevisions, uri)
			continue
		}
		obsoleteRevs := set.NewInts(s.secretsState.SecretObsoleteRevisions[uri]...)
		newObsoleteRevs := obsoleteRevs.Difference(set.NewInts(revs...)).SortedValues()
		if len(newObsoleteRevs) == 0 {
			delete(s.secretsState.SecretObsoleteRevisions, uri)
		} else {
			s.secretsState.SecretObsoleteRevisions[uri] = newObsoleteRevs
		}
	}
	s.logger.Debugf("secret revisions removed from %q unit state: %v", s.unitTag.Id(), deletedRevisions)
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
