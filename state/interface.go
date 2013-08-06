// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"launchpad.net/juju-core/agent/tools"
)

// Entity represents any entity that can be returned
// by State.FindEntity. All entities have a tag.
type Entity interface {
	Tag() string
}

// AgentEntity represents an entity that can
// have an agent responsible for it.
type AgentEntity interface {
	Lifer
	Authenticator
	MongoPassworder
	AgentTooler
}

// Lifer represents an entity with a life.
type Lifer interface {
	Life() Life
}

// AgentTooler is implemented by entities
// that have associated agent tools.
type AgentTooler interface {
	AgentTools() (*tools.Tools, error)
	SetAgentTools(*tools.Tools) error
}

// Remover represents entities with lifecycles, EnsureDead and Remove methods.
type Remover interface {
	EnsureDead() error
	Remove() error
}

// Authenticator represents entites capable of handling password
// authentication.
type Authenticator interface {
	Refresh() error
	SetPassword(pass string) error
	PasswordValid(pass string) bool
}

// MongoPassworder represents an entity that can
// have a mongo password set for it.
type MongoPassworder interface {
	SetMongoPassword(password string) error
}

// TaggedAuthenticator represents tagged entities capable of authentication.
type TaggedAuthenticator interface {
	Authenticator
	Entity
}

// Annotator represents entities capable of handling annotations.
type Annotator interface {
	Annotation(key string) (string, error)
	Annotations() (map[string]string, error)
	SetAnnotations(pairs map[string]string) error
}

// TaggedAnnotator represents tagged entities capable of handling annotations.
type TaggedAnnotator interface {
	Annotator
	Entity
}
