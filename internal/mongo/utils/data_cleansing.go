// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import "strings"

// EscapeKeys is used to escape bad keys in a map. A statusDoc without
// escaped keys is broken.
func EscapeKeys(input map[string]interface{}) map[string]interface{} {
	return mapKeys(escapeReplacer.Replace, input)
}

// UnescapeKeys is used to restore escaped keys from a map to their
// original values.
func UnescapeKeys(input map[string]interface{}) map[string]interface{} {
	return mapKeys(unescapeReplacer.Replace, input)
}

// EscapeKey escapes a string to be safe to store in Mongo as a document key.
func EscapeKey(s string) string {
	return escapeReplacer.Replace(s)
}

// UnescapeKey restores escaped characters from a key to their
// original values.
func UnescapeKey(s string) string {
	return unescapeReplacer.Replace(s)
}

// See: http://docs.mongodb.org/manual/faq/developers/#faq-dollar-sign-escaping
// for why we're using those replacements.
const (
	fullWidthDot    = "\uff0e"
	fullWidthDollar = "\uff04"
)

var (
	escapeReplacer   = strings.NewReplacer(".", fullWidthDot, "$", fullWidthDollar)
	unescapeReplacer = strings.NewReplacer(fullWidthDot, ".", fullWidthDollar, "$")
)

// mapKeys returns a copy of the supplied map, with all nested map[string]interface{}
// keys transformed by f. All other types are ignored.
func mapKeys(f func(string) string, input map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range input {
		if submap, ok := value.(map[string]interface{}); ok {
			value = mapKeys(f, submap)
		}
		result[f(key)] = value
	}
	return result
}
