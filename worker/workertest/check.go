// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workertest

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
)

var (
	// AliveDelay is the minimum time an Alive helper will Wait for its worker
	// to stop before returning successfully.
	aliveDelay = coretesting.ShortWait

	// KillTimeout is the maximum time a Kill helper will Wait for its worker
	// before failing the test.
	killTimeout = coretesting.LongWait
)

// CheckAlive Wait()s a short time for the supplied worker to return an error,
// and fails the test if it does. If it doesn't fail, it'll leave a goroutine
// running in the background, blocked on the worker's death; but that doesn't
// matter, because of *course* you correctly deferred a suitable Kill helper
// as soon as you created the worker in the first place. Right? Right.
//
// It doesn't Assert and is therefore suitable for use from any goroutine.
func CheckAlive(c *gc.C, w worker.Worker) {
	wait := make(chan error, 1)
	go func() {
		wait <- w.Wait()
	}()
	select {
	case <-time.After(aliveDelay):
	case err := <-wait:
		c.Errorf("expected alive worker; failed with %v", err)
	}
}

// CheckKilled Wait()s for the supplied worker's error, which it returns for
// further analysis, or fails the test after a timeout expires. It doesn't
// Assert and is therefore suitable for use from any goroutine.
func CheckKilled(c *gc.C, w worker.Worker) error {
	wait := make(chan error, 1)
	go func() {
		wait <- w.Wait()
	}()
	select {
	case err := <-wait:
		return err
	case <-time.After(killTimeout):
		c.Errorf("timed out waiting for worker to stop")
		return errors.New("workertest: worker not stopping")
	}
}

// CheckKill Kill()s the supplied worker and Wait()s for its error, which it
// returns for further analysis, or fails the test after a timeout expires.
// It doesn't Assert and is therefore suitable for use from any goroutine.
func CheckKill(c *gc.C, w worker.Worker) error {
	w.Kill()
	return CheckKilled(c, w)
}

// CleanKill calls CheckKill with the supplied arguments, and Checks that the
// returned error is nil. It's particularly suitable for deferring:
//
//     someWorker, err := some.NewWorker()
//     c.Assert(err, jc.ErrorIsNil)
//     defer workertest.CleanKill(c, someWorker)
//
// ...in the large number (majority?) of situations where a worker is expected
// to run successfully; and it doesn't Assert, and is therefore suitable for use
// from any goroutine.
func CleanKill(c *gc.C, w worker.Worker) {
	err := CheckKill(c, w)
	c.Check(err, jc.ErrorIsNil)
}

// DirtyKill calls CheckKill with the supplied arguments, and logs the returned
// error. It's particularly suitable for deferring:
//
//     someWorker, err := some.NewWorker()
//     c.Assert(err, jc.ErrorIsNil)
//     defer workertest.DirtyKill(c, someWorker)
//
// ...in the cases where we expect a worker to fail, but aren't specifically
// testing that failure; and it doesn't Assert, and is therefore suitable for
// use from any goroutine.
func DirtyKill(c *gc.C, w worker.Worker) {
	err := CheckKill(c, w)
	c.Logf("ignoring error: %v", err)
}
