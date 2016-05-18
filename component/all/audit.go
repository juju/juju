// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

type auditComponent struct{}

func (a auditComponent) registerForServer() error {
	return nil
}

func (a auditComponent) registerForClient() error {
	return nil
}
