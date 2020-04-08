// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"sync"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/retry.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	statewatcher "github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

var _ = gc.Suite(&StateSuite{})

// StateSuite provides setup and teardown for tests that require a
// state.State.
type StateSuite struct {
	jujutesting.MgoSuite
	testing.BaseSuite
	NewPolicy                 state.NewPolicyFunc
	Controller                *state.Controller
	StatePool                 *state.StatePool
	State                     *state.State
	Model                     *state.Model
	Owner                     names.UserTag
	AdminPassword             string
	Factory                   *factory.Factory
	InitialConfig             *config.Config
	InitialTime               time.Time
	ControllerConfig          map[string]interface{}
	ControllerInheritedConfig map[string]interface{}
	RegionConfig              cloud.RegionConfig
	Clock                     *testclock.Clock
	txnSyncNotify             chan struct{}
	modelWatcherIdle          chan string
	modelWatcherMutex         *sync.Mutex
}

func (s *StateSuite) SetUpSuite(c *gc.C) {
	s.MgoSuite.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
}

func (s *StateSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *StateSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)

	s.txnSyncNotify = make(chan struct{})
	s.modelWatcherIdle = nil
	s.modelWatcherMutex = &sync.Mutex{}
	s.PatchValue(&statewatcher.TxnPollNotifyFunc, s.txnNotifyFunc)
	s.PatchValue(&statewatcher.HubWatcherIdleFunc, s.hubWatcherIdleFunc)

	s.Owner = names.NewLocalUserTag("test-admin")
	initialTime := s.InitialTime
	if initialTime.IsZero() {
		initialTime = testing.NonZeroTime()
	}
	s.Clock = testclock.NewClock(initialTime)
	// Patch the polling policy of the primary txn watcher for the
	// state pool. Since we are using a testing clock the StartSync
	// method on the state object advances the clock one second.
	// Make the txn poller use a standard one second poll interval.
	s.PatchValue(
		&statewatcher.PollStrategy,
		retry.Exponential{
			Initial: time.Second,
			Factor:  1.0,
		})

	s.AdminPassword = "admin-secret"
	s.Controller = InitializeWithArgs(c, InitializeArgs{
		Owner:                     s.Owner,
		AdminPassword:             s.AdminPassword,
		InitialConfig:             s.InitialConfig,
		ControllerConfig:          s.ControllerConfig,
		ControllerInheritedConfig: s.ControllerInheritedConfig,
		RegionConfig:              s.RegionConfig,
		NewPolicy:                 s.NewPolicy,
		Clock:                     s.Clock,
	})
	s.AddCleanup(func(*gc.C) {
		_ = s.Controller.Close()
		close(s.txnSyncNotify)
	})
	s.StatePool = s.Controller.StatePool()
	s.State = s.StatePool.SystemState()
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.Model = model

	s.Factory = factory.NewFactory(s.State, s.StatePool)
}

func (s *StateSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}

func (s *StateSuite) txnNotifyFunc() {
	select {
	case s.txnSyncNotify <- struct{}{}:
		// Try to send something down the channel.
	default:
		// However don't get stressed if no one is listening.
	}
}

func (s *StateSuite) hubWatcherIdleFunc(modelUUID string) {
	s.modelWatcherMutex.Lock()
	idleChan := s.modelWatcherIdle
	s.modelWatcherMutex.Unlock()
	if idleChan == nil {
		return
	}
	// There is a very small race condition between when the
	// idle channel is cleared and when the function exits.
	// Under normal circumstances, there is a goroutine in a tight loop
	// reading off the idle channel. If the channel isn't read
	// within a short wait, we don't send the message.
	select {
	case idleChan <- modelUUID:
	case <-time.After(testing.ShortWait):
	}
}

// WaitForNextSync repeatedly advances the testing clock
// with short waits between until the txn poller doesn't find
// any more changes.
func (s *StateSuite) WaitForNextSync(c *gc.C) {
	done := make(chan struct{})
	go func() {
		<-s.txnSyncNotify
		close(done)
	}()
	timeout := time.After(jujutesting.LongWait)
	for {
		s.Clock.Advance(time.Second)
		loop := time.After(10 * time.Millisecond)
		select {
		case <-done:
			return
		case <-loop:
		case <-timeout:
			c.Fatal("no sync event sent, is the watcher dead?")
		}
	}
}

// WaitForModelWatchersIdle firstly waits for the txn poller to process
// all pending changes, then waits for the hub watcher on the state object
// to have finished processing all those events.
func (s *StateSuite) WaitForModelWatchersIdle(c *gc.C, modelUUID string) {
	// Use a logger rather than c.Log so we get timestamps.
	logger := loggo.GetLogger("test")
	logger.Infof("waiting for model %s to be idle", modelUUID)
	s.WaitForNextSync(c)
	// Create idle channel after the sync so as to be sure that at least
	// one sync is complete before signalling the idle timer.
	s.modelWatcherMutex.Lock()
	idleChan := make(chan string)
	s.modelWatcherIdle = idleChan
	s.modelWatcherMutex.Unlock()

	defer func() {
		s.modelWatcherMutex.Lock()
		s.modelWatcherIdle = nil
		s.modelWatcherMutex.Unlock()
		// Clear out any pending events.
		for {
			select {
			case <-idleChan:
			default:
				return
			}
		}
	}()

	timeout := time.After(jujutesting.LongWait)
	for {
		s.Clock.Advance(10 * time.Millisecond)
		loop := time.After(10 * time.Millisecond)
		select {
		case <-loop:
		case uuid := <-idleChan:
			if uuid == modelUUID {
				return
			} else {
				logger.Infof("model %s is idle", uuid)
			}
		case <-timeout:
			c.Fatal("no sync event sent, is the watcher dead?")
		}
	}
}
