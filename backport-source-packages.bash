#!/bin/bash

usage() {
    echo "usage: $0 <PURPOSE> dcs-file 'name-email'"
    echo "  PURPOSE: stable or devel which selects the archive."
    echo "  dsc-file: The path to source package DSC file."
    echo "  name-email: The 'name <email>' string used in the changelog."
    exit 1
}


check_deps() {
    echo "Phase 0: Checking requirements."
    has_deps=1
    which backportpackage || has_deps=0
    if [[ $has_deps == 0 ]]; then
        echo "Install backportpackage"
        exit 2
    fi
}


backport_packages() {
    echo "Phase 1: Backporting to $RELEASES."
    for release in $RELEASES; do
        DEBEMAIL=$DEBEMAIL \
            backportpackage -u $PPA -r -d $release -S $SUFFIX -y $DSC
    done
}


RELEASES="precise quantal"
SUFFIX="~juju1"

test $# -ne 3 && usage

PURPOSE=$1
if [[ $PURPOSE == "stable" ]]; then
    PPA="ppa:juju-packaging/stable"
elif [[ $PURPOSE == "devel" ]]; then
    PPA="ppa:juju-packaging/devel"
else
    usage
fi

DSC=$2
DEBEMAIL=$3

check_deps
backport_packages


