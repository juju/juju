// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sdelements

import (
	"fmt"
	"strings"

	"github.com/juju/juju/standards/rfc5424"
)

// Private is a custom structured data element associated with an
// IANA-registered Private Enterprise Number.
//
// See https://tools.ietf.org/html/rfc5424#section-6.3.1 and
// http://www.iana.org/assignments/smi-numbers/smi-numbers.xhtml.
type Private struct {
	// Name is the custom name for this SD element, relative to PEN.
	Name rfc5424.StructuredDataName

	// PEN is the IANA-registered Private Enterprise Number, with an
	// implicit SMI Network Management code prefixed by
	// "iso.org.dod.internet.private.enterprise".
	PEN PrivateEnterpriseNumber

	// Data is the multi-set of data items associated with this SD
	// element. The PEN's org should ensure that the format is
	// sufficiently well-communicated.
	Data []rfc5424.StructuredDataParam
}

// ID returns the SD-ID for this element.
func (sde Private) ID() rfc5424.StructuredDataName {
	return rfc5424.StructuredDataName(fmt.Sprintf("%s@%s", sde.Name, sde.PEN))
}

// Params returns the []SD-PARAM for this element.
func (sde Private) Params() []rfc5424.StructuredDataParam {
	params := make([]rfc5424.StructuredDataParam, len(sde.Data))
	copy(params, sde.Data)
	return params
}

// Validate ensures that the element is correct.
func (sde Private) Validate() error {
	if sde.Name == "" {
		return fmt.Errorf("empty Name")
	}
	if strings.Contains(string(sde.Name), "@") {
		return fmt.Errorf(`invalid char in %q`, sde.Name)
	}
	if err := sde.Name.Validate(); err != nil {
		return fmt.Errorf("invalid Name %q: %v", sde.Name, err)
	}

	if sde.PEN <= 0 {
		return fmt.Errorf("empty PEN")
	}
	if err := sde.PEN.Validate(); err != nil {
		return fmt.Errorf("invalid PEN %q: %v", sde.PEN, err)
	}

	for i, param := range sde.Data {
		if err := param.Validate(); err != nil {
			return fmt.Errorf("param %d not valid: %v", i, err)
		}
	}

	// TODO(ericsnow) ensure PEN matches origin.EnterpriseID (if any)
	return nil
}

// PrivateEnterpriseNumber is an IANA-registered positive integer that
// publicly identifies a specific organization.
//
// See http://www.iana.org/assignments/smi-numbers/smi-numbers.xhtml.
type PrivateEnterpriseNumber int

// String returns the string representation of the PEN.
func (pen PrivateEnterpriseNumber) String() string {
	return fmt.Sprint(int(pen))
}

// Validate ensures that the number is correct.
func (pen PrivateEnterpriseNumber) Validate() error {
	if pen <= 0 { // 0 is reserved
		fmt.Errorf("must be positive integer")
	}
	return nil
}
