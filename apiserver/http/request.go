// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strings"

	"github.com/juju/errors"
)

// NewRequest returns a new HTTP request suitable for the API.
func NewRequest(method string, baseURL *url.URL, pth, uuid, tag, pw string) (*http.Request, error) {
	baseURL.Path = path.Join("/environment", uuid, pth)

	req, err := http.NewRequest(method, baseURL.String(), nil)
	if err != nil {
		return nil, errors.Annotate(err, "while building HTTP request")
	}

	req.SetBasicAuth(tag, pw)
	return req, nil
}

// SetRequestArgs JSON-encodes the args and sets them as the request body.
func SetRequestArgs(req *http.Request, args interface{}) error {
	data, err := json.Marshal(args)
	if err != nil {
		return errors.Annotate(err, "while serializing args")
	}

	req.Header.Set("Content-Type", CTypeJSON)
	req.Body = ioutil.NopCloser(bytes.NewBuffer(data))
	return nil
}

// AttachToRequest attaches a reader's data to the request body as
// multi-part data, along with associated metadata. "name" is used to
// identify the attached "file", so a filename is an appropriate value.
func AttachToRequest(req *http.Request, attached io.Reader, meta interface{}, name string) error {
	var parts bytes.Buffer

	// Set up the multi-part portion of the body.
	writer := multipart.NewWriter(&parts)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Initialize the request body.
	req.Body = ioutil.NopCloser(io.MultiReader(
		&parts,
		attached,
		bytes.NewBufferString("\r\n--"+writer.Boundary()+"--\r\n"),
	))

	// Set the metadata part.
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="metadata"`)
	header.Set("Content-Type", CTypeJSON)
	part, err := writer.CreatePart(header)
	if err != nil {
		return errors.Trace(err)
	}
	if err := json.NewEncoder(part).Encode(meta); err != nil {
		return errors.Trace(err)
	}

	// Set the attached part.
	_, err = writer.CreateFormFile("attached", name)
	if err != nil {
		return errors.Trace(err)
	}
	// We don't actually write the reader's data to the part. Instead We
	// use a chained reader to facilitate streaming directly from the
	// reader.
	return nil
}

// ExtractRequestAttachment extracts the attached file and its metadata
// from the multipart data in the request.
func ExtractRequestAttachment(req *http.Request, metaResult interface{}) (io.ReadCloser, error) {
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

	if err := checkContentType(part.Header, CTypeJSON); err != nil {
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
	if err := checkContentType(part.Header, CTypeRaw); err != nil {
		return nil, errors.Trace(err)
	}
	// We're not going to worry about verifying that the file matches the
	// metadata (e.g. size, checksum).
	archive := part

	// We are going to trust that there aren't any more attachments after
	// the file. If there are, we ignore them.

	return archive, nil
}

type getter interface {
	Get(string) string
}

func checkContentType(header getter, expected string) error {
	ctype := header.Get("Content-Type")
	if ctype != expected {
		return errors.Errorf("expected Content-Type %q, got %q", expected, ctype)
	}
	return nil
}
