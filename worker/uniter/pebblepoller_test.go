// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	"regexp"
	"sync"
	"time"

	pebbleclient "github.com/canonical/pebble/client"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3/workertest"
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
	newClient := func(cfg *pebbleclient.Config) (uniter.PebbleClient, error) {
		c.Assert(cfg.Socket, gc.Matches, pebbleSocketPathRegexpString)
		res := pebbleSocketPathRegexp.FindAllStringSubmatch(cfg.Socket, 1)
		return clients[res[0][1]], nil
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

	for k, v := range clients {
		c.Assert(v.closed, jc.IsTrue, gc.Commentf("client %s not closed", k))
	}
}

type fakePebbleClient struct {
	sysInfo     pebbleclient.SysInfo
	err         error
	mut         sync.Mutex
	closed      bool
	clock       *testclock.Clock
	noticeAdded chan *pebbleclient.Notice
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

func (c *fakePebbleClient) CloseIdleConnections() {
	c.mut.Lock()
	defer c.mut.Unlock()
	c.closed = true
}

// AddNotice adds a notice for WaitNotices to receive. To have WaitNotices
// return an error, use notice.Type "error" with the message in notice.Key.
func (c *fakePebbleClient) AddNotice(checkC *gc.C, notice *pebbleclient.Notice) {
	select {
	case c.noticeAdded <- notice:
	case <-time.After(testing.LongWait):
		checkC.Fatalf("timed out waiting to add notice")
	}
}

func (c *fakePebbleClient) WaitNotices(ctx context.Context, serverTimeout time.Duration, opts *pebbleclient.NoticesOptions) ([]*pebbleclient.Notice, error) {
	timeoutCh := c.clock.After(serverTimeout)
	for {
		select {
		case notice := <-c.noticeAdded:
			if notice.Type == "error" {
				return nil, errors.New(notice.Key)
			}
			if noticeMatches(notice, opts) {
				return []*pebbleclient.Notice{notice}, nil
			}
		case <-timeoutCh:
			return nil, nil // no notices after serverTimeout is not an error
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func noticeMatches(notice *pebbleclient.Notice, opts *pebbleclient.NoticesOptions) bool {
	if opts == nil || opts.Types != nil || opts.Keys != nil {
		panic("not supported")
	}
	if !opts.After.IsZero() && !notice.LastRepeated.After(opts.After) {
		return false
	}
	return true
}
