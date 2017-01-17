#!/usr/bin/env python
"""Delete all machines from given MAAS environment."""

from __future__ import print_function

import argparse
from datetime import (
    datetime,
    timedelta,
    )
import sys

from jujupy import JujuData
import substrate


def main(argv):
    parser = argparse.ArgumentParser(
        description="Delete the machines in MAAS.")
    parser.add_argument("name", help="Name of the MAAS in juju config.")
    parser.add_argument('--hours', help='Minimum age in hours.', type=float)
    parser.add_argument('--dry-run', action='store_true',
                        help="Show what would be deleted, but don't delete.")
    args = parser.parse_args(argv[1:])
    boot_config = JujuData.from_config(args.name)
    with substrate.maas_account_from_boot_config(boot_config) as manager:
        machines = manager.get_allocated_nodes()
        if args.hours is not None:
            acquire_dates = manager.get_acquire_dates()
            threshold = datetime.now() - timedelta(hours=args.hours)
            machines = dict((k, v) for k, v in machines.items()
                            if acquire_dates[v['system_id']] < threshold)
        print("Found {} machines: {}".format(len(machines), machines.keys()))
        if not args.dry_run:
            manager.terminate_instances(machine["resource_uri"]
                                        for machine in machines.values())
        print("Released.")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
