// The macaroon package implements macaroons as described in
// the paper "Macaroons: Cookies with Contextual Caveats for
// Decentralized Authorization in the Cloud"
// (http://theory.stanford.edu/~ataly/Papers/macaroons.pdf)
//
// See the macaroon bakery packages at http://godoc.org/gopkg.in/macaroon-bakery.v1
// for higher level services and operations that use macaroons.
package macaroon

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"fmt"
	"io"
	"unicode/utf8"
)

// Macaroon holds a macaroon.
// See Fig. 7 of http://theory.stanford.edu/~ataly/Papers/macaroons.pdf
// for a description of the data contained within.
// Macaroons are mutable objects - use Clone as appropriate
// to avoid unwanted mutation.
type Macaroon struct {
	location      string
	id            []byte
	caveats       []Caveat
	sig           [hashLen]byte
	marshalAs     MarshalOpts
	unmarshaledAs MarshalOpts
}

// Caveat holds a first person or third party caveat.
type Caveat struct {
	// Id holds the id of the caveat. For first
	// party caveats this holds the condition;
	// for third party caveats this holds the encrypted
	// third party caveat.
	Id []byte

	// VerificationId holds the verification id. If this is
	// non-empty, it's a third party caveat.
	VerificationId []byte

	// For third-party caveats, Location holds the
	// ocation hint. Note that this is not signature checked
	// as part of the caveat, so should only
	// be used as a hint.
	Location string
}

// isThirdParty reports whether the caveat must be satisfied
// by some third party (if not, it's a first person caveat).
func (cav *Caveat) isThirdParty() bool {
	return len(cav.VerificationId) > 0
}

// New returns a new macaroon with the given root key,
// identifier and location.
func New(rootKey, id []byte, loc string) (*Macaroon, error) {
	var m Macaroon
	m.init(append([]byte(nil), id...), loc)
	derivedKey := makeKey(rootKey)
	m.sig = *keyedHash(derivedKey, m.id)
	return &m, nil
}

// init initializes the macaroon. It retains a reference to id.
func (m *Macaroon) init(id []byte, loc string) {
	m.location = loc
	m.id = append([]byte(nil), id...)
	m.marshalAs = DefaultMarshalOpts
	m.unmarshaledAs = DefaultMarshalOpts
}

// Clone returns a copy of the receiving macaroon.
func (m *Macaroon) Clone() *Macaroon {
	m1 := *m
	// Ensure that if any caveats are appended to the new
	// macaroon, it will copy the caveats.
	m1.caveats = m1.caveats[0:len(m1.caveats):len(m1.caveats)]
	return &m1
}

// Location returns the macaroon's location hint. This is
// not verified as part of the macaroon.
func (m *Macaroon) Location() string {
	return m.location
}

// Id returns the id of the macaroon. This can hold
// arbitrary information.
func (m *Macaroon) Id() []byte {
	return append([]byte(nil), m.id...)
}

// Signature returns the macaroon's signature.
func (m *Macaroon) Signature() []byte {
	// sig := m.sig
	// return sig[:]
	// Work around https://github.com/golang/go/issues/9537
	sig := new([hashLen]byte)
	*sig = m.sig
	return sig[:]
}

// Caveats returns the macaroon's caveats.
// This method will probably change, and it's important not to change the returned caveat.
func (m *Macaroon) Caveats() []Caveat {
	return m.caveats[0:len(m.caveats):len(m.caveats)]
}

// appendCaveat appends a caveat without modifying the macaroon's signature.
func (m *Macaroon) appendCaveat(caveatId, verificationId []byte, loc string) {
	m.caveats = append(m.caveats, Caveat{
		Id:             caveatId,
		VerificationId: verificationId,
		Location:       loc,
	})
}

func (m *Macaroon) addCaveat(caveatId, verificationId []byte, loc string) error {
	m.appendCaveat(caveatId, verificationId, loc)
	m.sig = *keyedHash2(&m.sig, verificationId, caveatId)
	return nil
}

func keyedHash2(key *[keyLen]byte, d1, d2 []byte) *[hashLen]byte {
	if len(d1) == 0 {
		return keyedHash(key, d2)
	}
	var data [hashLen * 2]byte
	copy(data[0:], keyedHash(key, d1)[:])
	copy(data[hashLen:], keyedHash(key, d2)[:])
	return keyedHash(key, data[:])
}

// Bind prepares the macaroon for being used to discharge the
// macaroon with the given signature sig. This must be
// used before it is used in the discharges argument to Verify.
func (m *Macaroon) Bind(sig []byte) {
	m.sig = *bindForRequest(sig, &m.sig)
}

// AddFirstPartyCaveat adds a caveat that will be verified
// by the target service. The caveat id must be a UTF-8 encoded
// string.
func (m *Macaroon) AddFirstPartyCaveat(condition string) error {
	if !utf8.ValidString(condition) {
		return fmt.Errorf("first party caveat condition is not a valid utf-8 string")
	}
	m.addCaveat([]byte(condition), nil, "")
	return nil
}

// AddThirdPartyCaveat adds a third-party caveat to the macaroon,
// using the given shared root key, caveat id and location hint.
// The caveat id should encode the root key in some
// way, either by encrypting it with a key known to the third party
// or by holding a reference to it stored in the third party's
// storage.
func (m *Macaroon) AddThirdPartyCaveat(rootKey, caveatId []byte, loc string) error {
	return m.addThirdPartyCaveatWithRand(rootKey, caveatId, loc, rand.Reader)
}

// addThirdPartyCaveatWithRand adds a third-party caveat to the macaroon, using
// the given source of randomness for encrypting the caveat id.
func (m *Macaroon) addThirdPartyCaveatWithRand(rootKey, caveatId []byte, loc string, r io.Reader) error {
	derivedKey := makeKey(rootKey)
	verificationId, err := encrypt(&m.sig, derivedKey, r)
	if err != nil {
		return err
	}
	m.addCaveat(caveatId, verificationId, loc)
	return nil
}

var zeroKey [hashLen]byte

// bindForRequest binds the given macaroon
// to the given signature of its parent macaroon.
func bindForRequest(rootSig []byte, dischargeSig *[hashLen]byte) *[hashLen]byte {
	if bytes.Equal(rootSig, dischargeSig[:]) {
		return dischargeSig
	}
	return keyedHash2(&zeroKey, rootSig, dischargeSig[:])
}

// Verify verifies that the receiving macaroon is valid.
// The root key must be the same that the macaroon was originally
// minted with. The check function is called to verify each
// first-party caveat - it should return an error if the
// condition is not met.
//
// The discharge macaroons should be provided in discharges.
//
// Verify returns nil if the verification succeeds.
func (m *Macaroon) Verify(rootKey []byte, check func(caveat string) error, discharges []*Macaroon) error {
	derivedKey := makeKey(rootKey)
	// TODO(rog) consider distinguishing between classes of
	// check error - some errors may be resolved by minting
	// a new macaroon; others may not.
	used := make([]int, len(discharges))
	if err := m.verify(&m.sig, derivedKey, check, discharges, used); err != nil {
		return err
	}
	for i, dm := range discharges {
		switch used[i] {
		case 0:
			return fmt.Errorf("discharge macaroon %q was not used", dm.Id())
		case 1:
			continue
		default:
			// Should be impossible because of check in verify, but be defensive.
			return fmt.Errorf("discharge macaroon %q was used more than once", dm.Id())
		}
	}
	return nil
}

func (m *Macaroon) verify(rootSig *[hashLen]byte, rootKey *[hashLen]byte, check func(caveat string) error, discharges []*Macaroon, used []int) error {
	caveatSig := keyedHash(rootKey, m.id)
	for i, cav := range m.caveats {
		if cav.isThirdParty() {
			cavKey, err := decrypt(caveatSig, cav.VerificationId)
			if err != nil {
				return fmt.Errorf("failed to decrypt caveat %d signature: %v", i, err)
			}
			// We choose an arbitrary error from one of the
			// possible discharge macaroon verifications
			// if there's more than one discharge macaroon
			// with the required id.
			found := false
			for di, dm := range discharges {
				if !bytes.Equal(dm.id, cav.Id) {
					continue
				}
				found = true

				// It's important that we do this before calling verify,
				// as it prevents potentially infinite recursion.
				if used[di]++; used[di] > 1 {
					return fmt.Errorf("discharge macaroon %q was used more than once", dm.Id())
				}
				if err := dm.verify(rootSig, cavKey, check, discharges, used); err != nil {
					return err
				}
				break
			}
			if !found {
				return fmt.Errorf("cannot find discharge macaroon for caveat %x", cav.Id)
			}
		} else {
			if err := check(string(cav.Id)); err != nil {
				return err
			}
		}
		caveatSig = keyedHash2(caveatSig, cav.VerificationId, cav.Id)
	}
	// TODO perhaps we should actually do this check before doing
	// all the potentially expensive caveat checks.
	boundSig := bindForRequest(rootSig[:], caveatSig)
	if !hmac.Equal(boundSig[:], m.sig[:]) {
		return fmt.Errorf("signature mismatch after caveat verification")
	}
	return nil
}

type Verifier interface {
	Verify(m *Macaroon, rootKey []byte) (bool, error)
}
