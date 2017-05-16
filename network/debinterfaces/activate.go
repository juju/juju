// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debinterfaces

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"github.com/pkg/errors"
)

var logger = loggo.GetLogger("juju.network.debinterfaces")

// ActivationParams contains options to use when bridging interfaces
type ActivationParams struct {
	Clock clock.Clock
	// map deviceName -> bridgeName
	Devices          map[string]string
	DryRun           bool
	Filename         string
	ReconfigureDelay int
	Timeout          time.Duration
}

// ActivationResult captures the result of actively bridging the
// interfaces using ifup/ifdown.
type ActivationResult struct {
	Stdout []byte
	Stderr []byte
	Code   int
}

func activationCmd(oldContent, newContent string, params *ActivationParams) string {
	if params.ReconfigureDelay < 0 {
		params.ReconfigureDelay = 0
	}
	deviceNames := make([]string, len(params.Devices))
	i := 0
	for k := range params.Devices {
		deviceNames[i] = k
		i++
	}
	sort.Strings(deviceNames)
	// The magic value of 25694 here causes the script to sleep for 30 seconds, simulating timeout
	// The value of 25695 causes the script to fail.
	return fmt.Sprintf(`
#!/bin/bash

set -eu

: ${DRYRUN:=}

if [ $DRYRUN ]; then
  if [ %[4]d == 25694 ]; then sleep 30; fi
  if [ %[4]d == 25695 ]; then echo "artificial failure" >&2; exit 1; fi
 fi

write_backup() {
    cat << 'EOF' > "$1"
%[5]s
EOF
}

write_content() {
    cat << 'EOF' > "$1"
%[6]s
EOF
}

if [ -n %[2]q ]; then
    ${DRYRUN} write_backup %[2]q
fi
${DRYRUN} write_content %[3]q
${DRYRUN} ifdown --interfaces=%[1]q %[7]s
${DRYRUN} sleep %[4]d
${DRYRUN} ifup --interfaces=%[3]q -a
${DRYRUN} mv %[3]q %[1]q
`,
		params.Filename,
		params.Filename+".backup",
		params.Filename+".new",
		params.ReconfigureDelay,
		oldContent,
		newContent,
		strings.Join(deviceNames, " "))[1:]
}

// BridgeAndActivate will parse a debian-styled interfaces(5) file,
// change the stanza definitions of the requested devices to be
// bridged, then reconfigure the network using the ifupdown package
// for the new bridges.
func BridgeAndActivate(params ActivationParams) (*ActivationResult, error) {
	if len(params.Devices) == 0 {
		return nil, errors.Errorf("no devices specified")
	}

	stanzas, err := Parse(params.Filename)

	if err != nil {
		return nil, err
	}

	origContent := FormatStanzas(FlattenStanzas(stanzas), 4)
	bridgedStanzas := Bridge(stanzas, params.Devices)
	bridgedContent := FormatStanzas(FlattenStanzas(bridgedStanzas), 4)

	if origContent == bridgedContent {
		return nil, nil // nothing to do; old == new.
	}

	cmd := activationCmd(origContent, bridgedContent, &params)

	environ := os.Environ()
	if params.DryRun {
		environ = append(environ, "DRYRUN=echo")
	}
	result, err := runCommand(cmd, environ, params.Clock, params.Timeout)

	activationResult := ActivationResult{
		Stderr: result.Stderr,
		Stdout: result.Stdout,
		Code:   result.Code,
	}

	if err != nil {
		return &activationResult, errors.Errorf("bridge activation error: %s", err)
	}

	logger.Infof("bridge activation result=%v", result.Code)

	if result.Code != 0 {
		logger.Errorf("bridge activation stdout\n%s\n", result.Stdout)
		logger.Errorf("bridge activation stderr\n%s\n", result.Stderr)
		return &activationResult, errors.Errorf("bridge activation failed: %s", string(result.Stderr))
	}

	logger.Tracef("bridge activation stdout\n%s\n", result.Stdout)
	logger.Tracef("bridge activation stderr\n%s\n", result.Stderr)

	return &activationResult, nil
}
