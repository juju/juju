// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"github.com/juju/worker/v4"

	"github.com/juju/juju/internal/testhelpers"
)

type mockNotifyWorker struct {
	worker.Worker
	testhelpers.Stub
}

func (w *mockNotifyWorker) Notify() {
	w.MethodCall(w, "Notify")
}
