// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
)

var alphaChar = regexp.MustCompile("[A-Z]")
var tailNumberSuffix = regexp.MustCompile("-[0-9]+$")

// ValidateApplicationName takes a name and attempts to validate the application
// name, before returning a reason why it's not valid.
func ValidateApplicationName(name string) error {
	if !names.IsValidApplication(name) {
		// If the application has uppercase characters, bail out and explain
		// why.
		if alphaChar.MatchString(name) {
			return errors.Errorf("invalid application name %q, unexpected uppercase character", name)
		}
		// If the application ends up being suffixed by a number, then we want
		// to mention it to users why.
		if tailNumberSuffix.MatchString(name) {
			return errors.Errorf("invalid application name %q, unexpected number(s) found after hyphen", name)
		}

		index := strings.IndexFunc(name, validApplicationNameRune)
		if index < 0 {
			return errors.Errorf("invalid application name %q", name)
		}

		// We have to ensure that we don't break up multi-rune characters, by
		// just selecting the index. Instead look at a slice of runes and use
		// the first one.
		invalidRune := []rune(name[index:])[0]
		return errors.Errorf("invalid application name %q, unexpected character %c", name, invalidRune)
	}

	return nil
}

// validApplicationNameRune works out if there is a valid application rune.
func validApplicationNameRune(r rune) bool {
	if (r >= 'a' && r <= 'z') || unicode.IsNumber(r) || r == '-' {
		return false
	}
	return true
}
