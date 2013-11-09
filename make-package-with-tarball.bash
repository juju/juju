#!/bin/bash
#
# Create source and binary packages using a source package branch and
# a release tarball.

bzr branch ubuntu-juju-core-packaging/ test-packaging
cd test-packaging/
bzr import-upstream 1.16.3 ../juju-core_1.16.3.tar.gz
bzr merge . -r upstream-1.16.3
gedit debian/changelog
bzr ci --fixes=lp:1247299 -m "New upstream point release (LP: #1247299)."
bzr tag 1.16.3-0ubuntu1
bzr bd -S
bzr bd
cd ../work-area
dput ppa:juju-packaging/stable juju-core_1.16.3-0ubuntu1_source.changes

changelog_entry=<<ENDENTRY
juju-core (${VERSION}-0ubuntu1) ${STABLE_SERIES}; urgency=low

  * New upstream ${DEVEL_STABLE_POINT} release.
    (${BUG_LIST).

 -- ${DEBFULNAME} <{$DEBEMAIL}>  $(date -R)

ENDENTRY
