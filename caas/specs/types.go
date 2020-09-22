// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// IntOrString is a type that can hold an int32 or a string.  When used in
// JSON or YAML marshalling and unmarshalling, it produces or consumes the
// inner type.  This allows you to have, for example, a JSON field that can
// accept a name or number.
// It is ported from k8s.io/apimachinery/pkg/util/intstr/intstr.go to avoid introducing k8s dependency in top level package.
type IntOrString struct {
	Type   Type   `json:"type"`
	IntVal int32  `json:"intVal,omitempty"`
	StrVal string `json:"strVal,omitempty"`
}

// Type represents the stored type of IntOrString.
type Type int64

const (
	// Int indicates the IntOrString holds an int.
	Int Type = iota
	// String indicates the IntOrString holds a string.
	String
)

// UnmarshalJSON implements the json.Unmarshaller interface.
func (intstr *IntOrString) UnmarshalJSON(value []byte) error {
	if value[0] == '"' {
		intstr.Type = String
		return json.Unmarshal(value, &intstr.StrVal)
	}
	intstr.Type = Int
	return json.Unmarshal(value, &intstr.IntVal)
}

// String returns the string value, or the Itoa of the int value.
func (intstr *IntOrString) String() string {
	if intstr.Type == String {
		return intstr.StrVal
	}
	return strconv.Itoa(intstr.IntValue())
}

// IntValue returns the IntVal if type Int, or if
// it is a String, will attempt a conversion to int.
func (intstr *IntOrString) IntValue() int {
	if intstr.Type == String {
		i, _ := strconv.Atoi(intstr.StrVal)
		return i
	}
	return int(intstr.IntVal)
}

// MarshalJSON implements the json.Marshaller interface.
func (intstr IntOrString) MarshalJSON() ([]byte, error) {
	switch intstr.Type {
	case Int:
		return json.Marshal(intstr.IntVal)
	case String:
		return json.Marshal(intstr.StrVal)
	default:
		return []byte{}, fmt.Errorf("impossible IntOrString.Type")
	}
}
