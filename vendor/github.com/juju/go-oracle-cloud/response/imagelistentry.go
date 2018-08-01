// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

// ImageListEntry represents the metadata of a machine image list
type ImageListEntry struct {

	// User-defined parameters, in JSON format,
	// that can be passed to an instance of this machine
	// image when it is launched.
	// This field can be used, for example, to specify the location
	// of a database server and login details.
	// Instance metadata, including user-defined data
	// is available at http://192.0.0.192/ within an instance.
	Attributes AttributesEntry `json:"attributes,omitempty"`

	// Imagelist is the name of the imagelist.
	Imagelist string `json:"imagelist"`

	// Version number of these machineImages in the imagelist.
	Version int `json:"version"`

	// Machineimages represetns a slice of machine images.
	Machineimages []string `json:"machineimages"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`
}

type AttributesEntry struct {
	// User-defined parameters, in JSON format,
	// that can be passed to an instance of this machine
	// image when it is launched.
	Userdata map[string]interface{} `json:"userdata,omitempty"`

	MinimumDiskSize string `json:"minimumdisksize,omitempty"`
	DefaultShape    string `json:"defaultshape,omitempty"`
	SupportedShapes string `json:"supportedShapes,omitempty"`
}

// ImageListEntryAdd custom response returned from CreateImageListEntryAdd
// This is used instead of ImageListEntry beacause the api is inconsistent
type ImageListEntryAdd struct {
	Attributes    interface{} `json:"attributes,omitempty"`
	Imagelist     ImageList   `json:"Imagelist"`
	Version       int         `json:"version"`
	Machineimages []string    `json:"machineimages"`
	Uri           string      `json:"uri"`
}
