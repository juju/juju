// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

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
	data, readErr := ioutil.ReadAll(decoder.Buffered())
	if readErr != nil {
		logger.Debugf("reading stream failed: %v", readErr)
	}
	jsonData := string(data)

	// if contents from io.Reader are not complete,
	// use the original raw data to prevent panic
	if int64(len(jsonData)) <= syntax.Offset {
		jsonData = string(d.rawData)
	}

	start := strings.LastIndex(jsonData[:syntax.Offset], "\n") + 1
	line := strings.Count(jsonData[:start], "\n")
	return k8syaml.JSONSyntaxError{
		Line: line,
		Err:  fmt.Errorf(syntax.Error()),
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
