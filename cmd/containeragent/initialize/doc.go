// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package initialize implements the "init" command run by the charm-init
// container of a Kubernetes unit or controller pod.
//
// The command prepares the charm side of the pod before the workload
// containers start:
//
//   - It introduces the pod to the controller by calling the
//     CAASApplication.UnitIntroduction API with the pod name, pod UUID and
//     nonce. The controller associates the pod with its Juju unit and
//     returns that unit's agent configuration; transient errors are retried
//     until the controller responds.
//   - It writes the returned unit agent configuration to
//     <data-dir>/template-agent.conf. For a controller pod (--controller) it
//     also writes the controller machine agent configuration to
//     <data-dir>/agents/machine-<id>/agent.conf.
//   - It writes the Pebble layer that runs the container agent as
//     <bin-dir>/containeragent unit.
//   - It copies the binaries the charm container needs into <bin-dir>
//     (normally /charm/bin): containeragent for the unit agent service,
//     jujuc for hooks, and pebble for the charm container's entrypoint and
//     workload supervision.
//   - It installs the introspection profile functions when a profile
//     directory is requested.
//
// The binaries are read from the operator image's standard filesystem layout
// (the Filesystem Hierarchy Standard): /usr/bin/containeragent,
// /usr/bin/jujuc and Juju's Pebble at /usr/lib/juju/pebble. The Docker
// image is being phased out; the /opt/containeragent, /opt/jujuc
// and /opt/pebble paths it needs are temporary fallbacks and will be
// removed with it.
//
// The pod start-up scripts and the full set of image paths are owned by
// github.com/juju/juju/caas, and the image layout itself is defined by
// rocks/jujud-operator/rockcraft.yaml.
package initialize
