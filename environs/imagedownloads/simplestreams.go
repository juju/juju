// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagedownloads

import (
	"fmt"
	"net/url"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/os/series"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
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
		return simplestreams.NewDataSource(
			simplestreams.Config{
				Description:          "ubuntu cloud images",
				BaseURL:              baseURL,
				PublicSigningKey:     imagemetadata.SimplestreamsImagesPublicKey,
				HostnameVerification: utils.VerifySSLHostnames,
				Priority:             simplestreams.DEFAULT_CLOUD_DATA,
				RequireSigned:        true,
			},
		)
	}
}

// Metadata models the information about a particular cloud image download
// product.
type Metadata struct {
	Arch    string `json:"arch,omitempty"`
	Release string `json:"release,omitempty"`
	Version string `json:"version,omitempty"`
	FType   string `json:"ftype,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Path    string `json:"path,omitempty"`
	Size    int64  `json:"size,omitempty"`
}

// DownloadURL returns the URL representing the image.
func (m *Metadata) DownloadURL(baseURL string) (*url.URL, error) {
	if baseURL == "" {
		baseURL = imagemetadata.UbuntuCloudImagesURL
	}
	u, err := url.Parse(baseURL + "/" + m.Path)
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
		ff = Filter("")
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

	Sort(md)

	return md, resolveInfo, nil
}

func validateArgs(arch, release, ftype string) error {
	bad := map[string]string{}

	if !validArches[arch] {
		bad[arch] = fmt.Sprintf("arch=%q", arch)
	}

	validSeries := false
	for _, supported := range series.SupportedSeries() {
		if release == supported {
			validSeries = true
			break
		}
	}
	if !validSeries {
		bad[release] = fmt.Sprintf("series=%q", release)
	}

	if !validFTypes[ftype] {
		bad[ftype] = fmt.Sprintf("ftype=%q", ftype)
	}

	if len(bad) > 0 {
		errMsg := "invalid parameters supplied"
		for _, k := range []string{arch, release, ftype} {
			if v, ok := bad[k]; ok {
				errMsg += fmt.Sprintf(" %s", v)
			}
		}
		return errors.New(errMsg)
	}
	return nil
}

// One gets Metadata for one content download item:
// The most recent of:
//   - architecture
//   - OS release
//   - Simplestreams stream
//   - File image type.
// src exists to pass in a data source for testing.
func One(arch, release, stream, ftype string, src func() simplestreams.DataSource) (*Metadata, error) {
	if err := validateArgs(arch, release, ftype); err != nil {
		return nil, errors.Trace(err)
	}
	if src == nil {
		src = DefaultSource
	}
	ds := []simplestreams.DataSource{src()}
	limit := imagemetadata.NewImageConstraint(
		simplestreams.LookupParams{
			Arches: []string{arch},
			Series: []string{release},
			Stream: stream,
		},
	)

	md, _, err := Fetch(ds, limit, Filter(ftype))
	if err != nil {
		// It doesn't appear that arch is vetted anywhere else so we can wind
		// up with empty results if someone requests any arch with valid series
		// and ftype..
		return nil, errors.Trace(err)
	}
	if len(md) < 1 {
		return nil, errors.Errorf("no results for %q, %q, %q, %q", arch, release, stream, ftype)
	}
	if len(md) > 1 {
		// Should not be possible.
		return nil, errors.Errorf("got %d results expected 1 for %q, %q, %q, %q", len(md), arch, release, stream, ftype)
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

// validArches are the arches we support running kvm containers on.
var validArches = map[string]bool{
	arch.AMD64:   true,
	arch.ARM64:   true,
	arch.PPC64EL: true,
}

// Filter collects only matching products. Series and Arch are filtered by
// imagemetadata.ImageConstraints. So this really only let's us filter on a
// file type.
func Filter(ftype string) simplestreams.AppendMatchingFunc {
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
