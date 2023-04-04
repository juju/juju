// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import "github.com/juju/juju/core/series"

// DefaultSupportedLTS returns the latest LTS that Juju supports and is
// compatible with.
func DefaultSupportedLTS() string {
	return "jammy"
}

// DefaultSupportedLTSBase returns the latest LTS base that Juju supports
// and is compatible with.
func DefaultSupportedLTSBase() series.Base {
	return series.MakeDefaultBase("ubuntu", "22.04")
}
