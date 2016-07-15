// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"regexp"
	"strings"
)

const RelationTagKind = "relation"

const RelationSnippet = "[a-z][a-z0-9]*(?:[_-][a-z0-9]+)*"

// Relation keys have the format "application1:relName1 application2:relName2".
// Except the peer relations, which have the format "application:relName"
// Relation tags have the format "relation-application1.rel1#application2.rel2".
// For peer relations, the format is "relation-application.rel"

var (
	validRelation     = regexp.MustCompile("^" + ApplicationSnippet + ":" + RelationSnippet + " " + ApplicationSnippet + ":" + RelationSnippet + "$")
	validPeerRelation = regexp.MustCompile("^" + ApplicationSnippet + ":" + RelationSnippet + "$")
)

// IsValidRelation returns whether key is a valid relation key.
func IsValidRelation(key string) bool {
	return validRelation.MatchString(key) || validPeerRelation.MatchString(key)
}

type RelationTag struct {
	key string
}

func (t RelationTag) String() string { return t.Kind() + "-" + t.key }
func (t RelationTag) Kind() string   { return RelationTagKind }
func (t RelationTag) Id() string     { return relationTagSuffixToKey(t.key) }

// NewRelationTag returns the tag for the relation with the given key.
func NewRelationTag(relationKey string) RelationTag {
	if !IsValidRelation(relationKey) {
		panic(fmt.Sprintf("%q is not a valid relation key", relationKey))
	}
	// Replace both ":" with "." and the " " with "#".
	relationKey = strings.Replace(relationKey, ":", ".", 2)
	relationKey = strings.Replace(relationKey, " ", "#", 1)
	return RelationTag{key: relationKey}
}

// ParseRelationTag parses a relation tag string.
func ParseRelationTag(relationTag string) (RelationTag, error) {
	tag, err := ParseTag(relationTag)
	if err != nil {
		return RelationTag{}, err
	}
	rt, ok := tag.(RelationTag)
	if !ok {
		return RelationTag{}, invalidTagError(relationTag, RelationTagKind)
	}
	return rt, nil
}

func relationTagSuffixToKey(s string) string {
	// Replace both "." with ":" and the "#" with " ".
	s = strings.Replace(s, ".", ":", 2)
	return strings.Replace(s, "#", " ", 1)
}
