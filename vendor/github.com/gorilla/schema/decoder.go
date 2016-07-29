// Copyright 2012 The Gorilla Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package schema

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// NewDecoder returns a new Decoder.
func NewDecoder() *Decoder {
	return &Decoder{cache: newCache()}
}

// Decoder decodes values from a map[string][]string to a struct.
type Decoder struct {
	cache             *cache
	zeroEmpty         bool
	ignoreUnknownKeys bool
}

// SetAliasTag changes the tag used to locate custom field aliases.
// The default tag is "schema".
func (d *Decoder) SetAliasTag(tag string) {
	d.cache.tag = tag
}

// ZeroEmpty controls the behaviour when the decoder encounters empty values
// in a map.
// If z is true and a key in the map has the empty string as a value
// then the corresponding struct field is set to the zero value.
// If z is false then empty strings are ignored.
//
// The default value is false, that is empty values do not change
// the value of the struct field.
func (d *Decoder) ZeroEmpty(z bool) {
	d.zeroEmpty = z
}

// IgnoreUnknownKeys controls the behaviour when the decoder encounters unknown
// keys in the map.
// If i is true and an unknown field is encountered, it is ignored. This is
// similar to how unknown keys are handled by encoding/json.
// If i is false then Decode will return an error. Note that any valid keys
// will still be decoded in to the target struct.
//
// To preserve backwards compatibility, the default value is false.
func (d *Decoder) IgnoreUnknownKeys(i bool) {
	d.ignoreUnknownKeys = i
}

// RegisterConverter registers a converter function for a custom type.
func (d *Decoder) RegisterConverter(value interface{}, converterFunc Converter) {
	d.cache.regconv[reflect.TypeOf(value)] = converterFunc
}

// Decode decodes a map[string][]string to a struct.
//
// The first parameter must be a pointer to a struct.
//
// The second parameter is a map, typically url.Values from an HTTP request.
// Keys are "paths" in dotted notation to the struct fields and nested structs.
//
// See the package documentation for a full explanation of the mechanics.
func (d *Decoder) Decode(dst interface{}, src map[string][]string) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return errors.New("schema: interface must be a pointer to struct")
	}
	v = v.Elem()
	t := v.Type()
	errors := MultiError{}
	for path, values := range src {
		if parts, err := d.cache.parsePath(path, t); err == nil {
			if err = d.decode(v, path, parts, values); err != nil {
				errors[path] = err
			}
		} else if !d.ignoreUnknownKeys {
			errors[path] = fmt.Errorf("schema: invalid path %q", path)
		}
	}
	if len(errors) > 0 {
		return errors
	}
	return nil
}

// decode fills a struct field using a parsed path.
func (d *Decoder) decode(v reflect.Value, path string, parts []pathPart, values []string) error {
	// Get the field walking the struct fields by index.
	for _, name := range parts[0].path {
		if v.Type().Kind() == reflect.Ptr {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.FieldByName(name)
	}

	// Don't even bother for unexported fields.
	if !v.CanSet() {
		return nil
	}

	// Dereference if needed.
	t := v.Type()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
		if v.IsNil() {
			v.Set(reflect.New(t))
		}
		v = v.Elem()
	}

	// Slice of structs. Let's go recursive.
	if len(parts) > 1 {
		idx := parts[0].index
		if v.IsNil() || v.Len() < idx+1 {
			value := reflect.MakeSlice(t, idx+1, idx+1)
			if v.Len() < idx+1 {
				// Resize it.
				reflect.Copy(value, v)
			}
			v.Set(value)
		}
		return d.decode(v.Index(idx), path, parts[1:], values)
	}

	// Get the converter early in case there is one for a slice type.
	conv := d.cache.converter(t)
	if conv == nil && t.Kind() == reflect.Slice {
		var items []reflect.Value
		elemT := t.Elem()
		isPtrElem := elemT.Kind() == reflect.Ptr
		if isPtrElem {
			elemT = elemT.Elem()
		}

		// Try to get a converter for the element type.
		conv := d.cache.converter(elemT)
		if conv == nil {
			// As we are not dealing with slice of structs here, we don't need to check if the type
			// implements TextUnmarshaler interface
			return fmt.Errorf("schema: converter not found for %v", elemT)
		}

		for key, value := range values {
			if value == "" {
				if d.zeroEmpty {
					items = append(items, reflect.Zero(elemT))
				}
			} else if item := conv(value); item.IsValid() {
				if isPtrElem {
					ptr := reflect.New(elemT)
					ptr.Elem().Set(item)
					item = ptr
				}
				if item.Type() != elemT && !isPtrElem {
					item = item.Convert(elemT)
				}
				items = append(items, item)
			} else {
				if strings.Contains(value, ",") {
					values := strings.Split(value, ",")
					for _, value := range values {
						if value == "" {
							if d.zeroEmpty {
								items = append(items, reflect.Zero(elemT))
							}
						} else if item := conv(value); item.IsValid() {
							if isPtrElem {
								ptr := reflect.New(elemT)
								ptr.Elem().Set(item)
								item = ptr
							}
							if item.Type() != elemT && !isPtrElem {
								item = item.Convert(elemT)
							}
							items = append(items, item)
						} else {
							return ConversionError{
								Key:   path,
								Type:  elemT,
								Index: key,
							}
						}
					}
				} else {
					return ConversionError{
						Key:   path,
						Type:  elemT,
						Index: key,
					}
				}
			}
		}
		value := reflect.Append(reflect.MakeSlice(t, 0, 0), items...)
		v.Set(value)
	} else {
		val := ""
		// Use the last value provided if any values were provided
		if len(values) > 0 {
			val = values[len(values)-1]
		}

		if val == "" {
			if d.zeroEmpty {
				v.Set(reflect.Zero(t))
			}
		} else if conv != nil {
			if value := conv(val); value.IsValid() {
				v.Set(value.Convert(t))
			} else {
				return ConversionError{
					Key:   path,
					Type:  t,
					Index: -1,
				}
			}
		} else {
			// When there's no registered conversion for the custom type, we will check if the type
			// implements the TextUnmarshaler interface. As the UnmarshalText function should be applied
			// to the pointer of the type, we convert the value to pointer.
			if v.CanAddr() {
				v = v.Addr()
			}

			if u, ok := v.Interface().(encoding.TextUnmarshaler); ok {
				if err := u.UnmarshalText([]byte(val)); err != nil {
					return ConversionError{
						Key:   path,
						Type:  t,
						Index: -1,
						Err:   err,
					}
				}

			} else {
				return fmt.Errorf("schema: converter not found for %v", t)
			}
		}
	}
	return nil
}

// Errors ---------------------------------------------------------------------

// ConversionError stores information about a failed conversion.
type ConversionError struct {
	Key   string       // key from the source map.
	Type  reflect.Type // expected type of elem
	Index int          // index for multi-value fields; -1 for single-value fields.
	Err   error        // low-level error (when it exists)
}

func (e ConversionError) Error() string {
	var output string

	if e.Index < 0 {
		output = fmt.Sprintf("schema: error converting value for %q", e.Key)
	} else {
		output = fmt.Sprintf("schema: error converting value for index %d of %q",
			e.Index, e.Key)
	}

	if e.Err != nil {
		output = fmt.Sprintf("%s. Details: %s", output, e.Err)
	}

	return output
}

// MultiError stores multiple decoding errors.
//
// Borrowed from the App Engine SDK.
type MultiError map[string]error

func (e MultiError) Error() string {
	s := ""
	for _, err := range e {
		s = err.Error()
		break
	}
	switch len(e) {
	case 0:
		return "(0 errors)"
	case 1:
		return s
	case 2:
		return s + " (and 1 other error)"
	}
	return fmt.Sprintf("%s (and %d other errors)", s, len(e)-1)
}
