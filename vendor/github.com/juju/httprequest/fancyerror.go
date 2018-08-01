package httprequest

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"gopkg.in/errgo.v1"
)

func isDecodeResponseError(err error) bool {
	_, ok := err.(*DecodeResponseError)
	return ok
}

// DecodeResponseError represents an error when an HTTP
// response could not be decoded.
type DecodeResponseError struct {
	// Response holds the problematic HTTP response.
	// The body of this does not need to be closed
	// and may be truncated if the response is large.
	Response *http.Response

	// DecodeError holds the error that was encountered
	// when decoding.
	DecodeError error
}

func (e *DecodeResponseError) Error() string {
	return e.DecodeError.Error()
}

// newDecodeResponseError returns a new DecodeResponseError that
// uses the given error for its message. The Response field
// holds a copy of req. If bodyData is non-nil, it
// will be used as the data in the Response.Body field;
// otherwise body data will be read from req.Body.
func newDecodeResponseError(resp *http.Response, bodyData []byte, err error) *DecodeResponseError {
	if bodyData == nil {
		bodyData = readBodyForError(resp.Body)
	}
	resp1 := *resp
	resp1.Body = ioutil.NopCloser(bytes.NewReader(bodyData))

	return &DecodeResponseError{
		Response:    &resp1,
		DecodeError: errgo.Mask(err, errgo.Any),
	}
}

// newDecodeRequestError returns a new DecodeRequestError that
// uses the given error for its message. The Request field
// holds a copy of req. If bodyData is non-nil, it
// will be used as the data in the Request.Body field;
// otherwise body data will be read from req.Body.
func newDecodeRequestError(req *http.Request, bodyData []byte, err error) *DecodeRequestError {
	if bodyData == nil {
		bodyData = readBodyForError(req.Body)
	}
	req1 := *req
	req1.Body = ioutil.NopCloser(bytes.NewReader(bodyData))

	return &DecodeRequestError{
		Request:     &req1,
		DecodeError: errgo.Mask(err, errgo.Any),
	}
}

// DecodeRequestError represents an error when an HTTP
// request could not be decoded.
type DecodeRequestError struct {
	// Request holds the problematic HTTP request.
	// The body of this does not need to be closed
	// and may be truncated if the response is large.
	Request *http.Request

	// DecodeError holds the error that was encountered
	// when decoding.
	DecodeError error
}

func (e *DecodeRequestError) Error() string {
	return e.DecodeError.Error()
}

// fancyDecodeError is an error type that tries to
// produce a nice error message when the content
// type of a request or response is wrong.
type fancyDecodeError struct {
	// contentType holds the contentType of the request or response.
	contentType string

	// body holds up to maxErrorBodySize saved bytes of the
	// request or response body.
	body []byte
}

func newFancyDecodeError(h http.Header, body io.Reader) *fancyDecodeError {
	return &fancyDecodeError{
		contentType: h.Get("Content-Type"),
		body:        readBodyForError(body),
	}
}

func readBodyForError(r io.Reader) []byte {
	data, _ := ioutil.ReadAll(io.LimitReader(noErrorReader{r}, int64(maxErrorBodySize)))
	return data
}

// maxErrorBodySize holds the maximum amount of body that
// we try to read for an error before extracting text from it.
// It's reasonably large because:
// a) HTML often has large embedded scripts which we want
// to skip and
// b) it should be an relatively unusual case so the size
// shouldn't harm.
//
// It's defined as a variable so that it can be redefined in tests.
var maxErrorBodySize = 200 * 1024

// isJSONMediaType reports whether the content type of the given header implies
// that the content is JSON.
func isJSONMediaType(header http.Header) bool {
	contentType := header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	return mediaType == "application/json"
}

// Error implements error.Error by trying to produce a decent
// error message derived from the body content.
func (e *fancyDecodeError) Error() string {
	mediaType, _, err := mime.ParseMediaType(e.contentType)
	if err != nil {
		// Even if there's no media type, we want to see something useful.
		mediaType = fmt.Sprintf("%q", e.contentType)
	}

	// TODO use charset.NewReader to convert from non-utf8 content?
	switch mediaType {
	case "text/html":
		text, err := htmlToText(bytes.NewReader(e.body))
		if err != nil {
			// Note: it seems that this can never actually
			// happen - the only way that the HTML parser
			// can fail is if there's a read error and we've
			// removed that possibility by using
			// noErrorReader above.
			return fmt.Sprintf("unexpected (and invalid) content text/html; want application/json; content: %q", sizeLimit(e.body))
		}
		if len(text) == 0 {
			return fmt.Sprintf(`unexpected content type text/html; want application/json; content: %q`, sizeLimit(e.body))
		}
		return fmt.Sprintf(`unexpected content type text/html; want application/json; content: %s`, sizeLimit(text))
	case "text/plain":
		return fmt.Sprintf(`unexpected content type text/plain; want application/json; content: %s`, sizeLimit(sanitizeText(string(e.body), true)))
	default:
		return fmt.Sprintf(`unexpected content type %s; want application/json; content: %q`, mediaType, sizeLimit(e.body))
	}
}

// noErrorReader wraps a reader, turning any errors into io.EOF
// so that we can extract some content even if we get an io error.
type noErrorReader struct {
	r io.Reader
}

func (r noErrorReader) Read(buf []byte) (int, error) {
	n, err := r.r.Read(buf)
	if err != nil {
		err = io.EOF
	}
	return n, err
}

func sizeLimit(data []byte) []byte {
	const max = 1024
	if len(data) < max {
		return data
	}
	return append(data[0:max], fmt.Sprintf(" ... [%d bytes omitted]", len(data)-max)...)
}

// htmlToText attempts to return some relevant textual content
// from the HTML content in the given reader, formatted
// as a single line.
func htmlToText(r io.Reader) ([]byte, error) {
	n, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	htmlNodeToText(&buf, n)
	return buf.Bytes(), nil
}

// htmlNodeToText tries to extract some text from an arbitrary HTML
// page. It doesn't try to avoid looking in the header, because the
// title is in the header and is often the most succinct description of
// the page.
func htmlNodeToText(w *bytes.Buffer, n *html.Node) {
	for ; n != nil; n = n.NextSibling {
		switch n.Type {
		case html.TextNode:
			data := sanitizeText(n.Data, false)
			if len(data) == 0 {
				break
			}
			if w.Len() > 0 {
				w.WriteString("; ")
			}
			w.Write(data)
		case html.ElementNode:
			if n.DataAtom != atom.Script {
				htmlNodeToText(w, n.FirstChild)
			}
		case html.DocumentNode:
			htmlNodeToText(w, n.FirstChild)
		}
	}
}

// sanitizeText tries to make the given string easier to read when presented
// as a single line. It squashes each run of white space into a single
// space, trims leading and trailing white space and trailing full
// stops. If newlineSemi is true, any newlines will be replaced with a
// semicolon.
func sanitizeText(s string, newlineSemi bool) []byte {
	out := make([]byte, 0, len(s))
	prevWhite := false
	for _, r := range s {
		if newlineSemi && r == '\n' && len(out) > 0 {
			out = append(out, ';')
			prevWhite = true
			continue
		}
		if unicode.IsSpace(r) {
			if len(out) > 0 {
				prevWhite = true
			}
			continue
		}
		if prevWhite {
			out = append(out, ' ')
			prevWhite = false
		}
		out = append(out, string(r)...)
	}
	// Remove final space, any full stops and any final semicolon
	// we might have added.
	out = bytes.TrimRightFunc(out, func(r rune) bool {
		return r == '.' || r == ' ' || r == ';'
	})
	return out
}
