// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"time"
)

// A gnuflag.Value for the --wait command line argument. If called
// as a boolean  with no arguments, the forever flag is set to true.
// If called  with an argument, d is set to the result of
// time.ParseDuration().
// eg:
//   --wait
//   --wait=10s
type waitFlag struct {
	forever bool
	d       time.Duration
}

func (f *waitFlag) Set(s string) error {
	if s == "true" {
		f.forever = true
		return nil
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	f.d = v
	return nil
}

func (f *waitFlag) String() string {
	if f.forever {
		return "true"
	}
	if f.d == 0 {
		return ""
	}
	return f.d.String()
}

func (f *waitFlag) IsBoolFlag() bool {
	return true
}
