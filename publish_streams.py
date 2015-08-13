#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import os
import sys
import traceback
import urllib2


AWS = "http://juju-dist.s3.amazonaws.com"
AZURE = "https://jujutools.blob.core.windows.net/juju-tools"
CANONISTACK = (
    "https://swift.canonistack.canonical.com"
    "/v1/AUTH_526ad877f3e3464589dc1145dfeaac60/juju-dist")
HP = (
    "https://region-a.geo-1.objects.hpcloudsvc.com"
    "/v1/60502529753910/juju-dist")
JOYENT = "https://us-east.manta.joyent.com/cpcjoyentsupport/public/juju-dist"


def get_remote_file(url):
    response = urllib2.urlopen(url)
    content = response.read()
    return content


def verify(stream, location, cloud, remote_root=None):
    return True, None


def publish(stream, location, cloud,
            remote_root=None, dry_run=False, verbose=False):
    pass


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
