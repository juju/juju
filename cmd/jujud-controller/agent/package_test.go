// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent // not agent_test for no good reason

import (
	"bufio"
	"encoding/json"
	"os"
	stdtesting "testing"
	"time"

	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/internal/pubsub/apiserver"
	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/machine_mock.go github.com/juju/juju/cmd/jujud-controller/agent CommandRunner

func TestPackage(t *stdtesting.T) {
	// TODO(waigani) 2014-03-19 bug 1294458
	// Refactor to use base suites
	coretesting.MgoSSLTestPackage(t)
}

func readAuditLog(c *tc.C, logPath string) []auditlog.Record {
	file, err := os.Open(logPath)
	c.Assert(err, jc.ErrorIsNil)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var results []auditlog.Record
	for scanner.Scan() {
		var record auditlog.Record
		err := json.Unmarshal(scanner.Bytes(), &record)
		c.Assert(err, jc.ErrorIsNil)
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

type cleanupSuite interface {
	AddCleanup(func(*tc.C))
}

func startAddressPublisher(suite cleanupSuite, c *tc.C, agent *MachineAgent) {
	// Start publishing a test API address on the central hub so that
	// dependent workers can start. The other way of unblocking them
	// would be to get the peergrouper healthy, but that has proved
	// difficult - trouble getting the replicaset correctly
	// configured.
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(500 * time.Millisecond):
				hub := agent.centralHub
				if hub == nil {
					continue
				}
				sent, err := hub.Publish(apiserver.DetailsTopic, apiserver.Details{
					Servers: map[string]apiserver.APIServer{
						"0": {ID: "0", InternalAddress: serverAddress},
					},
				})
				if err != nil {
					c.Logf("error publishing address: %s", err)
				}

				// Ensure that it has been sent, before moving on.
				select {
				case <-pubsub.Wait(sent):
				case <-time.After(testing.ShortWait):
				}
			}
		}
	}()
	suite.AddCleanup(func(c *tc.C) { close(stop) })
}
