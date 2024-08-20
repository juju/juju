// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The `jujud` binary provides, among other things, commands that are used in agent definition files to spawn various agents.
//   - [NewMachineAgentCmd] is used to spawn all Juju agents except the modeloperator and the unit agent on Kubernetes.
//   - [NewModelCommand] is used to spawn the modeloperator agent on Kubernetes.
//   - [NewCaasOperatorAgent] is used to spawn an agent relevant to podspec charms. Since this type of charms has been deprecated, this agent is as well.
//
// [NewMachineAgentCmd]: https://pkg.go.dev/github.com/juju/juju/cmd/jujud/agent#NewMachineAgentCmd
// [NewModelCommand]: https://pkg.go.dev/github.com/juju/juju/cmd/jujud/agent#NewModelCommand
// [NewCaasOperatorAgent]: https://pkg.go.dev/github.com/juju/juju/cmd/jujud/agent#NewCaasOperatorAgent
package main
