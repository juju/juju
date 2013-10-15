// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

func Password(config Config) string {
	c := config.(*configInternal)
	if c.stateDetails == nil {
		return c.apiDetails.password
	} else {
		return c.stateDetails.password
	}
	return ""
}

func WriteNewPassword(cfg Config) (string, error) {
	return cfg.(*configInternal).writeNewPassword()
}
