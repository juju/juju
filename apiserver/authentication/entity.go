// Copyright 2023 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/names/v6"

	coreuser "github.com/juju/juju/core/user"
)

// TagWrapper is a utility struct to take a names tag and wrap it as to conform
// to the entity interface set out in this packages interfaces.
type tagWrapper struct {
	tag names.Tag
}

// Tag implements Entity Tag.
func (t *tagWrapper) Tag() names.Tag {
	return t.tag
}

// TagToEntity takes a name names.Tag and concerts it an authentication Entity.
func TagToEntity(t names.Tag) Entity {
	return &tagWrapper{tag: t}
}

// TaggedUser is a user that has been tagged with a names.Tag.
type taggedUser struct {
	coreuser.User
	tag names.Tag
}

// TaggedUser returns a user that has been tagged with a names.Tag.
func TaggedUser(u coreuser.User, t names.Tag) Entity {
	return taggedUser{u, t}
}

// Tag returns the tag of the user.
func (u taggedUser) Tag() names.Tag {
	return u.tag
}

type externalUser struct {
	tag names.Tag
}

func (e externalUser) Tag() names.Tag {
	return e.tag
}
