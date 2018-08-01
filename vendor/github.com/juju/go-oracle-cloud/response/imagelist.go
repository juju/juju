// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

// An image list is a collection of
// Oracle Compute Cloud Service machine images.
// Each machine image in an image list is identified
// by a unique entry number. When you create an instance,
// by using a launch plan for example, you must
// specify the image list that contains the
// machine image you want to use.
type ImageList struct {
	// Default is the image list entry to be used,
	// by default, when launching instances
	// using this image list.
	// If you don't specify this value, it is set to 1.
	Default int `json:"default"`

	// Description is a description of this image list.
	Description *string `json:"description,omitempty"`

	// Entries represents each machine image in an
	// image list is identified by an image list entry.
	Entries []ImageListEntry `json:"entries,omitempty"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`

	// Name is the name of the imagelist
	Name string `json:"name"`
}

// AllImageLists contains a slice of all lists of images
// in the oracle cloud account
type AllImageLists struct {
	Result []ImageList `json:"result,omitempty"`
}
