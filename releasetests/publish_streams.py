#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import difflib
import os
import sys
import traceback
import urllib2


__metaclass__ = type


CPCS = {
    'aws': "http://juju-dist.s3.amazonaws.com",
    'azure': "https://jujutools.blob.core.windows.net/juju-tools",
    'canonistack': (
        "https://swift.canonistack.canonical.com"
        "/v1/AUTH_526ad877f3e3464589dc1145dfeaac60/juju-dist"),
    'joyent': (
        "https://us-east.manta.joyent.com/cpcjoyentsupport/public/juju-dist"),
    }


def get_remote_file(url):
    """Return the content of a remote file."""
    response = urllib2.urlopen(url)
    content = response.read()
    return content


def diff_files(local, remote):
    """Return the difference of a local and a remote file.

    :return: a tuple of identical (True, None) or different (False, str).
    """
    with open(local, 'r') as f:
        local_lines = f.read().splitlines()
    remote_lines = get_remote_file(remote).splitlines()
    diff_gen = difflib.unified_diff(local_lines, remote_lines, local, remote)
    diff = '\n'.join(list(diff_gen))
    if diff:
        return False, diff
    return True, None


def verify_metadata(location, remote_stream, verbose=False):
    """Verify all the streams metadata in a cloud matches the local metadata.


    This verifies all the streams, not a single stream, to ensure the cloud
    has exactly the same metadata as the local instance.
    """
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
    """Publish a stream to a cloud and verify it."""
    if remote_root:
        remote_stream = '{}/{}'.format(CPCS[cloud], remote_root)
    else:
        remote_stream = CPCS.get(cloud)
    verify_metadata(location, remote_stream, verbose=verbose)


def parse_args(argv=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("Publish streams to a cloud.")
    parser.add_argument(
        '-d', '--dry-run', action="store_true", help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action="store_true", help='Increase verbosity.')
    parser.add_argument(
        '-r', '--remote-root',
        help='An alternate root to publish to such as testing or weekly.')
    parser.add_argument(
        'stream', help='The agent-stream to publish.',
        choices=['released', 'proposed', 'devel'])
    parser.add_argument(
        'location', type=os.path.expanduser,
        help='The path to the local tree of all streams (tools/).')
    parser.add_argument(
        'cloud', help='The destination cloud.',
        choices=['streams', 'aws', 'azure', 'joyent', 'canonistack'])
    return parser.parse_args(argv)


def main(argv):
    """Publish streams to a cloud."""
    args = parse_args(argv)
    try:
        publish(
            args.stream, args.location, args.cloud,
            remote_root=args.remote_root, dry_run=args.dry_run,
            verbose=args.verbose)
    except Exception as e:
        print('{}: {}'.format(e.__class__.__name__, e))
        if args.verbose:
            traceback.print_tb(sys.exc_info()[2])
        return 2
    if args.verbose:
        print("Done.")
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
