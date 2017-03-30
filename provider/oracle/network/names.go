// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import "fmt"

// globalGroupName returns the global group name
// based from the juju environ config uuid
func (f Firewall) globalGroupName() string {
	return fmt.Sprintf("juju-%s-global", f.environ.Config().UUID())
}

// machineGroupName returns the machine group name
// based from the juju environ config uuid
func (f Firewall) machineGroupName(machineId string) string {
	return fmt.Sprintf("juju-%s-%s", f.environ.Config().UUID(), machineId)
}

// resourceName returns the resource name
// based from the juju environ config uuid
func (f Firewall) newResourceName(appName string) string {
	return fmt.Sprintf("juju-%s-%s", f.environ.Config().UUID(), appName)
}
