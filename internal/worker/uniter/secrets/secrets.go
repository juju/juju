// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"
	"reflect"
	"slices"
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/hook"
)

// SecretsClient is used by the secrets tracker to access the Juju model.
type SecretsClient interface {
	api.SecretsClient
}

// Secrets generates storage hooks in response to changes to
// storage Secrets, and provides access to information about
// storage Secrets to hooks.
type Secrets struct {
	client  SecretsClient
	unitTag names.UnitTag
	logger  logger.Logger

	secretsState *State
	stateOps     *stateOps

	mu sync.Mutex
}

// NewSecrets returns a new secrets tracker.
func NewSecrets(
	ctx context.Context,
	client SecretsClient,
	tag names.UnitTag,
	rw UnitStateReadWriter,
	logger logger.Logger,
) (SecretStateTracker, error) {
	s := &Secrets{
		client:   client,
		unitTag:  tag,
		logger:   logger,
		stateOps: NewStateOps(rw),
	}
	if err := s.init(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// init processes initialises the tracker based on the unit
// state read from the controller.
func (s *Secrets) init(ctx context.Context) error {
	existingSecretsState, err := s.stateOps.Read(ctx)
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
		info, err := s.client.GetConsumerSecretsRevisionInfo(ctx, s.unitTag.Id(), uris.SortedValues())
		if err != nil {
			return errors.Annotate(err, "getting consumed secret info")
		}
		updated := make(map[string]int)
		for u, v := range info {
			updated[u] = v.LatestRevision
		}
		changed = !reflect.DeepEqual(updated, s.secretsState.ConsumedSecretInfo)
		if changed {
			s.secretsState.ConsumedSecretInfo = updated
		}
	}
	if !changed {
		return nil
	}

	return s.stateOps.Write(ctx, s.secretsState)
}

// ConsumedSecretRevision implements SecretStateTracker.
func (s *Secrets) ConsumedSecretRevision(uri string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.secretsState.ConsumedSecretInfo[uri]
}

// CollectRemovedSecretObsoleteRevisions takes the list of known obsolete
// secrets and their revisions. It returns which secrets or revisions need to
// be trimmed from the local secret state. Secrets where all the revisions are
// to be trimmed have a length of 0.
// CollectRemovedSecretObsoleteRevisions implements SecretStateTracker.
func (s *Secrets) CollectRemovedSecretObsoleteRevisions(
	known map[string][]int,
) map[string][]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.secretsState.SecretObsoleteRevisions) == 0 {
		return nil
	}

	collected := map[string][]int{}
	for id, trackedRevs := range s.secretsState.SecretObsoleteRevisions {
		knownRevs, ok := known[id]
		if !ok {
			// Secret no longer exists, mark it for removal.
			collected[id] = nil
			continue
		}
		slices.Sort(knownRevs)

		var lostRevs []int
		for _, rev := range trackedRevs {
			_, found := slices.BinarySearch(knownRevs, rev)
			if !found {
				lostRevs = append(lostRevs, rev)
			}
		}
		if len(lostRevs) > 0 {
			collected[id] = lostRevs
		}
	}

	if len(collected) == 0 {
		return nil
	}

	return collected
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
func (s *Secrets) CommitHook(ctx context.Context, hi hook.Info) error {
	if !hi.Kind.IsSecret() {
		return errors.Errorf("not a secret hook: %#v", hi)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.secretsState.UpdateStateForHook(hi)
	if err := s.stateOps.Write(ctx, s.secretsState); err != nil {
		return err
	}
	return nil
}

// SecretsRemoved implements SecretStateTracker.
func (s *Secrets) SecretsRemoved(
	ctx context.Context,
	deletedRevisions map[string][]int,
	deletedObsoleteRevisions map[string][]int,
) error {
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
	for uri, revs := range deletedObsoleteRevisions {
		if len(revs) == 0 {
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
	s.logger.Debugf(ctx, "secret revisions removed from %q unit state: %v", s.unitTag.Id(), deletedRevisions)
	if err := s.stateOps.Write(ctx, s.secretsState); err != nil {
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
