// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	internalerrors "github.com/juju/juju/internal/errors"
)

// readVersion extracts the VCS version from a charm's version file.
func readVersion(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		return "", internalerrors.Errorf("cannot read version file: %w", err)
	}

	// bzr revision info starts with "revision-id: " so strip that.
	revLine := strings.TrimPrefix(scanner.Text(), "revision-id: ")
	return fmt.Sprintf("%.100s", revLine), nil
}
