// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The `jujud` binary provides, among other things, commands that are used in agent definition files to spawn various agents.
//   - [NewMachineAgentCommand] is used to spawn all Juju agents except the modeloperator and the unit agent on Kubernetes.
//   - [NewModelCommand] is used to spawn the modeloperator agent on Kubernetes.
//
// [NewMachineAgentCommand]: https://pkg.go.dev/github.com/juju/juju/cmd/jujud/agent#NewMachineAgentCommand
// [NewModelCommand]: https://pkg.go.dev/github.com/juju/juju/cmd/jujud/agent#NewModelCommand
package main
