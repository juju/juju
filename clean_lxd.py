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


def list_old_juju_containers(hours):
    env = dict(os.environ)
    containers = json.loads(subprocess.check_output([
        'lxc', 'list', '--format', 'json'], env=env))
    now = datetime.now()
    for container in containers:
        name = container['name']
        if not name.startswith('juju-'):
            continue
        # This produces local time.  lxc does not respect TZ=UTC.
        created_at = datetime.strptime(
            container['created_at'][:-6], '%Y-%m-%dT%H:%M:%S')
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
