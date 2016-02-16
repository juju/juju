// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io"

	"github.com/juju/testing"
)

type StubDataSource struct {
	*testing.Stub

	DescriptionFunc      func() string
	FetchFunc            func(path string) (io.ReadCloser, string, error)
	URLFunc              func(path string) (string, error)
	PublicSigningKeyFunc func() string
	SetAllowRetryFunc    func(allow bool)
	PriorityFunc         func() int
	RequireSignedFunc    func() bool
}

func NewStubDataSource() *StubDataSource {
	result := &StubDataSource{
		Stub: &testing.Stub{},
	}
	result.DescriptionFunc = func() string {
		return ""
	}
	result.FetchFunc = func(path string) (io.ReadCloser, string, error) {
		return nil, "", result.Stub.NextErr()
	}
	result.URLFunc = func(path string) (string, error) {
		return "", result.Stub.NextErr()
	}
	result.PublicSigningKeyFunc = func() string {
		return ""
	}
	result.SetAllowRetryFunc = func(allow bool) {}
	result.PriorityFunc = func() int {
		return 0
	}
	result.RequireSignedFunc = func() bool {
		return false
	}
	return result
}

// Description implements simplestreams.DataSource.
func (s *StubDataSource) Description() string {
	s.MethodCall(s, "Description")
	return s.DescriptionFunc()
}

// Description implements simplestreams.DataSource.
func (s *StubDataSource) Fetch(path string) (io.ReadCloser, string, error) {
	s.MethodCall(s, "Fetch", path)
	return s.FetchFunc(path)
}

// Description implements simplestreams.DataSource.
func (s *StubDataSource) URL(path string) (string, error) {
	s.MethodCall(s, "URL", path)
	return s.URLFunc(path)
}

// Description implements simplestreams.DataSource.
func (s *StubDataSource) PublicSigningKey() string {
	s.MethodCall(s, "PublicSigningKey")
	return s.PublicSigningKeyFunc()
}

// Description implements simplestreams.DataSource.
func (s *StubDataSource) SetAllowRetry(allow bool) {
	s.MethodCall(s, "SetAllowRetry", allow)
	s.SetAllowRetryFunc(allow)
}

// Description implements simplestreams.DataSource.
func (s *StubDataSource) Priority() int {
	s.MethodCall(s, "Priority")
	return s.PriorityFunc()
}

// Description implements simplestreams.DataSource.
func (s *StubDataSource) RequireSigned() bool {
	s.MethodCall(s, "RequireSigned")
	return s.RequireSignedFunc()
}
