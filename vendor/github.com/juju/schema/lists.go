// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package schema

import (
	"reflect"
	"strconv"
)

// List returns a Checker that accepts a slice value with values
// that are processed with the elem checker.  If any element of the
// provided slice value fails to be processed, processing will stop
// and return with the obtained error.
//
// The coerced output value has type []interface{}.
func List(elem Checker) Checker {
	return listC{elem}
}

type listC struct {
	elem Checker
}

func (c listC) Coerce(v interface{}, path []string) (interface{}, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil, error_{"list", v, path}
	}

	path = append(path, "[", "?", "]")

	l := rv.Len()
	out := make([]interface{}, 0, l)
	for i := 0; i != l; i++ {
		path[len(path)-2] = strconv.Itoa(i)
		elem, err := c.elem.Coerce(rv.Index(i).Interface(), path)
		if err != nil {
			return nil, err
		}
		out = append(out, elem)
	}
	return out, nil
}
