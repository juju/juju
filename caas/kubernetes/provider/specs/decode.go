// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/juju/errors"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

// YAMLOrJSONDecoder attempts to decode a stream of JSON documents or
// YAML documents by sniffing for a leading { character.
type YAMLOrJSONDecoder struct {
	bufferSize int
	r          io.Reader
	rawData    []byte

	strict bool
}

func newStrictYAMLOrJSONDecoder(r io.Reader, bufferSize int) *YAMLOrJSONDecoder {
	return newYAMLOrJSONDecoder(r, bufferSize, true)
}

func newYAMLOrJSONDecoder(r io.Reader, bufferSize int, strict bool) *YAMLOrJSONDecoder {
	return &YAMLOrJSONDecoder{
		r:          r,
		bufferSize: bufferSize,
		strict:     strict,
	}
}

func (d *YAMLOrJSONDecoder) jsonify() (err error) {
	buffer := bufio.NewReaderSize(d.r, d.bufferSize)
	rawData, _ := buffer.Peek(d.bufferSize)
	if rawData, err = k8syaml.ToJSON(rawData); err != nil {
		return errors.Trace(err)
	}
	d.r = bytes.NewReader(rawData)
	d.rawData = rawData
	return nil
}

func (d *YAMLOrJSONDecoder) processError(err error, decoder *json.Decoder) error {
	syntax, ok := err.(*json.SyntaxError)
	if !ok {
		return err
	}
	return k8syaml.JSONSyntaxError{
		Offset: syntax.Offset,
		Err:    fmt.Errorf("%s", syntax.Error()),
	}
}

// Decode unmarshals the next object from the underlying stream into the
// provide object, or returns an error.
func (d *YAMLOrJSONDecoder) Decode(into interface{}) error {
	if err := d.jsonify(); err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("decoding stream as JSON")
	decoder := json.NewDecoder(d.r)
	decoder.UseNumber()
	if d.strict {
		decoder.DisallowUnknownFields()
	}

	for decoder.More() {
		if err := decoder.Decode(into); err != nil {
			return d.processError(err, decoder)
		}
	}

	return nil
}
