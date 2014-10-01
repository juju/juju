// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/textproto"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Info implements the API method.
func (c *Client) Upload(archive io.ReadCloser, meta params.BackupsMetadataResult) (*params.BackupsMetadataResult, error) {
	defer archive.Close()

	// Initialize the HTTP request.
	req, err := c.http.NewHTTPRequest("PUT", "backups")
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Initialize the request body.
	var parts bytes.Buffer
	req.Body = ioutil.NopCloser(&chainedReader{[]io.Reader{&parts, archive}})

	// Set up the multi-part portion of the body.
	writer := multipart.NewWriter(&parts)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Set the metadata part.
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="metadata"`)
	header.Set("Content-Type", "application/json")
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := json.NewEncoder(part).Encode(&meta); err != nil {
		return nil, errors.Trace(err)
	}

	// Set the archive part.
	part, err = writer.CreateFormFile("archive", "juju-backup.tar.gz")
	if err != nil {
		return nil, errors.Trace(err)
	}
	// We don't actually write the file to the part.  We use a chained
	// reader instead to facilitate streaming directly from the archive.

	// Send the request.
	resp, err := c.http.SendHTTPRequest(req)
	if err != nil {
		return nil, errors.Annotate(err, "while sending HTTP request")
	}
	defer resp.Body.Close()

	// Handle the response.
	if err := base.CheckHTTPResponse(resp); err != nil {
		return nil, errors.Trace(err)
	}
	ctype := resp.Header.Get("Content-Type")
	if ctype != "application/json" {
		return nil, errors.Errorf(`expected conten type "application/json", got %s`, ctype)
	}
	var result params.BackupsMetadataResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return &result, errors.Trace(err)
	}

	return &result, nil
}

type chainedReader struct {
	readers []io.Reader
}

func (r *chainedReader) Read(p []byte) (int, error) {
	count := 0
	for _, reader := range r.readers {
		n, err := reader.Read(p)
		count += n
		if err != nil {
			return count, err
		}
	}
	return count, nil
}
