// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent // not agent_test for no good reason

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/auditlog"
	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/machine_mock.go github.com/juju/juju/cmd/jujud-controller/agent CommandRunner

func TestMain(m *testing.M) {
	os.Exit(func() int {
		defer coretesting.MgoSSLTestMain()()
		return m.Run()
	}())
}

func readAuditLog(c *tc.C, logPath string) []auditlog.Record {
	file, err := os.Open(logPath)
	c.Assert(err, tc.ErrorIsNil)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var results []auditlog.Record
	for scanner.Scan() {
		var record auditlog.Record
		err := json.Unmarshal(scanner.Bytes(), &record)
		c.Assert(err, tc.ErrorIsNil)
		results = append(results, record)
	}
	return results
}

type nullWorker struct {
	dead chan struct{}
}

func (w *nullWorker) Kill() {
	close(w.dead)
}

func (w *nullWorker) Wait() error {
	<-w.dead
	return nil
}
