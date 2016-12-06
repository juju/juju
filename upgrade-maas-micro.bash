#!/bin/bash
# This script upgrades the installed MAAS only when the major and minor
# version match. The MAAS archives switch the major/minor versions they
# have. An unattended upgrade could switch the version under test and
# invalidate tests.

set -eu
USER_AT_ADDRESS=$1


upgrade_maas() {
    set -eu
    POLICY=$(apt-cache policy maas | egrep '(Installed|Candidate)')
    INSTALLED=$(echo "$POLICY" | grep Installed | cut -d ' ' -f 4)
    CANDIDATE=$(echo "$POLICY" | grep Candidate | cut -d ' ' -f 4)
    INSTALLED_MINOR=$(echo "$INSTALLED" | cut -d '.' -f 1,2)
    CANDIDATE_MINOR=$(echo "$CANDIDATE" | cut -d '.' -f 1,2)

    echo "INSTALLED: $INSTALLED"
    echo "CANDIDATE: $CANDIDATE"
    echo "INSTALLED_MINOR: $INSTALLED_MINOR"
    echo "CANDIDATE_MINOR: $CANDIDATE_MINOR"

    if [[ "$INSTALLED_MINOR" != "$CANDIDATE_MINOR" ]]; then
        echo "Refusing to upgrade from $INSTALLED_MINOR to $CANDIDATE_MINOR"
        exit 0
    fi

    if [[ "$INSTALLED" != "$CANDIDATE" ]]; then
        echo "Upgrading from $INSTALLED_MINOR to $CANDIDATE_MINOR"
        sudo apt-get install -y maas=$CANDIDATE
    else
        echo "No upgrade available."
    fi
}


ssh $USER_AT_ADDRESS "$(typeset -f upgrade_maas); upgrade_maas"
