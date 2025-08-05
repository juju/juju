// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debinterfaces

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/utils/scriptrunner"
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
	backupFilename := fmt.Sprintf("%s.backup-%d", params.Filename, params.Clock.Now().Unix())
	// The magic value of 25694 here causes the script to sleep for 30 seconds, simulating timeout
	// The value of 25695 causes the script to fail.
	return fmt.Sprintf(`
#!/bin/bash

set -eu

: ${DRYRUN:=}

if [ $DRYRUN ]; then
  if [ %[4]d == 25694 ]; then sleep 30; fi
  if [ %[4]d == 25695 ]; then echo "artificial failure" >&2; exit 1; fi
  if [ %[4]d == 25696 ]; then echo "a very very VERY long artificial failure that should cause the code to shorten it and direct user to logs" >&2; exit 1; fi
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
${DRYRUN} ifdown --interfaces=%[1]q %[7]s || ${DRYRUN} ifdown --interfaces=%[1]q %[7]s || (${DRYRUN} ifup --interfaces=%[1]q -a; exit 1)
${DRYRUN} sleep %[4]d
${DRYRUN} cp %[3]q %[1]q
# we want to have full control over what happens next
set +e
${DRYRUN} ifup --interfaces=%[1]q -a || ${DRYRUN} ifup --interfaces=%[1]q -a
RESULT=$?
if [ ${RESULT} != 0 ]; then
    echo "Bringing up bridged interfaces failed, see system logs and %[3]q" >&2
    ${DRYRUN} ifdown --interfaces=%[1]q %[7]s
    ${DRYRUN} cp %[2]q %[1]q
    ${DRYRUN} ifup --interfaces=%[1]q -a
    exit ${RESULT}
fi
`,
		params.Filename,
		backupFilename,
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
		return nil, errors.New("no devices specified")
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
	result, err := scriptrunner.RunCommand(cmd, environ, params.Clock, params.Timeout)
	if err != nil {
		return nil, errors.Annotatef(err, "activating bridge")
	}

	activationResult := ActivationResult{
		Stderr: result.Stderr,
		Stdout: result.Stdout,
		Code:   result.Code,
	}

	logger.Infof("bridge activation result=%v", result.Code)

	if result.Code != 0 {
		logger.Errorf("bridge activation stdout\n%s\n", result.Stdout)
		logger.Errorf("bridge activation stderr\n%s\n", result.Stderr)
		// We want to suppress long output from ifup, ifdown - it will be shown in status message!
		if len(result.Stderr) < 40 {
			return &activationResult, fmt.Errorf("bridge activation failed: %s", string(result.Stderr))
		} else {
			return &activationResult, errors.New("bridge activation failed, see logs for details")
		}
	}

	logger.Tracef("bridge activation stdout\n%s\n", result.Stdout)
	logger.Tracef("bridge activation stderr\n%s\n", result.Stderr)

	return &activationResult, nil
}
