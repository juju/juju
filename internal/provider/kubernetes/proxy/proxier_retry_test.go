// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"context"
	"errors"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"net/url"
	"testing"
)

func TestRetryProxyConnectionRetriesTransientError(t *testing.T) {
	attempts := 0

	err := retryProxyConnection(context.Background(), func(context.Context) error {
		attempts++
		if attempts < 3 {
			return fmt.Errorf("forwarding ports: %v", errors.New("etcdserver: leader changed"))
		}
		return nil
	})

	if err != nil {
		t.Fatalf("retryProxyConnection failed: %v", err)
	}

	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryProxyConnectionDoesNotRetryNonTransientError(t *testing.T) {
	attempts := 0
	expectedErr := errors.New("some non-transient error")

	err := retryProxyConnection(context.Background(), func(context.Context) error {
		attempts++
		return expectedErr
	})

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestRetryProxyConnectionStopsAfterMaxAttempts(t *testing.T) {
	attempts := 0

	err := retryProxyConnection(context.Background(), func(context.Context) error {
		attempts++
		return fmt.Errorf("forwarding ports: %v", errors.New("etcdserver: leader changed"))
	})

	if err == nil {
		t.Fatal("expected retryProxyConnection to return an error")
	}

	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
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
				errors.New("etcdserver: leader changed"),
			),
			want: true,
		},
		{
			name: "wrapped kubernetes internal error",
			err: fmt.Errorf(
				"forwarding ports: %w",
				apierrors.NewInternalError(errors.New("etcdserver: leader changed")),
			),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("error upgrading connection: error sending request: connection refused"),
			want: false,
		},
		{
			name: "leader changed without port forward context",
			err:  errors.New("etcdserver: leader changed"),
			want: false,
		},
		{
			name: "url error",
			err: &url.Error{
				Op:  "POST",
				URL: "https://kubernetes.example/api",
				Err: fmt.Errorf("forwarding ports: %v", errors.New("etcdserver: leader changed")),
			},
			want: true,
		},
		{
			name: "forwarding ports error",
			err: fmt.Errorf(
				"forwarding ports: %v",
				apierrors.NewInternalError(errors.New("etcdserver: leader changed")),
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
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0

	err := retryProxyConnection(ctx, func(context.Context) error {
		attempts++
		cancel()
		return fmt.Errorf(
			"forwarding ports: %v", errors.New("etcdserver: leader changed"),
		)
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}
