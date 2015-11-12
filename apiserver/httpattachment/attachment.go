// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package httpattachment provides facilities for attaching a streaming
// blob of data and associated metadata to an HTTP API request,
// and for reading that blob on the server side.
package httpattachment

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
)

// NewBody returns an HTTP request body and content type
// suitable for using to make an HTTP request containing
// the given attached body data and JSON-marshaled metadata.
//
// The name parameter is used to identify the attached "file", so
// a filename is an appropriate value.
func NewBody(attached io.ReadSeeker, meta interface{}, name string) (body io.ReadSeeker, contentType string, err error) {
	var parts bytes.Buffer

	// Set up the multi-part portion of the body.
	writer := multipart.NewWriter(&parts)

	// Set the metadata part.
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="metadata"`)
	header.Set("Content-Type", params.ContentTypeJSON)
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	if err := json.NewEncoder(part).Encode(meta); err != nil {
		return nil, "", errors.Trace(err)
	}

	// Set the attached part.
	_, err = writer.CreateFormFile("attached", name)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	// We don't actually write the reader's data to the part.
	// Instead We use a chained reader to facilitate streaming
	// directly from the reader.
	//
	// Technically this is boundary-breaking, as the knowledge of
	// how to make multipart archives should be kept to the
	// mime/multipart package, but doing it this way means we don't
	// need to return a Writer which would be harder to turn into
	// a ReadSeeker.
	return newMultiReaderSeeker(
		bytes.NewReader(parts.Bytes()),
		attached,
		strings.NewReader("\r\n--"+writer.Boundary()+"--\r\n"),
	), writer.FormDataContentType(), nil
}

type multiReaderSeeker struct {
	readers []io.ReadSeeker
	index   int
}

// mewMultiReaderSeeker returns an io.ReadSeeker implementation that
// reads from all the given readers in turn. Its Seek method can be used
// to seek to the start, but returns an error if used to seek anywhere
// else (this corresponds with the needs of httpbakery.Client.DoWithBody
// which needs to re-read the body when retrying the request).
func newMultiReaderSeeker(readers ...io.ReadSeeker) *multiReaderSeeker {
	return &multiReaderSeeker{
		readers: readers,
	}
}

// Read implements io.Reader.Read.
func (r *multiReaderSeeker) Read(buf []byte) (int, error) {
	if r.index >= len(r.readers) {
		return 0, io.EOF
	}
	n, err := r.readers[r.index].Read(buf)
	if err == io.EOF {
		r.index++
		err = nil
	}
	return n, err
}

// Read implements io.Seeker.Seek. It can only be used to seek to the
// start.
func (r *multiReaderSeeker) Seek(offset int64, whence int) (int64, error) {
	if offset != 0 || whence != 0 {
		return 0, errors.New("cannot only seek to the start of multipart reader")
	}
	for _, reader := range r.readers {
		if _, err := reader.Seek(0, 0); err != nil {
			return 0, errors.Trace(err)
		}
	}
	r.index = 0
	return 0, nil
}

// Get extracts the attached file and its metadata from the multipart
// data in the request. The metadata is JSON-unmarshaled into the value
// pointed to by metaResult.
func Get(req *http.Request, metaResult interface{}) (io.ReadCloser, error) {
	ctype := req.Header.Get("Content-Type")
	mediaType, cParams, err := mime.ParseMediaType(ctype)
	if err != nil {
		return nil, errors.Annotate(err, "while parsing content type header")
	}

	if !strings.HasPrefix(mediaType, "multipart/") {
		return nil, errors.Errorf("expected multipart Content-Type, got %q", mediaType)
	}
	reader := multipart.NewReader(req.Body, cParams["boundary"])

	// Extract the metadata.
	part, err := reader.NextPart()
	if err != nil {
		if err == io.EOF {
			return nil, errors.New("missing metadata")
		}
		return nil, errors.Trace(err)
	}

	if err := checkContentType(part.Header, params.ContentTypeJSON); err != nil {
		return nil, errors.Trace(err)
	}
	if err := json.NewDecoder(part).Decode(metaResult); err != nil {
		return nil, errors.Trace(err)
	}

	// Extract the archive.
	part, err = reader.NextPart()
	if err != nil {
		if err == io.EOF {
			return nil, errors.New("missing archive")
		}
		return nil, errors.Trace(err)
	}
	if err := checkContentType(part.Header, params.ContentTypeRaw); err != nil {
		return nil, errors.Trace(err)
	}
	// We're not going to worry about verifying that the file matches the
	// metadata (e.g. size, checksum).
	archive := part

	// We are going to trust that there aren't any more attachments after
	// the file. If there are, we ignore them.

	return archive, nil
}

func checkContentType(h textproto.MIMEHeader, expected string) error {
	ctype := h.Get("Content-Type")
	if ctype != expected {
		return errors.Errorf("expected Content-Type %q, got %q", expected, ctype)
	}
	return nil
}
