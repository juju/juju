// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"reflect"

	"github.com/juju/errors"
)

// Ord represents a ordered datatype.
type Ord interface {
	// Less checks if a Ord is less than another Ord
	Less(Ord) bool

	// Equal checks if an Ord is equal to another Ord.
	Equal(Ord) bool

	// IsZero returns if the underlying value is zero.
	IsZero() bool

	// Value defines the shadow type value of the Ord.
	Value() interface{}
}

// OrdInteger defines an ordered integer.
type OrdInteger struct {
	value int64
}

// NewInteger creates a new Ord value
func NewInteger(value int64) *OrdInteger {
	return &OrdInteger{value: value}
}

// Less checks if a OrdInteger is less than another OrdInteger.
func (o *OrdInteger) Less(other Ord) bool {
	if i, ok := other.(*OrdInteger); ok {
		return o.value < i.value
	}
	return false
}

// Equal checks if an OrdInteger is equal to another OrdInteger.
func (o *OrdInteger) Equal(other Ord) bool {
	if i, ok := other.(*OrdInteger); ok {
		return o.value == i.value
	}
	return false
}

// IsZero returns if the underlying value is zero.
func (o *OrdInteger) IsZero() bool {
	return o.value < 1
}

// Value defines the shadow type value of the Ord.
func (o *OrdInteger) Value() interface{} {
	return o.value
}

// OrdFloat defines an ordered float.
type OrdFloat struct {
	value float64
}

// NewFloat creates a new Ord value
func NewFloat(value float64) *OrdFloat {
	return &OrdFloat{value: value}
}

// Less checks if a OrdFloat is less than another OrdFloat.
func (o *OrdFloat) Less(other Ord) bool {
	if i, ok := other.(*OrdFloat); ok {
		return o.value < i.value
	}
	return false
}

// Equal checks if an OrdFloat is equal to another OrdFloat.
func (o *OrdFloat) Equal(other Ord) bool {
	if i, ok := other.(*OrdFloat); ok {
		return o.value == i.value
	}
	return false
}

// IsZero returns if the underlying value is zero.
func (o *OrdFloat) IsZero() bool {
	return o.value < 1
}

// Value defines the shadow type value of the Ord.
func (o *OrdFloat) Value() interface{} {
	return o.value
}

// OrdString defines an ordered string.
type OrdString struct {
	value string
}

// NewString creates a new Ord value
func NewString(value string) *OrdString {
	return &OrdString{value: value}
}

// Less checks if a OrdString is less than another OrdString.
func (o *OrdString) Less(other Ord) bool {
	if i, ok := other.(*OrdString); ok {
		return o.value < i.value
	}
	return false
}

// Equal checks if an OrdString is equal to another OrdString.
func (o *OrdString) Equal(other Ord) bool {
	if i, ok := other.(*OrdString); ok {
		return o.value == i.value
	}
	return false
}

// IsZero returns if the underlying value is zero.
func (o *OrdString) IsZero() bool {
	return o.value == ""
}

// Value defines the shadow type value of the Ord.
func (o *OrdString) Value() interface{} {
	return o.value
}

// OrdBool defines an ordered float.
type OrdBool struct {
	value bool
}

// NewBool creates a new Ord value
func NewBool(value bool) *OrdBool {
	return &OrdBool{value: value}
}

// Less checks if a OrdBool is less than another OrdBool.
func (o *OrdBool) Less(other Ord) bool {
	return false
}

// Equal checks if an OrdBool is equal to another OrdBool.
func (o *OrdBool) Equal(other Ord) bool {
	if i, ok := other.(*OrdBool); ok {
		return o.value == i.value
	}
	return false
}

// IsZero returns if the underlying value is zero.
func (o *OrdBool) IsZero() bool {
	return o.value == false
}

// Value defines the shadow type value of the Ord.
func (o *OrdBool) Value() interface{} {
	return o.value
}

// OrdMapStringInterface defines an ordered map[string]interface{}.
type OrdMapStringInterface struct {
	value map[string]interface{}
}

// NewMapStringInterface creates a new Ord value
func NewMapStringInterface(value map[string]interface{}) *OrdMapStringInterface {
	return &OrdMapStringInterface{value: value}
}

// Less checks if a OrdMapStringInterface is less than another OrdMapStringInterface.
func (o *OrdMapStringInterface) Less(other Ord) bool {
	return false
}

// Equal checks if an OrdMapStringInterface is equal to another OrdMapStringInterface.
func (o *OrdMapStringInterface) Equal(other Ord) bool {
	if i, ok := other.(*OrdMapStringInterface); ok {
		return reflect.DeepEqual(o.value, i.value)
	}
	return false
}

// IsZero returns if the underlying value is zero.
func (o *OrdMapStringInterface) IsZero() bool {
	return len(o.value) == 0
}

// Value defines the shadow type value of the Ord.
func (o *OrdMapStringInterface) Value() interface{} {
	return o.value
}

// OrdMapInterfaceInterface defines an ordered map[interface{}]interface{}.
type OrdMapInterfaceInterface struct {
	value map[interface{}]interface{}
}

// NewMapInterfaceInterface creates a new Ord value
func NewMapInterfaceInterface(value map[interface{}]interface{}) *OrdMapInterfaceInterface {
	return &OrdMapInterfaceInterface{value: value}
}

// Less checks if a OrdMapInterfaceInterface is less than another OrdMapInterfaceInterface.
func (o *OrdMapInterfaceInterface) Less(other Ord) bool {
	return false
}

// Equal checks if an OrdMapInterfaceInterface is equal to another OrdMapInterfaceInterface.
func (o *OrdMapInterfaceInterface) Equal(other Ord) bool {
	if i, ok := other.(*OrdMapInterfaceInterface); ok {
		return reflect.DeepEqual(o.value, i.value)
	}
	return false
}

// IsZero returns if the underlying value is zero.
func (o *OrdMapInterfaceInterface) IsZero() bool {
	return len(o.value) == 0
}

// Value defines the shadow type value of the Ord.
func (o *OrdMapInterfaceInterface) Value() interface{} {
	return o.value
}

func expectStringIndex(i interface{}) (*OrdString, error) {
	ord, ok := i.(Ord)
	if !ok {
		return nil, errors.Annotatef(ErrInvalidIndex(), "expected string, but got %T", i)
	}

	idx, ok := i.(*OrdString)
	if !ok {
		return nil, errors.Annotatef(ErrInvalidIndex(), "expected string, but got %v", shadowType(ord))
	}

	return idx, nil
}

func expectOrdIndex(i interface{}) (Ord, error) {
	ord, ok := i.(Ord)
	if !ok {
		return nil, errors.Annotatef(ErrInvalidIndex(), "expected ord, but got %T", i)
	}

	return ord, nil
}

func shadowType(ord Ord) string {
	switch ord.(type) {
	case *OrdBool:
		return "bool"
	case *OrdInteger:
		return "int64"
	case *OrdFloat:
		return "float64"
	case *OrdString:
		return "string"
	case *OrdMapInterfaceInterface:
		return "map[interface{}]interface{}"
	case *OrdMapStringInterface:
		return "map[string]interface{}"
	}
	return "<unknown>"
}
