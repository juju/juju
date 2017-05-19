#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import datetime
import json
import os
import sys
import traceback


INDEX = {
    "index": {
        "com.ubuntu.juju:released:tools": {
            "updated": None,
            "format": "products:1.0",
            "datatype": "content-download",
            "path": "streams/v1/com.ubuntu.juju:released:tools.json",
            "products": [
                "com.ubuntu.juju:12.04:amd64",
                "com.ubuntu.juju:12.04:armhf",
                "com.ubuntu.juju:12.04:i386",
                "com.ubuntu.juju:12.10:amd64",
                "com.ubuntu.juju:12.10:i386",
                "com.ubuntu.juju:13.04:amd64",
                "com.ubuntu.juju:13.04:i386",
                "com.ubuntu.juju:13.10:amd64",
                "com.ubuntu.juju:13.10:armhf",
                "com.ubuntu.juju:13.10:i386",
                "com.ubuntu.juju:14.04:amd64",
                "com.ubuntu.juju:14.04:arm64",
                "com.ubuntu.juju:14.04:armhf",
                "com.ubuntu.juju:14.04:i386",
                "com.ubuntu.juju:14.04:powerpc",
                "com.ubuntu.juju:14.04:ppc64",
                "com.ubuntu.juju:14.04:ppc64el",
                "com.ubuntu.juju:14.10:amd64",
                "com.ubuntu.juju:14.10:arm64",
                "com.ubuntu.juju:14.10:armhf",
                "com.ubuntu.juju:14.10:i386",
                "com.ubuntu.juju:14.10:ppc64",
                "com.ubuntu.juju:14.10:ppc64el",
                "com.ubuntu.juju:15.04:amd64",
                "com.ubuntu.juju:15.04:arm64",
                "com.ubuntu.juju:15.04:armhf",
                "com.ubuntu.juju:15.04:i386",
                "com.ubuntu.juju:15.04:ppc64",
                "com.ubuntu.juju:15.04:ppc64el",
            ]
        }
    },
    "updated": None,
    "format": "index:1.0"
}


def generate_index_file(updated, streams_path,
                        verbose=False, dry_run=False):
    """Generate the historic index for old juju."""
    if verbose:
        print('Creating index.json')
    updated = updated.strftime('%a, %d %b %Y %H:%M:%S -0000')
    index = dict(INDEX)
    index['updated'] = updated
    index['index']['com.ubuntu.juju:released:tools']['updated'] = updated
    data = json.dumps(index, indent=4)
    file_path = '%s/index.json' % streams_path
    if not dry_run:
        with open(file_path, 'w') as index_file:
            index_file.write(data)


def parse_args(args=None):
    """Return the parsed args for this program."""
    parser = ArgumentParser("Generate an index for historic juju.")
    parser.add_argument(
        '-d', '--dry-run', action="store_true", default=False,
        help='Do not overwrite existing data.')
    parser.add_argument(
        '-v', '--verbose', action="store_true", default=False,
        help='Increse verbosity.')
    parser.add_argument(
        'streams_path',
        help="The streams base directory to create the files in. eg ./tools")
    return parser.parse_args(args)


def main(argv):
    """Generate an index.json file for historic juju."""
    args = parse_args(argv)
    try:
        streams_path = os.path.join(args.streams_path, 'streams', 'v1')
        updated = datetime.datetime.utcnow()
        generate_index_file(
            updated, streams_path, verbose=args.verbose, dry_run=args.dry_run)
    except Exception as e:
        print(e)
        if args.verbose:
            traceback.print_tb(sys.exc_info()[2])
        return 2
    if args.verbose:
        print("Created index json.")
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
