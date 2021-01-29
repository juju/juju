#!/usr/bin/env python3
"""Validate endpoint bindings functionality on MAAS."""

from __future__ import print_function

import argparse
import contextlib
import logging
import os
import shutil
import sys
import yaml

from deploy_stack import (
    BootstrapManager,
)
from jujucharm import (
    Charm,
)
from substrate import (
    maas_account_from_boot_config,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    temp_dir,
)


log = logging.getLogger("assess_endpoint_bindings")


script_identifier = "endpoint-bindings"

# To avoid clashes with other tests these space names must be seperately
# registered in jujupy to populate constraints.
space_data = script_identifier + "-data"
space_public = script_identifier + "-public"


def _generate_vids(start=10):
    """
    Generate a series of vid values beginning with start.

    Ideally these values would be carefully chosen to not clash with existing
    vlans, but for now just hardcode.
    """
    for vid in range(start, 4096):
        yield vid


def _generate_cidrs(start=40, inc=10, block_pattern="10.0.{}.0/24"):
    """
    Generate a series of cidrs based on block_pattern beginning with start.

    Would be good not to hardcode but inspecting network for free ranges is
    also non-trivial.
    """
    for n in range(start, 255, inc):
        yield block_pattern.format(n)


def ensure_spaces(manager, required_spaces):
    """Return details for each given required_spaces creating spaces as needed.

    :param manager: MAAS account manager.
    :param required_spaces: List of space names that may need to be created.
    """
    existing_spaces = manager.spaces()
    log.info("Have spaces: %s", ", ".join(s["name"] for s in existing_spaces))
    spaces_map = dict((s["name"], s) for s in existing_spaces)
    spaces = []
    for space_name in required_spaces:
        space = spaces_map.get(space_name)
        if space is None:
            space = manager.create_space(space_name)
            log.info("Created space: %r", space)
        spaces.append(space)
    return spaces


@contextlib.contextmanager
def reconfigure_networking(manager, required_spaces):
    """Create new MAAS networking primitives to prepare for testing.

    :param manager: MAAS account manager.
    :param required_spaces: List of spaces to make with vlans and subnets.
    """
    new_subnets = []
    new_vlans = []
    fabrics = manager.fabrics()
    log.info("Have fabrics: %s", ", ".join(f["name"] for f in fabrics))
    new_fabric = manager.create_fabric(script_identifier)
    try:
        log.info("Created fabric: %r", new_fabric)

        spaces = ensure_spaces(manager, required_spaces)

        for vid, space_name in zip(_generate_vids(), required_spaces):
            name = space_name + "-vlan"
            new_vlans.append(manager.create_vlan(new_fabric["id"], vid, name))
            log.info("Created vlan: %r", new_vlans[-1])

        for cidr, vlan, space in zip(_generate_cidrs(), new_vlans, spaces):
            new_subnets.append(manager.create_subnet(
                cidr, fabric_id=new_fabric["id"], vlan_id=vlan["id"],
                space=space["id"], gateway_ip=cidr.replace(".0/24", ".1")))
            log.info("Created subnet: %r", new_subnets[-1])

        yield new_fabric, spaces, list(new_vlans), list(new_subnets)

    finally:
        for subnet in new_subnets:
            manager.delete_subnet(subnet["id"])
            log.info("Deleted subnet: %s", subnet["name"])

        for vlan in new_vlans:
            manager.delete_vlan(new_fabric["id"], vlan["vid"])
            log.info("Deleted vlan: %s", vlan["name"])

        try:
            manager.delete_fabric(new_fabric["id"])
        except Exception:
            log.exception("Failed to delete fabric: %s", new_fabric["id"])
        else:
            log.info("Deleted fabric: %s", new_fabric["id"])


@contextlib.contextmanager
def reconfigure_machines(manager, fabric, required_machine_subnets):
    """
    Reconfigure MAAS machines with new interfaces to prepare for testing.

    There are some unavoidable races if multiple jobs attempt to reconfigure
    machines at the same time. Also, in heterogenous environments an
    inadequate machine may be reserved at this time.

    Ideally this function would just allocate some machines before operating
    on them. Alas, MAAS doesn't allow interface changes on allocated machines
    and Juju will not select them for deployment.

    :param manager: MAAS account manager.
    :param fabric: Data from MAAS about the fabric to be used.
    :param required_machine_subnets: List of list of vlan and subnet ids.
    """

    # Find all machines not currently being used
    all_machines = manager.machines()
    candidate_machines = [
        m for m in all_machines if m["status"] == manager.STATUS_READY]
    # Take the id of the default vlan on the new fabric
    default_vlan = fabric["vlans"][0]["id"]

    configured_machines = []
    machine_interfaces = {}
    try:
        for machine_subnets in required_machine_subnets:
            if not candidate_machines:
                raise Exception("No ready maas machines to configure")

            machine = candidate_machines.pop()
            system_id = machine["system_id"]
            # TODO(gz): Add logic to pick sane parent?
            existing_interface = [
                interface for interface in machine["interface_set"]
                if not any("subnet" in link for link in interface["links"])
                ][0]
            previous_vlan_id = existing_interface["vlan"]["id"]
            new_interfaces = []
            machine_interfaces[system_id] = (
                existing_interface, previous_vlan_id, new_interfaces)
            manager.interface_update(
                system_id, existing_interface["id"], vlan_id=default_vlan)
            log.info("Changed existing interface: %s %s", system_id,
                     existing_interface["name"])
            parent = existing_interface["id"]

            for vlan_id, subnet_id in machine_subnets:
                links = []
                interface = manager.interface_create_vlan(
                    system_id, parent, vlan_id)
                new_interfaces.append(interface)
                log.info("Created interface: %r", interface)

                updated_subnet = manager.interface_link_subnet(
                    system_id, interface["id"], "AUTO", subnet_id)
                # TODO(gz): Need to pick out right link if multiple are added.
                links.append(updated_subnet["links"][0])
                log.info("Created link: %r", links[-1])

            configured_machines.append(machine)
        yield configured_machines
    finally:
        log.info("About to reset machine interfaces to original states.")
        for system_id in machine_interfaces:
            parent, vlan, children = machine_interfaces[system_id]
            for child in children:
                manager.delete_interface(system_id, child["id"])
                log.info("Deleted interface: %s %s", system_id, child["id"])
            manager.interface_update(system_id, parent["id"], vlan_id=vlan)
            log.info("Reset original interface: %s", parent["name"])


def create_test_charms():
    """Create charms for testing and bundle using them."""
    charm_datastore = Charm("datastore", "Testing datastore charm.")
    charm_datastore.metadata["provides"] = {
        "datastore": {"interface": "data"},
    }

    charm_frontend = Charm("frontend", "Testing frontend charm.")
    charm_frontend.metadata["extra-bindings"] = {
        "website": None,
    }
    charm_frontend.metadata["requires"] = {
        "datastore": {"interface": "data"},
    }

    bundle = {
        "machines": {
            "0": {
                "constraints": "spaces={},^{}".format(
                    space_data, space_public),
                "series": "bionic",
            },
            "1": {
                "constraints": "spaces={},{}".format(space_data, space_public),
                "series": "bionic",
            },
            "2": {
                "constraints": "spaces={},{}".format(space_data, space_public),
                "series": "bionic",
            },
        },
        "services": {
            "datastore": {
                "charm": "./bionic/datastore",
                "series": "bionic",
                "num_units": 1,
                "to": ["0"],
                "bindings": {
                    "datastore": space_data,
                },
            },
            "frontend": {
                "charm": "./bionic/frontend",
                "series": "bionic",
                "num_units": 1,
                "to": ["1"],
                "bindings": {
                    "website": space_public,
                    "datastore": space_data,
                },
            },
            "monitor": {
                "charm": "./bionic/datastore",
                "series": "bionic",
                "num_units": 1,
                "to": ["2"],
                "bindings": {
                    "": space_data,
                },
            },
        },
        "relations": [
            ["datastore:datastore", "frontend:datastore"],
        ],
    }
    return bundle, [charm_datastore, charm_frontend]


@contextlib.contextmanager
def using_bundle_and_charms(bundle, charms, bundle_name="bundle.yaml"):
    """Commit bundle and charms to disk and gives path to bundle."""
    with temp_dir() as working_dir:
        for charm in charms:
            charm.to_repo_dir(working_dir)

        # TODO(gz): Create a bundle abstration in jujucharm module
        bundle_path = os.path.join(working_dir, bundle_name)
        with open(bundle_path, "w") as f:
            yaml.safe_dump(bundle, f)

        yield bundle_path


def machine_spaces_for_bundle(bundle):
    """Return a list of sets of spaces required for machines in bundle."""
    machines = []
    for service in bundle["services"].values():
        spaces = frozenset(service.get("bindings", {}).values())
        num_units = service.get("num_units", 1)
        machines.extend([spaces] * num_units)
    return machines


def bootstrap_and_test(bootstrap_manager, bundle_path, machine):
    shutil.copy(bundle_path, bootstrap_manager.log_dir)
    with bootstrap_manager.booted_context(False, no_gui=True):
        client = bootstrap_manager.client
        log.info("Deploying bundle.")
        client.deploy(bundle_path)
        log.info("Waiting for all units to start.")
        client.wait_for_started()
        client.wait_for_workloads()
        log.info("Validating bindings.")
        validate(client)


def validate(client):
    """Ensure relations are bound to the correct spaces."""


def assess_endpoint_bindings(maas_manager, bootstrap_manager):
    required_spaces = [space_data, space_public]

    bundle, charms = create_test_charms()

    machine_spaces = machine_spaces_for_bundle(bundle)
    # Add a bootstrap machine in all spaces
    machine_spaces.insert(0, frozenset().union(*machine_spaces))

    log.info("About to write charms to disk.")
    with using_bundle_and_charms(bundle, charms) as bundle_path:
        log.info("About to reconfigure maas networking.")
        with reconfigure_networking(maas_manager, required_spaces) as nets:

            fabric, spaces, vlans, subnets = nets
            # Derive the vlans and subnets that need to be added to machines
            vlan_subnet_per_machine = []
            for spaces in machine_spaces:
                idxs = sorted(required_spaces.index(space) for space in spaces)
                vlans_subnets = [
                    (vlans[i]["id"], subnets[i]["id"]) for i in idxs]
                vlan_subnet_per_machine.append(vlans_subnets)

            log.info("About to add new interfaces to machines.")
            with reconfigure_machines(
                    maas_manager, fabric, vlan_subnet_per_machine) as machines:

                bootstrap_manager.client.use_reserved_spaces(required_spaces)

                base_machine = machines[0]["hostname"]

                log.info("About to bootstrap.")
                bootstrap_and_test(
                    bootstrap_manager, bundle_path, base_machine)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="assess endpoint bindings")
    add_basic_testing_arguments(parser, existing=False)
    args = parser.parse_args(argv)
    if args.upload_tools:
        parser.error("giving --upload-tools meaningless on 2.0 only test")
    return args


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with maas_account_from_boot_config(bs_manager.client.env) as account:
        assess_endpoint_bindings(account, bs_manager)
    return 0


if __name__ == '__main__':
    sys.exit(main())
