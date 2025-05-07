// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"io"

	"github.com/juju/juju/internal/testhelpers"
)

type StubDataSource struct {
	testhelpers.Stub

	DescriptionFunc      func() string
	FetchFunc            func(path string) (io.ReadCloser, string, error)
	URLFunc              func(path string) (string, error)
	PublicSigningKeyFunc func() string
	PriorityFunc         func() int
	RequireSignedFunc    func() bool
}

func NewStubDataSource() *StubDataSource {
	result := &StubDataSource{
		DescriptionFunc: func() string {
			return ""
		},
		PublicSigningKeyFunc: func() string {
			return ""
		},
		PriorityFunc: func() int {
			return 0
		},
		RequireSignedFunc: func() bool {
			return false
		},
	}
	result.FetchFunc = func(path string) (io.ReadCloser, string, error) {
		return nil, "", result.Stub.NextErr()
	}
	result.URLFunc = func(path string) (string, error) {
		return "", result.Stub.NextErr()
	}
	return result
}

// Description implements simplestreams.DataSource.
func (s *StubDataSource) Description() string {
	s.MethodCall(s, "Description")
	return s.DescriptionFunc()
}

// Fetch implements simplestreams.DataSource.
func (s *StubDataSource) Fetch(ctx context.Context, path string) (io.ReadCloser, string, error) {
	s.MethodCall(s, "Fetch", path)
	return s.FetchFunc(path)
}

// URL implements simplestreams.DataSource.
func (s *StubDataSource) URL(path string) (string, error) {
	s.MethodCall(s, "URL", path)
	return s.URLFunc(path)
}

// PublicSigningKey implements simplestreams.DataSource.
func (s *StubDataSource) PublicSigningKey() string {
	s.MethodCall(s, "PublicSigningKey")
	return s.PublicSigningKeyFunc()
}

// Priority implements simplestreams.DataSource.
func (s *StubDataSource) Priority() int {
	s.MethodCall(s, "Priority")
	return s.PriorityFunc()
}

// RequireSigned implements simplestreams.DataSource.
func (s *StubDataSource) RequireSigned() bool {
	s.MethodCall(s, "RequireSigned")
	return s.RequireSignedFunc()
}
