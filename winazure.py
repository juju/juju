#!/usr/bin/python
# The azure lib checks the environment for two vars that can be sourced
# or defined before the command.
# export AZURE_MANAGEMENT_SUBSCRIPTIONID=ID
# export AZURE_MANAGEMENT_CERTFILE=path/to/azure.pem

from __future__ import print_function

from argparse import ArgumentParser
from datetime import (
    datetime,
    timedelta,
)
import fnmatch
import os
import sys
from time import sleep

from azure.servicemanagement import (
    AZURE_MANAGEMENT_CERTFILE,
    AZURE_MANAGEMENT_SUBSCRIPTIONID,
    ServiceManagementService,
)
from utility import until_timeout


OLD_MACHINE_AGE = 12
DATETIME_PATTERN = '%Y-%m-%dT%H:%M:%SZ'
SUCCEEDED = 'Succeeded'


def delete_unused_disks(sms, dry_run=False, verbose=False):
    for disk in sms.list_disks():
        unused = (not disk.attached_to or
                  disk.attached_to.hosted_service_name == '')
        if unused:
            if verbose:
                print("Deleting {}".format(disk.name))
            if not dry_run:
                sms.delete_disk(disk.name, delete_vhd=True)
        else:
            if verbose:
                print("Skipping {}".format(disk.name))


def list_services(sms, glob='*', verbose=False):
    services = []
    all_services = sms.list_hosted_services()
    for service in all_services:
        if not fnmatch.fnmatch(service.service_name, glob):
            continue
        if verbose:
            print(service.service_name)
        services.append(service)
    return services


def is_old_deployment(deployments, now, ago, verbose=False):
    for deployment in deployments:
        created = datetime.strptime(
            deployment.created_time, DATETIME_PATTERN)
        age = now - created
        if age > ago:
            hours_old = (age.seconds / 3600) + (age.days * 24)
            if verbose:
                print('{} is {} hours old:'.format(deployment.name, hours_old))
                print('  {}'.format(deployment.created_time))
            return True
    return False


def wait_for_success(sms, request, pause=3, verbose=False):
    for ignored in until_timeout(600):
        if verbose:
            print('.', end='')
            sys.stdout.flush()
        sleep(pause)
        op = sms.get_operation_status(request.request_id)
        if op.status == SUCCEEDED:
            break


def delete_service(sms, service, deployments,
                   pause=3, dry_run=False, verbose=False):
    for deployment in deployments:
        if verbose:
            print("Deleting deployment {}".format(deployment.name))
        if not dry_run:
            request = sms.delete_deployment(
                service.service_name, deployment.name)
            wait_for_success(sms, request, pause=pause, verbose=verbose)
    if verbose:
        print("Deleting service {}".format(service.service_name))
    if not dry_run:
        sms.delete_hosted_service(service.service_name)


def delete_services(sms, glob='*', old_age=OLD_MACHINE_AGE,
                    pause=3, dry_run=False, verbose=False):
    now = datetime.utcnow()
    ago = timedelta(hours=old_age)
    services = list_services(sms, glob=glob, verbose=False)
    for service in services:
        name = service.service_name
        properties = sms.get_hosted_service_properties(
            name, embed_detail=True)
        if not is_old_deployment(properties.deployments, now, ago,
                                 verbose=verbose):
            if len(properties.deployments) == 0 and verbose:
                print("{} does not have deployements".format(name))
            continue
        delete_service(
            sms, service, properties.deployments,
            pause=pause, dry_run=dry_run, verbose=verbose)


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser('Query and manage azure.')
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action="store_true", help='Increse verbosity.')
    parser.add_argument(
        "-c", "--cert-file", dest="certificate_path",
        help="The certificate path. Environment: $AZURE_MANAGEMENT_CERTFILE.",
        default=os.environ.get(AZURE_MANAGEMENT_CERTFILE))
    parser.add_argument(
        '-s', '--subscription-id', dest='subscription_id',
        help=("The subscription id to make requests with. "
              "Environment: $AZURE_MANAGEMENT_SUBSCRIPTIONID."),
        default=os.environ.get(AZURE_MANAGEMENT_SUBSCRIPTIONID))
    subparsers = parser.add_subparsers(help='sub-command help', dest="command")
    subparsers.add_parser(
        'delete-unused-disks',
        help='Delete unsed disks and files left behind by services.')
    ls_parser = subparsers.add_parser(
        'list-services', help='List running services.')
    ls_parser.add_argument(
        'filter', default='*', nargs='?',
        help='A glob pattern to match services to.')
    ds_parser = subparsers.add_parser(
        'delete-services',
        help='delete running services and their deployments.')
    ds_parser.add_argument(
        '-o', '--old-age', default=OLD_MACHINE_AGE, type=int,
        help='Set old machine age to n hours.')
    ds_parser.add_argument(
        'filter', help='An exact name or glob pattern to match services to.')
    args = parser.parse_args(args)
    if not args.certificate_path or not args.certificate_path.endswith('.pem'):
        print("$AZURE_MANAGEMENT_CERTFILE is not a pem file.")
    if not args.subscription_id:
        print("$AZURE_MANAGEMENT_SUBSCRIPTIONID was not provided.")
    return args


def main(argv):
    args = parse_args(argv)
    sms = ServiceManagementService(args.subscription_id, args.certificate_path)
    if args.command == 'delete-unused-disks':
        delete_unused_disks(sms, dry_run=args.dry_run, verbose=args.verbose)
    elif args.command == 'list-services':
        list_services(sms, glob=args.filter, verbose=args.verbose)
    elif args.command == 'delete-services':
        delete_services(
            sms, glob=args.filter, old_age=args.old_age,
            dry_run=args.dry_run, verbose=args.verbose)
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
