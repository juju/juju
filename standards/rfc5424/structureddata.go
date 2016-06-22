// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424

import (
	"fmt"
	"strings"
)

// StructuredData holds the structured data of a log record, if any.
type StructuredData []StructuredDataElement

// String returns the RFC 5424 representation of the structured data.
func (sd StructuredData) String() string {
	if len(sd) == 0 {
		return "-"
	}

	elems := make([]string, len(sd))
	for i, elem := range sd {
		elems[i] = structuredDataElementString(elem)
	}
	return strings.Join(elems, "")
}

// Validate ensures that the structured data is correct.
func (sd StructuredData) Validate() error {
	for i, elem := range sd {
		if err := structuredDataElementValidate(elem); err != nil {
			return fmt.Errorf("element %d not valid: %v", i, err)
		}
	}
	return nil
}

// StructuredDataElement, AKA "SD-ELEMENT", provides the functionality
// that StructuredData needs from each of its elements.
type StructuredDataElement interface {
	// ID returns the "SD-ID" for the element.
	ID() StructuredDataName

	// Params returns all the elements items (if any), in order.
	Params() []StructuredDataParam

	// Validate ensures that the element is correct.
	Validate() error
}

func structuredDataElementString(sde StructuredDataElement) string {
	params := sde.Params()
	if len(params) == 0 {
		return fmt.Sprintf("[%s]", sde.ID())
	}

	paramStrs := make([]string, len(params))
	for i, param := range params {
		paramStrs[i] = param.String()
	}
	return fmt.Sprintf("[%s %s]", sde.ID(), strings.Join(paramStrs, " "))
}

func structuredDataElementValidate(sde StructuredDataElement) error {
	if err := sde.Validate(); err != nil {
		return err
	}

	id := sde.ID()
	if id == "" {
		return fmt.Errorf("empty ID")
	}
	if err := id.Validate(); err != nil {
		return fmt.Errorf("invalid ID %q: %v", id, err)
	}

	for i, param := range sde.Params() {
		if err := param.Validate(); err != nil {
			return fmt.Errorf("param %d not valid: %v", i, err)
		}
	}

	return nil
}

// StructuredDataName is a single name used in an element or its params.
type StructuredDataName string

// Validate ensures that the name is correct.
func (sdn StructuredDataName) Validate() error {
	if sdn == "" {
		return fmt.Errorf("empty name")
	}
	if strings.ContainsAny(string(sdn), `= ]"`) {
		return fmt.Errorf(`invalid character`)
	}
	return validatePrintUSASCII(string(sdn), 32)
}

// StructuredDataParam, AKA "SD-PARAM", is a single item in an element's list.
type StructuredDataParam struct {
	// Name identifies the item relative to an element. Note that an
	// element may have more than one item with the same name.
	Name StructuredDataName

	// Value is the value associated with the item.
	Value StructuredDataParamValue
}

// String returns the RFC 5424 representation of the item.
func (sdp StructuredDataParam) String() string {
	return fmt.Sprintf("%s=%q", sdp.Name, sdp.Value)
}

// Validated ensures that the item is correct.
func (sdp StructuredDataParam) Validate() error {
	if sdp.Name == "" {
		return fmt.Errorf("empty Name")
	}
	if err := sdp.Name.Validate(); err != nil {
		return fmt.Errorf("bad Name %q: %v", sdp.Name, err)
	}

	if err := sdp.Value.Validate(); err != nil {
		return fmt.Errorf("bad Value for %q (%s): %v", sdp.Name, sdp.Value, err)
	}

	return nil
}

// StructuredDataParamValue is the value of a single element item.
type StructuredDataParamValue string // RFC 3629

// String returns the RFC 5424 representation of the value. In
// particular, it escapes \, ", and ].
func (sdv StructuredDataParamValue) String() string {
	str := string(sdv)
	for _, char := range []string{`\`, `"`, `]`} {
		str = strings.Replace(str, char, `\`+char, -1)
	}
	return str
}

// Validate ensures that the value is correct.
func (sdv StructuredDataParamValue) Validate() error {
	return validateUTF8(string(sdv))
}
