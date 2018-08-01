package httpbakery

import (
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	errgo "gopkg.in/errgo.v1"
)

// newRetrableRequest wraps an HTTP request so that it can
// be retried without incurring race conditions and reports
// whether the request can be retried.
// The client instance will be used to make the request
// when the do method is called.
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
func newRetryableRequest(client *http.Client, req *http.Request) (*retryableRequest, bool) {
	if req.Body == nil {
		return &retryableRequest{
			client:     client,
			ref:        1,
			req:        req,
			origCookie: req.Header.Get("Cookie"),
		}, true
	}
	body := seekerFromBody(req.Body)
	if body == nil {
		return nil, false
	}
	return &retryableRequest{
		client:     client,
		ref:        1,
		req:        req,
		body:       body,
		origCookie: req.Header.Get("Cookie"),
	}, true
}

type retryableRequest struct {
	client      *http.Client
	ref         int32
	origCookie  string
	body        readSeekCloser
	readStopper *readStopper
	req         *http.Request
}

// do performs the HTTP request.
func (rreq *retryableRequest) do(ctx context.Context) (*http.Response, error) {
	req, err := rreq.prepare()
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return ctxhttp.Do(ctx, rreq.client, req)
}

// prepare returns a new HTTP request object
// by copying the original request and seeking
// back to the start of the original body if needed.
//
// It needs to make a copy of the request because
// the HTTP code can access the Request.Body field
// after Client.Do has returned, which means we can't
// replace it for the second request.
func (rreq *retryableRequest) prepare() (*http.Request, error) {
	req := new(http.Request)
	*req = *rreq.req
	// Make sure that the original cookie header is still in place
	// so that we only end up with the cookies that are actually
	// added by the HTTP cookie logic, and not the ones that were
	// added in previous requests too.
	req.Header.Set("Cookie", rreq.origCookie)
	if rreq.body == nil {
		// No need for any of the seek shenanigans.
		return req, nil
	}
	if rreq.readStopper != nil {
		// We've made a previous request. Close its request
		// body so it can't interfere with the new request's body
		// and then seek back to the start.
		rreq.readStopper.Close()
		if _, err := rreq.body.Seek(0, 0); err != nil {
			return nil, errgo.Notef(err, "cannot seek to start of request body")
		}
	}
	atomic.AddInt32(&rreq.ref, 1)
	// Replace the request body with a new readStopper so that
	// we can stop a second request from interfering with current
	// request's body.
	rreq.readStopper = &readStopper{
		rreq: rreq,
		r:    rreq.body,
	}
	req.Body = rreq.readStopper
	return req, nil
}

// close closes the request. It closes the underlying reader
// when all references have gone.
func (req *retryableRequest) close() error {
	if atomic.AddInt32(&req.ref, -1) == 0 && req.body != nil {
		// We've closed it for the last time, so actually close
		// the original body.
		return req.body.Close()
	}
	return nil
}

// readStopper works around an issue with the net/http
// package (see http://golang.org/issue/12796).
// Because the first HTTP request might not have finished
// reading from its body when it returns, we need to
// ensure that the second request does not race on Read,
// so this type implements a Reader that prevents all Read
// calls to the underlying Reader after Close has been called.
type readStopper struct {
	rreq *retryableRequest
	mu   sync.Mutex
	r    io.ReadSeeker
}

func (r *readStopper) Read(buf []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.r == nil {
		// Note: we have to use io.EOF here because otherwise
		// another connection can in rare circumstances be
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
	alreadyClosed := r.r == nil
	r.r = nil
	r.mu.Unlock()
	if alreadyClosed {
		return nil
	}
	return r.rreq.close()
}

var nopCloserType = reflect.TypeOf(ioutil.NopCloser(nil))

type readSeekCloser interface {
	io.ReadSeeker
	io.Closer
}

// seekerFromBody tries to obtain a seekable reader
// from the given request body.
func seekerFromBody(r io.ReadCloser) readSeekCloser {
	if r, ok := r.(readSeekCloser); ok {
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
	rs, ok := rv.Field(0).Interface().(io.ReadSeeker)
	if !ok {
		return nil
	}
	return readSeekerWithNopClose{rs}
}

type readSeekerWithNopClose struct {
	io.ReadSeeker
}

func (r readSeekerWithNopClose) Close() error {
	return nil
}
