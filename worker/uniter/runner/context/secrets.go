// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/secrets"
)

// secretsChangeRecorder records the intent tp update, remove or
// change secret access permission during a hook execution.
type secretsChangeRecorder struct {
	logger loggo.Logger

	pendingCreates []uniter.SecretCreateArg
	pendingUpdates []uniter.SecretUpdateArg
	pendingDeletes []uniter.SecretDeleteArg
	pendingGrants  []uniter.SecretGrantRevokeArgs
	pendingRevokes []uniter.SecretGrantRevokeArgs
}

func newSecretsChangeRecorder(logger loggo.Logger) *secretsChangeRecorder {
	return &secretsChangeRecorder{
		logger: logger,
	}
}

func (s *secretsChangeRecorder) haveContentUpdates() bool {
	return len(s.pendingCreates) > 0 || len(s.pendingUpdates) > 0 ||
		len(s.pendingDeletes) > 0
}

func (s *secretsChangeRecorder) create(arg uniter.SecretCreateArg) error {
	for i, d := range s.pendingDeletes {
		if d.URI.ID == arg.URI.ID {
			s.pendingDeletes = append(s.pendingDeletes[:i], s.pendingDeletes[i+1:]...)
			break
		}
	}
	for _, c := range s.pendingCreates {
		if c.Label != nil && arg.Label != nil && *c.Label == *arg.Label {
			return errors.AlreadyExistsf("secret with label %q", *arg.Label)
		}
	}
	s.pendingCreates = append(s.pendingCreates, arg)
	return nil
}

func (s *secretsChangeRecorder) update(arg uniter.SecretUpdateArg) {
	for i, d := range s.pendingDeletes {
		if d.URI.ID == arg.URI.ID {
			s.pendingDeletes = append(s.pendingDeletes[:i], s.pendingDeletes[i+1:]...)
			break
		}
	}
	for i, c := range s.pendingCreates {
		if c.URI.ID != arg.URI.ID {
			continue
		}
		if arg.Label != nil {
			c.Label = arg.Label
		}
		if arg.Description != nil {
			c.Description = arg.Description
		}
		if !arg.Value.IsEmpty() {
			c.Value = arg.Value
		}
		if arg.RotatePolicy != nil {
			c.RotatePolicy = arg.RotatePolicy
		}
		if arg.ExpireTime != nil {
			c.ExpireTime = arg.ExpireTime
		}
		s.pendingCreates[i] = c
		return
	}
	s.pendingUpdates = append(s.pendingUpdates, arg)
}

func (s *secretsChangeRecorder) remove(uri *secrets.URI, revision *int) {
	s.pendingDeletes = append(s.pendingDeletes, uniter.SecretDeleteArg{URI: uri, Revision: revision})
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
