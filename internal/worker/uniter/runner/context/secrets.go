// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/secrets"
)

// secretsChangeRecorder records the intent tp update, remove or
// change secret access permission during a hook execution.
type secretsChangeRecorder struct {
	logger logger.Logger

	pendingCreates map[string]uniter.SecretCreateArg
	pendingUpdates map[string]uniter.SecretUpdateArg
	pendingDeletes map[string]uniter.SecretDeleteArg
	// pendingGrants maps secret URI to a map of relation key to grant args.
	// Because we need to track the grant args per relation key per secret URI.
	pendingGrants map[string]map[string]uniter.SecretGrantRevokeArgs
	// pendingRevokes maps secret URI to a list of revoke args.
	// For revoke, we only need to know the target URI and target entity(unit/application).
	pendingRevokes     map[string][]uniter.SecretGrantRevokeArgs
	pendingTrackLatest map[string]bool
}

func newSecretsChangeRecorder(logger logger.Logger) *secretsChangeRecorder {
	return &secretsChangeRecorder{
		logger:             logger,
		pendingCreates:     make(map[string]uniter.SecretCreateArg),
		pendingUpdates:     make(map[string]uniter.SecretUpdateArg),
		pendingDeletes:     make(map[string]uniter.SecretDeleteArg),
		pendingGrants:      make(map[string]map[string]uniter.SecretGrantRevokeArgs),
		pendingRevokes:     make(map[string][]uniter.SecretGrantRevokeArgs),
		pendingTrackLatest: make(map[string]bool),
	}
}

func (s *secretsChangeRecorder) haveContentUpdates() bool {
	return len(s.pendingCreates) > 0 || len(s.pendingUpdates) > 0 ||
		len(s.pendingDeletes) > 0
}

func (s *secretsChangeRecorder) create(arg uniter.SecretCreateArg) error {
	delete(s.pendingDeletes, arg.URI.ID)
	for _, c := range s.pendingCreates {
		if c.Label != nil && arg.Label != nil && *c.Label == *arg.Label {
			return errors.AlreadyExistsf("secret with label %q", *arg.Label)
		}
	}
	s.pendingCreates[arg.URI.ID] = arg
	return nil
}

func (s *secretsChangeRecorder) update(arg uniter.SecretUpdateArg) {
	delete(s.pendingDeletes, arg.URI.ID)
	if c, ok := s.pendingCreates[arg.URI.ID]; ok {
		if arg.Label != nil {
			c.Label = arg.Label
		}
		if arg.Description != nil {
			c.Description = arg.Description
		}
		if arg.Value != nil && !arg.Value.IsEmpty() {
			c.Value = arg.Value
			c.Checksum = arg.Checksum
		}
		if arg.RotatePolicy != nil {
			c.RotatePolicy = arg.RotatePolicy
		}
		if arg.ExpireTime != nil {
			c.ExpireTime = arg.ExpireTime
		}
		s.pendingCreates[arg.URI.ID] = c
		return
	}
	previous, ok := s.pendingUpdates[arg.URI.ID]
	if !ok {
		s.pendingUpdates[arg.URI.ID] = arg
		return
	}
	if arg.Label != nil {
		previous.Label = arg.Label
	}
	if arg.Description != nil {
		previous.Description = arg.Description
	}
	if arg.Value != nil && !arg.Value.IsEmpty() {
		previous.Value = arg.Value
		previous.Checksum = arg.Checksum
	}
	if arg.RotatePolicy != nil {
		previous.RotatePolicy = arg.RotatePolicy
	}
	if arg.ExpireTime != nil {
		previous.ExpireTime = arg.ExpireTime
	}
	s.pendingUpdates[arg.URI.ID] = previous
}

func (s *secretsChangeRecorder) remove(uri *secrets.URI, revision *int) {
	delete(s.pendingCreates, uri.ID)
	delete(s.pendingUpdates, uri.ID)
	delete(s.pendingGrants, uri.ID)
	delete(s.pendingRevokes, uri.ID)
	delete(s.pendingTrackLatest, uri.ID)
	s.pendingDeletes[uri.ID] = uniter.SecretDeleteArg{URI: uri, Revision: revision}
}

func (s *secretsChangeRecorder) grant(arg uniter.SecretGrantRevokeArgs) {
	if _, ok := s.pendingGrants[arg.URI.ID]; !ok {
		s.pendingGrants[arg.URI.ID] = make(map[string]uniter.SecretGrantRevokeArgs)
	}
	s.pendingGrants[arg.URI.ID][*arg.RelationKey] = arg
}

func (s *secretsChangeRecorder) revoke(arg uniter.SecretGrantRevokeArgs) {
	for _, revoke := range s.pendingRevokes[arg.URI.ID] {
		if revoke.Equal(arg) {
			return
		}
		if revoke.Role != arg.Role || revoke.URI.ID != arg.URI.ID {
			continue
		}
		if revoke.ApplicationName != nil && arg.UnitName != nil {
			unitApp, _ := names.UnitApplication(*arg.UnitName)
			if unitApp == *revoke.ApplicationName {
				// No need to revoke a unit grant if the application grant has been revoked.
				return
			}
		}
	}

	s.pendingRevokes[arg.URI.ID] = append(s.pendingRevokes[arg.URI.ID], arg)
}

func (s *secretsChangeRecorder) secretGrantInfo(uri *secrets.URI, applied ...secrets.AccessInfo) ([]secrets.AccessInfo, error) {
	mergePendingGrants := func() {
		grants, ok := s.pendingGrants[uri.ID]
		if !ok {
			return
		}
		for _, grant := range grants {
			params := grant.ToParams()
			if len(params.SubjectTags) == 0 {
				// This should never happen.
				s.logger.Warningf("missing SubjectTags: %+v", params)
				continue
			}
			applied = append(applied, secrets.AccessInfo{
				Target: params.SubjectTags[0],
				Scope:  params.ScopeTag,
				Role:   secrets.SecretRole(params.Role),
			})
		}
	}
	excludeRevokedGrants := func() {
		revokes, ok := s.pendingRevokes[uri.ID]
		if !ok {
			return
		}
		for _, revoke := range revokes {
			params := revoke.ToParams()
			if len(params.SubjectTags) == 0 {
				// This should never happen.
				s.logger.Warningf("missing SubjectTags: %+v", params)
				continue
			}
			for j, grant := range applied {
				if grant.Target == params.SubjectTags[0] && grant.Role == secrets.SecretRole(params.Role) {
					applied = append(applied[:j], applied[j+1:]...)
					break
				}
			}
		}
	}
	mergePendingGrants()
	excludeRevokedGrants()
	return applied, nil
}
