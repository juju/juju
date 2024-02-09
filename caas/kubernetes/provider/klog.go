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
		"lost connection to pod",
		"an error occurred forwarding",
		"error copying from remote stream to local connection",
		"error copying from local connection to remote stream",
	}
)

// klogAdaptor is an adaptor for Kubernetes logger onto juju loggo. We use this
// to suppress logging from client-go and force it through juju logging methods
type klogAdaptor struct {
	loggo.Logger
}

// newKlogAdaptor creates a new klog adaptor to juju loggo
func newKlogAdaptor() logr.Logger {
	return logr.New(&klogAdaptor{
		Logger: loggo.GetLogger("juju.kubernetes.klog"),
	})
}

func (k *klogAdaptor) Init(info logr.RuntimeInfo) {
}

// Enabled see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdaptor) Enabled(level int) bool {
	return true
}

// Error see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdaptor) Error(err error, msg string, keysAndValues ...interface{}) {
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
func (k *klogAdaptor) Info(level int, msg string, keysAndValues ...interface{}) {
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
func (k *klogAdaptor) V(level int) logr.LogSink {
	return k
}

// WithValues see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdaptor) WithValues(keysAndValues ...interface{}) logr.LogSink {
	return k
}

// WithName see https://pkg.go.dev/github.com/go-logr/logr#Logger
func (k *klogAdaptor) WithName(name string) logr.LogSink {
	return k
}
