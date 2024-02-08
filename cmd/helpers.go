// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
)

// This file contains helper functions for generic operations commonly needed
// when implementing a command.

const yesNoMsg = "\nContinue [y/N]? "

var nameVerificationMsg = "\nTo continue, enter the name of the %s to be destroyed: "

type userAbortedError string

func (e userAbortedError) Error() string {
	return string(e)
}

// IsUserAbortedError returns true if err is of type userAbortedError.
func IsUserAbortedError(err error) bool {
	_, ok := errors.Cause(err).(userAbortedError)
	return ok
}

// UserConfirmYes returns an error if we do not read a "y" or "yes" from user
// input.
func UserConfirmYes(ctx *cmd.Context) error {
	fmt.Fprint(ctx.Stderr, yesNoMsg)
	scanner := bufio.NewScanner(ctx.Stdin)
	scanner.Scan()
	err := scanner.Err()
	if err != nil && err != io.EOF {
		return errors.Trace(err)
	}
	answer := strings.ToLower(scanner.Text())
	if answer != "y" && answer != "yes" {
		return errors.Trace(userAbortedError("aborted"))
	}
	return nil
}

// UserConfirmName returns an error if we do not read a "name" of the model/controller/etc from user
// input.
func UserConfirmName(verificationName string, objectType string, ctx *cmd.Context) error {
	fmt.Fprintf(ctx.Stderr, nameVerificationMsg, objectType)
	scanner := bufio.NewScanner(ctx.Stdin)
	scanner.Scan()
	err := scanner.Err()
	if err != nil && err != io.EOF {
		return errors.Trace(err)
	}
	answer := strings.ToLower(scanner.Text())
	if answer != verificationName {
		return errors.Trace(userAbortedError("aborted"))
	}
	return nil
}
