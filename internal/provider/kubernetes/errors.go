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
}

func (e ClusterQueryError) Error() string {
	return e.Message
}

// IsClusterQueryError returns true if err is a ClusterQueryError.
func IsClusterQueryError(err error) bool {
	_, ok := err.(ClusterQueryError)
	return ok
}

// NoRecommendedStorageError represents when Juju is unable to determine which storage a cluster uses (or should use)
type NoRecommendedStorageError struct {
	Message      string
	ProviderName string
}

func (e NoRecommendedStorageError) Error() string {
	return e.Message
}

func (e NoRecommendedStorageError) StorageProvider() string {
	return e.ProviderName
}

// IsNoRecommendedStorageError returns true if err is a NoRecommendedStorageError
func IsNoRecommendedStorageError(err error) bool {
	_, ok := err.(NoRecommendedStorageError)
	return ok
}

// MaskError is used to signify that an error
// should not be reported back to the caller.
func MaskError(err error) bool {
	_, ok := errors.Cause(err).(*k8serrors.StatusError)
	return ok
}
