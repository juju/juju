// Copyright 2012 The Gorilla Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package schema

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

var invalidPath = errors.New("schema: invalid path")

// newCache returns a new cache.
func newCache() *cache {
	c := cache{
		m:       make(map[reflect.Type]*structInfo),
		conv:    make(map[reflect.Kind]Converter),
		regconv: make(map[reflect.Type]Converter),
		tag:     "schema",
	}
	for k, v := range converters {
		c.conv[k] = v
	}
	return &c
}

// cache caches meta-data about a struct.
type cache struct {
	l       sync.RWMutex
	m       map[reflect.Type]*structInfo
	conv    map[reflect.Kind]Converter
	regconv map[reflect.Type]Converter
	tag     string
}

// parsePath parses a path in dotted notation verifying that it is a valid
// path to a struct field.
//
// It returns "path parts" which contain indices to fields to be used by
// reflect.Value.FieldByString(). Multiple parts are required for slices of
// structs.
func (c *cache) parsePath(p string, t reflect.Type) ([]pathPart, error) {
	var struc *structInfo
	var field *fieldInfo
	var index64 int64
	var err error
	parts := make([]pathPart, 0)
	path := make([]string, 0)
	keys := strings.Split(p, ".")
	for i := 0; i < len(keys); i++ {
		if t.Kind() != reflect.Struct {
			return nil, invalidPath
		}
		if struc = c.get(t); struc == nil {
			return nil, invalidPath
		}
		if field = struc.get(keys[i]); field == nil {
			return nil, invalidPath
		}
		// Valid field. Append index.
		path = append(path, field.name)
		if field.ss {
			// Parse a special case: slices of structs.
			// i+1 must be the slice index.
			//
			// Now that struct can implements TextUnmarshaler interface,
			// we don't need to force the struct's fields to appear in the path.
			// So checking i+2 is not necessary anymore.
			i++
			if i+1 > len(keys) {
				return nil, invalidPath
			}
			if index64, err = strconv.ParseInt(keys[i], 10, 0); err != nil {
				return nil, invalidPath
			}
			parts = append(parts, pathPart{
				path:  path,
				field: field,
				index: int(index64),
			})
			path = make([]string, 0)

			// Get the next struct type, dropping ptrs.
			if field.typ.Kind() == reflect.Ptr {
				t = field.typ.Elem()
			} else {
				t = field.typ
			}
			if t.Kind() == reflect.Slice {
				t = t.Elem()
				if t.Kind() == reflect.Ptr {
					t = t.Elem()
				}
			}
		} else if field.typ.Kind() == reflect.Ptr {
			t = field.typ.Elem()
		} else {
			t = field.typ
		}
	}
	// Add the remaining.
	parts = append(parts, pathPart{
		path:  path,
		field: field,
		index: -1,
	})
	return parts, nil
}

// get returns a cached structInfo, creating it if necessary.
func (c *cache) get(t reflect.Type) *structInfo {
	c.l.RLock()
	info := c.m[t]
	c.l.RUnlock()
	if info == nil {
		info = c.create(t, nil)
		c.l.Lock()
		c.m[t] = info
		c.l.Unlock()
	}
	return info
}

// create creates a structInfo with meta-data about a struct.
func (c *cache) create(t reflect.Type, info *structInfo) *structInfo {
	if info == nil {
		info = &structInfo{fields: []*fieldInfo{}}
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Anonymous {
			ft := field.Type
			if ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				c.create(ft, info)
			}
		}
		c.createField(field, info)
	}
	return info
}

// createField creates a fieldInfo for the given field.
func (c *cache) createField(field reflect.StructField, info *structInfo) {
	alias := fieldAlias(field, c.tag)
	if alias == "-" {
		// Ignore this field.
		return
	}
	// Check if the type is supported and don't cache it if not.
	// First let's get the basic type.
	isSlice, isStruct := false, false
	ft := field.Type
	if ft.Kind() == reflect.Ptr {
		ft = ft.Elem()
	}
	if isSlice = ft.Kind() == reflect.Slice; isSlice {
		ft = ft.Elem()
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
	}
	if ft.Kind() == reflect.Array {
		ft = ft.Elem()
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
	}
	if isStruct = ft.Kind() == reflect.Struct; !isStruct {
		if conv := c.conv[ft.Kind()]; conv == nil {
			// Type is not supported.
			return
		}
	}

	info.fields = append(info.fields, &fieldInfo{
		typ:   field.Type,
		name:  field.Name,
		ss:    isSlice && isStruct,
		alias: alias,
	})
}

// converter returns the converter for a type.
func (c *cache) converter(t reflect.Type) Converter {
	conv := c.regconv[t]
	if conv == nil {
		conv = c.conv[t.Kind()]
	}
	return conv
}

// ----------------------------------------------------------------------------

type structInfo struct {
	fields []*fieldInfo
}

func (i *structInfo) get(alias string) *fieldInfo {
	for _, field := range i.fields {
		if strings.EqualFold(field.alias, alias) {
			return field
		}
	}
	return nil
}

type fieldInfo struct {
	typ   reflect.Type
	name  string // field name in the struct.
	ss    bool   // true if this is a slice of structs.
	alias string
}

type pathPart struct {
	field *fieldInfo
	path  []string // path to the field: walks structs using field names.
	index int      // struct index in slices of structs.
}

// ----------------------------------------------------------------------------

// fieldAlias parses a field tag to get a field alias.
func fieldAlias(field reflect.StructField, tagName string) string {
	var alias string
	if tag := field.Tag.Get(tagName); tag != "" {
		// For now tags only support the name but let's follow the
		// comma convention from encoding/json and others.
		if idx := strings.Index(tag, ","); idx == -1 {
			alias = tag
		} else {
			alias = tag[:idx]
		}
	}
	if alias == "" {
		alias = field.Name
	}
	return alias
}
