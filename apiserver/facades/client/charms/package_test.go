// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"net/http"
	"net/url"
	"os"
	stdtesting "testing"
	"time"

	"github.com/juju/juju/internal/testing"
)

func TestMain(m *stdtesting.M) {
	os.Exit(func() int {
		defer testing.MgoTestMain()()
		return m.Run()
	}())
}

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/repository.go github.com/juju/juju/core/charm Repository,CharmArchive
//go:generate go run go.uber.org/mock/mockgen -typed -package charms -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/charms ModelConfigService,ApplicationService,MachineService

type noopRequestRecorder struct{}

// Record an outgoing request which produced an http.Response.
func (noopRequestRecorder) Record(method string, url *url.URL, res *http.Response, rtt time.Duration) {
}

// Record an outgoing request which returned back an error.
func (noopRequestRecorder) RecordError(method string, url *url.URL, err error) {}
