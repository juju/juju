// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"regexp"
)

const (
	UserTagKind     = "user"
	LocalUserDomain = "local"
)

var (
	// TODO this does not allow single character usernames or
	// domains. Is that deliberate?
	// https://github.com/juju/names/issues/54
	validUserNameSnippet = "[a-zA-Z0-9][a-zA-Z0-9.+-]*[a-zA-Z0-9]"
	validUserSnippet     = fmt.Sprintf("(?:%s(?:@%s)?)", validUserNameSnippet, validUserNameSnippet)
	validName            = regexp.MustCompile(fmt.Sprintf("^(?P<name>%s)(?:@(?P<domain>%s))?$", validUserNameSnippet, validUserNameSnippet))
	validUserName        = regexp.MustCompile("^" + validUserNameSnippet + "$")
)

// IsValidUser returns whether id is a valid user id.
// Valid users may or may not be qualified with an
// @domain suffix. Examples of valid users include
// bob, bob@local, bob@somewhere-else, 0-a-f@123.
func IsValidUser(id string) bool {
	return validName.MatchString(id)
}

// IsValidUserName returns whether the given
// name is a valid name part of a user. That is,
// usernames with a domain suffix will return
// false.
func IsValidUserName(name string) bool {
	return validUserName.MatchString(name)
}

// IsValidUserDomain returns whether the given user
// domain is valid.
func IsValidUserDomain(domain string) bool {
	return validUserName.MatchString(domain)
}

// UserTag represents a user that may be stored locally
// or associated with some external domain.
type UserTag struct {
	name   string
	domain string
}

func (t UserTag) Kind() string   { return UserTagKind }
func (t UserTag) String() string { return UserTagKind + "-" + t.Id() }

// Id implements Tag.Id. Local users will always have
// an Id value without any domain.
func (t UserTag) Id() string {
	if t.domain == "" || t.domain == LocalUserDomain {
		return t.name
	}
	return t.name + "@" + t.domain
}

// Name returns the name part of the user name
// without its associated domain.
func (t UserTag) Name() string { return t.name }

// IsLocal returns true if the tag represents a local user.
// Users without an explicit domain are considered local.
func (t UserTag) IsLocal() bool {
	return t.Domain() == LocalUserDomain || t.Domain() == ""
}

// Domain returns the user domain. Users in the local database
// are from the LocalDomain. Other users are considered 'remote' users.
func (t UserTag) Domain() string {
	return t.domain
}

// WithDomain returns a copy of the user tag with the
// domain changed to the given argument.
// The domain must satisfy IsValidUserDomain
// or this function will panic.
func (t UserTag) WithDomain(domain string) UserTag {
	if !IsValidUserDomain(domain) {
		panic(fmt.Sprintf("invalid user domain %q", domain))
	}
	return UserTag{
		name:   t.name,
		domain: domain,
	}
}

// NewUserTag returns the tag for the user with the given name.
// It panics if the user name does not satisfy IsValidUser.
func NewUserTag(userName string) UserTag {
	parts := validName.FindStringSubmatch(userName)
	if len(parts) != 3 {
		panic(fmt.Sprintf("invalid user tag %q", userName))
	}
	domain := parts[2]
	if domain == LocalUserDomain {
		domain = ""
	}
	return UserTag{name: parts[1], domain: domain}
}

// NewLocalUserTag returns the tag for a local user with the given name.
func NewLocalUserTag(name string) UserTag {
	if !IsValidUserName(name) {
		panic(fmt.Sprintf("invalid user name %q", name))
	}
	return UserTag{name: name}
}

// ParseUserTag parses a user tag string.
func ParseUserTag(tag string) (UserTag, error) {
	t, err := ParseTag(tag)
	if err != nil {
		return UserTag{}, err
	}
	ut, ok := t.(UserTag)
	if !ok {
		return UserTag{}, invalidTagError(tag, UserTagKind)
	}
	return ut, nil
}
