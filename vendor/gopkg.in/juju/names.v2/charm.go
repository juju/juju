// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"regexp"
)

// CharmTagKind specifies charm tag kind
const CharmTagKind = "charm"

// Valid charm url is of the form
// schema:~user/series/name-revision
// where
//     schema    is optional and can be either "local" or "cs".
//               When not supplied, "cs" is implied.
//     user      is optional and is only applicable for "cs" schema
//     series    is optional and is a valid series name
//     name      is mandatory and is the name of the charm
//     revision  is optional and can be -1 if revision is unset

var (
	// SeriesSnippet is a regular expression representing series
	SeriesSnippet = "[a-z]+([a-z0-9]+)?"

	// CharmNameSnippet is a regular expression representing charm name
	CharmNameSnippet = "[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*"

	localSchemaSnippet      = "local:"
	charmStoreSchemaSnippet = "cs:(~" + validUserPart + "/)?"
	revisionSnippet         = "(-1|0|[1-9][0-9]*)"

	validCharmRegEx = regexp.MustCompile("^(" +
		localSchemaSnippet + "|" +
		charmStoreSchemaSnippet + ")?(" +
		SeriesSnippet + "/)?" +
		CharmNameSnippet + "(-" +
		revisionSnippet + ")?$")
)

// CharmTag represents tag for charm
// using charm's URL
type CharmTag struct {
	url string
}

// String satisfies Tag interface.
// Produces string representation of charm tag.
func (t CharmTag) String() string { return t.Kind() + "-" + t.Id() }

// Kind satisfies Tag interface.
// Returns Charm tag kind.
func (t CharmTag) Kind() string { return CharmTagKind }

// Id satisfies Tag interface.
// Returns charm URL.
func (t CharmTag) Id() string { return t.url }

// NewCharmTag returns the tag for the charm with the given url.
// It will panic if the given charm url is not valid.
func NewCharmTag(charmURL string) CharmTag {
	if !IsValidCharm(charmURL) {
		panic(fmt.Sprintf("%q is not a valid charm name", charmURL))
	}
	return CharmTag{url: charmURL}
}

var emptyTag = CharmTag{}

// ParseCharmTag parses a charm tag string.
func ParseCharmTag(charmTag string) (CharmTag, error) {
	tag, err := ParseTag(charmTag)
	if err != nil {
		return emptyTag, err
	}
	ct, ok := tag.(CharmTag)
	if !ok {
		return emptyTag, invalidTagError(charmTag, CharmTagKind)
	}
	return ct, nil
}

// IsValidCharm returns whether name is a valid charm url.
func IsValidCharm(url string) bool {
	return validCharmRegEx.MatchString(url)
}
