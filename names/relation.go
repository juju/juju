// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"regexp"
)

var validRelation = regexp.MustCompile("^" + NumberSnippet + "$")

// IsRelation returns whether id is a valid relation id.
func IsRelation(id string) bool {
	return validRelation.MatchString(id)
}

// RelationTag returns the tag for the relation with the given id.
func RelationTag(relationId string) string {
	if !IsRelation(relationId) {
		panic(fmt.Sprintf("%q is not a valid relation id", relationId))
	}
	return makeTag(RelationTagKind, relationId)
}
