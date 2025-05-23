// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/time/rate"

	corelogger "github.com/juju/juju/core/logger"
	internallogger "github.com/juju/juju/internal/logger"
)

// klogAdapter is an adapter for Kubernetes logger onto juju loggo. We use this
// to suppress logging from client-go and force it through juju logging methods
type klogAdapter struct {
	corelogger.Logger
}

// newKlogAdaptor creates a new klog adapter to juju loggo
func newKlogAdaptor() logr.Logger {
	return logr.New(&klogAdapter{
		Logger: internallogger.GetLogger("juju.kubernetes.klog"),
	})
}

func (k *klogAdapter) Init(info logr.RuntimeInfo) {}

// Enabled see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdapter) Enabled(level int) bool {
	return true
}

// Error see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdapter) Error(err error, msg string, keysAndValues ...any) {
	if err != nil {
		k.Logger.Errorf(context.TODO(), msg+": "+err.Error(), keysAndValues...)
		return
	}

	if klogIgnorePrefixes.Matches(msg) {
		return
	}
	k.Logger.Errorf(context.TODO(), msg, keysAndValues...)
}

// Info see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdapter) Info(level int, msg string, keysAndValues ...any) {
	if prefix, ok := klogSuppressedPrefixes.Matches(msg); ok && prefix != nil {
		prefix.Do(k.Logger.Infof, msg, keysAndValues...)
		return
	}

	k.Logger.Infof(context.TODO(), msg, keysAndValues...)
}

// V see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdapter) V(level int) logr.LogSink {
	return k
}

// WithValues see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdapter) WithValues(keysAndValues ...any) logr.LogSink {
	return k
}

// WithName see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdapter) WithName(name string) logr.LogSink {
	return k
}

// klogMessagePrefixes is a list of prefixes to ignore.
type klogMessagePrefixes []string

var (
	klogIgnorePrefixes = klogMessagePrefixes{
		"lost connection to pod",
		"an error occurred forwarding",
		"error copying from remote stream to local connection",
		"error copying from local connection to remote stream",
	}
)

func (k klogMessagePrefixes) Matches(log string) bool {
	for _, v := range k {
		if strings.HasPrefix(log, v) {
			return true
		}
	}
	return false
}

// klogSuppressMessagePrefix is a log suppression type that suppresses log
// messages with a given prefix at a given rate. If the rate is nil, the
// suppression is disabled.
type klogSuppressMessagePrefix struct {
	prefix string
	rate   *rate.Sometimes
}

// Do calls the logger function if the rate allows it. If the rate is nil, the
// function is called directly, thus bypassing the rate.
func (k klogSuppressMessagePrefix) Do(loggerFn func(context.Context, string, ...any), msg string, args ...any) {
	// If we don't have a rate, just call the function directly.
	if k.rate == nil {
		loggerFn(context.TODO(), msg, args)
		return
	}

	// If we have a rate, use it to suppress the message.
	k.rate.Do(func() {
		loggerFn(context.TODO(), msg, args)
	})
}

// klogSuppressMessagePrefixes is a list of prefixes to suppress and their
// suppression rates.
type klogSuppressMessagePrefixes []*klogSuppressMessagePrefix

var (
	klogSuppressedPrefixes = klogSuppressMessagePrefixes{
		&klogSuppressMessagePrefix{
			prefix: "Use tokens from the TokenRequest API or manually created secret-based tokens instead of auto-generated secret-based tokens",
			// We suppress the message at a rate of 1 per 5 minute, but we
			// allow the first message to go through.
			rate: &rate.Sometimes{
				First:    1,
				Interval: time.Minute * 5,
			},
		},
	}
)

// Matches returns the prefix and whether it matches the log message.
func (k klogSuppressMessagePrefixes) Matches(log string) (*klogSuppressMessagePrefix, bool) {
	for _, p := range k {
		if p == nil {
			continue
		}
		if strings.HasPrefix(log, p.prefix) {
			return p, true
		}
	}
	return nil, false
}
