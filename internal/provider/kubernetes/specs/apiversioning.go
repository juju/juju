// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"bytes"
	"reflect"
)

// APIVersion defines the k8s API version to use.
type APIVersion string

func unmarshalJSONStrict(value []byte, into interface{}) error {
	err := newStrictYAMLOrJSONDecoder(bytes.NewReader(value), len(value)).Decode(into)
	if err != nil {
		v := reflect.ValueOf(into)
		v.Elem().Set(reflect.Zero(v.Elem().Type()))
	}
	return err
}
