// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sync"
	stdtesting "testing"
	"time"

	pebbleclient "github.com/canonical/pebble/client"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/worker/uniter"
	"github.com/juju/juju/internal/worker/uniter/container"
)

type pebblePollerSuite struct{}

func TestPebblePollerSuite(t *stdtesting.T) { tc.Run(t, &pebblePollerSuite{}) }

const (
	pebbleSocketPathRegexpString = "/charm/containers/([^/]+)/pebble.socket"
)

var (
	pebbleSocketPathRegexp = regexp.MustCompile(pebbleSocketPathRegexpString)
)

func (s *pebblePollerSuite) TestStart(c *tc.C) {
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
		c.Assert(cfg.Socket, tc.Matches, pebbleSocketPathRegexpString)
		res := pebbleSocketPathRegexp.FindAllStringSubmatch(cfg.Socket, 1)
		return clients[res[0][1]], nil
	}
	clock := testclock.NewClock(time.Time{})
	containerNames := []string{
		"a", "b", "c",
	}
	workloadEventChan := make(chan string)
	workloadEvents := container.NewWorkloadEvents()
	worker := uniter.NewPebblePoller(loggertesting.WrapCheckLog(c), clock, containerNames, workloadEventChan, workloadEvents, newClient)

	doRestart := func(containerName string) {
		client := clients[containerName]
		c.Assert(workloadEvents.Events(), tc.HasLen, 0)
		client.TriggerStart()
		timeout := time.After(testing.LongWait)
		for {
			select {
			case id := <-workloadEventChan:
				c.Logf("got queued log id %s", id)
				c.Assert(workloadEvents.Events(), tc.HasLen, 1)
				evt, cb, err := workloadEvents.GetWorkloadEvent(id)
				c.Assert(err, tc.ErrorIsNil)
				c.Assert(evt, tc.DeepEquals, container.WorkloadEvent{
					Type:         container.ReadyEvent,
					WorkloadName: containerName,
				})
				c.Assert(cb, tc.NotNil)
				workloadEvents.RemoveWorkloadEvent(id)
				cb(nil)
				c.Assert(workloadEvents.Events(), tc.HasLen, 0)
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
		c.Assert(v.closed, tc.IsTrue, tc.Commentf("client %s not closed", k))
	}
}

type fakePebbleClient struct {
	sysInfo     pebbleclient.SysInfo
	err         error
	mut         sync.Mutex
	closed      bool
	clock       *testclock.Clock
	noticeAdded chan *pebbleclient.Notice
	changes     map[string]*pebbleclient.Change
	changeErr   error
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
	c.sysInfo.BootID = uuid.MustNewUUID().String()
}

func (c *fakePebbleClient) CloseIdleConnections() {
	c.mut.Lock()
	defer c.mut.Unlock()
	c.closed = true
}

// AddNotice adds a notice for WaitNotices to receive. To have WaitNotices
// return an error, use notice.Type "error" with the message in notice.Key.
func (c *fakePebbleClient) AddNotice(checkC *tc.C, notice *pebbleclient.Notice) {
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

// AddChange adds a change for Change to return.
func (c *fakePebbleClient) AddChange(checkC *tc.C, change *pebbleclient.Change) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if c.changes == nil {
		c.changes = make(map[string]*pebbleclient.Change)
	}
	c.changes[change.ID] = change
}

// Change returns a change by ID, or a NotFound error if it doesn't exist.
func (c *fakePebbleClient) Change(id string) (*pebbleclient.Change, error) {
	if c.changeErr != nil {
		return nil, c.changeErr
	}
	change, exists := c.changes[id]
	if exists {
		return change, nil
	}
	return nil, &pebbleclient.Error{
		Message:    fmt.Sprintf("cannot find change with id %q", id),
		StatusCode: http.StatusNotFound,
	}
}
