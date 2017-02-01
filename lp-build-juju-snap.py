#! /usr/bin/python

import os
import time

from datetime import datetime
from launchpadlib.launchpad import Launchpad

# basic data
arches = ['amd64', 'arm64', 'ppc64el']
series = 'xenial'

# basic paths
home = os.getenv("HOME")
workdir = os.path.join(home, "juju-daily-snap")

# we need to store credentials once for cronned builds
cachedir = os.path.join(workdir, "cache")
creds = os.path.join(workdir, "credentials")

# log in
launchpad = Launchpad.login_with('Juju Snap Builds',
                                 'production', cachedir,
                                 credentials_file=creds,
                                 version='devel')

# get team data and ppa
jujuqa = launchpad.people['jujuisquality']

# get snap
juju_snap = launchpad.snaps.getByName(name='juju-edge',
                                      owner=jujuqa)

# get distro info
ubuntu = launchpad.distributions['ubuntu']
release = ubuntu.getSeries(name_or_version=series)

# print a stamp
stamp = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
print("Trying to trigger builds at: {}".format(stamp))

# loop over arches and trigger builds
mybuilds = []
for buildarch in arches:
    arch = release.getDistroArchSeries(archtag=buildarch)
    request = juju_snap.requestBuild(archive=release.main_archive,
                                     distro_arch_series=arch,
                                     pocket='Updates')
    buildid = str(request).rsplit('/', 1)[-1]
    mybuilds.append(buildid)
    print("Arch: {} is building under: {}".format(buildarch,
                                                  request))

# check the status each minute til all builds have finished
failures = []
while len(mybuilds):
    for build in mybuilds:
        response = juju_snap.getBuildSummariesForSnapBuildIds(
            snap_build_ids=[build])
        status = response[build]['status']
        if status == "FULLYBUILT":
            mybuilds.remove(build)
            continue
        elif status == "FAILEDTOBUILD":
            failures.append(build)
            mybuilds.remove(build)
            continue
        elif status == "CANCELLED":
            mybuilds.remove(build)
            continue
    time.sleep(60)

# if we had failures, raise them
if len(failures):
    for failure in failures:
        response = juju_snap.getBuildSummariesForSnapBuildIds(
            snap_build_ids=[failure])
        buildlog = response[build]['build_log_url']
        if buildlog != 'None':
            print(buildlog)
            arch = str(buildlog).split('_')[4]
        raise("juju snap {} build at {} failed for id: {} log: {}".format(
            arch, stamp, failure, buildlog))
print("Builds complete")
