#!/bin/bash

usage() {
    echo "usage: $0 dcs-file 'name-email'"
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
    for release in $RELEASES; do
        DEBEMAIL=$DEBEMAIL \
            backportpackage -u $PPA -r -d $release -S $SUFFIX -y $DSC
    done
}


PPA="ppa:juju-packaging/devel "
RELEASES="precise quantal raring saucy"
SUFFIX="~juju1"

test $# -ge 3 || usage

DSC=$1
DEBEMAIL=$2

check_deps
backport_packages


