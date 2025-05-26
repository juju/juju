// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"github.com/juju/utils/v4"

	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type uuid string

func newUUID() (uuid, error) {
	id, err := internaluuid.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}
	return uuid(id.String()), nil
}

func (u uuid) validate() error {
	if u == "" {
		return errors.New("empty uuid")
	}
	if !utils.IsValidUUIDString(string(u)) {
		return errors.Errorf("invalid uuid %q", u)
	}
	return nil
}

// NetNodeUUID uniquely identifies a net node.
type NetNodeUUID uuid

// NewNetNodeUUID creates a new, valid IP net node identifier.
func NewNetNodeUUID() (NetNodeUUID, error) {
	u, err := newUUID()
	return NetNodeUUID(u), err
}

// Validate returns an error if the receiver is not a valid UUID.
func (u NetNodeUUID) Validate() error {
	return uuid(u).validate()
}

// String returns the identifier in string form.
func (u NetNodeUUID) String() string {
	return string(u)
}

// InterfaceUUID uniquely identifies a network device.
type InterfaceUUID uuid

// NewInterfaceUUID creates a new, valid network interface identifier.
func NewInterfaceUUID() (InterfaceUUID, error) {
	u, err := newUUID()
	return InterfaceUUID(u), err
}

// Validate returns an error if the receiver is not a valid UUID.
func (u InterfaceUUID) Validate() error {
	return uuid(u).validate()
}

// String returns the identifier in string form.
func (u InterfaceUUID) String() string {
	return string(u)
}

// AddressUUID uniquely identifies an IP address.
type AddressUUID uuid

// NewAddressUUID creates a new, valid IP address identifier.
func NewAddressUUID() (AddressUUID, error) {
	u, err := newUUID()
	return AddressUUID(u), err
}

// Validate returns an error if the receiver is not a valid UUID.
func (u AddressUUID) Validate() error {
	return uuid(u).validate()
}

// String returns the identifier in string form.
func (u AddressUUID) String() string {
	return string(u)
}
