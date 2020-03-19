// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azuretesting

import (
	"encoding/json"
	"reflect"
)

// JsonMarshalRaw does the same a json.Marshal, except that it does not
// use the MarshalJSON methods of the value being marshaled. If any types
// are specified in the allow set then those will use their MarshalJSON
// method.
//
// Many of the types in the Azure SDK have MarshalJSON which skip over
// fields that are marked as READ-ONLY, this is useful for the client,
// but a problem when pretending to be a server.
func JsonMarshalRaw(v interface{}, allow ...reflect.Type) ([]byte, error) {
	allowed := make(map[reflect.Type]bool, len(allow))
	for _, a := range allow {
		allowed[a] = true
	}
	if v != nil {
		v = rawValueMaker{allowed}.rawValue(reflect.ValueOf(v)).Interface()
	}
	return json.Marshal(v)
}

type rawValueMaker struct {
	allowed map[reflect.Type]bool
}

func (m rawValueMaker) rawValue(v reflect.Value) reflect.Value {
	t := v.Type()
	if m.allowed[t] {
		return v
	}
	switch t.Kind() {
	case reflect.Ptr:
		return m.rawPointerValue(v)
	case reflect.Struct:
		return m.rawStructValue(v)
	case reflect.Map:
		return m.rawMapValue(v)
	case reflect.Slice:
		return m.rawSliceValue(v)
	default:
		return v
	}
}

func (m rawValueMaker) rawPointerValue(v reflect.Value) reflect.Value {
	if v.IsNil() {
		return v
	}
	rv := m.rawValue(v.Elem())
	if rv.CanAddr() {
		return rv.Addr()
	}
	pv := reflect.New(rv.Type())
	pv.Elem().Set(rv)
	return pv
}

func (m rawValueMaker) rawStructValue(v reflect.Value) reflect.Value {
	t := v.Type()

	fields := make([]reflect.StructField, 0, t.NumField())
	values := make([]reflect.Value, 0, t.NumField())

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" || sf.Tag.Get("json") == "-" {
			// Skip fields that won't ever be marshaled.
			continue
		}
		if tag, ok := sf.Tag.Lookup("json"); ok && tag == "" {
			// Also skip fields with a present, but empty, json tag.
			continue
		}
		rv := m.rawValue(v.Field(i))
		sf.Type = rv.Type()
		fields = append(fields, sf)
		values = append(values, rv)
	}

	newT := reflect.StructOf(fields)
	newV := reflect.New(newT).Elem()

	for i, v := range values {
		newV.Field(i).Set(v)
	}

	return newV
}

var interfaceType = reflect.TypeOf((*interface{})(nil)).Elem()

func (m rawValueMaker) rawMapValue(v reflect.Value) reflect.Value {
	newV := reflect.MakeMap(reflect.MapOf(v.Type().Key(), interfaceType))
	for _, key := range v.MapKeys() {
		value := v.MapIndex(key)
		newV.SetMapIndex(key, m.rawValue(value))
	}

	return newV
}

func (m rawValueMaker) rawSliceValue(v reflect.Value) reflect.Value {
	newV := reflect.MakeSlice(reflect.SliceOf(interfaceType), v.Len(), v.Len())
	for i := 0; i < v.Len(); i++ {
		newV.Index(i).Set(m.rawValue(v.Index(i)))
	}
	return newV
}
