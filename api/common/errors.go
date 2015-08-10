// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
)

var (
	ErrPartialResults = errors.New("API call only returned partial results")
)
