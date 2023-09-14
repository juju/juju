// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !linux

package provider

import (
	"io"

	"github.com/juju/errors"
	"github.com/moby/sys/mountinfo"
)

func getMountsFromReader(r io.Reader, filter mountinfo.FilterFunc) ([]*mountinfo.Info, error) {
	return nil, errors.NotSupported
}
