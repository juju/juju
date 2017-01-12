#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
from datetime import (
    datetime,
    timedelta,
    )
import json
import os
import subprocess
import sys

from dateutil import (
    parser as date_parser,
    tz,
    )


def list_old_juju_containers(hours):
    env = dict(os.environ)
    containers = json.loads(subprocess.check_output([
        'lxc', 'list', '--format', 'json'], env=env))
    now = datetime.now(tz.gettz('UTC'))
    for container in containers:
        name = container['name']
        if not name.startswith('juju-'):
            continue
        created_at = date_parser.parse(container['created_at'])
        age = now - created_at
        if age <= timedelta(hours=hours):
            continue
        yield name, age


def main():
    parser = ArgumentParser('Delete old juju containers')
    parser.add_argument('--dry-run', action='store_true',
                        help='Do not actually delete.')
    parser.add_argument('--hours', type=int, default=1,
                        help='Number of hours a juju container may exist.')
    args = parser.parse_args()
    for container, age in list_old_juju_containers(args.hours):
        print('deleting {} ({} old)'.format(container, age))
        if args.dry_run:
            continue
        subprocess.check_call(('lxc', 'delete', '--verbose', '--force',
                               container))


if __name__ == '__main__':
    sys.exit(main())
