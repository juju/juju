// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"

	"launchpad.net/juju-core/thirdparty/pbkdf2"
)

// CompatSalt is because Juju 1.16 and older used a hard-coded salt to compute
// the password hash for all users and agents
var CompatSalt = string([]byte{0x75, 0x82, 0x81, 0xca})

const randomPasswordBytes = 18

// MinAgentPasswordLength describes how long agent passwords should be. We
// require this length because we assume enough entropy in the Agent password
// that it is safe to not do extra rounds of iterated hashing.
var MinAgentPasswordLength = base64.StdEncoding.EncodedLen(randomPasswordBytes)

// RandomBytes returns n random bytes.
func RandomBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		return nil, fmt.Errorf("cannot read random bytes: %v", err)
	}
	return buf, nil
}

// RandomPassword generates a random base64-encoded password.
func RandomPassword() (string, error) {
	b, err := RandomBytes(randomPasswordBytes)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// RandomSalt generates a random base64 data suitable for using as a password
// salt The pbkdf2 guideline is to use 8 bytes of salt, so we do 12 raw bytes
// into 16 base64 bytes. (The alternative is 6 raw into 8 base64).
func RandomSalt() (string, error) {
	b, err := RandomBytes(12)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// FastInsecureHash specifies whether a fast, insecure version of the hash
// algorithm will be used.  Changing this will cause PasswordHash to
// produce incompatible passwords.  It should only be changed for
// testing purposes - to make tests run faster.
var FastInsecureHash = false

// UserPasswordHash returns base64-encoded one-way hash password that is
// computationally hard to crack by iterating through possible passwords.
func UserPasswordHash(password string, salt string) string {
	if salt == "" {
		panic("salt is not allowed to be empty")
	}
	iter := 8192
	if FastInsecureHash {
		iter = 1
	}
	// Generate 18 byte passwords because we know that MongoDB
	// uses the MD5 sum of the password anyway, so there's
	// no point in using more bytes. (18 so we don't get base 64
	// padding characters).
	h := pbkdf2.Key([]byte(password), []byte(salt), iter, 18, sha512.New)
	return base64.StdEncoding.EncodeToString(h)
}

// AgentPasswordHash returns base64-encoded one-way hash of password. This is
// not suitable for User passwords because those will have limited entropy (see
// UserPasswordHash). However, since we generate long random passwords for
// agents, we can trust that there is sufficient entropy to prevent brute force
// search. And using a faster hash allows us to restart the state machines and
// have 1000s of agents log in in a reasonable amount of time.
func AgentPasswordHash(password string) string {
	sum := sha512.New()
	sum.Write([]byte(password))
	h := sum.Sum(nil)
	return base64.StdEncoding.EncodeToString(h[:18])
}
