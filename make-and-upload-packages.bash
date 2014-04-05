#!/bin/bash
#
# Create and upload source packages for the devel or stable PPA.
# This script assumes some knowledge of how
# make-package-with-tarball select source package branches
# and creates versions. Maybe the intelligence shouls be removed
# from the other script.

SCRIPT_DIR=$(cd $(dirname "${BASH_SOURCE[0]}") && pwd )


usage() {
    echo "usage: $0 <PURPOSE> tarball 'name-email' [bug-number ...]"
    echo "  PURPOSE: stable or devel"
    echo "  tarball: The path to the juju-core tarball."
    echo "  name-email: The 'name <email>' string used in the changelog."
    echo "  bug-number: Zero or more Lp bug numbers"
    exit 1
}


PPATCH="1"
while getopts ":pt:" o; do
    case "${o}" in
        p)
            PPATCH=${OPTARG}
            echo "Setting package patch to $PPATCH"
            ;;
        *)
            usage
            ;;
    esac
done
shift $((OPTIND - 1))

test $# -ge 3 || usage

PURPOSE=$1
if [[ $PURPOSE == "stable" ]]; then
    PPA="ppa:juju-packaging/stable"
elif [[ $PURPOSE == "devel" ]]; then
    PPA="ppa:juju-packaging/devel"
else
    usage
fi

TARBALL=$(readlink -f $2)
if [[ ! -f "$TARBALL" ]]; then
    echo "Tarball not found."
    usage
fi

DEBEMAIL=$3
if [[ ! $DEBEMAIL =~ ^[a-zA-Z].*\<.*@.*\>$ ]]; then
    usage
fi

shift; shift; shift
FIXED_BUGS=$@

summary="The source package can be uploaded:"
for series in "trusty" "saucy" "quantal" "precise"; do
    source $SCRIPT_DIR/make-package-with-tarball.bash \
        -p $PPATCH $series $TARBALL "$DEBEMAIL" $FIXED_BUGS
    summary="$summary\n  cd $TMP_DIR"
    summary="$summary\n  dput $PPA juju-core_${UBUNTU_VERSION}_source.changes"
done
echo -e "$summary"

