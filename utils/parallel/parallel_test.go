// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package parallel_test

import (
	"sort"
	"sync"
	"time"

	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils/parallel"
)

type parallelSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&parallelSuite{})

func (*parallelSuite) TestParallelMaxPar(c *gc.C) {
	const (
		totalDo                 = 10
		maxConcurrentRunnersPar = 3
	)
	var mu sync.Mutex
	maxConcurrentRunners := 0
	nbRunners := 0
	nbRuns := 0
	parallelRunner := parallel.NewRun(maxConcurrentRunnersPar)
	for i := 0; i < totalDo; i++ {
		parallelRunner.Do(func() error {
			mu.Lock()
			nbRuns++
			nbRunners++
			if nbRunners > maxConcurrentRunners {
				maxConcurrentRunners = nbRunners
			}
			mu.Unlock()
			time.Sleep(time.Second / 10)
			mu.Lock()
			nbRunners--
			mu.Unlock()
			return nil
		})
	}
	err := parallelRunner.Wait()
	if nbRunners != 0 {
		c.Errorf("%d functions still running", nbRunners)
	}
	if nbRuns != totalDo {
		c.Errorf("all functions not executed; want %d got %d", totalDo, nbRuns)
	}
	c.Check(err, gc.IsNil)
	if maxConcurrentRunners != maxConcurrentRunnersPar {
		c.Errorf("wrong number of do's ran at once; want %d got %d", maxConcurrentRunnersPar, maxConcurrentRunners)
	}
}

type intError int

func (intError) Error() string {
	return "error"
}

func (*parallelSuite) TestParallelError(c *gc.C) {
	const (
		totalDo = 10
		errDo   = 5
	)
	parallelRun := parallel.NewRun(6)
	for i := 0; i < totalDo; i++ {
		i := i
		if i >= errDo {
			parallelRun.Do(func() error {
				return intError(i)
			})
		} else {
			parallelRun.Do(func() error {
				return nil
			})
		}
	}
	err := parallelRun.Wait()
	c.Check(err, gc.NotNil)
	errs := err.(parallel.Errors)
	c.Check(len(errs), gc.Equals, totalDo-errDo)
	ints := make([]int, len(errs))
	for i, err := range errs {
		ints[i] = int(err.(intError))
	}
	sort.Ints(ints)
	for i, n := range ints {
		c.Check(n, gc.Equals, i+errDo)
	}
}

func (*parallelSuite) TestZeroWorkerPanics(c *gc.C) {
	defer func() {
		r := recover()
		c.Check(r, gc.Matches, "parameter maxParallel must be >= 1")
	}()
	parallel.NewRun(0)
}
