// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package controllerupgrader domain provides the interface required for
// managing the version of the controllers that form the current cluster.
//
// A separate domain was created to handle controller upgrades to start
// separating some of the cross over that controller upgrades had with model
// upgrades. While fundamentally different concepts with the same end results,
// they have always been conflated as a similar idea.
//
// Another driving factor for a separate domain is that we know currently the
// operation of a controller upgrade requires modifying state across both the
// controller database and the model database. Because of this it is hard to
// expose this domain's service via a normal controller service factory.
//
// A Juju controller currently only supports having its patch version upgraded.
// We don't support upgrades between major and minor versions instead
// encouraging users to migrate their workloads to a new controller running the
// new desired version.
//
// Under the hood, controller upgrades still result in the controller's model
// target agent version being updated. What this does is kick every agent and
// controller in the model to start upgrading their agent binary. As an idea
// this approach has the floor that you are potentially restarting the
// controllers of a cluster while agents of the model are also trying to
// download and upgrade their agent binaries. These agent binaries may need to
// be stable during a controller upgrade to support the upgrade of the
// controller. Dependencies the controller relies on to restart may also be sent
// into a bad state because of the model upgrade.
package controllerupgrader
