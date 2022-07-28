// Copyright 2014 Canonical Ltd.  All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsender

import "github.com/juju/testing"

func PatchHost(host string) func() {
	restoreHost := testing.PatchValue(&defaultSenderFactory, func(url string) MetricSender {
		return &HTTPSender{url: host}
	})
	return func() {
		restoreHost()
	}
}
