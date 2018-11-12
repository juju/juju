// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"gopkg.in/juju/worker.v1"
)

func WorkerCredentialDeleted(w worker.Worker) bool {
	return w.(*validator).credentialDeleted
}
