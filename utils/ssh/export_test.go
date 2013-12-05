// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

func ReadAuthorisedKeys() ([]string, error) {
	return readAuthorisedKeys()
}
