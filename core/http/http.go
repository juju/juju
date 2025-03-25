// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"context"
	"net/http"

	"github.com/juju/juju/internal/errors"
)

const (
	// ErrHTTPClientDying is used to indicate to *third parties* that the
	// http client worker is dying, instead of catacomb.ErrDying, which is
	// unsuitable for propagating inter-worker.
	// This error indicates to consuming workers that their dependency has
	// become unmet and a restart by the dependency engine is imminent.
	ErrHTTPClientDying = errors.ConstError("http client worker is dying")
)

// HTTPClientGetter is the interface that is used to get a http client for a
// given namespace.
type HTTPClientGetter interface {
	// GetHTTPClient returns a http client for the given namespace.
	GetHTTPClient(context.Context, Purpose) (HTTPClient, error)
}

// HTTPClient is the interface that is used to do http requests.
type HTTPClient interface {
	// Do sends an HTTP request and returns an HTTP response. The client will
	// follow policy (such as redirects, cookies, auth) as configured on the
	// client.
	Do(*http.Request) (*http.Response, error)
}

// Purpose is a type used to define the namespace of a http client.
// This allows multiple http clients to be created with different namespaces.
type Purpose string

const (
	// CharmhubPurpose is the namespace for the charmhub http client.
	CharmhubPurpose Purpose = "charmhub"
	// S3Purpose is the namespace for the s3 http client.
	S3Purpose Purpose = "s3"
	// SSHImporterPurpose is the namespace for the ssh importer http client.
	SSHImporterPurpose Purpose = "ssh-importer"
)

func (n Purpose) String() string {
	return string(n)
}
