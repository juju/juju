// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package audit

import (
	"time"

	"gopkg.in/juju/charm.v6-unstable"
)

// Operation represents the type of an entry.
type Operation string

const (
	// OpSetPerm represents the setting of ACLs on an entity.
	// Required fields: Entity, ACL
	OpSetPerm Operation = "set-perm"

	// OpPromulgate, OpUnpromulgate represent the promulgation on an entity.
	// Required fields: Entity
	OpPromulgate   Operation = "promulgate"
	OpUnpromulgate Operation = "unpromulgate"
)

// ACL represents an access control list.
type ACL struct {
	Read  []string `json:"read,omitempty"`
	Write []string `json:"write,omitempty"`
}

// Entry represents an audit log entry.
type Entry struct {
	Time   time.Time  `json:"time"`
	User   string     `json:"user"`
	Op     Operation  `json:"op"`
	Entity *charm.URL `json:"entity,omitempty"`
	ACL    *ACL       `json:"acl,omitempty"`
}
