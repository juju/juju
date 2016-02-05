#!/usr/bin/env python
from __future__ import print_function

from argparse import (
    ArgumentParser,
    Namespace,
    )
from datetime import datetime
import glob
import os
import shutil
import subprocess
import sys
from urlparse import (
    urlsplit,
    urlunsplit,
    )

from agent_archive import get_agents


# These are the archives that are searched for matching releases.
UBUNTU_ARCH = "http://archive.ubuntu.com/ubuntu/pool/universe/j/juju-core/"
ARM_ARCH = "http://ports.ubuntu.com/pool/universe/j/juju-core/"
PUBLIC_ARCHIVES = [UBUNTU_ARCH, ARM_ARCH]


def retrieve_packages(release, upatch, archives, dest_debs, s3_config):
    # Retrieve the packages that contain a jujud for this version.
    print("Retrieving juju-core packages from archives")
    print(datetime.now().replace(microsecond=0).isoformat())
    os.chdir(dest_debs)
    for archive in archives:
        scheme, netloc, path, query, fragment = urlsplit(archive)
        # Strip username / password
        netloc = netloc.rsplit('@')[-1]
        safe_archive = urlunsplit((scheme, netloc, path, query, fragment))
        print("checking {} for {}".format(safe_archive, release))
        subprocess.check_call([
            'lftp', '-c', 'mirror', '-I',
            "juju-core_{}*.{}~juj*.deb".format(release, upatch),
            archive])
    juju_core_dir = os.path.join(dest_debs, 'juju-core')
    if os.path.isdir(juju_core_dir):
        debs = glob.glob(os.path.join(juju_core_dir, '*deb'))
        for deb in debs:
            shutil.move(deb, './')
        shutil.rmtree(juju_core_dir)
    if os.path.exists(s3_config):
        print(
            'checking s3://juju-qa-data/agent-archive for'
            ' {}.'.format(release))
        args = Namespace(
            version=release, destination=dest_debs, config=s3_config,
            dry_run=False, verbose=False)
        try:
            get_agents(args)
        except subprocess.CalledProcessError as e:
            print()
            sys.stderr.write(e.output)
            print("FAILED: get_agents() failed.")
            sys.exit(1)


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
    dest_debs = os.path.join(args.destination, 'debs')
    juju_dir = os.environ.get(
        'JUJU_HOME', os.path.join(os.environ.get('HOME'), '.juju'))
    s3_config = os.path.join(juju_dir, 'juju-qa.s3cfg')
    archives = list_ppas(juju_dir)
    if archives is None:
        print("Only public archives will be searched.")
        archives = PUBLIC_ARCHIVES
    else:
        print("Searching the build archives.")
    retrieve_packages(args.release, args.upatch, archives, dest_debs,
                      s3_config)


if __name__ == '__main__':
    main()
