#!/usr/bin/env python
from argparse import ArgumentParser
import yaml

from deploy_stack import BootstrapManager
from fakejuju import (
    FakeBackend,
    FakeControllerState,
    )
from jujuconfig import get_juju_home
from jujupy import (
    EnvJujuClient,
    JujuData,
    get_client_class,
    WaitMachineNotPresent,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    )


def client_from_args(args):
    if args.juju_bin == 'FAKE':
        client_class = EnvJujuClient
        controller_state = FakeControllerState()
        version = '2.0.0'
        backend = FakeBackend(controller_state, full_path=args.juju_bin,
                              version=version)
    else:
        version = EnvJujuClient.get_version(args.juju_bin)
        client_class = get_client_class(version)
        backend = None
    juju_home = get_juju_home()
    with open(args.example_clouds) as f:
        clouds = yaml.safe_load(f)
    juju_data = client_class.config_class.from_cloud_region(
        args.cloud, args.region, {}, clouds, juju_home)
    return client_class(juju_data, version, args.juju_bin, debug=args.debug,
                        soft_deadline=args.deadline, _backend=backend)


def assess_cloud_combined(bs_manager):
    client = bs_manager.client
    with bs_manager.booted_context(upload_tools=False):
        old_status = client.get_status()
        client.juju('add-machine', ())
        new_status = client.wait_for_started()
        new_machines = [k for k, v in new_status.iter_new_machines(old_status)]
        client.juju('remove-machine', tuple(new_machines))
        new_status = client.wait([WaitMachineNotPresent(n)
                                  for n in new_machines])


def main():
    parser = ArgumentParser()
    parser.add_argument('example_clouds',
                        help='A clouds.yaml file to use for testing.')
    parser.add_argument('cloud', help='Specific cloud to test.')
    add_basic_testing_arguments(parser, env=False)
    args = parser.parse_args()
    configure_logging(args.verbose)
    client = client_from_args(args)
    bs_manager = BootstrapManager.from_client(args, client)
    assess_cloud_combined(bs_manager)


if __name__ == '__main__':
    main()
