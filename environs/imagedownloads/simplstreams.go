// Package imagedownloads implements image-downloads metadata from
// simplestreams.
// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package imagedownloads

import (
	"net/url"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/utils"
)

func init() {
	simplestreams.RegisterStructTags(Metadata{})
}

const (
	// DataType is the simplestreams datatype.
	DataType = "image-downloads"
)

// DefaultSource creates a new signed simplestreams datasource for use with the
// image-downloads datatype.
func DefaultSource() simplestreams.DataSource {
	ubuntuImagesURL := imagemetadata.UbuntuCloudImagesURL + "/" + imagemetadata.ReleasedImagesPath
	return newDataSourceFunc(ubuntuImagesURL)()
}

// NewDataSource returns a new simplestreams.DataSource from the provided
// baseURL. baseURL MUST include the image stream.
func NewDataSource(baseURL string) simplestreams.DataSource {
	return newDataSourceFunc(baseURL)()
}

// NewDataSource returns a datasourceFunc from the baseURL provided.
func newDataSourceFunc(baseURL string) func() simplestreams.DataSource {
	return func() simplestreams.DataSource {
		return simplestreams.NewURLSignedDataSource(
			"ubuntu cloud images",
			baseURL,
			imagemetadata.SimplestreamsImagesPublicKey,
			utils.VerifySSLHostnames,
			simplestreams.DEFAULT_CLOUD_DATA,
			true)
	}
}

// Metadata models the inforamtion about a particular cloud image download
// product.
type Metadata struct {
	Arch string `json:"arch,omitempty"`
	// For testing.
	BaseURL string `json:"-"`
	Release string `json:"release,omitempty"`
	Version string `json:"version,omitempty"`
	FType   string `json:"ftype,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Path    string `json:"path,omitempty"`
	Size    int64  `json:"size,omitempty"`
}

// DownloadURL returns the URL representing the image.
func (m *Metadata) DownloadURL() (*url.URL, error) {
	if m.BaseURL == "" {
		m.BaseURL = imagemetadata.UbuntuCloudImagesURL
	}
	u, err := url.Parse(m.BaseURL + "/" + m.Path)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create url")
	}
	return u, nil
}

// Fetch gets product results as Metadata from the provided datasources, given
// some constraints and an optional filter function.
func Fetch(
	src []simplestreams.DataSource,
	cons *imagemetadata.ImageConstraint,
	ff simplestreams.AppendMatchingFunc) ([]*Metadata, *simplestreams.ResolveInfo, error) {
	if ff == nil {
		ff = filter("")
	}
	params := simplestreams.GetMetadataParams{
		StreamsVersion:   imagemetadata.StreamsVersionV1,
		LookupConstraint: cons,
		ValueParams: simplestreams.ValueParams{
			DataType:      DataType,
			FilterFunc:    ff,
			ValueTemplate: Metadata{},
		},
	}
	items, resolveInfo, err := simplestreams.GetMetadata(src, params)
	if err != nil {
		return nil, resolveInfo, err
	}
	md := make([]*Metadata, len(items))
	for i, im := range items {
		md[i] = im.(*Metadata)
	}
	// TODO(ro) 2016-11-04 Do we really need to sort here? If so what on? Other
	// sstreams do it, but I'm not sure this is useful wihtout a knob to tune
	// it on. For now it sorts on release.
	Sort(md)
	return md, resolveInfo, nil
}

// One gets Metadata for one content download item -- the most recent of
// 'series', for architecture, 'arch', of the format 'ftype'. 'src' exists to
// pass in a data source for testing.
func One(arch, series, ftype string, src func() simplestreams.DataSource) (*Metadata, error) {
	if arch == "" {
		return nil, errors.Errorf("%q is not a valid arch", arch)
	}
	// Do we validate this here or elsewhere? Incoming args should already be
	// validated, ideally. Belt ans suspenders can't hurt, just check they
	// aren't empty.
	if series == "" {
		return nil, errors.Errorf("%q is not a valid series", series)
	}
	// ftype isn't validated elsewhere, so we do it here.
	if !validFTypes[ftype] {
		return nil, errors.Errorf("%q is not a valid file type", ftype)
	}
	if src == nil {
		src = DefaultSource
	}
	ds := []simplestreams.DataSource{src()}
	limit := &imagemetadata.ImageConstraint{
		simplestreams.LookupParams{
			Arches: []string{arch},
			Series: []string{series},
		},
	}

	md, _, err := Fetch(ds, limit, filter(ftype))
	if err != nil {
		// It doesn't appear that arch is vetted anywhere else so we can wind
		// up with empty results if someone requests any arch with valid series
		// and ftype..
		return nil, errors.Trace(err)
	}
	if len(md) < 1 {
		return nil, errors.Errorf("no results for %q, %q, %q", arch, series, ftype)
	}
	if len(md) > 1 {
		// Should not be possible.
		return nil, errors.Errorf("got %d results xpected 1 for %q, %q, %q", len(md), arch, series, ftype)
	}
	return md[0], nil
}

// validFTypes is a simple map of file types that we can glean from looking at
// simple streams.
var validFTypes = map[string]bool{
	"disk1.img":   true,
	"lxd.tar.xz":  true,
	"manifest":    true,
	"ova":         true,
	"root.tar.gz": true,
	"root.tar.xz": true,
	"tar.gz":      true,
	"uefi1.img":   true,
}

// filter collects only matching products. Series and Arch are filtered by
// imagemetadata.ImageConstraints. So this really only let's us filter on a
// file type.
func filter(ftype string) simplestreams.AppendMatchingFunc {
	return func(source simplestreams.DataSource, matchingImages []interface{},
		images map[string]interface{}, cons simplestreams.LookupConstraint) ([]interface{}, error) {

		imagesMap := make(map[imageKey]*Metadata, len(matchingImages))
		for _, val := range matchingImages {
			im := val.(*Metadata)
			imagesMap[imageKey{im.Arch, im.FType, im.Release, im.Version}] = im
		}
		for _, val := range images {
			im := val.(*Metadata)
			if ftype != "" {
				if im.FType != ftype {
					continue
				}
			}
			if _, ok := imagesMap[imageKey{im.Arch, im.FType, im.Release, im.Version}]; !ok {
				matchingImages = append(matchingImages, im)
			}
		}
		return matchingImages, nil
	}
}

// imageKey is the key used while filtering products.
type imageKey struct {
	arch    string
	ftype   string
	release string
	version string
}

// Sort sorts a slice of ImageMetadata in ascending order of their id
// in order to ensure the results of Fetch are ordered deterministically.
func Sort(metadata []*Metadata) {
	sort.Sort(byRelease(metadata))
}

type byRelease []*Metadata

func (b byRelease) Len() int           { return len(b) }
func (b byRelease) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byRelease) Less(i, j int) bool { return b[i].Release < b[j].Release }
