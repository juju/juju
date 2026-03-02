// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"strings"

	"github.com/juju/names/v6"
)

// IsRemoteSecretGrant checks if the given tag represents a remote application, ie a synthetic application
// that represents a remote application on the consumer of side of the relation. In the context of secrets, this
// is used to grant access to offerer secrets to a consumer application through a relation.
func IsRemoteSecretGrant(tag names.Tag) bool {
	return tag.Kind() == names.ApplicationTagKind && strings.HasPrefix(tag.Id(), "remote-")
}
