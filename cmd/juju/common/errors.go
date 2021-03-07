// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"io"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v3/httpbakery"
)

func PermissionsMessage(writer io.Writer, command string) {
	const (
		perm  = "You do not have permission to %s."
		grant = `You may ask an administrator to grant you access with "juju grant".`
	)

	if command == "" {
		command = "complete this operation"
	}
	fmt.Fprintf(writer, "\n%s\n%s\n\n", fmt.Sprintf(perm, command), grant)
}

// MaybeTermsAgreementError returns err as a *TermsAgreementError
// if it has a "terms agreement required" error code, otherwise
// it returns err unchanged.
func MaybeTermsAgreementError(err error) error {
	const code = "term agreement required"
	e, ok := errors.Cause(err).(*httpbakery.DischargeError)
	if !ok || e.Reason == nil || e.Reason.Code != code {
		return err
	}
	magicMarker := code + ":"
	index := strings.LastIndex(e.Reason.Message, magicMarker)
	if index == -1 {
		return err
	}
	return &TermsRequiredError{strings.Fields(e.Reason.Message[index+len(magicMarker):])}
}

// TermsRequiredError is an error returned when agreement to terms is required.
type TermsRequiredError struct {
	Terms []string
}

// Error implements error.
func (e *TermsRequiredError) Error() string {
	return fmt.Sprintf("please agree to terms %q", strings.Join(e.Terms, " "))
}

// UserErr returns an error containing a user-friendly message describing how
// to agree to required terms.
func (e *TermsRequiredError) UserErr() error {
	terms := strings.Join(e.Terms, " ")
	return errors.Wrap(e,
		errors.Errorf(`Declined: some terms require agreement. Try: "juju agree %s"`, terms))
}

const missingModelNameMessage = `
juju: no model name was passed. See "juju %[1]s --help".

Did you mean:
	juju %[1]s <model name>`

// MissingModelNameError returns an error stating that the model name is missing
// and provides a better UX experience to the user.
func MissingModelNameError(cmdName string) error {
	return errors.Errorf(missingModelNameMessage[1:], cmdName)
}
