// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/secrets"
)

// secretsChangeRecorder records the intent tp update, remove or
// change secret access permission during a hook execution.
type secretsChangeRecorder struct {
	logger loggo.Logger

	pendingUpdates []uniter.SecretUpdateArg
	pendingDeletes []*secrets.URI
	pendingGrants  []uniter.SecretGrantRevokeArgs
	pendingRevokes []uniter.SecretGrantRevokeArgs
}

func newSecretsChangeRecorder(logger loggo.Logger) *secretsChangeRecorder {
	return &secretsChangeRecorder{
		logger: logger,
	}
}

func (s *secretsChangeRecorder) update(arg uniter.SecretUpdateArg) {
	s.pendingUpdates = append(s.pendingUpdates, arg)
}

func (s *secretsChangeRecorder) remove(uri *secrets.URI) {
	s.pendingDeletes = append(s.pendingDeletes, uri)
	for i, u := range s.pendingUpdates {
		if u.URI.ID == uri.ID {
			s.pendingUpdates = append(s.pendingUpdates[:i], s.pendingUpdates[i+1:]...)
			break
		}
	}
	for i, u := range s.pendingGrants {
		if u.URI.ID == uri.ID {
			s.pendingGrants = append(s.pendingGrants[:i], s.pendingGrants[i+1:]...)
			break
		}
	}
	for i, u := range s.pendingRevokes {
		if u.URI.ID == uri.ID {
			s.pendingRevokes = append(s.pendingRevokes[:i], s.pendingRevokes[i+1:]...)
			break
		}
	}
}

func (s *secretsChangeRecorder) grant(arg uniter.SecretGrantRevokeArgs) {
	s.pendingGrants = append(s.pendingGrants, arg)
}

func (s *secretsChangeRecorder) revoke(arg uniter.SecretGrantRevokeArgs) {
	s.pendingRevokes = append(s.pendingRevokes, arg)
}
