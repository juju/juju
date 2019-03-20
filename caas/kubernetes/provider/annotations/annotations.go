// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

const jujuAnnotationPrefix = "juju.io"

var (
	logger = loggo.GetLogger("juju.kubernetes.provider.annotations")
)

// Annotation extends k8s annotation map.
type Annotation struct {
	prefix string
	vals   map[string]string
}

// New contructs an annotation.
func New(as map[string]string) Annotation {
	newA := Annotation{prefix: jujuAnnotationPrefix, vals: make(map[string]string)}
	if as == nil {
		return newA
	}
	for k, v := range as {
		newA.Add(k, v)
	}
	return newA
}

// Exist check if the provided key value pair exists in this annotation or not.
func (a Annotation) Exist(key, expectedValue string) bool {
	v, ok := a.getVal(key)
	return ok && v == expectedValue
}

// ExistAll check if all the provided key value pairs exist in this annotation or not.
func (a Annotation) ExistAll(expected map[string]string) bool {
	for k, v := range expected {
		if !a.Exist(k, v) {
			return false
		}
	}
	return true
}

// ExistAny check if any provided key value pairs exists in this annotation or not.
func (a Annotation) ExistAny(expected map[string]string) bool {
	for k, v := range expected {
		if a.Exist(k, v) {
			return true
		}
	}
	return false
}

// Add inserts a new key value pair.
func (a Annotation) Add(key, value string) Annotation {
	a.setVal(key, value)
	return a
}

// Merge merges an annotation with current one.
func (a Annotation) Merge(as Annotation) Annotation {
	for k, v := range as.ToMap() {
		a.Add(k, v)
	}
	return a
}

// ToMap returns the map format of the annotation.
func (a Annotation) ToMap() map[string]string {
	return a.vals
}

// CheckKeysNonEmpty checks if the provided keys are all set to non empty value.
func (a Annotation) CheckKeysNonEmpty(keys ...string) error {
	for _, k := range keys {
		if v, ok := a.getVal(k); !ok || v == "" {
			return errors.NotFoundf("annotation key %q", k)
		}
	}
	return nil
}

func (a Annotation) getKey(key string) string {
	if strings.HasPrefix(key, a.prefix) {
		return key
	}
	return a.prefix + "/" + key
}

// getVal returns the value for the specified key.
func (a Annotation) getVal(key string) (string, bool) {
	v, ok := a.vals[a.getKey(key)]
	return v, ok
}

func (a Annotation) setVal(key, val string) {
	key = a.getKey(key)
	if val == "" {
		logger.Warningf("setting empty value for annotation %q", key)
	}

	oldVal, existing := a.getVal(key)
	if existing {
		logger.Debugf("annotation %q changed from %q to %q", key, oldVal, val)
	}
	a.vals[key] = val
}
