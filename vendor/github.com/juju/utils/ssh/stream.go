// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package ssh

import (
	"bytes"
	"io"
)

// stripCR implements an io.Reader wrapper that removes carriage return bytes.
type stripCR struct {
	reader io.Reader
}

// StripCRReader returns a new io.Reader wrapper that strips carriage returns.
func StripCRReader(reader io.Reader) io.Reader {
	if reader == nil {
		return nil
	}
	return &stripCR{reader: reader}
}

var byteEmpty = []byte{}
var byteCR = []byte{'\r'}

// Read implements io.Reader interface.
// This copies data around much more than needed so should be optimized if
// used on a performance critical path.
func (s *stripCR) Read(bufOut []byte) (int, error) {
	bufTemp := make([]byte, len(bufOut))
	n, err := s.reader.Read(bufTemp)
	bufReplaced := bytes.Replace(bufTemp[:n], byteCR, byteEmpty, -1)
	copy(bufOut, bufReplaced)
	return len(bufReplaced), err
}
