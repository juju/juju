// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"unicode"
)

// TODO(ericsnow) Move the quoting helpers into the shell package?

// ShQuote quotes s so that when read by bash, no metacharacters
// within s will be interpreted as such.
func ShQuote(s string) string {
	// single-quote becomes single-quote, double-quote, single-quote, double-quote, single-quote
	return `'` + strings.Replace(s, `'`, `'"'"'`, -1) + `'`
}

// WinPSQuote quotes s so that when read by powershell, no metacharacters
// within s will be interpreted as such.
func WinPSQuote(s string) string {
	// See http://ss64.com/ps/syntax-esc.html#quotes.
	// Double quotes inside single quotes are fine, double single quotes inside
	// single quotes, not so much so. Having double quoted strings inside single
	// quoted strings, ensure no expansion happens.
	return `'` + strings.Replace(s, `'`, `"`, -1) + `'`
}

// WinCmdQuote quotes s so that when read by cmd.exe, no metacharacters
// within s will be interpreted as such.
func WinCmdQuote(s string) string {
	// See http://blogs.msdn.com/b/twistylittlepassagesallalike/archive/2011/04/23/everyone-quotes-arguments-the-wrong-way.aspx.
	quoted := winCmdQuote(s)
	return winCmdEscapeMeta(quoted)
}

func winCmdQuote(s string) string {
	var escaped string
	for _, c := range s {
		switch c {
		case '\\', '"':
			escaped += `\`
		}
		escaped += string(c)
	}
	return `"` + escaped + `"`
}

func winCmdEscapeMeta(str string) string {
	const meta = `()%!^"<>&|`
	var newStr string
	for _, c := range str {
		if strings.Contains(meta, string(c)) {
			newStr += "^"
		}
		newStr += string(c)
	}
	return newStr
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
