package bakery

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"

	"golang.org/x/crypto/nacl/box"

	"gopkg.in/errgo.v1"
)

type caveatInfo struct {
	peerPublicKey *PublicKey
	rootKey       []byte
	condition     string
}

type caveatIdRecord struct {
	RootKey   []byte
	Condition string
}

// caveatId defines the format of a third party caveat id.
type caveatId struct {
	ThirdPartyPublicKey *PublicKey
	FirstPartyPublicKey *PublicKey
	Nonce               []byte
	Id                  string
}

// encodeJSONCaveatId creates a JSON encoded third-party caveat.
func encodeJSONCaveatId(key *KeyPair, ci caveatInfo) ([]byte, error) {
	var nonce [NonceLen]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, errgo.Notef(err, "cannot generate random number for nonce")
	}
	plain := caveatIdRecord{
		RootKey:   ci.rootKey,
		Condition: ci.condition,
	}
	plainData, err := json.Marshal(&plain)
	if err != nil {
		return nil, errgo.Notef(err, "cannot marshal %#v", &plain)
	}
	sealed := box.Seal(nil, plainData, &nonce, ci.peerPublicKey.boxKey(), key.Private.boxKey())
	id := caveatId{
		ThirdPartyPublicKey: ci.peerPublicKey,
		FirstPartyPublicKey: &key.Public,
		Nonce:               nonce[:],
		Id:                  base64.StdEncoding.EncodeToString(sealed),
	}
	data, err := json.Marshal(id)
	if err != nil {
		return nil, errgo.Notef(err, "cannot marshal %#v", id)
	}
	buf := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
	base64.StdEncoding.Encode(buf, data)
	return buf, nil
}

const (
	publicKeyPrefixLen = 4
)

// encodeCaveatIdV0 creates a version 0 third-party caveat.
//
// The v0 format has the following packed binary fields:
// version 0 [1 byte]
// first 4 bytes of third-party Curve25519 public key [4 bytes]
// first-party Curve25519 public key [32 bytes]
// nonce [24 bytes]
// encrypted secret part [rest of message]
func encodeCaveatIdV0(key *KeyPair, ci caveatInfo) ([]byte, error) {
	var nonce [NonceLen]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, errgo.Notef(err, "cannot generate random number for nonce")
	}
	data := make([]byte, 0, 1+publicKeyPrefixLen+KeyLen+NonceLen+1+binary.MaxVarintLen64+len(ci.rootKey)+len(ci.condition)+box.Overhead)
	data = append(data, 0) //version
	data = append(data, ci.peerPublicKey.Key[:publicKeyPrefixLen]...)
	data = append(data, key.Public.Key[:]...)
	data = append(data, nonce[:]...)
	data = box.Seal(data, encodeSecretPartV0(ci), &nonce, ci.peerPublicKey.boxKey(), key.Private.boxKey())
	return data, nil
}

// encodeSecretPartV0 creates a version 0 secret part of the third party
// caveat. The generated secret part is not encrypted.
//
// The v0 format has the following packed binary fields:
// version 0 [1 byte]
// root key [24 bytes]
// predicate [rest of message]
func encodeSecretPartV0(ci caveatInfo) []byte {
	data := make([]byte, 0, 1+binary.MaxVarintLen64+len(ci.rootKey)+len(ci.condition))
	data = append(data, 0) // version
	n := binary.PutUvarint(data[1:1+binary.MaxVarintLen64], uint64(len(ci.rootKey)))
	data = data[0 : len(data)+n]
	data = append(data, ci.rootKey...)
	data = append(data, ci.condition...)
	return data
}

// decodeCaveatId attempts to decode id decrypting the encrypted part
// using key.
func decodeCaveatId(key *KeyPair, id []byte) (caveatInfo, error) {
	if len(id) == 0 {
		return caveatInfo{}, errgo.New("caveat id empty")
	}
	switch id[0] {
	case 0:
		return decodeCaveatIdV0(key, []byte(id))
	case 'e':
		// 'e' will be the first byte if the caveatid is a base64 encoded JSON object.
		return decodeJSONCaveatId(key, id)
	default:
		return caveatInfo{}, errgo.Newf("caveat id has unsupported version %d", id[0])
	}
}

// decodeJSONCaveatId attempts to decode a base64 encoded JSON id. This
// encoding is nominally version -1.
func decodeJSONCaveatId(key *KeyPair, id []byte) (caveatInfo, error) {
	data := make([]byte, (3*len(id)+3)/4)
	n, err := base64.StdEncoding.Decode(data, id)
	if err != nil {
		return caveatInfo{}, errgo.Notef(err, "cannot base64-decode caveat id")
	}
	data = data[:n]
	var tpid caveatId
	if err := json.Unmarshal(data, &tpid); err != nil {
		return caveatInfo{}, errgo.Notef(err, "cannot unmarshal caveat id %q", data)
	}
	if !bytes.Equal(key.Public.Key[:], tpid.ThirdPartyPublicKey.Key[:]) {
		return caveatInfo{}, errgo.New("public key mismatch")
	}
	if tpid.FirstPartyPublicKey == nil {
		return caveatInfo{}, errgo.New("target service public key not specified")
	}
	// The encrypted string is base64 encoded in the JSON representation.
	secret, err := base64.StdEncoding.DecodeString(tpid.Id)
	if err != nil {
		return caveatInfo{}, errgo.Notef(err, "cannot base64-decode encrypted data")
	}
	var nonce [NonceLen]byte
	if copy(nonce[:], tpid.Nonce) < NonceLen {
		return caveatInfo{}, errgo.Newf("nonce too short %x", tpid.Nonce)
	}
	cid, ok := box.Open(nil, secret, &nonce, tpid.FirstPartyPublicKey.boxKey(), key.Private.boxKey())
	if !ok {
		return caveatInfo{}, errgo.Newf("cannot decrypt caveat id %#v", tpid)
	}
	var record caveatIdRecord
	if err := json.Unmarshal(cid, &record); err != nil {
		return caveatInfo{}, errgo.Notef(err, "cannot decode third party caveat record")
	}
	return caveatInfo{
		peerPublicKey: tpid.FirstPartyPublicKey,
		rootKey:       record.RootKey,
		condition:     record.Condition,
	}, nil
}

// decodeCaveatIdV0 decodes a version 0 caveat id.
func decodeCaveatIdV0(key *KeyPair, id []byte) (caveatInfo, error) {
	if len(id) < 1+publicKeyPrefixLen+KeyLen+NonceLen+box.Overhead {
		return caveatInfo{}, errgo.New("caveat id too short")
	}
	id = id[1:] // skip version (already checked)

	publicKeyPrefix, id := id[:publicKeyPrefixLen], id[publicKeyPrefixLen:]
	if !bytes.Equal(key.Public.Key[:publicKeyPrefixLen], publicKeyPrefix) {
		return caveatInfo{}, errgo.New("public key mismatch")
	}

	var peerPublicKey PublicKey
	copy(peerPublicKey.Key[:], id[:KeyLen])
	id = id[KeyLen:]

	var nonce [NonceLen]byte
	copy(nonce[:], id[:NonceLen])
	id = id[NonceLen:]

	data, ok := box.Open(nil, id, &nonce, peerPublicKey.boxKey(), key.Private.boxKey())
	if !ok {
		return caveatInfo{}, errgo.Newf("cannot decrypt caveat id")
	}
	ci, err := decodeSecretPartV0(data)
	if err != nil {
		return caveatInfo{}, errgo.Notef(err, "invalid secret part")
	}
	ci.peerPublicKey = &peerPublicKey
	return ci, nil
}

func decodeSecretPartV0(data []byte) (caveatInfo, error) {
	if len(data) < 1 {
		return caveatInfo{}, errgo.New("secret part too short")
	}

	version, data := data[0], data[1:]
	if version != 0 {
		return caveatInfo{}, errgo.Newf("unsupported secret part version %d", version)
	}

	l, n := binary.Uvarint(data)
	if n <= 0 || uint64(n)+l > uint64(len(data)) {
		return caveatInfo{}, errgo.Newf("invalid root key length")
	}
	data = data[n:]

	return caveatInfo{
		rootKey:   data[:l],
		condition: string(data[l:]),
	}, nil
}
