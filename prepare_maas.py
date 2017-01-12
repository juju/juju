from __future__ import print_function

from argparse import ArgumentParser
from datetime import (
    datetime,
    timedelta,
    )
import json
import subprocess
import sys

from dateutil import (
    parser as date_parser,
    )


ACQUIRING = 'User acquiring node'
NODE = 'node'
CREATED = 'created'
HOSTNAME = 'hostname'
SYSTEM_ID = 'system_id'


def get_acquire_dates(profile):
    events = json.loads(subprocess.check_output(
        ['maas', profile, 'events', 'query']))
    acquire_dates = {}
    for event in events['events']:
        if event['type'] == ACQUIRING:
            date = date_parser.parse(event[CREATED])
            acquire_dates.setdefault(event[NODE], date)
    return acquire_dates


def list_juju_nodes(profile):
    allocated_machines = json.loads(subprocess.check_output(
        ['maas', profile, 'machines', 'list-allocated']))
    return [(m[SYSTEM_ID], m) for m in allocated_machines
            if m[HOSTNAME].startswith('juju-')]


def main():
    parser = ArgumentParser()
    parser.add_argument('profile')
    args = parser.parse_args()
    profile = args.profile
    acquire_dates = get_acquire_dates(profile)
    now = datetime.now()
    for node, node_info in list_juju_nodes(profile):
        age = now - acquire_dates[node]
        if age < timedelta(hours=2):
            continue
        print('Deleting {} ({})'.format(node_info[HOSTNAME], age))
        subprocess.check_call(['maas', profile, 'machine', 'release', node])


if __name__ == '__main__':
    sys.exit(main())
