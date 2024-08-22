// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

/*
Package errors defines a set of common error types for use within Juju. If you
are reaching for these errors to use please consider defining more specific
package based errors for your needs. Generic error types don't always relay
context.

The error types defined here are equivalent and comparable to the error types
found in juju/errors and can safely be used as replacements.
*/
package errors
