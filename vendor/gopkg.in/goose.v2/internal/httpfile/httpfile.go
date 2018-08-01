package httpfile

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
)

// NoSkip holds the maximum number of bytes that we
// will read instead of discarding a connection in order
// to seek.
const NoSkip = 64 * 1024

// unlimited is used as a special case to signify an unlimited number of
// bytes. We trust that no files are as big as 4 exabytes for now.
const unlimited = int64(1 << 62)

// Request represents a request made by a File instance.
// It will usually be mapped into an HTTP request
// to the object that has been opened.
type Request struct {
	Method string
	Header http.Header
}

// Response represents a response from a Request.
// It will usually be created from an HTTP response.
type Response struct {
	StatusCode    int
	Header        http.Header
	ContentLength int64
	Body          io.ReadCloser
}

// Client represents a client that can send HTTP requests.
type Client interface {
	// Do sends an HTTP request to the file.
	// The provided header fields are added
	// to the request, and the given request is
	// send with the given method (GET or HEAD).
	Do(req *Request) (*Response, error)
}

// SupportedResponseStatuses holds the set of HTTP statuses that a
// response may legimately have.
var SupportedResponseStatuses = []int{
	http.StatusOK,
	http.StatusNotFound,
	http.StatusPartialContent,
	http.StatusPreconditionFailed,
}

var (
	// ErrNotFound is returned by Open when the file
	// does not exist.
	ErrNotFound = errors.New("file not found")

	// errChanged is returned by a readerAhead
	errChanged = errors.New("file has changed since it was opened")

	// errOutOfRange is returned by readerAhead to signify
	// that a given read request is out of the range of
	// available data.
	errOutOfRange = errors.New("read out of range")
)

type File struct {
	client Client

	// length holds the length of the file.
	// While initializing the file, this is -1
	// to signify that the length is unknown.
	length int64

	// etag holds the entity tag of the file, as returned by the
	// server in the Etag header. Note that this includes the double
	// quotes around it too.
	etag string

	// readAhead holds the number of bytes
	// to request in advance of any Read.
	readAhead int64

	// pos holds the current seek position of the reader.
	pos int64

	// reader0 represents the currently running GET request.
	reader0 *readerAhead

	// reader1 represents the speculative GET request
	// one block ahead of reader0. This is never
	// non-nil when reader0 is nil.
	reader1 *readerAhead
}

// Open opens a new file that uses the given client to issue read
// requests. It returns the open file and any HTTP header returned by
// the initial client request.
//
// If the file is not found, it returned ErrNotFound.
//
// The readAhead parameter governs the amount of data that will be
// requested before any Read calls are made. If it's zero, no data will
// be requested before Read calls are made. If it's -1, unlimited data
// will be requested; otherwise the given number of bytes will be
// requested.
//
// If readAhead is less than NoSkip and not -1, all HTTP connections
// will be available for reuse regardless of how Seek is used.
func Open(c Client, readAhead int64) (*File, http.Header, error) {
	if readAhead == -1 {
		readAhead = unlimited
	}
	f := &File{
		client:    c,
		length:    -1, // Unknown.
		readAhead: readAhead,
	}
	ra := f.newReaderAhead(0, readAhead)
	if err := ra.wait(); err != nil {
		if err == errChanged {
			// OpenStack can return StatusPreconditionFailed
			// when the container or object does not exist,
			// so return ErrNotFound in that case.
			return nil, ra.header, ErrNotFound
		}
		return nil, ra.header, err
	}
	f.reader0 = ra
	f.etag = ra.header.Get("Etag")
	f.length = ra.length
	return f, ra.header, nil
}

// Size returns the total size of the file.
func (f *File) Size() int64 {
	return f.length
}

// Read implements io.Reader.Read.
func (f *File) Read(buf []byte) (int, error) {
	if f.pos >= f.length {
		return 0, io.EOF
	}
	if f.reader0 == nil {
		f.reader0 = f.newReaderAhead(f.pos, f.pos+int64(len(buf)))
	}
	n, err := f.reader0.readAt(buf, f.pos)
	f.pos += int64(n)
	if err != nil && err != errOutOfRange {
		if err == io.EOF {
			return n, io.ErrUnexpectedEOF
		}
		return n, err
	}
	if err == nil {
		if f.reader1 == nil && f.reader0.remaining() < int64(f.readAhead/2) && f.reader0.p1 < f.length {
			// We're well advanced through the current reader,
			// so kick off a new one.
			f.reader1 = f.newReaderAhead(f.reader0.p1, f.reader0.p1+f.readAhead)
		}
		return n, nil
	}
	// We're trying to read out of range of the current reader, so
	// throw it away, replace reader0 with reader1 and try
	// everything again.
	f.reader0.close()
	f.reader0, f.reader1 = f.reader1, nil
	return f.Read(buf)
}

// Close implements io.Closer.Close. It does not block and never returns
// an error.
func (f *File) Close() error {
	if f.reader0 != nil {
		f.reader0.close()
		f.reader0 = nil
	}
	if f.reader1 != nil {
		f.reader1.close()
		f.reader1 = nil
	}
	return nil
}

// Seek implements io.Seeker.Seek.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	default:
		return 0, errors.New("Seek: invalid whence")
	case io.SeekStart:
		break
	case io.SeekCurrent:
		offset += f.pos
	case io.SeekEnd:
		offset += f.length
	}
	if offset < 0 {
		return 0, errors.New("Seek: invalid offset")
	}
	f.pos = offset
	return f.pos, nil
}

// readerAhead represents an in-progress data request.
type readerAhead struct {
	// reqp0 and reqp1 hold the range as
	// initially requested.
	reqp0, reqp1 int64

	done chan struct{}
	// The following fields are filled in with information about the
	// read request after the response has arrived. These are only
	// valid to read after the done channel has been closed.
	r          io.ReadCloser
	header     http.Header
	length     int64
	statusCode int
	err        error
	// p0 and p1 hold the actual range received,
	// which may not be the same as reqp0 and reqp1.
	p0, p1 int64
}

// newReaderAhead starts a request to read data
// in the range [p0, p1). It will constrain the read
// request if the end of the range is beyond the end
// of the file.
//
// If p1 is unlimited, the request will ask for all
// of the file from p0 onwards.
func (f *File) newReaderAhead(p0, p1 int64) *readerAhead {
	if f.length != -1 {
		if p1-p0 < f.readAhead {
			// Never issue a request for less than readAhead bytes
			p1 = p0 + f.readAhead
		}
		if p1 > f.length {
			// Constrain to end of file.
			p1 = f.length
		}
	}
	ra := &readerAhead{
		reqp0: p0,
		reqp1: p1,
		done:  make(chan struct{}),
	}
	go ra.do(f, f.client, newRequest(f.etag, p0, p1))
	return ra
}

// do initiates the client request and updates ra
// according to the response when it is available.
func (ra *readerAhead) do(f *File, client Client, req *Request) {
	defer close(ra.done)
	resp, err := client.Do(req)
	if resp != nil {
		ra.header = resp.Header
		ra.statusCode = resp.StatusCode
	}
	defer func() {
		if resp != nil && ra.err != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()
	if err != nil {
		ra.err = err
		return
	}
	switch resp.StatusCode {
	case http.StatusNotFound:
		ra.err = ErrNotFound
	case http.StatusPreconditionFailed:
		ra.err = errChanged
	case http.StatusOK, http.StatusPartialContent:
		ra.r = resp.Body
		if err := ra.updateRange(f, resp); err != nil {
			ra.err = err
			return
		}
		ra.r = resp.Body
	default:
		ra.err = fmt.Errorf("unexpected response status %v", resp.StatusCode)
	}
}

// updateRange updates ra's range and length from the
// given response.
func (ra *readerAhead) updateRange(f *File, resp *Response) error {
	cr, err := contentRangeFromResponse(resp)
	if err != nil {
		return err
	}
	if ra.reqp0 == ra.reqp1 {
		// Because we weren't asking for any data, we issued a
		// HEAD request which fools contentRangeFromResponse
		// into thinking it's getting the whole thing, but we're
		// actually getting nothing.
		cr.p1 = cr.p0
	}
	if f.length != -1 && cr.length != f.length {
		return fmt.Errorf("response range has unexpected length; got %d want %d", cr.length, f.length)
	}
	// In practice, servers are likely to return exactly the
	// requested range, but allow some leeway anyway.
	if cr.p0 > ra.reqp0 || cr.p0 < ra.reqp0-NoSkip {
		return fmt.Errorf("response range [%d, %d] out of range of requested range starting at %d", cr.p0, cr.p1, ra.reqp0)
	}
	ra.p0 = cr.p0
	ra.p1 = cr.p1
	ra.length = cr.length
	return nil
}

// wait waits for the request to complete and returns
// any error encountered.
func (ra *readerAhead) wait() error {
	<-ra.done
	return ra.err
}

// readAt tries to read data into buf at the given offset.
// It differs from the ioutil.ReaderAt contract in that
// it does not attempt to read the entire buffer when
// less data is immediately available.
func (ra *readerAhead) readAt(buf []byte, p0 int64) (int, error) {
	select {
	case <-ra.done:
	default:
		// The initial request is still in progress, so make an initial check
		// against the requested range, so we
		// don't bother waiting for the response if we're known to be
		// out of range.
		if p0 < ra.reqp0 || p0 >= ra.reqp1 || p0 > ra.reqp0+NoSkip {
			return 0, errOutOfRange
		}
	}
	if err := ra.wait(); err != nil {
		return 0, err
	}
	// Check the actual range, because it may be
	// different from the request.
	if p0 < ra.p0 || p0 >= ra.p1 || p0 > ra.p0+NoSkip || p0 >= ra.p1 {
		return 0, errOutOfRange
	}
	if p0 > ra.p0 {
		// We want to read a relatively short distance ahead of
		// the current position, so rather than return
		// errOutOfRange, we read and discard data until we get
		// to the right point.
		if err := ra.discard(p0 - ra.p0); err != nil {
			return 0, err
		}
	}
	n, err := ra.r.Read(buf)
	if err == io.EOF && n > 0 {
		// Readers are entitled to return EOF at the end of a
		// file, even when the read has been fulfilled. In our
		// case, we only want to return EOF if there really are
		// no more bytes to be had so suppress the EOF in that
		// case.
		err = nil
	}
	ra.p0 += int64(n)
	return n, err
}

// remaining returns the number of bytes remaining
// to be read.
func (ra *readerAhead) remaining() int64 {
	ra.wait()
	return ra.p1 - ra.p0
}

// close closes ra. It does not block.
func (ra *readerAhead) close() {
	select {
	case <-ra.done:
		if ra.r == nil {
			return
		}
		// Request is done already. Close the body
		// synchronously if there's nothing left to read
		// or we're going to throw the connection away
		// anyway.
		if remaining := ra.remaining(); remaining == 0 || remaining > NoSkip {
			ra.r.Close()
			return
		}
	default:
	}
	// Either the request hasn't completed yet or
	// there's something left to read, so start
	// a goroutine so that we don't block the
	// close.
	go func() {
		ra.wait()
		if ra.r == nil {
			return
		}
		if ra.remaining() <= NoSkip {
			// There's not much data left in the section, so
			// read it so that the HTTP connection can be
			// reused rather than discarded.
			ra.discard(ra.remaining())
		}
		ra.r.Close()
	}()
}

// discard reads and discards n bytes from the reader.
// It returns io.ErrUnexpectedEOF if all the bytes
// were not read successfully.
func (ra *readerAhead) discard(n int64) error {
	n, err := io.CopyN(ioutil.Discard, ra.r, n)
	ra.p0 += n
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}

// contentRangeFromResponse infers the response content
// range from the given response.
func contentRangeFromResponse(resp *Response) (contentRange, error) {
	if resp.StatusCode == http.StatusOK {
		// No range - the ContentLength should provide
		// the length and the range is the whole thing.
		if cr := resp.Header.Get("Content-Range"); cr != "" {
			return contentRange{}, fmt.Errorf("received unexpected Content-Range %q in response", cr)
		}
		if resp.ContentLength == -1 {
			return contentRange{}, fmt.Errorf("unknown file length in response")
		}
		return contentRange{
			p0:     0,
			p1:     resp.ContentLength,
			length: resp.ContentLength,
		}, nil
	}
	got := resp.Header.Get("Content-Range")
	if got == "" {
		return contentRange{}, fmt.Errorf("missing Content-Range in response")
	}
	r, ok := parseContentRange(got)
	if !ok {
		return contentRange{}, fmt.Errorf("bad Content-Range header %q", got)
	}
	return r, nil
}

// newRequest returns a new Request to read the data
// in the interval [p0, p1). If p1 is unlimited, an
// unlimited amount of data is requested.
// If etag is non-empty, the request is constrained
// to succeed only if the file's etag matches.
func newRequest(etag string, p0, p1 int64) *Request {
	header := make(http.Header)
	req := &Request{
		Method: "GET",
		Header: header,
	}
	switch {
	case p0 == p1:
		// No bytes requested - no need for a range request.
		req.Method = "HEAD"
	case p1 == unlimited && p0 == 0:
		// We want the whole thing - no need for a range request.
	case p1 == unlimited:
		// Indefinite range not starting from the beginning.
		// This case is here just for completeness - it doesn't
		// happen in practice.
		header.Set("Range", fmt.Sprintf("bytes=%d-", p0))
	default:
		// Note that the Range header uses a closed interval,
		// hence the -1.
		header.Set("Range", fmt.Sprintf("bytes=%d-%d", p0, p1-1))
	}
	if etag != "" {
		// Ensure that the read fails if the file has been
		// written to since we opened it.
		header.Set("If-Match", etag)
	}
	return req
}

// contentRange holds the values in a Content-Range header. The content
// holds the interval [p0, p1) - note that this corresponds to the usual
// Go convention of half-open intervals, even though the actual HTTP
// encoding uses closed intervals.
type contentRange struct {
	p0     int64
	p1     int64
	length int64
}

// parseContentRange parses a limited subset of the Content-Range as
// specified by RFC 7233, section 4.2 and reports whether it has parsed
// OK.
//
// It understands ranges of the form "bytes 42-1233/1234" but not "bytes
// 42-1233/*" or "*/1234".
func parseContentRange(s string) (r contentRange, ok bool) {
	s, ok = trimPrefix(s, "bytes ")
	if !ok {
		return contentRange{}, false
	}
	r.p0, s, ok = parseInt(s)
	if !ok {
		return contentRange{}, false
	}
	s, ok = trimPrefix(s, "-")
	if !ok {
		return contentRange{}, false
	}
	r.p1, s, ok = parseInt(s)
	if !ok {
		return contentRange{}, false
	}
	// Use the usual Go convention for half-open ranges.
	r.p1++
	s, ok = trimPrefix(s, "/")
	if !ok {
		return contentRange{}, false
	}
	r.length, s, ok = parseInt(s)
	if !ok {
		return contentRange{}, false
	}
	if s != "" {
		return contentRange{}, false
	}
	if r.p1 <= r.p0 {
		// Note that HTTP prohibits zero-length ranges.
		return contentRange{}, false
	}
	return r, true
}

func trimPrefix(s, prefix string) (string, bool) {
	if !strings.HasPrefix(s, prefix) {
		return "", false
	}
	return s[len(prefix):], true
}

func parseInt(s string) (int64, string, bool) {
	end := len(s)
	for i, c := range s {
		if c < '0' || c > '9' {
			end = i
			break
		}
	}
	n, err := strconv.ParseInt(s[0:end], 10, 64)
	if err != nil || n < 0 {
		return 0, "", false
	}
	return n, s[end:], true
}
