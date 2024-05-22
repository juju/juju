// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
)

// FieldPresenceMap indicates which keys of a parsed bundle yaml document were
// present when the document was parsed. This map is used by the overlay merge
// code to figure out whether empty/nil field values were actually specified as
// such in the yaml document.
type FieldPresenceMap map[interface{}]interface{}

func (fpm FieldPresenceMap) fieldPresent(fieldName string) bool {
	_, exists := fpm[fieldName]
	return exists
}

func (fpm FieldPresenceMap) forField(fieldName string) FieldPresenceMap {
	v, exists := fpm[fieldName]
	if !exists {
		return nil
	}

	// Always returns a FieldPresenceMap even if the underlying type is empty.
	// As the only way to interact with the map is through the use of the two
	// methods, then it will allow you to walk over the map in a much saner way.
	asMap, _ := v.(FieldPresenceMap)
	if asMap == nil {
		return FieldPresenceMap{}
	}
	return asMap
}

// BundleDataPart combines a parsed BundleData instance with a nested map that
// can be used to discriminate between fields that are missing from the data
// and those that are present but defined to be empty.
type BundleDataPart struct {
	Data            *BundleData
	PresenceMap     FieldPresenceMap
	UnmarshallError error
}

// BundleDataSource is implemented by types that can parse bundle data into a
// list of composable parts.
type BundleDataSource interface {
	Parts() []*BundleDataPart
	BundleBytes() []byte
	BasePath() string
	ResolveInclude(path string) ([]byte, error)
}

type resolvedBundleDataSource struct {
	basePath    string
	bundleBytes []byte
	parts       []*BundleDataPart
}

func (s *resolvedBundleDataSource) Parts() []*BundleDataPart {
	return s.parts
}

func (s *resolvedBundleDataSource) BundleBytes() []byte {
	return s.bundleBytes
}

func (s *resolvedBundleDataSource) BasePath() string {
	return s.basePath
}

func (s *resolvedBundleDataSource) ResolveInclude(path string) ([]byte, error) {
	absPath := path
	if !filepath.IsAbs(absPath) {
		var err error
		absPath, err = filepath.Abs(filepath.Clean(filepath.Join(s.basePath, absPath)))
		if err != nil {
			return nil, errors.Annotatef(err, "resolving relative include %q", path)
		}
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if isNotExistsError(err) {
			return nil, errors.NotFoundf("include file %q", absPath)
		}

		return nil, errors.Annotatef(err, "stat failed for %q", absPath)
	}

	if info.IsDir() {
		return nil, errors.Errorf("include path %q resolves to a folder", absPath)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, errors.Annotatef(err, "reading include file at %q", absPath)
	}

	return data, nil
}

// LocalBundleDataSource reads a (potentially multi-part) bundle from path and
// returns a BundleDataSource for it. Path may point to a yaml file, a bundle
// directory or a bundle archive.
func LocalBundleDataSource(path string) (BundleDataSource, error) {
	info, err := os.Stat(path)
	if err != nil {
		if isNotExistsError(err) {
			return nil, errors.NotFoundf("%q", path)
		}

		return nil, errors.Annotatef(err, "stat failed for %q", path)
	}

	// Treat as an exploded bundle archive directory
	if info.IsDir() {
		path = filepath.Join(path, "bundle.yaml")
	}

	// Try parsing as a yaml file first
	f, err := os.Open(path)
	if err != nil {
		if isNotExistsError(err) {
			return nil, errors.NotFoundf("%q", path)
		}
		return nil, errors.Annotatef(err, "access bundle data at %q", path)
	}
	defer func() { _ = f.Close() }()

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	parts, pErr := parseBundleParts(b)
	if pErr == nil {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, errors.Annotatef(err, "resolve absolute path to %s", path)
		}
		return &resolvedBundleDataSource{
			basePath:    filepath.Dir(absPath),
			parts:       parts,
			bundleBytes: b,
		}, nil
	}

	// As a fallback, try to parse as a bundle archive
	zo := newZipOpenerFromPath(path)
	zrc, err := zo.openZip()
	if err != nil {
		// Not a zip file; return the original parse error
		return nil, errors.NewNotValid(pErr, "cannot unmarshal bundle contents")
	}
	defer func() { _ = zrc.Close() }()

	r, err := zipOpenFile(zrc, "bundle.yaml")
	if err != nil {
		// It is a zip file but not one that contains a bundle.yaml
		return nil, errors.NotFoundf("interpret bundle contents as a bundle archive: %v", err)
	}
	defer func() { _ = r.Close() }()

	b, err = io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if parts, pErr = parseBundleParts(b); pErr == nil {
		return &resolvedBundleDataSource{
			basePath:    "", // use empty base path for archives
			parts:       parts,
			bundleBytes: b,
		}, nil
	}

	return nil, errors.NewNotValid(pErr, "cannot unmarshal bundle contents")
}

func isNotExistsError(err error) bool {
	if os.IsNotExist(err) {
		return true
	}
	// On Windows, we get a path error due to a GetFileAttributesEx syscall.
	// To avoid being too proscriptive, we'll simply check for the error
	// type and not any content.
	if _, ok := err.(*os.PathError); ok {
		return true
	}
	return false
}

// StreamBundleDataSource reads a (potentially multi-part) bundle from r and
// returns a BundleDataSource for it.
func StreamBundleDataSource(r io.Reader, basePath string) (BundleDataSource, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	parts, err := parseBundleParts(b)
	if err != nil {
		return nil, errors.NotValidf("cannot unmarshal bundle contents: %v", err)
	}

	return &resolvedBundleDataSource{parts: parts, bundleBytes: b, basePath: basePath}, nil
}

func parseBundleParts(b []byte) ([]*BundleDataPart, error) {
	var (
		// Ideally, we would be using a single reader and we would
		// rewind it to read each block in structured and raw mode.
		// Unfortunately, the yaml parser seems to parse all documents
		// at once so we need to use two decoders. The third is to allow
		// for validation of the yaml by using strict decoding. However
		// we still want to return non strict bundle parts so that
		// force may be used in deploy.
		structDec = yaml.NewDecoder(bytes.NewReader(b))
		strictDec = yaml.NewDecoder(bytes.NewReader(b))
		rawDec    = yaml.NewDecoder(bytes.NewReader(b))
		parts     []*BundleDataPart
	)

	for docIdx := 0; ; docIdx++ {
		var part BundleDataPart

		err := structDec.Decode(&part.Data)
		if err == io.EOF {
			break
		} else if err != nil && !strings.HasPrefix(err.Error(), "yaml: unmarshal errors:") {
			return nil, errors.Annotatef(err, "unmarshal document %d", docIdx)
		}

		var data *BundleData
		strictDec.SetStrict(true)
		err = strictDec.Decode(&data)
		if err == io.EOF {
			break
		} else if err != nil {
			if strings.HasPrefix(err.Error(), "yaml: unmarshal errors:") {
				friendlyErrors := userFriendlyUnmarshalErrors(err)
				part.UnmarshallError = errors.Annotatef(friendlyErrors, "unmarshal document %d", docIdx)
			} else {
				return nil, errors.Annotatef(err, "unmarshal document %d", docIdx)
			}
		}

		// We have already checked for errors for the previous unmarshal attempt
		_ = rawDec.Decode(&part.PresenceMap)
		parts = append(parts, &part)
	}

	return parts, nil
}

func userFriendlyUnmarshalErrors(err error) error {
	friendlyText := err.Error()
	friendlyText = strings.ReplaceAll(friendlyText, "type charm.ApplicationSpec", "applications")
	friendlyText = strings.ReplaceAll(friendlyText, "type charm.legacyBundleData", "bundle")
	friendlyText = strings.ReplaceAll(friendlyText, "type charm.RelationSpec", "relations")
	friendlyText = strings.ReplaceAll(friendlyText, "type charm.MachineSpec", "machines")
	friendlyText = strings.ReplaceAll(friendlyText, "type charm.SaasSpec", "saas")
	return errors.New(friendlyText)
}
