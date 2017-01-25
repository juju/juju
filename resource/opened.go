// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

// TODO(ericsnow) Move this file to the charm repo?

import (
	"io"
	"strings"

	"github.com/juju/errors"
)

type multiError []error

func (m multiError) Error() string {
	messages := make([]string, len(m))
	for i, err := range m {
		messages[i] = err.Error()
	}
	return strings.Join(messages, ", and also ")
}

// CombineErrors converts a set of errors (which might be nil) into
// one. If there are no errors, this returns an untyped nil, and if
// there's one error it's passed through directly.
func CombineErrors(errs ...error) error {
	merr := make(multiError, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			merr = append(merr, err)
		}
	}
	if len(merr) == 0 {
		return nil
	}
	if len(merr) == 1 {
		return merr[0]
	}
	return merr
}

// Opened provides both the resource info and content.
type Opened struct {
	Resource
	io.ReadCloser

	Closer func() error
}

// Content returns the "content" for the opened resource.
func (o Opened) Content() Content {
	return Content{
		Data:        o.ReadCloser,
		Size:        o.Size,
		Fingerprint: o.Fingerprint,
	}
}

func (o Opened) Close() error {
	var err1 error
	if o.Closer != nil {
		err1 = errors.Trace(o.Closer())
	}
	err2 := errors.Trace(o.ReadCloser.Close())
	return CombineErrors(err1, err2)
}

// Opener exposes the functionality for opening a resource.
type Opener interface {
	// OpenResource returns an opened resource with a reader that will
	// stream the resource content.
	OpenResource(name string) (Opened, error)
}
