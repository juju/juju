#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
import glob
import os
import shutil
import subprocess


# These are the archives that are search for matching releases.
UBUNTU_ARCH="http://archive.ubuntu.com/ubuntu/pool/universe/j/juju-core/"
ARM_ARCH="http://ports.ubuntu.com/pool/universe/j/juju-core/"
PUBLIC_ARCHIVES=[UBUNTU_ARCH, ARM_ARCH]


def retrieve_packages(release, upatch, archives, dest_debs, juju_dir,
                      script_dir):
    # Retrieve the $RELEASE packages that contain jujud,
    # or copy a locally built package.
    print("Retrieving juju-core packages from archives")
    print(subprocess.check_output(["date", "+%Y-%m-%dT%H:%M:%S"]))
    os.chdir(dest_debs)
    for archive in archives:
        proc = subprocess.Popen(
            ['sed', '-e', 's,//.*@,//,'], stdin=subprocess.PIPE,
            stdout=subprocess.PIPE)
        stdout, stderrdata = proc.communicate(archive)
        safe_archive = stdout.rstrip('\n')
        print("checking {} for {}".format(safe_archive, release))
        subprocess.check_call([
            'lftp', '-c', 'mirror', '-I',
            "juju-core_{}*.{}~juj*.deb".format(release, upatch),
            archive])
    if os.path.isdir(os.path.join(dest_debs, 'juju-core')):
        found = subprocess.check_output([
            'find', os.path.join(dest_debs, 'juju-core/'), '-name',
            "*deb"]).rstrip('\n')
        if found != "":
            debs = glob.glob(os.path.join(dest_debs, 'juju-core', '*deb'))
            for deb in debs:
                shutil.move(deb, './')
        shutil.rmtree(os.path.join(dest_debs, 'juju-core'))
    if os.path.exists(os.path.join(juju_dir, 'juju-qa.s3cfg')):
        print(
            'checking s3://juju-qa-data/agent-archive for'
            ' {}.'.format(release))
        subprocess.check_call([
            os.path.join(script_dir, 'agent_archive.py'), '--config',
            os.path.join(juju_dir, 'juju-qa.s3cfg'), 'get', release,
               dest_debs])


def parse_args():
    parser = ArgumentParser()
    parser.add_argument('release', help='The juju release to prepare')
    parser.add_argument('destination', help='The simplestreams destination')
    parser.add_argument('--upatch', help='Ubuntu patchlevel', default='1')
    return parser.parse_args()


def list_ppas(juju_home):
    config = os.path.join(juju_home, 'buildarchrc')
    if not os.path.exists(config):
        return None
    listing = subprocess.check_output(
        ['/bin/bash', '-c', 'source {}; echo '
         '"$BUILD_STABLE_ARCH\n'
         '$BUILD_DEVEL_ARCH\n'
         '$BUILD_SUPPORTED_ARCH"'.format(config)])
    return listing.splitlines()


def main():
    args = parse_args()
    dest_debs=os.path.join(args.destination, 'debs')
    juju_dir = os.environ.get(
        'JUJU_HOME', os.path.join(os.environ.get('HOME'), '.juju'))
    archives = list_ppas(juju_dir)
    if archives is None:
        print("Only public archives will be searched.")
        archives = PUBLIC_ARCHIVES
    else:
        print("Searching the build archives.")
    archives.extend(list_ppas(juju_dir))
    script_dir = os.path.dirname(__file__)
    retrieve_packages(args.release, args.upatch, archives, dest_debs,
                      juju_dir, script_dir)


if __name__ == '__main__':
    main()
