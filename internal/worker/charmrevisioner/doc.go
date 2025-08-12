// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package charmrevisioner defines the charm revision updater worker. This
// worker is responsible for polling Charmhub every 24 hours to check if there
// are new revisions available of any repository charm deployed in the model. If
// so, it will put a document in the Juju database, so that the next time the
// user runs `juju status`, they can see that there is an update available. This
// worker also sends anonymised usage metrics to Charmhub when it polls.
package charmrevisioner
