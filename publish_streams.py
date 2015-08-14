#!/usr/bin/python

from __future__ import print_function

__metaclass__ = type

from argparse import ArgumentParser
import difflib
import os
import sys
import traceback
import urllib2


class CPCS:
    AWS = "http://juju-dist.s3.amazonaws.com"
    AZURE = "https://jujutools.blob.core.windows.net/juju-tools"
    CANONISTACK = (
        "https://swift.canonistack.canonical.com"
        "/v1/AUTH_526ad877f3e3464589dc1145dfeaac60/juju-dist")
    HP = (
        "https://region-a.geo-1.objects.hpcloudsvc.com"
        "/v1/60502529753910/juju-dist")
    JOYENT = (
        "https://us-east.manta.joyent.com/cpcjoyentsupport/public/juju-dist")

    @classmethod
    def get(cls, name):
        proper_name = name.upper()
        if proper_name in cls.__dict__:
            return cls.__dict__[proper_name]
        raise ValueError('{} is not a registered CPC'.format(proper_name))


def get_remote_file(url):
    response = urllib2.urlopen(url)
    content = response.read()
    return content


def diff_files(local, remote):
    with open(local, 'r') as f:
        local_lines = f.read().splitlines()
    remote_lines = get_remote_file(remote).splitlines()
    diff_gen = difflib.unified_diff(local_lines, remote_lines, local, remote)
    diff = '\n'.join([l for l in diff_gen])
    if diff:
        return False, diff
    return True, None


def verify_metadata(location, remote_stream, verbose=False):
    local_metadata = os.path.join(location, 'tools', 'streams', 'v1')
    remote_metadata = '{}/tools/streams/v1'.format(remote_stream)
    if verbose:
        print('comparing {} to {}'.format(local_metadata, remote_metadata))
    for data_file in os.listdir(local_metadata):
        if data_file.endswith('.json'):
            local_file = os.path.join(local_metadata, data_file)
            remote_file = '{}/{}'.format(remote_metadata, data_file)
            if verbose:
                print('comparing {}'.format(data_file))
            identical, diff = diff_files(local_file, remote_file)
            if not identical:
                return False, diff
    if verbose:
        print('All json matches')
    return True, None


def publish(stream, location, cloud,
            remote_root=None, dry_run=False, verbose=False):
    if remote_root:
        remote_stream = '{}/{}'.format(CPCS.get(cloud), remote_root)
    else:
        remote_stream = CPCS.get(cloud)
    verify_metadata(location, remote_stream, verbose=verbose)


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("Publish streams to a cloud.")
    parser.add_argument(
        '-d', '--dry-run', action="store_true", default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action="store_true", default=False,
        help='Increase verbosity.')
    parser.add_argument(
        '-r', '--remote-root', default=None,
        help='An alternate root to publish to such as testing or weekly.')
    parser.add_argument(
        'stream', help='The agent-stream to publish.',
        choices=['released', 'proposed', 'devel'])
    parser.add_argument(
        'location', type=os.path.expanduser,
        help='The path to the local tree of all streams (tools/).')
    parser.add_argument(
        'cloud', help='The destination cloud.',
        choices=['streams', 'aws', 'azure', 'hp', 'joyent', 'canonistack'])
    return parser.parse_args(args)


def main(argv):
    """Manage agent-archive in the archive."""
    args = parse_args(argv)
    try:
        publish(
            args.stream, args.location, args.cloud,
            remote_root=args.remote_root, dry_run=args.dry_run,
            verbose=args.verbose)
    except Exception as e:
        print(e)
        if args.verbose:
            traceback.print_tb(sys.exc_info()[2])
        return 2
    if args.verbose:
        print("Done.")
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
