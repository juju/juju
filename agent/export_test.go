// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

func PasswordHash(config Config) string {
	confInternal := config.(*configInternal)
	return confInternal.passwordHash()
}
