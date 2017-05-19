#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import json
import os
import sys
import traceback


STREAM_KEY_TEMPLATE = 'com.ubuntu.juju:{}:tools'


def copy(location, from_stream, to_stream, dry_run=False, verbose=False):
    with open(location, 'r') as index_file:
        index = json.load(index_file)
    from_key = STREAM_KEY_TEMPLATE.format(from_stream)
    to_key = STREAM_KEY_TEMPLATE.format(to_stream)
    if to_key in index['index'] and verbose:
        print('Redefining {} in index2.json'.print(to_key))
    stanza = dict(index['index'][from_key])
    index['index'][to_key] = stanza
    if verbose:
        product_path = stanza['path']
        print('copied {} with {} to {}'.format(
              from_stream, product_path, to_stream))
    if not dry_run:
        with open(location, 'w') as index_file:
            json.dump(index, index_file, indent=4)


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("Copy simple stream stanzas.")
    parser.add_argument(
        '-d', '--dry-run', action="store_true", default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action="store_true", default=False,
        help='Increase verbosity.')
    parser.add_argument(
        'location', type=os.path.expanduser,
        help='The path to the index2.json')
    parser.add_argument('from_stream', help='The agent-stream to copy.')
    parser.add_argument('to_stream', help='The agent-stream to create.')
    return parser.parse_args(args)


def main(argv=None):
    """Copy simple stream stanzas."""
    args = parse_args(argv)
    try:
        copy(
            args.location, args.from_stream, args.to_stream,
            dry_run=args.dry_run, verbose=args.verbose)
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
