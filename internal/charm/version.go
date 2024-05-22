// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/juju/errors"
)

// ReadVersion extracts the VCS version from a charm's version file.
func ReadVersion(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		return "", errors.Annotate(err, "cannot read version file")
	}

	// bzr revision info starts with "revision-id: " so strip that.
	revLine := strings.TrimPrefix(scanner.Text(), "revision-id: ")
	return fmt.Sprintf("%.100s", revLine), nil
}
