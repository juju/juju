// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/progress"
	"github.com/vmware/govmomi/vim25/types"
)

type taskWaiter struct {
	clock                  clock.Clock
	updateProgress         func(string)
	updateProgressInterval time.Duration
}

func (w *taskWaiter) waitTask(ctx context.Context, t *object.Task, action string) (*types.TaskInfo, error) {
	var info *types.TaskInfo
	var err error
	withStatusUpdater(
		ctx, action, w.clock,
		w.updateProgress,
		w.updateProgressInterval,
		func(ctx context.Context, sinker progress.Sinker) {
			info, err = t.WaitForResult(ctx, sinker)
		},
	)
	return info, errors.Trace(err)
}

func withStatusUpdater(
	ctx context.Context,
	action string,
	clock clock.Clock,
	updateProgress func(string),
	updateProgressInterval time.Duration,
	f func(context.Context, progress.Sinker),
) {
	statusUpdater := statusUpdater{
		ch:       make(chan progress.Report),
		clock:    clock,
		update:   updateProgress,
		action:   action,
		interval: updateProgressInterval,
	}
	var wg sync.WaitGroup
	wg.Add(1)
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer wg.Done()
		statusUpdater.loop(ctx.Done())
	}()
	defer wg.Wait()
	defer cancel()
	f(ctx, &statusUpdater)
}

type statusUpdater struct {
	clock    clock.Clock
	ch       chan progress.Report
	update   func(string)
	action   string
	interval time.Duration
}

// Sink is part of the progress.Sinker interface.
func (u *statusUpdater) Sink() chan<- progress.Report {
	return u.ch
}

func (u *statusUpdater) loop(done <-chan struct{}) {
	timer := u.clock.NewTimer(u.interval)
	defer timer.Stop()
	var timerChan <-chan time.Time

	var message string
	for {
		select {
		case <-done:
			return
		case <-timerChan:
			u.update(message)
			timer.Reset(u.interval)
			timerChan = nil
		case report, ok := <-u.ch:
			if !ok {
				return
			}
			if err := report.Error(); err != nil {
				message = fmt.Sprintf("%s: %s", u.action, err)
			} else {
				message = fmt.Sprintf(
					"%s: %.2f%%",
					u.action,
					report.Percentage(),
				)
				if detail := report.Detail(); detail != "" {
					message += " (" + detail + ")"
				}
			}
			timerChan = timer.Chan()
		}
	}
}
