// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import "github.com/juju/errors"

const (
	// SSHRule is a rule for SSH connections.
	SSHRule = WellKnownServiceType("ssh")

	// JujuControllerRule is a rule for connections to the Juju controller.
	JujuControllerRule = WellKnownServiceType("juju-controller")

	// JujuApplicationOfferRule is a rule for connections to a Juju offer.
	JujuApplicationOfferRule = WellKnownServiceType("juju-application-offer")
)

// WellKnownService defines a service for which firewall rules may be applied.
type WellKnownServiceType string

func (v WellKnownServiceType) Validate() error {
	switch v {
	case SSHRule, JujuControllerRule, JujuApplicationOfferRule:
		return nil
	}
	return errors.NotValidf("well known service type %q", v)
}
