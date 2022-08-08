// Copyright 2022 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import "time"

// RotatePolicy defines a policy for how often
// to rotate a secret.
type RotatePolicy string

const (
	RotateNever       = RotatePolicy("never")
	RotateHourly      = RotatePolicy("hourly")
	RotateDaily       = RotatePolicy("daily")
	RotateWeekly      = RotatePolicy("weekly")
	RotateHalfMonthly = RotatePolicy("half-monthly")
	RotateMonthly     = RotatePolicy("monthly")
	RotateQuarterly   = RotatePolicy("quarterly")
	RotateHalfYearly  = RotatePolicy("half-yearly")
	RotateYearly      = RotatePolicy("yearly")
)

func (p RotatePolicy) String() string {
	if p == "" {
		return string(RotateNever)
	}
	return string(p)
}

// IsValid returns true if v is a valid rotate policy.
func (p RotatePolicy) IsValid() bool {
	switch p {
	case RotateNever, RotateHourly, RotateDaily, RotateWeekly, RotateHalfMonthly,
		RotateMonthly, RotateQuarterly, RotateHalfYearly,
		RotateYearly:
		return true
	}
	return false
}

// NextRotateTime returns when the policy dictates a secret should be next
// rotated given the last rotation time.
func (p RotatePolicy) NextRotateTime(lastRotateTime *time.Time) *time.Time {
	now := time.Now()
	var lastRotated = now
	if lastRotateTime != nil {
		lastRotated = *lastRotateTime
	}
	var result time.Time
	switch p {
	case RotateNever:
		return nil
	case RotateHourly:
		result = lastRotated.Add(time.Hour)
	case RotateDaily:
		result = lastRotated.AddDate(0, 0, 1)
	case RotateWeekly:
		result = lastRotated.AddDate(0, 0, 7)
	case RotateHalfMonthly:
		result = lastRotated.AddDate(0, 0, 14)
	case RotateMonthly:
		result = lastRotated.AddDate(0, 1, 0)
	case RotateQuarterly:
		result = lastRotated.AddDate(0, 3, 0)
	case RotateHalfYearly:
		result = lastRotated.AddDate(0, 6, 0)
	case RotateYearly:
		result = lastRotated.AddDate(1, 0, 0)
	}
	if result.Before(now) {
		result = now
	}
	return &result
}
