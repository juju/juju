// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/go-logr/logr"
	"github.com/juju/loggo"
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
	} else {
		k.Logger.Errorf(msg, keysAndValues...)
	}
}

// Info see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdapter) Info(msg string, keysAndValues ...interface{}) {
	k.Logger.Infof(msg, keysAndValues...)
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
