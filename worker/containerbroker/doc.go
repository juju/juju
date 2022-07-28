// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package containerbroker worker sole responsibility is to manage the lifecycle
// of a instance-broker. Configuration of the instance-broker relies on talking
// to the provisioner to ensure that we correctly configure the correct
// availability zones. Failure to do so, will result in an error.
//
// The instance-broker is created for LXD types only and any other container
// types cause the worker to uninstall itself.
package containerbroker
