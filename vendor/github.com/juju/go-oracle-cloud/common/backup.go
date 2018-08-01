// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package common

import (
	"errors"
	"strings"
)

// BackupState represents current state of the backup
type BackupState string

// Lists of backup states
const (
	Submitted       BackupState = "SUBMITTED"
	Inprogress      BackupState = "INPROGRESS"
	Completed       BackupState = "COMPLETED"
	Failed          BackupState = "FAILED"
	Canceling       BackupState = "CANCELLING"
	Canceled        BackupState = "CANCELED"
	Timeout         BackupState = "TIMEOUT"
	DeleteSubmitted BackupState = "DELETE_SUBMITTED"
	Deleting        BackupState = "DELETING"
	Deleted         BackupState = "DELETED"
)

// Week type for using all week days
type Week string

const (
	Monday    Week = "MONDAY"
	Tuesday   Week = "TUESDAY"
	Wednesday Week = "WEDNESDAY"
	Thursday  Week = "THURSDAY"
	Friday    Week = "FRIDAY"
	Saturday  Week = "SATURDAY"
	Sunday    Week = "SUNDAY"
)

// Interval type used for providing an Interval in the
// BackupCOnfigurationParams
type Interval struct {
	// Hourly is the count of backups in a hour
	Hourly Hour `json:"Hourly,omitempty"`
	// DaysOfWeek what are the days the backup should run
	DaysOfWeek []Week `json:"DaysOfWeek,omitempty"`
	// TimeOfDay is the time of the day that the backup should run
	TimeOfDay string `json:"timeOfDay,omitempty"`
	// UserTimeZone the user timezone
	// The user timezone is IANA user timezone
	UserTimeZone string `json:"userTimeZone,omitempty"`
}

// Hour represents the how many backup should be performed
type Hour struct {
	// HourlyInterval number of backups
	HourlyInterval int `json:"hourlyInterval,omitempty"`
}

// NewInterval returns a new Interval used for constructing the
// interval in the backup configuration params
func NewInterval(hourlyInterval int) Interval {
	return Interval{
		Hourly: Hour{
			HourlyInterval: hourlyInterval,
		},
	}
}

// NewDaylyWeekly returns a new DaylyWeekly used for constructing the
// interval in the backup configuration params
func NewDailyWeekly(
	days []Week,
	time string,
	timezone string,
) Interval {
	return Interval{
		DaysOfWeek:   days,
		TimeOfDay:    time,
		UserTimeZone: timezone,
	}
}

// Validate will validate the interval in the backup configuration params
func (i Interval) Validate() (err error) {

	if i.DaysOfWeek != nil &&
		i.TimeOfDay != "" &&
		i.UserTimeZone != "" &&
		i.Hourly.HourlyInterval != 0 {
		return errors.New(
			"go-oracle-cloud: Cannot use both invervals",
		)
	}

	if i.DaysOfWeek != nil &&
		i.TimeOfDay != "" &&
		i.UserTimeZone != "" {

		if i.DaysOfWeek == nil {
			return errors.New("go-oracle-cloud: Empty days")
		}

		weeks := map[Week]Week{
			Monday:    Monday,
			Tuesday:   Tuesday,
			Wednesday: Wednesday,
			Thursday:  Thursday,
			Friday:    Friday,
			Saturday:  Saturday,
			Sunday:    Sunday,
		}

		for _, val := range i.DaysOfWeek {
			if _, ok := weeks[val]; !ok {
				return errors.New(
					"go-oracle-cloud: Invalid week day",
				)
			}
		}

		n := strings.SplitN(i.TimeOfDay, ":", 2)
		if n[0] == "" && n[1] == "" {
			return errors.New(
				"go-oracle-cloud: Invalid time format",
			)
		}

		m := strings.SplitN(i.UserTimeZone, ":", 2)
		if m[0] == "" && m[1] == "" {
			return errors.New(
				"go-oracle-cloud: Invalid timezone format",
			)
		}

	}

	return nil
}
