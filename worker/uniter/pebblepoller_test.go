// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"regexp"
	"sync"
	"time"

	pebbleclient "github.com/canonical/pebble/client"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/container"
)

type pebblePollerSuite struct{}

var _ = gc.Suite(&pebblePollerSuite{})

const (
	pebbleSocketPathRegexpString = "/charm/containers/([^/]+)/pebble.socket"
)

var (
	pebbleSocketPathRegexp = regexp.MustCompile(pebbleSocketPathRegexpString)
)

func (s *pebblePollerSuite) TestStart(c *gc.C) {
	clients := map[string]*fakePebbleClient{
		"a": {
			sysInfo: pebbleclient.SysInfo{
				BootID: "1",
			},
			err: errors.Errorf("not yet workin"),
		},
		"b": {
			sysInfo: pebbleclient.SysInfo{
				BootID: "1",
			},
			err: errors.Errorf("not yet workin"),
		},
		"c": {
			sysInfo: pebbleclient.SysInfo{
				BootID: "1",
			},
			err: errors.Errorf("not yet workin"),
		},
	}
	newClient := func(cfg *pebbleclient.Config) uniter.PebbleClient {
		c.Assert(cfg.Socket, gc.Matches, pebbleSocketPathRegexpString)
		res := pebbleSocketPathRegexp.FindAllStringSubmatch(cfg.Socket, 1)
		return clients[res[0][1]]
	}
	clock := testclock.NewClock(time.Time{})
	containerNames := []string{
		"a", "b", "c",
	}
	workloadEventChan := make(chan string)
	workloadEvents := container.NewWorkloadEvents()
	worker := uniter.NewPebblePoller(loggo.GetLogger("test"), clock, containerNames, workloadEventChan, workloadEvents, newClient)

	doRestart := func(containerName string) {
		client := clients[containerName]
		c.Assert(workloadEvents.Events(), gc.HasLen, 0)
		client.TriggerStart()
		timeout := time.After(testing.LongWait)
		for {
			select {
			case id := <-workloadEventChan:
				c.Logf("got queued log id %s", id)
				c.Assert(workloadEvents.Events(), gc.HasLen, 1)
				evt, cb, err := workloadEvents.GetWorkloadEvent(id)
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(evt, gc.DeepEquals, container.WorkloadEvent{
					Type:         container.ReadyEvent,
					WorkloadName: containerName,
				})
				c.Assert(cb, gc.NotNil)
				workloadEvents.RemoveWorkloadEvent(id)
				cb(nil)
				c.Assert(workloadEvents.Events(), gc.HasLen, 0)
				return
			case <-time.After(testing.ShortWait):
				clock.Advance(5 * time.Second)
			case <-timeout:
				c.Fatalf("timed out waiting for event id")
				return
			}
		}
	}

	doRestart("a")
	doRestart("b")
	doRestart("c")
	doRestart("a")
	doRestart("a")
	doRestart("a")

	workertest.CleanKill(c, worker)
}

type fakePebbleClient struct {
	sysInfo pebbleclient.SysInfo
	err     error
	mut     sync.Mutex
}

func (c *fakePebbleClient) SysInfo() (*pebbleclient.SysInfo, error) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	sysInfoCopy := c.sysInfo
	return &sysInfoCopy, nil
}

func (c *fakePebbleClient) TriggerStart() {
	c.mut.Lock()
	defer c.mut.Unlock()
	c.err = nil
	c.sysInfo.BootID = utils.MustNewUUID().String()
}
