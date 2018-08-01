package schema

import (
	"reflect"

	"github.com/pkg/errors"
)

func errInvalidType(s string, v interface{}) error {
	return errors.Errorf(
		"invalid type: expected %s, got %s",
		s,
		reflect.TypeOf(v).String(),
	)
}
