// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
)

// AllDeviceAddresses returns all known addresses assigned to
// link-layer devices on the machine.
func (m *Machine) AllDeviceAddresses() ([]*Address, error) {
	var allAddresses []*Address
	callbackFunc := func(doc *ipAddressDoc) {
		allAddresses = append(allAddresses, newIPAddress(m.st, *doc))
	}

	findQuery := findAddressesQuery(m.doc.Id, "")
	if err := m.st.forEachIPAddressDoc(findQuery, callbackFunc); err != nil {
		return nil, errors.Trace(err)
	}
	return allAddresses, nil
}
