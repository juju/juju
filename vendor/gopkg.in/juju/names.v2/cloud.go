// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"regexp"
)

const CloudTagKind = "cloud"

var (
	cloudSnippet = "[a-zA-Z0-9][a-zA-Z0-9.-]*"
	validCloud   = regexp.MustCompile("^" + cloudSnippet + "$")
)

type CloudTag struct {
	id string
}

func (t CloudTag) String() string { return t.Kind() + "-" + t.id }
func (t CloudTag) Kind() string   { return CloudTagKind }
func (t CloudTag) Id() string     { return t.id }

// NewCloudTag returns the tag for the cloud with the given ID.
// It will panic if the given cloud ID is not valid.
func NewCloudTag(id string) CloudTag {
	if !IsValidCloud(id) {
		panic(fmt.Sprintf("%q is not a valid cloud ID", id))
	}
	return CloudTag{id}
}

// ParseCloudTag parses a cloud tag string.
func ParseCloudTag(cloudTag string) (CloudTag, error) {
	tag, err := ParseTag(cloudTag)
	if err != nil {
		return CloudTag{}, err
	}
	dt, ok := tag.(CloudTag)
	if !ok {
		return CloudTag{}, invalidTagError(cloudTag, CloudTagKind)
	}
	return dt, nil
}

// IsValidCloud returns whether id is a valid cloud ID.
func IsValidCloud(id string) bool {
	return validCloud.MatchString(id)
}
