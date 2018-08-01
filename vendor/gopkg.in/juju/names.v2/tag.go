// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
)

// A Tag tags things that are taggable. Its purpose is to uniquely
// identify some resource and provide a consistent representation of
// that identity in both a human-readable and a machine-friendly format.
// The latter benefits use of the tag in over-the-wire transmission
// (e.g. in HTTP RPC calls) and in filename paths. The human-readable
// tag "name" is available through the Id method. The machine-friendly
// representation is provided by the String method.
//
// The ParseTag function may be used to build a tag from the machine-
// formatted string. As well each kind of tag has its own Parse* method.
// Each kind also has a New* method (e.g. NewMachineTag) which produces
// a tag from the human-readable tag "ID".
//
// In the context of juju, the API *must* use tags to represent the
// various juju entities. This contrasts with user-facing code, where
// tags *must not* be used. Internal to juju the use of tags is a
// judgement call based on the situation.
type Tag interface {
	// Kind returns the kind of the tag.
	// This method is for legacy compatibility, callers should
	// use equality or type assertions to verify the Kind, or type
	// of a Tag.
	Kind() string

	// Id returns an identifier for this Tag.
	// The contents and format of the identifier are specific
	// to the implementer of the Tag.
	Id() string

	fmt.Stringer // all Tags should be able to print themselves
}

// tagString returns the canonical string representation of a tag.
// It round-trips with splitTag().
func tagString(tag Tag) string {
	return tag.Kind() + "-" + tag.Id()
}

// TagKind returns one of the *TagKind constants for the given tag, or
// an error if none matches.
func TagKind(tag string) (string, error) {
	i := strings.Index(tag, "-")
	if i <= 0 || !validKinds(tag[:i]) {
		return "", fmt.Errorf("%q is not a valid tag", tag)
	}
	return tag[:i], nil
}

func validKinds(kind string) bool {
	switch kind {
	case UnitTagKind, MachineTagKind, ApplicationTagKind, ApplicationOfferTagKind, EnvironTagKind, UserTagKind,
		RelationTagKind, ActionTagKind, VolumeTagKind, CharmTagKind, StorageTagKind,
		FilesystemTagKind, IPAddressTagKind, SpaceTagKind, SubnetTagKind,
		PayloadTagKind, ModelTagKind, ControllerTagKind, CloudTagKind, CloudCredentialTagKind, CAASModelTagKind:
		return true
	}
	return false
}

func splitTag(tag string) (string, string, error) {
	kind, err := TagKind(tag)
	if err != nil {
		return "", "", err
	}
	return kind, tag[len(kind)+1:], nil
}

// ParseTag parses a string representation into a Tag.
func ParseTag(tag string) (Tag, error) {
	kind, id, err := splitTag(tag)
	if err != nil {
		return nil, invalidTagError(tag, "")
	}
	switch kind {
	case UnitTagKind:
		id = unitTagSuffixToId(id)
		if !IsValidUnit(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewUnitTag(id), nil
	case MachineTagKind:
		id = machineTagSuffixToId(id)
		if !IsValidMachine(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewMachineTag(id), nil
	case ApplicationTagKind:
		if !IsValidApplication(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewApplicationTag(id), nil
	case ApplicationOfferTagKind:
		if !IsValidApplicationOffer(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewApplicationOfferTag(id), nil
	case UserTagKind:
		if !IsValidUser(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewUserTag(id), nil
	case EnvironTagKind:
		if !IsValidEnvironment(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewEnvironTag(id), nil
	case ModelTagKind:
		if !IsValidModel(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewModelTag(id), nil
	case ControllerTagKind:
		if !IsValidController(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewControllerTag(id), nil

	case RelationTagKind:
		id = relationTagSuffixToKey(id)
		if !IsValidRelation(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewRelationTag(id), nil
	case ActionTagKind:
		if !IsValidAction(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewActionTag(id), nil
	case VolumeTagKind:
		id = filesystemOrVolumeTagSuffixToId(id)
		if !IsValidVolume(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewVolumeTag(id), nil
	case CharmTagKind:
		if !IsValidCharm(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewCharmTag(id), nil
	case StorageTagKind:
		id = storageTagSuffixToId(id)
		if !IsValidStorage(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewStorageTag(id), nil
	case FilesystemTagKind:
		id = filesystemOrVolumeTagSuffixToId(id)
		if !IsValidFilesystem(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewFilesystemTag(id), nil
	case IPAddressTagKind:
		uuid, err := utils.UUIDFromString(id)
		if err != nil {
			return nil, invalidTagError(tag, kind)
		}
		return NewIPAddressTag(uuid.String()), nil
	case SubnetTagKind:
		if !IsValidSubnet(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewSubnetTag(id), nil
	case SpaceTagKind:
		if !IsValidSpace(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewSpaceTag(id), nil
	case PayloadTagKind:
		if !isValidPayload(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewPayloadTag(id), nil
	case CloudTagKind:
		if !IsValidCloud(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewCloudTag(id), nil
	case CloudCredentialTagKind:
		id, err = cloudCredentialTagSuffixToId(id)
		if err != nil {
			return nil, errors.Wrap(err, invalidTagError(tag, kind))
		}
		if !IsValidCloudCredential(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewCloudCredentialTag(id), nil
	case CAASModelTagKind:
		if !IsValidCAASModel(id) {
			return nil, invalidTagError(tag, kind)
		}
		return NewCAASModelTag(id), nil
	default:
		return nil, invalidTagError(tag, "")
	}
}

func invalidTagError(tag, kind string) error {
	if kind != "" {
		return fmt.Errorf("%q is not a valid %s tag", tag, kind)
	}
	return fmt.Errorf("%q is not a valid tag", tag)
}

// ReadableString returns a human-readable string from the tag passed in.
// It currently supports unit and machine tags. Support for additional types
// can be added in as needed.
func ReadableString(tag Tag) string {
	if tag == nil {
		return ""
	}

	return tag.Kind() + " " + tag.Id()
}
