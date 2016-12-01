#!/usr/bin/env python
from __future__ import print_function

from argparse import (
    ArgumentParser,
    Namespace,
    )
from datetime import datetime
import errno
import os
import shutil
import subprocess
import sys

from agent_archive import get_agents
from make_agent_json import StanzaWriter


def retrieve_packages(release, dest_debs, s3_config):
    # Retrieve the packages that contain a jujud for this version.
    print("Retrieving juju-core packages from archives")
    print(datetime.now().replace(microsecond=0).isoformat())
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
    return parser.parse_args()


def move_create_parent(source, target):
    try:
        os.makedirs(os.path.dirname(target))
    except OSError as e:
        if e.errno != errno.EEXIST:
            raise
    shutil.move(source, target)


def make_ubuntu_agent(dest_debs, agent_stream, release):
    for arch in ['amd64', 'arm64', 'ppc64el', 's390x']:
        tarfile = os.path.join(
            dest_debs, 'juju-{}-ubuntu-{}.tgz'.format(release, arch))
        writer = StanzaWriter.for_living_ubuntu(
            arch, release, tarfile, agent_stream=agent_stream)
        writer.write_stanzas()
        agent_path = os.path.join(dest_debs, writer.path)
        shutil.copy2(tarfile, agent_path)
        arch_name = '{}-{}'.format(arch, writer.filename)
        os.rename(writer.filename, os.path.join(dest_debs, arch_name))


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
    retrieve_packages(args.release, dest_debs, s3_config)
    make_ubuntu_agent(dest_debs, args.agent_stream, args.release)
    make_windows_agent(dest_debs, args.agent_stream, args.release)
    make_centos_agent(dest_debs, args.agent_stream, args.release)


if __name__ == '__main__':
    main()
