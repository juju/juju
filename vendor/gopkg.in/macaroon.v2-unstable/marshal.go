package macaroon

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
)

// MarshalOpts specifies how a macaroon is marshaled.
type MarshalOpts uint16

const (
	_ = MarshalOpts(iota)

	// MarshalV1 specifies marshaling in V1 format.
	MarshalV1

	// MarshalV2 specifies marshaling in V2 format.
	MarshalV2

	// MarshalJSONObject specifies that when marshaling JSON,
	// the macaroon should be marshaled as a JSON object
	// rather than a base64-encoded string.
	MarshalJSONObject = 1 << 5

	// MarshalJSON specifies that the object was unmarshaled
	// as JSON. This is ignored when marshaling, as the method
	// called to marshal the macaroon will determine whether
	// the macaroon is marshaled as JSON or not.
	MarshalJSON = 1 << 6

	// MarshalVersion is the mask for the version bits.
	MarshalVersion = 0xf

	// DefaultMarshalOpts holds the formatting options
	// that will be used by default.
	DefaultMarshalOpts = MarshalV1 | MarshalJSONObject
)

// String returns a string representation of the marshal opts;
// for example MarshalV1|MarshalJSON formats as
// "v1,json".
func (o MarshalOpts) String() string {
	buf := make([]byte, 0, 10)
	buf = append(buf, 'v')
	buf = strconv.AppendInt(buf, int64(o&MarshalVersion), 10)
	o &^= MarshalVersion
	if o&MarshalJSON != 0 {
		buf = append(buf, ",json"...)
	}
	if o&MarshalJSONObject != 0 {
		buf = append(buf, ",object"...)
	}
	o &^= MarshalJSON | MarshalJSONObject
	if o != 0 {
		buf = append(buf, fmt.Sprintf(",other%x", int(o))...)
	}
	return string(buf)
}

// MarshalAs specifies that the macaroon should be marshaled according
// to the specified options.
// By default, macaroons are formatted using DefaultMarshalOpts.
func (m *Macaroon) MarshalAs(opts MarshalOpts) {
	m.marshalAs = opts
}

// UnmarshaledAs returned information on the format
// that the macaroon was unmarshaled as.
// If the macaroon was created with New, it returns DefaultMarshalOpts.
func (m *Macaroon) UnmarshaledAs() MarshalOpts {
	return m.unmarshaledAs
}

// MarshalJSON implements json.Marshaler by marshaling the
// macaroon in JSON format, using the version specified in M
func (m *Macaroon) MarshalJSON() ([]byte, error) {
	switch m.marshalAs &^ MarshalJSON {
	case MarshalV1 | MarshalJSONObject:
		return m.marshalJSONV1()
	case MarshalV1:
		data, err := m.appendBinaryV1(nil)
		if err != nil {
			return nil, err
		}
		return json.Marshal(data)
	case MarshalV2 | MarshalJSONObject:
		return m.marshalJSONV2()
	case MarshalV2:
		return json.Marshal(m.appendBinaryV2(nil))
	default:
		return nil, fmt.Errorf("bad marshal options %v", m.marshalAs)
	}
}

// UnmarshalJSON implements json.Unmarshaller by unmarshaling
// the given macaroon in JSON format. It accepts both V1 and V2
// forms encoded forms, and also a base64-encoded JSON string
// containing the binary-marshaled macaroon.
func (m *Macaroon) UnmarshalJSON(data []byte) error {
	if data[0] == '"' {
		// It's a string, so it must be a base64-encoded binary form.
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		data, err := base64Decode([]byte(s))
		if err != nil {
			return err
		}
		if err := m.UnmarshalBinary(data); err != nil {
			return err
		}
		m.unmarshaledAs |= MarshalJSON
		return nil
	}
	// Not a string; try to unmarshal into both kinds of macaroon object.
	// This assumes that neither format has any fields in common.
	// For subsequent versions we may need to change this approach.
	type MacaroonJSONV1 macaroonJSONV1
	type MacaroonJSONV2 macaroonJSONV2
	var both struct {
		*MacaroonJSONV1
		*MacaroonJSONV2
	}
	if err := json.Unmarshal(data, &both); err != nil {
		return err
	}
	switch {
	case both.MacaroonJSONV1 != nil && both.MacaroonJSONV2 != nil:
		return fmt.Errorf("cannot determine macaroon encoding version")
	case both.MacaroonJSONV1 != nil:
		if err := m.initJSONV1((*macaroonJSONV1)(both.MacaroonJSONV1)); err != nil {
			return err
		}
		m.unmarshaledAs = MarshalV1 | MarshalJSON | MarshalJSONObject
		return nil
	case both.MacaroonJSONV2 != nil:
		if err := m.initJSONV2((*macaroonJSONV2)(both.MacaroonJSONV2)); err != nil {
			return err
		}
		m.unmarshaledAs = MarshalV2 | MarshalJSON | MarshalJSONObject
		return nil
	default:
		return fmt.Errorf("invalid JSON macaroon encoding")
	}
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
// It accepts both V1 and V2 binary encodings.
func (m *Macaroon) UnmarshalBinary(data []byte) error {
	// Copy the data to avoid retaining references to it
	// in the internal data structures.
	data = append([]byte(nil), data...)
	_, err := m.parseBinary(data)
	return err
}

// parseBinary parses the macaroon in binary format
// from the given data and returns where the parsed data ends.
//
// It retains references to data.
func (m *Macaroon) parseBinary(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty macaroon data")
	}
	v := data[0]
	if v == 2 {
		// Version 2 binary format.
		data, err := m.parseBinaryV2(data)
		if err != nil {
			return nil, fmt.Errorf("unmarshal v2: %v", err)
		}
		m.unmarshaledAs = MarshalV2
		return data, nil
	}
	if isASCIIHex(v) {
		// It's a hex digit - version 1 binary format
		data, err := m.parseBinaryV1(data)
		if err != nil {
			return nil, fmt.Errorf("unmarshal v1: %v", err)
		}
		m.unmarshaledAs = MarshalV1
		return data, nil
	}
	return nil, fmt.Errorf("cannot determine data format of binary-encoded macaroon")
}

// MarshalBinary implements encoding.BinaryMarshaler by
// formatting the macaroon according to the version specified
// by MarshalAs.
func (m *Macaroon) MarshalBinary() ([]byte, error) {
	return m.appendBinary(nil)
}

// appendBinary appends the binary-formatted macaroon to
// the given data, formatting it according to the version
// specified by MarshalAs.
func (m *Macaroon) appendBinary(data []byte) ([]byte, error) {
	switch m.marshalAs & MarshalVersion {
	case MarshalV1:
		return m.appendBinaryV1(data)
	case MarshalV2:
		return m.appendBinaryV2(data), nil
	default:
		return nil, fmt.Errorf("bad marshal options %v", m.marshalAs)
	}
}

// Slice defines a collection of macaroons. By convention, the
// first macaroon in the slice is a primary macaroon and the rest
// are discharges for its third party caveats.
type Slice []*Macaroon

// MarshalBinary implements encoding.BinaryMarshaler.
func (s Slice) MarshalBinary() ([]byte, error) {
	var data []byte
	var err error
	for _, m := range s {
		data, err = m.appendBinary(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal macaroon %q: %v", m.Id(), err)
		}
	}
	return data, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
// It accepts all known binary encodings for the data - all the
// embedded macaroons need not be encoded in the same format.
func (s *Slice) UnmarshalBinary(data []byte) error {
	// Prevent the internal data structures from holding onto the
	// slice by copying it first.
	data = append([]byte(nil), data...)
	*s = (*s)[:0]
	for len(data) > 0 {
		var m Macaroon
		rest, err := m.parseBinary(data)
		if err != nil {
			return fmt.Errorf("cannot unmarshal macaroon: %v", err)
		}
		*s = append(*s, &m)
		data = rest
	}
	return nil
}

// base64Decode base64-decodes the given data.
// It accepts both standard padded encoding and unpadded
// URL encoding.
func base64Decode(data []byte) ([]byte, error) {
	// Make a buffer that's big enough for both padded and
	// unpadded cases.
	buf := make([]byte, base64.RawStdEncoding.DecodedLen(len(data)))
	if n, err := base64.StdEncoding.Decode(buf, data); err == nil {
		return buf[0:n], nil
	}
	n, err := base64.RawURLEncoding.Decode(buf, data)
	if err == nil {
		return buf[0:n], nil
	}
	return nil, err
}
