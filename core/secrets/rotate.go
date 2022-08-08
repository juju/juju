// Copyright 2022 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

// RotatePolicy defines a policy for how often
// to rotate a secret.
type RotatePolicy string

const (
	RotateHourly      = RotatePolicy("hourly")
	RotateDaily       = RotatePolicy("daily")
	RotateWeekly      = RotatePolicy("weekly")
	RotateHalfMonthly = RotatePolicy("half-monthly")
	RotateMonthly     = RotatePolicy("monthly")
	RotateQuarterly   = RotatePolicy("quarterly")
	RotateHalfYearly  = RotatePolicy("half-yearly")
	RotateYearly      = RotatePolicy("yearly")
)

// IsValid returns true if v is a valid rotate policy.
func IsValid(v string) bool {
	switch RotatePolicy(v) {
	case RotateHourly, RotateDaily, RotateWeekly, RotateHalfMonthly,
		RotateMonthly, RotateQuarterly, RotateHalfYearly,
		RotateYearly:
		return true
	}
	return false
}
