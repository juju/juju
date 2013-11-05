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

// In Juju 1.16 and older we used a hard-coded salt for all Users
var oldDefaultSalt = []byte{0x75, 0x82, 0x81, 0xca}

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
	b, err := RandomBytes(18)
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

// CompatPasswordHash returns the UserPasswordHash hashed with the old default
// salt. This is the password hash that was always generated for Juju 1.16 and
// older. Newer versions of Juju use UserPasswordHash with a Salt value or
// AgentPasswordHash with a required longer password for machine/unit agents.
func CompatPasswordHash(password string) string {
	return UserPasswordHash(password, string(oldDefaultSalt))
}

// AgentPasswordHash returns base64-encoded one-way hash of password. This is
// not suitable for User passwords because those will have limited entropy (see
// UserPasswordHash). However, since we generate long random passwords for
// agents, we can trust that there is sufficient entropy to prevent brute force
// search. And using a faster hash allows us to restart the state machines and
// have 1000s of agents log in in a reasonable amount of time.
// As a sanity check that AgentPasswordHash is being used correctly, we require
// the minimum length of password to be 18 characters.
func AgentPasswordHash(password string) (string, error) {
	if len(password) < 24 {
		return "", fmt.Errorf("password is only %d bytes long, and is not a valid Agent password", len(password))
	}
	sum := sha512.New()
	sum.Write([]byte(password))
	h := make([]byte, 0, sum.Size())
	h = sum.Sum(h)
	return base64.StdEncoding.EncodeToString(h[:18]), nil
}
