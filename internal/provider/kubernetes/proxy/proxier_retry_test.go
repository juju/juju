// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"context"
	stderrors "errors"
	"fmt"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/juju/caas/kubernetes"
	"github.com/juju/retry"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func TestRetryProxyConnectionRetriesTransientError(t *testing.T) {
	clock := testclock.NewClock(time.Time{})
	attempts := 0
	attempted := make(chan struct{}, 3)

	done := make(chan error, 1)
	go func() {
		done <- retry.Call(retry.CallArgs{
			Func: func() error {
				attempts++
				attempted <- struct{}{}

				if attempts < 3 {
					return &kubernetes.PortForwardError{
						Err: stderrors.New("etcdserver: leader changed"),
					}
				}
				return nil
			},
			IsFatalError: func(err error) bool {
				return !isRetryableProxyError(err)
			},
			Attempts: 3,
			Delay:    time.Second,
			Clock:    clock,
		})
	}()

	<-attempted
	clock.WaitAdvance(time.Second, time.Second, 1)

	<-attempted
	clock.WaitAdvance(time.Second, time.Second, 1)

	err := <-done
	if err != nil {
		t.Fatalf("retry.Call failed: %v", err)
	}

	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryProxyConnectionDoesNotRetryNonTransientError(t *testing.T) {
	clock := testclock.NewClock(time.Time{})
	attempts := 0
	expectedErr := stderrors.New("some non-transient error")

	err := retry.Call(retry.CallArgs{
		Func: func() error {
			attempts++
			return expectedErr
		},
		IsFatalError: func(err error) bool {
			return !isRetryableProxyError(err)
		},
		Attempts: 3,
		Delay:    time.Second,
		Clock:    clock,
	})

	if !stderrors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestRetryProxyConnectionStopsAfterMaxAttempts(t *testing.T) {
	clock := testclock.NewClock(time.Time{})
	attempts := 0
	transientError := apierrors.NewInternalError(
		stderrors.New("etcdserver: leader changed"),
	)

	done := make(chan error, 1)
	go func() {
		done <- retry.Call(retry.CallArgs{
			Func: func() error {
				attempts++
				return transientError
			},
			Attempts: 3,
			Delay:    time.Second,
			Clock:    clock,
		})
	}()

	clock.WaitAdvance(time.Second, time.Second, 1)
	clock.WaitAdvance(time.Second, time.Second, 1)

	err := <-done

	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	if err == nil {
		t.Fatal("expected an error after exhausting attempts")
	}
	if !isRetryableProxyError(retry.LastError(err)) {
		t.Fatalf("expected last error to be retryable, got: %v", retry.LastError(err))
	}
}

func TestIsRetryableProxyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "kubernetes internal error",
			err: apierrors.NewInternalError(
				stderrors.New("etcdserver: leader changed"),
			),
			want: true,
		},
		{
			name: "wrapped kubernetes internal error",
			err: fmt.Errorf(
				"connecting to proxy: %v",
				apierrors.NewInternalError(stderrors.New("etcdserver: leader changed")),
			),
			want: false,
		},
		{
			name: "unrelated error",
			err:  stderrors.New("error upgrading connection: error sending request: connection refused"),
			want: false,
		},
		{
			name: "leader changed without port forward context",
			err:  stderrors.New("etcdserver: leader changed"),
			want: false,
		},
		{
			name: "port forward error",
			err: &kubernetes.PortForwardError{
				Err: stderrors.New("etcdserver: leader changed"),
			},
			want: true,
		},
		{
			name: "traced kubernetes internal error",
			err: jujuerrors.Trace(
				apierrors.NewInternalError(
					stderrors.New("etcdserver: leader changed"),
				),
			),
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isRetryableProxyError(test.err); got != test.want {
				t.Fatalf("isRetryableProxyError() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRetryProxyConnectionContextCancellation(t *testing.T) {
	clock := testclock.NewClock(time.Time{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	attempts := 0

	err := retry.Call(retry.CallArgs{
		Func: func() error {
			attempts++
			cancel()

			return &kubernetes.PortForwardError{
				Err: stderrors.New("etcdserver: leader changed"),
			}
		},
		IsFatalError: func(err error) bool {
			return !isRetryableProxyError(err)
		},
		Attempts: 3,
		Delay:    time.Second,
		Clock:    clock,
		Stop:     ctx.Done(),
	})

	if !retry.IsRetryStopped(err) {
		t.Fatalf("expected retry stopped error, got %v", err)
	}

	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}
