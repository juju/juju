// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auth

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/juju/errors"
	"golang.org/x/crypto/pbkdf2"
)

// Password hides and protects a plain text passwords in Juju from accidentally
// being consumed or printed to a log.
type Password struct {
	// password is the plain text password being protected
	password []byte
}

const (
	// ErrPasswordDestroyed is used when a password has been destroyed and the
	// operation cannot be performed.
	ErrPasswordDestroyed = errors.ConstError("password destroyed")

	// ErrPasswordNotValid is used when a password has failed validation.
	ErrPasswordNotValid = errors.ConstError("password not valid")

	// saltLength is the number of bytes that we randomly generate for new
	// salts. The pbkdf2 guidelines is to use 8 bytes for salt.
	saltLength = 12

	// pbkdf2Iterations is the number iterations to perform for pbkdf2. The
	// more iterations performed the harder brute forcing is. Each iteration
	// repeatedly hashes the password adding computational complexity.
	//
	// Warning updating this value will break already established passwords on
	// the old value!
	pbkdf2Iterations = 8192

	// pbkdf2KeySize is the size of the produced key by pbkdf2KeySize, expressed
	// in number of bytes.
	//
	// Warning updating this value will break already established passwords on
	// the old value!
	pbkdf2KeySize = 18

	// maxPasswordSizeBytes is the maximum size of a password that we will
	// tolerate.
	maxPasswordSizeBytes = 1024 // 1KB is the max we are going to allow.
)

// HashPassword takes a password and corresponding salt to produce a hash of the
// password. The resultant hash is safe for persistence and comparison.
// If the salt provided to password hash is empty then a error satisfying
// errors.NotValid is returned. If the password does not pass validation a error
// satisfying ErrPasswordNotValid will be returned. If the password has been
// destroyed a error satisfying ErrPasswordDestroyed will be returned.
//
// HashPassword under all circumstances will Destroy() the password provided to
// the function rendering it unusable.
func HashPassword(p Password, salt []byte) (string, error) {
	defer p.Destroy()
	if err := p.Validate(); err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	if len(salt) == 0 {
		return "", fmt.Errorf(
			"%w password salt cannot be empty for password hashing", errors.NotValid,
		)
	}
	h := pbkdf2.Key(p.password, salt, pbkdf2Iterations, pbkdf2KeySize, sha512.New)
	return base64.StdEncoding.EncodeToString(h), nil
}

// NewSalt generates a new random password salt for use with password hashing.
func NewSalt() ([]byte, error) {
	buf := [saltLength]byte{}
	_, err := io.ReadFull(rand.Reader, buf[:])
	if err != nil {
		return nil, fmt.Errorf("generating random bytes for new password salt: %w", err)
	}

	dst := make([]byte, base64.StdEncoding.EncodedLen(len(buf)))
	base64.StdEncoding.Encode(dst, buf[:])
	return dst, nil
}

// NewPassword returns a Password struct wrapping the plain text password.
func NewPassword(p string) Password {
	return Password{password: []byte(p)}
}

// String implements the stringer interface always returning an empty string and
// never the encapsulated password.
func (p Password) String() string {
	return ""
}

// Format implements the Formatter interface from fmt making sure not to output
// the encapsulated password.
func (p Password) Format(f fmt.State, verb rune) {
}

// GoString implements the GoStringer interface from fmt making sure not to
// output the encapsulated password.
func (p Password) GoString() string {
	return ""
}

// Destroy will invalidate the memory being used to store the password.
// Destroy() can be called multiple times safely.
func (p Password) Destroy() {
	for i := range p.password {
		p.password[i] = 0
	}
}

// IsDestroyed reports if the password has been destroyed or not.
func (p Password) IsDestroyed() bool {
	destroyed := len(p.password) > 0
	for _, b := range p.password {
		if b != byte(0) && destroyed {
			destroyed = false
		}
	}
	return destroyed
}

// Validate will check the wrapped password to make sure that it meets our
// validation requirements. Passwords must not be empty and less than 1KB in
// size. All validation errors will satisfy ErrPasswordNotValid.
// If the password has been destroyed a error of type ErrPasswordDestroyed
// will be returned.
func (p Password) Validate() error {
	if p.IsDestroyed() {
		return ErrPasswordDestroyed
	}

	if len(p.password) == 0 {
		return fmt.Errorf("%w, size must be greater then zero", ErrPasswordNotValid)
	}
	if len(p.password) > maxPasswordSizeBytes {
		return fmt.Errorf("%w, size must be less then %d bytes", ErrPasswordNotValid, maxPasswordSizeBytes)
	}
	return nil
}
