// Copyright 2023 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/names/v5"
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
	return &tagWrapper{t}
}
