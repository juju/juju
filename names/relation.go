// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"regexp"
	"strings"
)

const RelationSnippet = "[a-z][a-z0-9]*([_-][a-z0-9]+)*"

// Relation keys have the format "service1:relName1 service2:relName2".
// Except the peer relations, which have the format "service:relName"
// Relation tags have the format "relation-service1.rel1#service2.rel2".
// For peer relations, the format is "relation-service.rel"

var (
	validRelation     = regexp.MustCompile("^" + ServiceSnippet + ":" + RelationSnippet + " " + ServiceSnippet + ":" + RelationSnippet + "$")
	validPeerRelation = regexp.MustCompile("^" + ServiceSnippet + ":" + RelationSnippet + "$")
)

// IsRelation returns whether key is a valid relation key.
func IsRelation(key string) bool {
	return validRelation.MatchString(key) || validPeerRelation.MatchString(key)
}

// RelationTag returns the tag for the relation with the given key.
func RelationTag(relationKey string) string {
	if !IsRelation(relationKey) {
		panic(fmt.Sprintf("%q is not a valid relation key", relationKey))
	}
	// Replace both ":" with "." and the " " with "#".
	relationKey = strings.Replace(relationKey, ":", ".", 2)
	relationKey = strings.Replace(relationKey, " ", "#", 1)
	return makeTag(RelationTagKind, relationKey)
}

func relationTagSuffixToKey(s string) string {
	// Replace both "." with ":" and the "#" with " ".
	s = strings.Replace(s, ".", ":", 2)
	return strings.Replace(s, "#", " ", 1)
}
