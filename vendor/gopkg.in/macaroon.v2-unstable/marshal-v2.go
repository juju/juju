package macaroon

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// macaroonJSONV2 defines the V2 JSON format for macaroons.
type macaroonJSONV2 struct {
	Caveats       []caveatJSONV2 `json:"c,omitempty"`
	Location      string         `json:"l,omitempty"`
	Identifier    string         `json:"i,omitempty"`
	IdentifierHex string         `json:"iH,omitempty"`
	Identifier64  string         `json:"i64,omitempty"`
	Signature     string         `json:"s,omitempty"`
	SignatureHex  string         `json:"sH,omitempty"`
	Signature64   string         `json:"s64,omitempty"`
}

// caveatJSONV2 defines the V2 JSON format for caveats within a macaroon.
type caveatJSONV2 struct {
	CID      string `json:"i,omitempty"`
	CIDHex   string `json:"iH,omitempty"`
	CID64    string `json:"i64,omitempty"`
	VID      string `json:"v,omitempty"`
	VIDHex   string `json:"vH,omitempty"`
	VID64    string `json:"v64,omitempty"`
	Location string `json:"l,omitempty"`
}

func (m *Macaroon) marshalJSONV2() ([]byte, error) {
	mjson := macaroonJSONV2{
		Location: m.location,
		Caveats:  make([]caveatJSONV2, len(m.caveats)),
	}
	putJSONBinaryField(m.id, &mjson.Identifier, &mjson.IdentifierHex, &mjson.Identifier64)
	putJSONBinaryField(m.sig[:], &mjson.Signature, &mjson.SignatureHex, &mjson.Signature64)
	for i, cav := range m.caveats {
		cavjson := caveatJSONV2{
			Location: cav.Location,
		}
		putJSONBinaryField(cav.Id, &cavjson.CID, &cavjson.CIDHex, &cavjson.CID64)
		putJSONBinaryField(cav.VerificationId, &cavjson.VID, &cavjson.VIDHex, &cavjson.VID64)
		mjson.Caveats[i] = cavjson
	}
	data, err := json.Marshal(mjson)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal json data: %v", err)
	}
	return data, nil
}

// initJSONV2 initializes m from the JSON-unmarshaled data
// held in mjson.
func (m *Macaroon) initJSONV2(mjson *macaroonJSONV2) error {
	id, err := jsonBinaryField(mjson.Identifier, mjson.IdentifierHex, mjson.Identifier64)
	if err != nil {
		return fmt.Errorf("invalid identifier: %v", err)
	}
	m.init(id, mjson.Location)
	sig, err := jsonBinaryField(mjson.Signature, mjson.SignatureHex, mjson.Signature64)
	if err != nil {
		return fmt.Errorf("invalid signature: %v", err)
	}
	if len(sig) != hashLen {
		return fmt.Errorf("signature has unexpected length %d", len(sig))
	}
	copy(m.sig[:], sig)
	m.caveats = make([]Caveat, 0, len(mjson.Caveats))
	for _, cav := range mjson.Caveats {
		cid, err := jsonBinaryField(cav.CID, cav.CIDHex, cav.CID64)
		if err != nil {
			return fmt.Errorf("invalid cid in caveat: %v", err)
		}
		vid, err := jsonBinaryField(cav.VID, cav.VIDHex, cav.VID64)
		if err != nil {
			return fmt.Errorf("invalid vid in caveat: %v", err)
		}
		m.appendCaveat(cid, vid, cav.Location)
	}
	return nil
}

// putJSONBinaryField puts the value of x into one
// of the appropriate fields depending on its value.
func putJSONBinaryField(x []byte, s, shex, sb64 *string) {
	if utf8.Valid(x) {
		*s = string(x)
		return
	}
	// TODO use hex encoding when the field is small?
	*sb64 = base64.RawURLEncoding.EncodeToString(x)
}

// jsonBinaryField returns the value of a JSON field that may
// be string, hex or base64-encoded.
func jsonBinaryField(s, shex, sb64 string) ([]byte, error) {
	switch {
	case s != "":
		if shex != "" || sb64 != "" {
			return nil, fmt.Errorf("ambiguous field encoding")
		}
		return []byte(s), nil
	case shex != "":
		if sb64 != "" {
			return nil, fmt.Errorf("ambiguous field encoding")
		}
		return hex.DecodeString(shex)
	case sb64 != "":
		return base64Decode([]byte(sb64))
	}
	return []byte{}, nil
}

// The v2 binary format of a macaroon is as follows.
// All entries other than the version are packets as
// parsed by parsePacketV2.
//
// version [1 byte]
// location?
// identifier
// eos
// (
//	location?
//	identifier
//	verificationId?
//	eos
// )*
// eos
// signature
//
// See also https://github.com/rescrv/libmacaroons/blob/master/doc/format.txt

// parseBinaryV1 parses the given data in V1 format into the macaroon. The macaroon's
// internal data structures will retain references to the data. It
// returns the data after the end of the macaroon.
func (m *Macaroon) parseBinaryV2(data []byte) ([]byte, error) {
	// The version has already been checked, so
	// skip it.
	data = data[1:]

	data, section, err := parseSectionV2(data)
	if err != nil {
		return nil, err
	}
	var loc string
	if len(section) > 0 && section[0].fieldType == fieldLocation {
		loc = string(section[0].data)
		section = section[1:]
	}
	if len(section) != 1 || section[0].fieldType != fieldIdentifier {
		return nil, fmt.Errorf("invalid macaroon header")
	}
	id := section[0].data
	m.init(id, loc)
	for {
		rest, section, err := parseSectionV2(data)
		if err != nil {
			return nil, err
		}
		data = rest
		if len(section) == 0 {
			break
		}
		var cav Caveat
		if len(section) > 0 && section[0].fieldType == fieldLocation {
			cav.Location = string(section[0].data)
			section = section[1:]
		}
		if len(section) == 0 || section[0].fieldType != fieldIdentifier {
			return nil, fmt.Errorf("no identifier in caveat")
		}
		cav.Id = section[0].data
		section = section[1:]
		if len(section) == 0 {
			// First party caveat.
			if cav.Location != "" {
				return nil, fmt.Errorf("location not allowed in first party caveat")
			}
			m.caveats = append(m.caveats, cav)
			continue
		}
		if len(section) != 1 {
			return nil, fmt.Errorf("extra fields found in caveat")
		}
		if section[0].fieldType != fieldVerificationId {
			return nil, fmt.Errorf("invalid field found in caveat")
		}
		cav.VerificationId = section[0].data
		m.caveats = append(m.caveats, cav)
	}
	data, sig, err := parsePacketV2(data)
	if err != nil {
		return nil, err
	}
	if sig.fieldType != fieldSignature {
		return nil, fmt.Errorf("unexpected field found instead of signature")
	}
	if len(sig.data) != hashLen {
		return nil, fmt.Errorf("signature has unexpected length")
	}
	copy(m.sig[:], sig.data)
	m.unmarshaledAs = MarshalV2
	return data, nil
}

// appendBinaryV2 appends the binary-encoded macaroon
// in v2 format to data.
func (m *Macaroon) appendBinaryV2(data []byte) []byte {
	// Version byte.
	data = append(data, 2)
	if len(m.location) > 0 {
		data = appendPacketV2(data, packetV2{
			fieldType: fieldLocation,
			data:      []byte(m.location),
		})
	}
	data = appendPacketV2(data, packetV2{
		fieldType: fieldIdentifier,
		data:      m.id,
	})
	data = appendEOSV2(data)
	for _, cav := range m.caveats {
		if len(cav.Location) > 0 {
			data = appendPacketV2(data, packetV2{
				fieldType: fieldLocation,
				data:      []byte(cav.Location),
			})
		}
		data = appendPacketV2(data, packetV2{
			fieldType: fieldIdentifier,
			data:      cav.Id,
		})
		if len(cav.VerificationId) > 0 {
			data = appendPacketV2(data, packetV2{
				fieldType: fieldVerificationId,
				data:      []byte(cav.VerificationId),
			})
		}
		data = appendEOSV2(data)
	}
	data = appendEOSV2(data)
	data = appendPacketV2(data, packetV2{
		fieldType: fieldSignature,
		data:      m.sig[:],
	})
	return data
}
