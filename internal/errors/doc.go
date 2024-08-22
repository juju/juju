// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

/*
Package errors implements a set of error helpers and proxies on to standard
go errors for use within Juju.

Package level errors through out Juju can be made by using [ConstError]

	package MyPackage

	const (
		MyFabulousError = ConstError("sparkling")
	)

	return Errorf("this error %w happened", MyFabulousError)

Errors can be extended further by turning a given error into a [Error] type.
[Error] types are obtained by first annotating an already existing error with
more context with [Errorf], creating a new error with [New] or combining a set
of errors with [Join].

In this package there exists several helper types for checking an error chain.
[AsType] allows the caller to check if there is an error of a given type T in
the chain and returns back a copy of T if the type was found. [HasType] is the
same as [AsType] but instead of returning T only a bool indicating if T was
found is returned.
*/
package errors
