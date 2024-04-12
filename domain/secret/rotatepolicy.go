// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

import coresecrets "github.com/juju/juju/core/secrets"

// RotatePolicy represents the rotate policy of a secret.
// as recorded in the secret_rotate_policy lookup table.
type RotatePolicy int

const (
	RotateNever RotatePolicy = iota
	RotateHourly
	RotateDaily
	RotateWeekly
	RotateMonthly
	RotateQuarterly
	RotateYearly
)

// MarshallRotatePolicy converts a secret rotate policy to a db rotate policy id.
func MarshallRotatePolicy(policy *coresecrets.RotatePolicy) RotatePolicy {
	if policy == nil {
		return RotateNever
	}
	switch *policy {
	case coresecrets.RotateHourly:
		return RotateHourly
	case coresecrets.RotateDaily:
		return RotateDaily
	case coresecrets.RotateWeekly:
		return RotateWeekly
	case coresecrets.RotateMonthly:
		return RotateMonthly
	case coresecrets.RotateQuarterly:
		return RotateQuarterly
	case coresecrets.RotateYearly:
		return RotateYearly
	}
	return RotateNever
}
