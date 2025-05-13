// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auth

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/juju/tc"

	internalpassword "github.com/juju/juju/internal/password"
)

type passwordSuite struct {
}

var _ = tc.Suite(&passwordSuite{})

func ExampleHashPassword() {
	userExposedPassword := "topsecret"

	password := NewPassword(userExposedPassword)
	salt, err := NewSalt()
	if err != nil {
		log.Fatalf("generating password salt: %v", salt)
	}

	hash, err := HashPassword(password, salt)
	if err != nil {
		log.Fatalf("generating password hash with salt: %v", err)
	}

	fmt.Println(hash)
}

// TestPasswordEncapsulation is asserting that a wrapped password does not leak
// the encapsulated plain text password.
func (*passwordSuite) TestPasswordEncapsulation(c *tc.C) {
	p := NewPassword("topsecret")

	c.Assert(p.String(), tc.Equals, "")
	c.Assert(p.GoString(), tc.Equals, "")
	c.Assert(fmt.Sprintf("%s", p), tc.Equals, "")
	c.Assert(fmt.Sprintf("%v", p), tc.Equals, "")
	c.Assert(fmt.Sprintf("%#v", p), tc.Equals, "")
	c.Assert(fmt.Sprintf("%T", p), tc.Equals, "auth.Password")
}

// TestPasswordValidation exists to assert the validation rules we apply to
// passwords. All passwords in this test are invalid and should cause a
// ErrPasswordNotValid error.
func (*passwordSuite) TestPasswordValidation(c *tc.C) {
	tests := []string{
		"",                        // We don't allow empty passwords
		strings.Repeat("1", 1025), // We don't allow password over 1KB
	}

	for _, test := range tests {
		err := NewPassword(test).Validate()
		c.Assert(err, tc.ErrorIs, ErrPasswordNotValid,
			tc.Commentf("expected password %q to return ErrPasswordNotValid", test),
		)
	}
}

// TestPasswordValidationDestroyed asserts that after destroying a password and
// then validating the password a error that satisfies ErrPasswordDestroyed is
// returned.
func (*passwordSuite) TestPasswordValidationDestroyed(c *tc.C) {
	p := NewPassword("topsecret")
	p.Destroy()
	c.Assert(p.IsDestroyed(), tc.IsTrue)
	err := p.Validate()
	c.Assert(err, tc.ErrorIs, ErrPasswordDestroyed)
}

// TestPasswordHashing is testing some known password and their respective
// hashes to make sure we are always getting the same hash output.
func (*passwordSuite) TestPasswordHashing(c *tc.C) {
	tests := []struct {
		Password string
		Salt     string
		Hash     string
	}{
		{
			Password: "topsecretpassword",
			Salt:     "xVwuRk5pzUg",
			Hash:     "TEJvoj03UpTREYTUVs+KmOTv",
		},
		{
			Password: "テストパスワード",
			Salt:     "xVwuRk5pzUg",
			Hash:     "p5kmNGdEeQHSJ1fy2u7lOKOJ",
		},
		{
			Password: "西蒙是最好的",
			Salt:     "xVwuRk5pzUg",
			Hash:     "8U1/Oj8LHmD+ejpfc8mnFWZM",
		},
		{
			Password: "علی",
			Salt:     "xVwuRk5pzUg",
			Hash:     "YW0UnyAEFq152ukVHAjRVjDz",
		},
		{
			Password: "123😱😱😱😱.testpassword",
			Salt:     "xVwuRk5pzUg",
			Hash:     "jm2w/Q+bC3AdjVD4kTrRT95o",
		},
	}

	for _, test := range tests {
		p := NewPassword(test.Password)
		hash, err := HashPassword(p, []byte(test.Salt))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(hash, tc.Equals, test.Hash,
			tc.Commentf("computed hash %q != expected hash %q for password %q and salt %q",
				hash, test.Hash, test.Password, test.Salt),
		)

		// We want to assert that HashPassword destroys the password after use.
		// Because of the way the password structure is formed we have to look
		// inside the password slice and make sure all the values are zero.
		for _, b := range p.password {
			c.Assert(b, tc.Equals, byte(0),
				tc.Commentf("checking that all bytes in the password have been set to zero"),
			)
		}
	}
}

// TestPasswordHashingDestroyed tests that when hashing a destroyed password a
// error is returned satisfying ErrPasswordDestroyed.
func (*passwordSuite) TestPasswordHashingDestroyed(c *tc.C) {
	p := NewPassword("topsecret")
	p.Destroy()
	c.Assert(p.IsDestroyed(), tc.IsTrue)
	_, err := HashPassword(p, []byte("secretsauce"))
	c.Assert(err, tc.ErrorIs, ErrPasswordDestroyed)
}

// TestPasswordHashWithUtils is testing the password hashing inside of Juju with
// that of Juju utils. This it to check that both algorithms come to the same
// conclusion. This test will assert that moving password hashing back into Juju
// from utils has not broken anything.
func (*passwordSuite) TestPasswordHashWithUtils(c *tc.C) {
	tests := []string{
		"testmctestface",
		"テストパスワード",
		"password1234",
		"1",
		"😁❗️",
		"علی",
	}

	salt := "xVwuRk5pzUg"

	for _, test := range tests {
		utilsHash := internalpassword.UserPasswordHash(test, salt)
		jujuHash, err := HashPassword(NewPassword(test), []byte(salt))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(utilsHash, tc.Equals, jujuHash,
			tc.Commentf("juju/utils/v4 hash %q != internal/password hash %q for password %q",
				utilsHash, jujuHash, test),
		)
	}
}

// TestNewSalt is here to check that a salt can be generated with no errors and
// the length of the salt is equal to that of what we expect.
func (*passwordSuite) TestNewSalt(c *tc.C) {
	salt, err := NewSalt()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(salt) != 0, tc.IsTrue)
}

// TestDestroyPasswordMultiple checks that we can call Destroy() on a password
// multiple times and that no panics occur.
func (*passwordSuite) TestDestroyPasswordMultiple(c *tc.C) {
	p := NewPassword("topsecret")
	// Three times should be plenty
	p.Destroy()
	p.Destroy()
	p.Destroy()
	c.Assert(p.IsDestroyed(), tc.IsTrue)
}

// FuzzPasswordHashing is a fuzz test to both try and break our password hashing
// inputs and to also confirm that for a wide range of inputs that utils hashing
// is the same this implementation in internal/password.
func FuzzPasswordHashing(f *testing.F) {
	corpase := []string{
		"testmctestface",
		"テストパスワード",
		"password1234",
		"1",
		"😁❗️",
		"revving-churl-brat-femur",
		"علی",
	}

	for _, c := range corpase {
		f.Add(c)
	}

	salt := "xVwuRk5pzUg"
	f.Fuzz(func(t *testing.T, password string) {
		utilsHash := internalpassword.UserPasswordHash(password, salt)
		jujuHash, err := HashPassword(NewPassword(password), []byte(salt))
		// Fuzz testing will give us a string that is all nil chars and that
		// will cause us to think the error is destroyed. This is perfectly
		// valid. Fuzz testing is trying to break us and assert logic paths we
		// don't have.
		if errors.Is(err, ErrPasswordNotValid) ||
			errors.Is(err, ErrPasswordDestroyed) {
			return
		}

		if err != nil {
			t.Fatalf("expected nil error from HashPassword() but got %v", err)
		}

		if utilsHash != jujuHash {
			t.Errorf(
				"expected juju utils hash %q for password %q to equal HashPassword() %q",
				utilsHash, password, jujuHash)
		}
	})
}
