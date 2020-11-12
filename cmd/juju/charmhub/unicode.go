// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/juju/cmd/output"
	"golang.org/x/crypto/ssh/terminal"
)

// OSEnviron represents an interface around the os environment.
type OSEnviron interface {
	Getenv(string) string
	IsTerminal() bool
}

type defaultOSEnviron struct{}

func (defaultOSEnviron) Getenv(s string) string {
	return os.Getenv(s)
}

func (defaultOSEnviron) IsTerminal() bool {
	return terminal.IsTerminal(1)
}

// canUnicode is taken from the snapd equivalent. They have a battle tested
// version that is identical to this, but is not exported as a package or
// library for us to use. So it has been lifted in it's entirety here.
// https://github.com/snapcore/snapd/blob/master/cmd/snap/color.go
func canUnicode(mode string, os OSEnviron) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	}
	if !os.IsTerminal() {
		return false
	}

	var lang string
	for _, k := range []string{"LC_MESSAGES", "LC_ALL", "LANG"} {
		if lang = os.Getenv(k); lang != "" {
			break
		}
	}
	if lang == "" {
		return false
	}
	lang = strings.ToUpper(lang)
	return strings.Contains(lang, "UTF-8") || strings.Contains(lang, "UTF8")
}

// UnicodeCharIdent represents a type of character to print when using the
// writer.
type UnicodeCharIdent string

// UnicodeWriter allows the addition of printing unicode characters if
// available.
type UnicodeWriter struct {
	output.Wrapper
	unicodes map[UnicodeCharIdent]string
}

// PrintlnUnicode prints a unicode character from a map of stored unicodes
// available to the writer.
func (w *UnicodeWriter) PrintlnUnicode(char UnicodeCharIdent) (int, error) {
	value := string(char)
	if v, ok := w.unicodes[char]; ok {
		value = v
	}
	return fmt.Fprintln(w.Writer, value)
}
