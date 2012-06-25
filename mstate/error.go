package mstate

import (
	"errors"
	"fmt"
)

// errorContextf prefixes any error stored in err with text formatted
// according to the format specifier.  If err does not contain an error,
// errorContextf does nothing.
func errorContextf(err *error, format string, args ...interface{}) {
	if *err != nil {
		*err = errors.New(fmt.Sprintf(format, args...) + ": " + (*err).Error())
	}
}
