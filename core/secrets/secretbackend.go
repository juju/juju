// Copyright 2022 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import (
	"fmt"
	"time"
)

// SecretBackend defines a secrets backend.
type SecretBackend struct {
	ID                  string
	Name                string
	BackendType         string
	TokenRotateInterval *time.Duration
	Config              map[string]interface{}
}

// ValueRef represents a reference to a secret
// content value stored in a backend.
type ValueRef struct {
	BackendID  string
	RevisionID string
}

func (r *ValueRef) String() string {
	return fmt.Sprintf("%s:%s", r.BackendID, r.RevisionID)
}
