// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package test

import (
	"github.com/juju/errors"

	"github.com/juju/juju/pki"
)

// NewTestAuthority returns a valid pki Authority for testing
func NewTestAuthority() (pki.Authority, error) {
	signer, err := pki.DefaultKeyProfile()
	if err != nil {
		return nil, errors.Trace(err)
	}

	caCert, err := pki.NewCA("juju-testing", signer)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return pki.NewDefaultAuthority(caCert, signer)
}
