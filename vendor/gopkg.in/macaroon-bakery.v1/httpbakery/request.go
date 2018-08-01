package httpbakery

import (
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"

	errgo "gopkg.in/errgo.v1"
)

// newRetrableRequest wraps an HTTP request so that it can
// be retried without incurring race conditions and reports
// whether the request can be retried.
//
// Because http.NewRequest often wraps its request bodies
// with ioutil.NopCloser, which hides whether the body is
// seekable, we extract the seeker from inside the nopCloser if
// possible.
//
// We also work around Go issue 12796 by preventing concurrent
// reads to the underlying reader after the request body has
// been closed by Client.Do.
//
// The returned value should be closed after use.
func newRetryableRequest(req *http.Request) (*retryableRequest, bool) {
	if req.Body == nil {
		return &retryableRequest{
			ref: 1,
			req: req,
		}, true
	}
	body := seekerFromBody(req.Body)
	if body == nil {
		return nil, false
	}
	rreq := &retryableRequest{
		ref:      1,
		req:      req,
		origBody: req.Body,
		body:     body,
	}
	req.Body = nil
	return rreq, true
}

type retryableRequest struct {
	ref      int32
	origBody io.ReadCloser
	body     io.ReadSeeker
	req      *http.Request
}

// try should be called just before invoking http.Client.Do.
func (req *retryableRequest) try() error {
	if req.body == nil {
		return nil
	}
	if req.req.Body != nil {
		// Close the old readStopper.
		req.req.Body.Close()
		if _, err := req.body.Seek(0, 0); err != nil {
			return errgo.Notef(err, "cannot seek to start of request body")
		}
	}
	atomic.AddInt32(&req.ref, 1)
	// Replace the body with a new readStopper so that
	// the old request cannot interfere with the new request's reader.
	req.req.Body = &readStopper{
		req: req,
		r:   req.body,
	}
	return nil
}

// close closes the request. It closes the underlying reader
// when all references have gone.
func (req *retryableRequest) close() {
	if atomic.AddInt32(&req.ref, -1) == 0 {
		// We've closed it for the last time, so actually close
		// the original body.
		if req.origBody != nil {
			req.origBody.Close()
		}
	}
}

// readStopper works around an issue with the net/http
// package (see http://golang.org/issue/12796).
// Because the first HTTP request might not have finished
// reading from its body when it returns, we need to
// ensure that the second request does not race on Read,
// so this type implements a Reader that prevents all Read
// calls to the underlying Reader after Close has been called.
type readStopper struct {
	req *retryableRequest
	mu  sync.Mutex
	r   io.ReadSeeker
}

func (r *readStopper) Read(buf []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.r == nil {
		// Note: we have to use io.EOF here because otherwise
		// another connection can (in rare circumstances) be
		// polluted by the error returned here. Although this
		// means the file may appear truncated to the server,
		// that shouldn't matter because the body will only
		// be closed after the server has replied.
		return 0, io.EOF
	}
	return r.r.Read(buf)
}

func (r *readStopper) Close() error {
	r.mu.Lock()
	closed := r.r == nil
	r.r = nil
	r.mu.Unlock()
	if !closed {
		r.req.close()
	}
	return nil
}

var nopCloserType = reflect.TypeOf(ioutil.NopCloser(nil))

// seekerFromBody tries to obtain a seekable reader
// from the given request body.
func seekerFromBody(r io.ReadCloser) io.ReadSeeker {
	if r, ok := r.(io.ReadSeeker); ok {
		return r
	}
	rv := reflect.ValueOf(r)
	if rv.Type() != nopCloserType {
		return nil
	}
	// It's a value created by nopCloser. Extract the
	// underlying Reader. Note that this works
	// because the ioutil.nopCloser type exports
	// its Reader field.
	rs, _ := rv.Field(0).Interface().(io.ReadSeeker)
	return rs
}
