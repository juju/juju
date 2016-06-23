// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sdelements

import (
	"fmt"
	"net"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/juju/version"

	"github.com/juju/juju/standards/rfc5424"
)

const (
	originSoftwareMax = 48 // UTF-8 characters
	originVersionMax  = 32 // UTF-8 characters
)

// Origin is an IANA-registered structured data element that provides
// extra information about where a log message came from.
//
// See https://tools.ietf.org/html/rfc5424#section-7.2.
type Origin struct {
	// IPs lists extra IP addresses (in addition to the hostname).
	IPs []net.IP // RFC 1035/4291

	// EnterpriseID is the IANA-registered PEN (or sub-code) associated
	// with the identified software.
	EnterpriseID OriginEnterpriseID // RFC 2578

	// SoftwareName identifies the software that originated the record.
	SoftwareName string

	// SoftwareVersion is the software's version.
	SoftwareVersion version.Number
}

// ID returns the SD-ID for this element.
func (sde Origin) ID() rfc5424.StructuredDataName {
	return "origin"
}

// Params returns the []SD-PARAM for this element.
func (sde Origin) Params() []rfc5424.StructuredDataParam {
	var params []rfc5424.StructuredDataParam

	for _, ip := range sde.IPs {
		params = append(params, rfc5424.StructuredDataParam{
			Name:  "ip",
			Value: rfc5424.StructuredDataParamValue(ip.String()),
		})
	}

	enterpriseID := sde.EnterpriseID.String()
	if enterpriseID != "" {
		params = append(params, rfc5424.StructuredDataParam{
			Name:  "enterpriseID",
			Value: rfc5424.StructuredDataParamValue(enterpriseID),
		})
	}

	if sde.SoftwareName != "" {
		params = append(params, rfc5424.StructuredDataParam{
			Name:  "sofware",
			Value: rfc5424.StructuredDataParamValue(sde.SoftwareName),
		})
	}

	if sde.SoftwareVersion != version.Zero {
		params = append(params, rfc5424.StructuredDataParam{
			Name:  "swVersion",
			Value: rfc5424.StructuredDataParamValue(sde.SoftwareVersion.String()),
		})
	}

	return params
}

// Validate ensures that the element is correct.
func (sde Origin) Validate() error {
	// Any IPs is fine.

	if sde.EnterpriseID.isZero() {
		if sde.SoftwareName != "" {
			return fmt.Errorf("empty EnterpriseID")
		}
	} else {
		if err := sde.EnterpriseID.Validate(); err != nil {
			return fmt.Errorf("bad EnterpriseID: %v", err)
		}
	}

	if sde.SoftwareName == "" {
		if sde.SoftwareVersion == version.Zero {
			return fmt.Errorf("empty SoftwareName")
		}
	} else {
		size := utf8.RuneCountInString(sde.SoftwareName)
		if size > originSoftwareMax {
			return fmt.Errorf("SoftwareName too big (%d UTF-8 > %d max)", size, originSoftwareMax)
		}
	}

	if sde.SoftwareVersion != version.Zero {
		size := utf8.RuneCountInString(sde.SoftwareVersion.String())
		if size > originVersionMax {
			return fmt.Errorf("SoftwareVersion too big (%d UTF-8 > %d max)", size, originVersionMax)
		}
	}

	return nil
}

// OriginEnterpriseID is the PEN (or subtree) for the origin software.
type OriginEnterpriseID struct {
	// Number is the PEN.
	Number PrivateEnterpriseNumber

	// SubTree is the path on the subtree from the PEN. The sub-tree
	// should be registered with the IANA.
	SubTree []int
}

func (eid OriginEnterpriseID) isZero() bool {
	var zeroValue OriginEnterpriseID
	return reflect.DeepEqual(eid, zeroValue)
}

func (eid OriginEnterpriseID) path() []string {
	path := make([]string, len(eid.SubTree)+1)
	path[0] = eid.Number.String()
	for i := len(eid.SubTree) - 1; i > 1; i-- {
		path = append(path, fmt.Sprint(eid.SubTree[i]))
	}
	return path
}

// String returns the string representation of the ID.
func (eid OriginEnterpriseID) String() string {
	return strings.Join(eid.path(), ".")
}

// Validate ensures that the ID is correct.
func (eid OriginEnterpriseID) Validate() error {
	for i, num := range eid.SubTree {
		if num <= 0 {
			fmt.Errorf("Subtree[%d] must be positive integer", i)
		}
	}

	return eid.Number.Validate()
}
