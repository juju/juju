// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

import (
	"regexp"
)

var validRelation = regexp.MustCompile("^" + NumberSnippet + "$")

// IsRelation returns whether id is a valid relation id.
func IsRelation(id string) bool {
	return validRelation.MatchString(id)
}

// RelationTag returns the tag for the relation with the given id.
func RelationTag(relationId string) string {
	return makeTag(RelationTagKind, relationId)
}
