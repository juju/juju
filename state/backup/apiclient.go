// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/juju/juju/state/api/params"
)

const (
	TimestampFormat  = "%02d%02d%02d-%02d%02d%02d" // YYMMDD-hhmmss
	FilenameTemplate = "jujubackup-%s.tar.gz"      // takes a timestamp
	DigestAlgorithm  = "SHA"
)

func defaultFilename(now *time.Time) string {
	if now == nil {
		_now := time.Now().UTC()
		now = &_now
	}
	Y, M, S := now.Date()
	h, m, s := now.Clock()
	formattedDate := fmt.Sprintf(TimestampFormat, Y, M, S, h, m, s)
	return fmt.Sprintf(FilenameTemplate, formattedDate)
}

// CreateEmptyFile returns a new file (and its filename).  The file is
// created fresh and is intended as the target for writing a new backup
// archive.
func CreateEmptyFile(filename string) (*os.File, string, error) {
	if filename == "" {
		filename = defaultFilename(nil)
	}
	file, err := os.Create(filename)
	if err != nil {
		return nil, "", fmt.Errorf("could not create backup file: %v", err)
	}
	return file, filename, nil
}

// NewAPIRequest returns a new HTTP request that may be used to make the
// backup API call.
func NewAPIRequest(URL *url.URL, uuid, tag, pw string) (*http.Request, error) {
	// XXX This needs to be env-based.
	URL.Path += "/backup"
	req, err := http.NewRequest("POST", URL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(tag, pw)
	return req, nil
}

func parseJSONError(resp *http.Response) (string, error) {
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("could not read HTTP response: %v", err)
	}
	// XXX Change this to params.Error
	var jsonResponse params.BackupResponse
	if err := json.Unmarshal(body, &jsonResponse); err != nil {
		return "", fmt.Errorf("could not extract error from HTTP response: %v", err)
	}
	return jsonResponse.Error, nil
}

// CheckAPIResponse checks the HTTP response for an API failure.  This
// involves both the HTTP status code and the response body.  If the
// status code indicates a failure (i.e. not StatusOK) then the response
// body will be consumed and parsed as a JSON serialization of the
// error type used by backup.
func CheckAPIResponse(resp *http.Response) *params.Error {
	var code string

	// Check the status code.
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		fallthrough
	case http.StatusMethodNotAllowed:
		code = params.CodeNotImplemented
	default:
		code = ""
	}

	// Handle the error body.
	failure, err := parseJSONError(resp)
	if err != nil {
		failure = fmt.Sprintf("(%v)", err)
	}

	return &params.Error{failure, code}
}

// WriteBackup writes an input stream into an archive file.  It returns
// the SHA-1 hash of the data written to the archive.
//
// Note that the hash is of the compressed file rather than uncompressed
// data since it is simpler.  Ultimately it doesn't matter as long as
// the API server does the same thing (which it will if the juju version
// is the same).
func WriteBackup(archive io.Writer, infile io.Reader) (string, error) {
	// Set up hashing the archive.
	hasher := sha1.New()
	target := io.MultiWriter(archive, hasher)

	// Copy into the archive.
	_, err := io.Copy(target, infile)
	if err != nil {
		return "", fmt.Errorf("could not write to the backup file: %v", err)
	}

	// Compute the hash.
	hash := fmt.Sprintf("%x", hasher.Sum(nil))

	return hash, nil
}

// ParseDigestHeader returns a map of (algorithm, digest) for all the
// digests found in the "Digest" header.  See RFC 3230.
func ParseDigestHeader(header http.Header) (map[string]string, error) {
	rawdigests := header.Get("digest")
	if rawdigests == "" {
		return nil, fmt.Errorf(`missing or blank "Digest" header`)
	}
	digests := make(map[string]string)

	// We do not handle quoted digests that have commas in them.
	for _, rawdigest := range strings.Split(rawdigests, ",") {
		parts := strings.SplitN(rawdigest, "=", 2)
		if len(parts) != 2 {
			return digests, fmt.Errorf(`bad "Digest" header: %s`, rawdigest)
		}

		// We do not take care of quoted digests.
		algorithm, digest := parts[0], parts[1]

		_, exists := digests[algorithm]
		if exists {
			return digests, fmt.Errorf("duplicate digest: %s", rawdigest)
		}
		digests[algorithm] = digest
	}

	return digests, nil
}

// ParseDigest is a light wrapper around ParseDigestHeader which returns
// just the SHA digest.
func ParseDigest(header http.Header) (string, error) {
	digests, err := ParseDigestHeader(header)
	if err != nil {
		return "", err
	}
	digest, exists := digests[DigestAlgorithm]
	if !exists {
		return "", fmt.Errorf(`"%s" missing from "Digest" header`, DigestAlgorithm)
	}
	return digest, nil
}
