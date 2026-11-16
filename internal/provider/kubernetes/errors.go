// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"github.com/juju/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	errNoNamespace = errors.NotProvisionedf("bootstrap broker or no namespace")
)

// ClusterQueryError represents an issue when querying a cluster.
type ClusterQueryError struct {
	Message string
	// Err is the underlying error that caused the cluster query to fail,
	// if any. Callers can use errors.Is / errors.As on a ClusterQueryError
	// to inspect the cause.
	Err error
}

func (e ClusterQueryError) Error() string {
	return e.Message
}

// Unwrap returns the underlying error, if any.
func (e ClusterQueryError) Unwrap() error {
	return e.Err
}

// IsClusterQueryError returns true if err is a ClusterQueryError.
func IsClusterQueryError(err error) bool {
	var e ClusterQueryError
	return errors.As(err, &e)
}

// MaskError is used to signify that an error
// should not be reported back to the caller.
func MaskError(err error) bool {
	_, ok := errors.Cause(err).(*k8serrors.StatusError)
	return ok
}
