// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	StorageTagKind = "storage"

	// StorageNameSnippet is the regular expression that describes valid
	// storage names (without the storage instance sequence number).
	StorageNameSnippet = "(?:[a-z][a-z0-9]*(?:-[a-z0-9]*[a-z][a-z0-9]*)*)"
)

var validStorage = regexp.MustCompile("^(" + StorageNameSnippet + ")/" + NumberSnippet + "$")

type StorageTag struct {
	id string
}

func (t StorageTag) String() string { return t.Kind() + "-" + t.id }
func (t StorageTag) Kind() string   { return StorageTagKind }
func (t StorageTag) Id() string     { return storageTagSuffixToId(t.id) }

// NewStorageTag returns the tag for the storage instance with the given ID.
// It will panic if the given string is not a valid storage instance Id.
func NewStorageTag(id string) StorageTag {
	tag, ok := tagFromStorageId(id)
	if !ok {
		panic(fmt.Sprintf("%q is not a valid storage instance ID", id))
	}
	return tag
}

// ParseStorageTag parses a storage tag string.
func ParseStorageTag(s string) (StorageTag, error) {
	tag, err := ParseTag(s)
	if err != nil {
		return StorageTag{}, err
	}
	st, ok := tag.(StorageTag)
	if !ok {
		return StorageTag{}, invalidTagError(s, StorageTagKind)
	}
	return st, nil
}

// IsValidStorage returns whether id is a valid storage instance ID.
func IsValidStorage(id string) bool {
	return validStorage.MatchString(id)
}

// StorageName returns the storage name from a storage instance ID.
// StorageName returns an error if "id" is not a valid storage
// instance ID.
func StorageName(id string) (string, error) {
	s := validStorage.FindStringSubmatch(id)
	if s == nil {
		return "", fmt.Errorf("%q is not a valid storage instance ID", id)
	}
	return s[1], nil
}

func tagFromStorageId(id string) (StorageTag, bool) {
	// replace only the last "/" with "-".
	i := strings.LastIndex(id, "/")
	if i <= 0 || !IsValidStorage(id) {
		return StorageTag{}, false
	}
	id = id[:i] + "-" + id[i+1:]
	return StorageTag{id}, true
}

func storageTagSuffixToId(s string) string {
	// Replace only the last "-" with "/", as it is valid for storage
	// names to contain hyphens.
	if i := strings.LastIndex(s, "-"); i > 0 {
		s = s[:i] + "/" + s[i+1:]
	}
	return s
}
