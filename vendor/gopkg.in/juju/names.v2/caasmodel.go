// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

const CAASModelTagKind = "caasmodel"

// CAASModelTag represents a tag used to describe a model.
type CAASModelTag struct {
	uuid string
}

// NewCAASModelTag returns the tag of a CAAS model with the given CAAS model UUID.
func NewCAASModelTag(uuid string) CAASModelTag {
	return CAASModelTag{uuid: uuid}
}

// ParseCAASModelTag parses a CAAS model tag string.
func ParseCAASModelTag(caasModelTag string) (CAASModelTag, error) {
	tag, err := ParseTag(caasModelTag)
	if err != nil {
		return CAASModelTag{}, err
	}
	cmt, ok := tag.(CAASModelTag)
	if !ok {
		return CAASModelTag{}, invalidTagError(caasModelTag, CAASModelTagKind)
	}
	return cmt, nil
}

func (t CAASModelTag) String() string { return t.Kind() + "-" + t.Id() }
func (t CAASModelTag) Kind() string   { return CAASModelTagKind }
func (t CAASModelTag) Id() string     { return t.uuid }

// IsValidCAASModel returns whether id is a valid CAAS model UUID.
func IsValidCAASModel(id string) bool {
	return validUUID.MatchString(id)
}

// IsValidCAASModelName returns whether name is a valid string safe for a CAAS model name.
func IsValidCAASModelName(name string) bool {
	return validModelName.MatchString(name)
}
