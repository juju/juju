// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/domain/application"
	"github.com/juju/tc"
)

// createUnitStorageArgChecker returns a new [tc.MultiChecker] capable of
// validating a [github.com/juju/juju/domain/application.CreateUnitStorageArg]
// value.
func createUnitStorageArgChecker() *tc.MultiChecker {
	expectedStorageInstanceChecker := tc.NewMultiChecker()
	expectedStorageInstanceChecker.AddExpr("_.UUID", tc.IsNonZeroUUID)
	expectedStorageInstanceChecker.AddExpr("_.Filesystem.UUID", tc.IsNonZeroUUID)

	expectedStorageAttachmentChecker := tc.NewMultiChecker()
	expectedStorageAttachmentChecker.AddExpr("_.UUID", tc.IsNonZeroUUID)
	expectedStorageAttachmentChecker.AddExpr("_.FilesystemAttachment.UUID", tc.IsNonZeroUUID)
	expectedStorageAttachmentChecker.AddExpr("_.VolumeAttachment.UUID", tc.IsNonZeroUUID)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.StorageDirectives", tc.SameContents, tc.ExpectedValue)
	mc.AddExpr(
		"_.StorageInstances",
		tc.UnorderedMatch[[]application.CreateUnitStorageInstanceArg](
			expectedStorageInstanceChecker,
		),
		tc.ExpectedValue,
	)
	mc.AddExpr(
		"_.StorageToAttach",
		tc.UnorderedMatch[[]application.CreateUnitStorageAttachmentArg](
			expectedStorageAttachmentChecker,
		),
		tc.ExpectedValue,
	)
	mc.AddExpr(
		"_.StorageToOwn",
		tc.SameContents,
		tc.ExpectedValue,
	)

	return mc
}

// registerUnitStorageArgChecker returns a new [tc.MultiChecker] capable of
// validating a [github.com/juju/juju/domain/application.RegisterUnitStorageArg]
// value.
func registerUnitStorageArgChecker() *tc.MultiChecker {
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.CreateUnitStorageArg", createUnitStorageArgChecker(), tc.ExpectedValue)
	return mc
}
