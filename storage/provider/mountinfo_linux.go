// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/moby/sys/mountinfo"
)

var getMountsFromReader = mountinfo.GetMountsFromReader
