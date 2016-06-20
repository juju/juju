// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424

import (
	"fmt"
	"unicode/utf8"
)

func validatePrintUSASCII(val string, size int) error { // RFC 5234
	if len(val) > size {
		return fmt.Errorf("too big (max %d)", size)
	}
	for i, c := range val {
		if c < 32 || c > 126 {
			return fmt.Errorf("must be printable US ASCII (\\x%02x at pos %d)", c, i)
		}
	}
	return nil
}

func validateUTF8(val string) error {
	for len(val) > 0 {
		r, size := utf8.DecodeRuneInString(val)
		if r == utf8.RuneError {
			return fmt.Errorf("invalid UTF-8")
		}
		val = val[size:]
	}
	return nil
}
