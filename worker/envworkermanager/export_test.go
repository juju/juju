// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envworkermanager

func DyingEnvWorkerId(uuid string) string {
	return dyingEnvWorkerId(uuid)
}
