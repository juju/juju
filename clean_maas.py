#!/usr/bin/env python
"""Delete all machines from given MAAS environment."""

from __future__ import print_function

import argparse
import sys

import jujuconfig
import substrate


def main(argv):
    parser = argparse.ArgumentParser(description="Delete all machines in MAAS")
    parser.add_argument("name", help="Name of the MAAS in juju config.")
    args = parser.parse_args(argv[1:])
    environments = jujuconfig.get_environments()
    if args.name not in environments:
        parser.error("No maas '{}' found in juju config".format(args.name))
    config = environments[args.name]
    with substrate.maas_account_from_config(config) as manager:
        machines = manager.get_allocated_nodes()
        print("Found {} machines: {}".format(len(machines), machines.keys()))
        manager.terminate_instances(machine["resource_uri"]
                                    for machine in machines.values())
        print("Released.")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
