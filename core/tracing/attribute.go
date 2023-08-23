// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracing

// Attribute allows you to add additional information to help identify
// an operation (event, error or span end).
type Attribute interface {
	Key() string
	Value() string
}

// StringAttribute defines an attribute with a string value.
type StringAttribute struct {
	key, value string
}

// StringAttr creates a StringAttribute.
func StringAttr(key, value string) StringAttribute {
	return StringAttribute{key: key, value: value}
}

// Key defines the identifier for the attribute.
func (a StringAttribute) Key() string {
	return a.key
}

// Value returns a string.
func (a StringAttribute) Value() string {
	return a.value
}
