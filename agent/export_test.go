// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

func EmptyConfig() Config {
	return &configInternal{}
}
