// Copyright 2012-2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"encoding/json"
	"errors"
	"fmt"
)

// JSONObject is a wrapper around a JSON structure which provides
// methods to extract data from that structure.
// A JSONObject provides a simple structure consisting of the data types
// defined in JSON: string, number, object, list, and bool.  To get the
// value you want out of a JSONObject, you must know (or figure out) which
// kind of value you have, and then call the appropriate Get*() method to
// get at it.  Reading an item as the wrong type will return an error.
// For instance, if your JSONObject consists of a number, call GetFloat64()
// to get the value as a float64.  If it's a list, call GetArray() to get
// a slice of JSONObjects.  To read any given item from the slice, you'll
// need to "Get" that as the right type as well.
// There is one exception: a MAASObject is really a special kind of map,
// so you can read it as either.
// Reading a null item is also an error.  So before you try obj.Get*(),
// first check obj.IsNil().
type JSONObject struct {
	// Parsed value.  May actually be any of the types a JSONObject can
	// wrap, except raw bytes.  If the object can only be interpreted
	// as raw bytes, this will be nil.
	value interface{}
	// Raw bytes, if this object was parsed directly from an API response.
	// Is nil for sub-objects found within other objects.  An object that
	// was parsed directly from a response can be both raw bytes and some
	// other value at the same time.
	// For example, "[]" looks like a JSON list, so you can read it as an
	// array.  But it may also be the raw contents of a file that just
	// happens to look like JSON, and so you can read it as raw bytes as
	// well.
	bytes []byte
	// Client for further communication with the API.
	client Client
	// Is this a JSON null?
	isNull bool
}

// Our JSON processor distinguishes a MAASObject from a jsonMap by the fact
// that it contains a key "resource_uri".  (A regular map might contain the
// same key through sheer coincide, but never mind: you can still treat it
// as a jsonMap and never notice the difference.)
const resourceURI = "resource_uri"

// maasify turns a completely untyped json.Unmarshal result into a JSONObject
// (with the appropriate implementation of course).  This function is
// recursive.  Maps and arrays are deep-copied, with each individual value
// being converted to a JSONObject type.
func maasify(client Client, value interface{}) JSONObject {
	if value == nil {
		return JSONObject{isNull: true}
	}
	switch value.(type) {
	case string, float64, bool:
		return JSONObject{value: value}
	case map[string]interface{}:
		original := value.(map[string]interface{})
		result := make(map[string]JSONObject, len(original))
		for key, value := range original {
			result[key] = maasify(client, value)
		}
		return JSONObject{value: result, client: client}
	case []interface{}:
		original := value.([]interface{})
		result := make([]JSONObject, len(original))
		for index, value := range original {
			result[index] = maasify(client, value)
		}
		return JSONObject{value: result}
	}
	msg := fmt.Sprintf("Unknown JSON type, can't be converted to JSONObject: %v", value)
	panic(msg)
}

// Parse a JSON blob into a JSONObject.
func Parse(client Client, input []byte) (JSONObject, error) {
	var obj JSONObject
	if input == nil {
		panic(errors.New("Parse() called with nil input"))
	}
	var parsed interface{}
	err := json.Unmarshal(input, &parsed)
	if err == nil {
		obj = maasify(client, parsed)
		obj.bytes = input
	} else {
		switch err.(type) {
		case *json.InvalidUTF8Error:
		case *json.SyntaxError:
			// This isn't JSON.  Treat it as raw binary data.
		default:
			return obj, err
		}
		obj = JSONObject{value: nil, client: client, bytes: input}
	}
	return obj, nil
}

// JSONObjectFromStruct takes a struct and converts it to a JSONObject
func JSONObjectFromStruct(client Client, input interface{}) (JSONObject, error) {
	j, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return JSONObject{}, err
	}
	return Parse(client, j)
}

// Return error value for failed type conversion.
func failConversion(wantedType string, obj JSONObject) error {
	msg := fmt.Sprintf("Requested %v, got %T.", wantedType, obj.value)
	return errors.New(msg)
}

// MarshalJSON tells the standard json package how to serialize a JSONObject
// back to JSON.
func (obj JSONObject) MarshalJSON() ([]byte, error) {
	if obj.IsNil() {
		return json.Marshal(nil)
	}
	return json.MarshalIndent(obj.value, "", "  ")
}

// With MarshalJSON, JSONObject implements json.Marshaler.
var _ json.Marshaler = (*JSONObject)(nil)

// IsNil tells you whether a JSONObject is a JSON "null."
// There is one irregularity.  If the original JSON blob was actually raw
// data, not JSON, then its IsNil will return false because the object
// contains the binary data as a non-nil value.  But, if the original JSON
// blob consisted of a null, then IsNil returns true even though you can
// still retrieve binary data from it.
func (obj JSONObject) IsNil() bool {
	if obj.value != nil {
		return false
	}
	if obj.bytes == nil {
		return true
	}
	// This may be a JSON null.  We can't expect every JSON null to look
	// the same; there may be leading or trailing space.
	return obj.isNull
}

// GetString retrieves the object's value as a string.  If the value wasn't
// a JSON string, that's an error.
func (obj JSONObject) GetString() (value string, err error) {
	value, ok := obj.value.(string)
	if !ok {
		err = failConversion("string", obj)
	}
	return
}

// GetFloat64 retrieves the object's value as a float64.  If the value wasn't
// a JSON number, that's an error.
func (obj JSONObject) GetFloat64() (value float64, err error) {
	value, ok := obj.value.(float64)
	if !ok {
		err = failConversion("float64", obj)
	}
	return
}

// GetMap retrieves the object's value as a map.  If the value wasn't a JSON
// object, that's an error.
func (obj JSONObject) GetMap() (value map[string]JSONObject, err error) {
	value, ok := obj.value.(map[string]JSONObject)
	if !ok {
		err = failConversion("map", obj)
	}
	return
}

// GetArray retrieves the object's value as an array.  If the value wasn't a
// JSON list, that's an error.
func (obj JSONObject) GetArray() (value []JSONObject, err error) {
	value, ok := obj.value.([]JSONObject)
	if !ok {
		err = failConversion("array", obj)
	}
	return
}

// GetBool retrieves the object's value as a bool.  If the value wasn't a JSON
// bool, that's an error.
func (obj JSONObject) GetBool() (value bool, err error) {
	value, ok := obj.value.(bool)
	if !ok {
		err = failConversion("bool", obj)
	}
	return
}

// GetBytes retrieves the object's value as raw bytes.  A JSONObject that was
// parsed from the original input (as opposed to one that's embedded in
// another JSONObject) can contain both the raw bytes and the parsed JSON
// value, but either can be the case without the other.
// If this object wasn't parsed directly from the original input, that's an
// error.
// If the object was parsed from an original input that just said "null", then
// IsNil will return true but the raw bytes are still available from GetBytes.
func (obj JSONObject) GetBytes() ([]byte, error) {
	if obj.bytes == nil {
		return nil, failConversion("bytes", obj)
	}
	return obj.bytes, nil
}
