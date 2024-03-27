// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package access

import (
	"regexp"

	"github.com/juju/errors"

	usererrors "github.com/juju/juju/domain/access/errors"
)

const (
	// usernameValidationRegex is the regex used to validate that user names are
	// valid for consumption by Juju. User names must be 1 or more runes long,
	// can contain any unicode rune from the letter/number class and may contain
	// zero or more of .,+ or - runes as long as they don't appear at the
	// start or end of the user name. User names can be a maximum of 255
	// characters long.
	usernameValidationRegex = "^([\\pL\\pN]|[\\pL\\pN][\\pL\\pN.+-]{0,253}[\\pL\\pN])$"
)

var (
	// validUserName is a compiled regex that is used to validate that a user
	validUserName = regexp.MustCompile(usernameValidationRegex)
)

// ValidateUserName takes a user name and validates that it is
// conformant to our regex rules defined in usernameValidationRegex.
// If a user name is not valid, an error is returned that satisfies
// usererrors.UserNameNotValid.
//
// User names must be one or more runes long, can contain any unicode rune from
// the letter or number class and may contain zero or more of .,+ or - runes as
// long as they don't appear at the start or end of the user name. User names can
// be a maximum length of 255 characters.
func ValidateUserName(name string) error {
	if !validUserName.MatchString(name) {
		return errors.Annotatef(usererrors.UserNameNotValid, "%q", name)
	}
	return nil
}
