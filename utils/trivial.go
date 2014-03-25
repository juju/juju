// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"unicode"

	"launchpad.net/goyaml"
)

// WriteYaml marshals obj as yaml and then writes it to a file, atomically,
// by first writing a sibling with the suffix ".preparing" and then moving
// the sibling to the real path.
func WriteYaml(path string, obj interface{}) error {
	data, err := goyaml.Marshal(obj)
	if err != nil {
		return err
	}
	prep := path + ".preparing"
	f, err := os.OpenFile(prep, os.O_WRONLY|os.O_CREATE|os.O_SYNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	return ReplaceFile(prep, path)
}

// ReadYaml unmarshals the yaml contained in the file at path into obj. See
// goyaml.Unmarshal.
func ReadYaml(path string, obj interface{}) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return goyaml.Unmarshal(data, obj)
}

// ErrorContextf prefixes any error stored in err with text formatted
// according to the format specifier. If err does not contain an error,
// ErrorContextf does nothing.
func ErrorContextf(err *error, format string, args ...interface{}) {
	if *err != nil {
		*err = errors.New(fmt.Sprintf(format, args...) + ": " + (*err).Error())
	}
}

// ShQuote quotes s so that when read by bash, no metacharacters
// within s will be interpreted as such.
func ShQuote(s string) string {
	// single-quote becomes single-quote, double-quote, single-quote, double-quote, single-quote
	return `'` + strings.Replace(s, `'`, `'"'"'`, -1) + `'`
}

// CommandString flattens a sequence of command arguments into a
// string suitable for executing in a shell, escaping slashes,
// variables and quotes as necessary; each argument is double-quoted
// if and only if necessary.
func CommandString(args ...string) string {
	var buf bytes.Buffer
	for i, arg := range args {
		needsQuotes := false
		var argBuf bytes.Buffer
		for _, r := range arg {
			if unicode.IsSpace(r) {
				needsQuotes = true
			} else if r == '"' || r == '$' || r == '\\' {
				needsQuotes = true
				argBuf.WriteByte('\\')
			}
			argBuf.WriteRune(r)
		}
		if i > 0 {
			buf.WriteByte(' ')
		}
		if needsQuotes {
			buf.WriteByte('"')
			argBuf.WriteTo(&buf)
			buf.WriteByte('"')
		} else {
			argBuf.WriteTo(&buf)
		}
	}
	return buf.String()
}

// Gzip compresses the given data.
func Gzip(data []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		// Compression should never fail unless it fails
		// to write to the underlying writer, which is a bytes.Buffer
		// that never fails.
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// Gunzip uncompresses the given data.
func Gunzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(r)
}

// ReadSHA256 returns the SHA256 hash of the contents read from source
// (hex encoded) and the size of the source in bytes.
func ReadSHA256(source io.Reader) (string, int64, error) {
	hash := sha256.New()
	size, err := io.Copy(hash, source)
	if err != nil {
		return "", 0, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	return digest, size, nil
}

// ReadFileSHA256 is like ReadSHA256 but reads the contents of the
// given file.
func ReadFileSHA256(filename string) (string, int64, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	return ReadSHA256(f)
}
