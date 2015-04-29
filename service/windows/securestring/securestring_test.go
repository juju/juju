// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.
//
// +build windows !linux

package securestring_test

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/windows/securestring"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type SecureStringSuite struct{}

var _ = gc.Suite(&SecureStringSuite{})

func (s *SecureStringSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS != "windows" {
		c.Skip("Package go-securestring requires running on Windows.")
	}
}

var testInputs []string = []string{
	"Simple",
	"A longer string",
	"A!string%with(a4239lot#of$&*special@characters{[]})",
	"Quite a very much longer string meant to push the envelope",
	"fsdafsgdfgdfgdfgdfgsdfgdgdfgdmmghnh kv dfv dj fkvjjenrwenvfvvslfvnsljfvnlsfvlnsfjlvnssdwoewivdsvmxxvsdvsdv",
}

// tests whether encryption and decryption are symmetrical operations
func (s *SecureStringSuite) TestEncryptDecryptSymmetry(c *gc.C) {
	for _, input := range testInputs {
		enc, err := securestring.Encrypt(input)
		c.Assert(err, gc.IsNil)
		dec, err := securestring.Decrypt(enc)
		c.Assert(err, gc.IsNil)

		c.Assert(dec, gc.Equals, input)
	}
}

var invokePowerShellParams []string = []string{
	"-NoProfile",
	"-NonInteractive",
	"-Command",
	"try{$input|iex; exit $LastExitCode}catch{Write-Error -Message $Error[0]; exit 1}",
}

func runPowerShellCommands(cmds string) (string, error) {
	ps := exec.Command("powershell.exe", invokePowerShellParams...)

	ps.Stdin = strings.NewReader(cmds)
	stdout := &bytes.Buffer{}
	ps.Stdout = stdout

	err := ps.Run()
	if err != nil {
		return "", err
	}

	output := string(stdout.String())
	return strings.TrimSpace(output), nil
}

// tests whether the output of ConvertFrom-SecureString is compatible with the module and can be decrypted
func (s *SecureStringSuite) TestDecryptFromCFSS(c *gc.C) {
	for _, input := range testInputs {
		psenc, err := runPowerShellCommands(fmt.Sprintf("ConvertTo-SecureString \"%s\" -AsPlainText -Force | ConvertFrom-SecureString", input))
		c.Assert(err, gc.IsNil)

		dec, err := securestring.Decrypt(psenc)
		c.Assert(err, gc.IsNil)

		c.Assert(dec, gc.Equals, input)
	}
}

// tests whether the output of the module is compatible with PowerShell's SecureString and is accepted as valid
func (s *SecureStringSuite) TestConvertEncryptedToPowerShellSS(c *gc.C) {
	for _, input := range testInputs {
		enc, err := securestring.Encrypt(input)
		c.Assert(err, gc.IsNil)

		psresp, err := runPowerShellCommands(fmt.Sprintf("\"%s\" | ConvertTo-SecureString", enc))
		c.Assert(err, gc.IsNil)

		c.Assert(psresp, gc.Equals, "System.Security.SecureString")
	}
}
