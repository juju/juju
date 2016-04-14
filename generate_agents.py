#!/usr/bin/env python
from __future__ import print_function

from argparse import (
    ArgumentParser,
    Namespace,
    )
from datetime import datetime
import errno
import glob
import os
import re
import shutil
import subprocess
import sys
import tarfile
from urlparse import (
    urlsplit,
    urlunsplit,
    )

from debian import deb822

from agent_archive import get_agents
from build_package import juju_series
from make_agent_json import StanzaWriter
from utils import temp_dir


class NoDebsFound(Exception):
    """Raised when no .deb files could be found."""


# These are the archives that are searched for matching releases.
UBUNTU_ARCH = "http://archive.ubuntu.com/ubuntu/pool/universe/j/juju-core/"
ARM_ARCH = "http://ports.ubuntu.com/pool/universe/j/juju-core/"
PUBLIC_ARCHIVES = [UBUNTU_ARCH, ARM_ARCH]


def move_debs(dest_debs):
    juju_core_dir = os.path.join(dest_debs, 'juju-2.0')
    debs = glob.glob(os.path.join(juju_core_dir, '*deb'))
    if len(debs) == 0:
        # The juju-2.0 package was not found, try the juju-core package.
        print('No debs in {}'.format(juju_core_dir))
        juju_core_dir = os.path.join(dest_debs, 'juju-core')
        debs = glob.glob(os.path.join(juju_core_dir, '*deb'))
    if len(debs) == 0:
        print('No debs in {}'.format(juju_core_dir))
        raise NoDebsFound('No deb files found.')
    for deb in debs:
        shutil.move(deb, dest_debs)
    shutil.rmtree(juju_core_dir)


def retrieve_packages(release, upatch, archives, dest_debs, s3_config):
    # Retrieve the packages that contain a jujud for this version.
    print("Retrieving juju-core packages from archives")
    print(datetime.now().replace(microsecond=0).isoformat())
    for archive in archives:
        scheme, netloc, path, query, fragment = urlsplit(archive)
        # Strip username / password
        netloc = netloc.rsplit('@')[-1]
        safe_archive = urlunsplit((scheme, netloc, path, query, fragment))
        print("checking {} for {}".format(safe_archive, release))
        subprocess.call([
            'lftp', '-c', 'mirror', '-i',
            "(juju-2.0|juju-core).*{}.*\.{}~juj.*\.deb$".format(
                release, upatch),
            archive], cwd=dest_debs)
    move_debs(dest_debs)
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
    parser.add_argument('agent_stream', help='The juju agent-stream.')
    parser.add_argument('destination', help='The simplestreams destination')
    parser.add_argument('--upatch', help='Ubuntu patchlevel', default='1')
    return parser.parse_args()


def list_ppas(juju_home):
    config = os.path.join(juju_home, 'buildarchrc')
    if not os.path.exists(config):
        return None
    listing = subprocess.check_output(
        ['/bin/bash', '-c', 'source {}; set -u; echo '
         '"$BUILD_STABLE2_ARCH\n'
         '$BUILD_DEVEL2_ARCH\n'
         '$BUILD_STABLE1_ARCH\n'
         '$BUILD_DEVEL1_ARCH\n'
         '$BUILD_SUPPORTED_ARCH"'.format(config)])
    return listing.splitlines()


def deb_to_agent(deb_path, dest_dir, agent_stream):
    control_str = subprocess.check_output(['dpkg-deb', '-I', deb_path,
                                           'control'])
    control = deb822.Deb822(control_str)
    control_version = control['Version']
    base_version = re.sub('-0ubuntu.*$', '', control_version)
    series = juju_series.get_name_from_package_version(control_version)
    architecture = control['Architecture']
    with temp_dir() as work_dir:
        contents = os.path.join(work_dir, 'contents')
        os.mkdir(contents)
        subprocess.check_call(['dpkg-deb', '-x', deb_path, contents])
        jujud_path = os.path.join(
            contents, 'usr', 'lib', 'juju-{}'.format(base_version), 'bin',
            'jujud')
        basename = 'juju-{}-{}-{}.tgz'.format(base_version, series,
                                              architecture)
        agent_filename = os.path.join(work_dir, basename)
        with tarfile.open(agent_filename, 'w:gz') as tf:
            tf.add(jujud_path, 'jujud')
        writer = StanzaWriter.for_ubuntu(
            juju_series.get_version(series), series, architecture,
            base_version, agent_filename, agent_stream=agent_stream)
        writer.write_stanzas()
        shutil.move(writer.filename, dest_dir)
        final_agent_path = os.path.join(dest_dir, writer.path)
        move_create_parent(agent_filename, final_agent_path)


def debs_to_agents(dest_debs, agent_stream):
    for deb_path in glob.glob(os.path.join(dest_debs, '*.deb')):
        deb_to_agent(deb_path, dest_debs, agent_stream)


def move_create_parent(source, target):
    try:
        os.makedirs(os.path.dirname(target))
    except OSError as e:
        if e.errno != errno.EEXIST:
            raise
    shutil.move(source, target)


def make_windows_agent(dest_debs, agent_stream, release):
    source = os.path.join(
        dest_debs, 'juju-{}-win2012-amd64.tgz'.format(release))
    target = os.path.join(
        dest_debs, 'juju-{}-windows-amd64.tgz'.format(release))
    shutil.copy2(source, target)
    writer = StanzaWriter.for_windows(
        release, target, agent_stream=agent_stream)
    writer.write_stanzas()
    agent_path = os.path.join(dest_debs, writer.path)
    move_create_parent(target, agent_path)
    os.rename(writer.filename, os.path.join(dest_debs, writer.filename))


def make_centos_agent(dest_debs, agent_stream, release):
    tarfile = os.path.join(
        dest_debs, 'juju-{}-centos7-amd64.tgz'.format(release))
    writer = StanzaWriter.for_centos(release, tarfile,
                                     agent_stream=agent_stream)
    writer.write_stanzas()
    agent_path = os.path.join(dest_debs, writer.path)
    shutil.copy2(tarfile, agent_path)
    os.rename(writer.filename, os.path.join(dest_debs, writer.filename))


def main():
    args = parse_args()
    dest_debs = os.path.abspath(os.path.join(args.destination, 'debs'))
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
    debs_to_agents(dest_debs, args.agent_stream)
    make_windows_agent(dest_debs, args.agent_stream, args.release)
    make_centos_agent(dest_debs, args.agent_stream, args.release)


if __name__ == '__main__':
    main()
