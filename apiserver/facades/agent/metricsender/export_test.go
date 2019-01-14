// Copyright 2014 Canonical Ltd.  All rights reserved.

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
