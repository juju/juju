// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"github.com/juju/utils"
)

const IPAddressTagKind = "ipaddress"

// IsValidIPAddress returns whether id is a valid IP address ID.
// Here it simply is checked if it is a valid UUID.
func IsValidIPAddress(id string) bool {
	return utils.IsValidUUIDString(id)
}

type IPAddressTag struct {
	id utils.UUID
}

func (t IPAddressTag) String() string { return t.Kind() + "-" + t.id.String() }
func (t IPAddressTag) Kind() string   { return IPAddressTagKind }
func (t IPAddressTag) Id() string     { return t.id.String() }

// NewIPAddressTag returns the tag for the IP address with the given ID (UUID).
func NewIPAddressTag(id string) IPAddressTag {
	uuid, err := utils.UUIDFromString(id)
	if err != nil {
		panic(err)
	}
	return IPAddressTag{id: uuid}
}

// ParseIPAddressTag parses an IP address tag string.
func ParseIPAddressTag(ipAddressTag string) (IPAddressTag, error) {
	tag, err := ParseTag(ipAddressTag)
	if err != nil {
		return IPAddressTag{}, err
	}
	ipat, ok := tag.(IPAddressTag)
	if !ok {
		return IPAddressTag{}, invalidTagError(ipAddressTag, IPAddressTagKind)
	}
	return ipat, nil
}
