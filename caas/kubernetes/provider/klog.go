// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"strings"

	"github.com/go-logr/logr"
	"github.com/juju/loggo/v2"
)

type KlogMessagePrefixes []string

var (
	klogIgnorePrefixes = KlogMessagePrefixes{
		"an error occurred forwarding",
		"error copying from remote stream to local connection",
		"error copying from local connection to remote stream",
	}
)

// klogAdapter is an adapter for Kubernetes logger onto juju loggo. We use this
// to suppress logging from client-go and force it through juju logging methods
type klogAdapter struct {
	loggo.Logger
}

// newKlogAdapter creates a new klog adapter to juju loggo
func newKlogAdapter() *klogAdapter {
	return &klogAdapter{
		Logger: loggo.GetLogger("juju.kubernetes.klog"),
	}
}

// Enabled see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdapter) Enabled() bool {
	return true
}

// Error see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdapter) Error(err error, msg string, keysAndValues ...interface{}) {
	if err != nil {
		k.Logger.Errorf(msg+": "+err.Error(), keysAndValues...)
		return
	}

	if klogIgnorePrefixes.Matches(msg) {
		return
	}
	k.Logger.Errorf(msg, keysAndValues...)
}

// Info see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdapter) Info(msg string, keysAndValues ...interface{}) {
	k.Logger.Infof(msg, keysAndValues...)
}

func (k KlogMessagePrefixes) Matches(log string) bool {
	for _, v := range k {
		if strings.HasPrefix(log, v) {
			return true
		}
	}
	return false
}

// V see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdapter) V(level int) logr.Logger {
	return k
}

// WithValues see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdapter) WithValues(keysAndValues ...interface{}) logr.Logger {
	return k
}

// WithName see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdapter) WithName(name string) logr.Logger {
	return k
}
