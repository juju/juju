// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const CloudCredentialTagKind = "cloudcred"

var (
	cloudCredentialNameSnippet = "[a-zA-Z][a-zA-Z0-9.@_+-]*"
	validCloudCredentialName   = regexp.MustCompile("^" + cloudCredentialNameSnippet + "$")
	validCloudCredential       = regexp.MustCompile(
		"^" +
			"(" + cloudSnippet + ")" +
			"/(" + validUserSnippet + ")" + // credential owner
			"/(" + cloudCredentialNameSnippet + ")" +
			"$",
	)
)

type CloudCredentialTag struct {
	cloud CloudTag
	owner UserTag
	name  string
}

// IsZero reports whether t is zero.
func (t CloudCredentialTag) IsZero() bool {
	return t == CloudCredentialTag{}
}

// Kind is part of the Tag interface.
func (t CloudCredentialTag) Kind() string { return CloudCredentialTagKind }

// Id implements Tag.Id. It returns the empty string
// if t is zero.
func (t CloudCredentialTag) Id() string {
	if t.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", t.cloud.Id(), t.owner.Id(), t.name)
}

func quoteCredentialSeparator(in string) string {
	return strings.Replace(in, "_", `%5f`, -1)
}

// String implements Tag.String. It returns the empty
// string if t is zero.
func (t CloudCredentialTag) String() string {
	if t.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s-%s_%s_%s", t.Kind(),
		quoteCredentialSeparator(t.cloud.Id()),
		quoteCredentialSeparator(t.owner.Id()),
		quoteCredentialSeparator(t.name))
}

// Cloud returns the tag of the cloud to which the credential pertains.
func (t CloudCredentialTag) Cloud() CloudTag {
	return t.cloud
}

// Owner returns the tag of the user that owns the credential.
func (t CloudCredentialTag) Owner() UserTag {
	return t.owner
}

// Name returns the cloud credential name, excluding the
// cloud and owner qualifiers.
func (t CloudCredentialTag) Name() string {
	return t.name
}

// NewCloudCredentialTag returns the tag for the cloud with the given ID.
// It will panic if the given cloud ID is not valid.
func NewCloudCredentialTag(id string) CloudCredentialTag {
	parts := validCloudCredential.FindStringSubmatch(id)
	if len(parts) != 4 {
		panic(fmt.Sprintf("%q is not a valid cloud credential ID", id))
	}
	cloud := NewCloudTag(parts[1])
	owner := NewUserTag(parts[2])
	return CloudCredentialTag{cloud, owner, parts[3]}
}

// ParseCloudCredentialTag parses a cloud tag string.
func ParseCloudCredentialTag(s string) (CloudCredentialTag, error) {
	tag, err := ParseTag(s)
	if err != nil {
		return CloudCredentialTag{}, err
	}
	dt, ok := tag.(CloudCredentialTag)
	if !ok {
		return CloudCredentialTag{}, invalidTagError(s, CloudCredentialTagKind)
	}
	return dt, nil
}

// IsValidCloudCredential returns whether id is a valid cloud credential ID.
func IsValidCloudCredential(id string) bool {
	return validCloudCredential.MatchString(id)
}

// IsValidCloudCredentialName returns whether name is a valid cloud credential name.
func IsValidCloudCredentialName(name string) bool {
	return validCloudCredentialName.MatchString(name)
}

func cloudCredentialTagSuffixToId(s string) (string, error) {
	s = strings.Replace(s, "_", "/", -1)
	return url.QueryUnescape(s)
}
