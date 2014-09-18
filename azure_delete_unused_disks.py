#!/usr/bin/python
# The azure lib checks the environment for two vars that can be sourced
# or defined before the command.
# export AZURE_MANAGEMENT_SUBSCRIPTIONID=ID
# export AZURE_MANAGEMENT_CERTFILE=path/to/azure.pem

from __future__ import print_function

import os
import sys

#from azure import *
from azure.servicemanagement import (
    AZURE_MANAGEMENT_CERTFILE,
    AZURE_MANAGEMENT_SUBSCRIPTIONID,
    ServiceManagementService,
)


def delete_unused_disks(subscription_id, certificate_path):
    sms = ServiceManagementService(subscription_id, certificate_path)
    for disk in sms.list_disks():
        unused = (not disk.attached_to
                  or disk.attached_to.hosted_service_name == '')
        if unused:
            print("Deleting {}".format(disk.name))
            sms.delete_disk(disk.name, delete_vhd=True)
        else:
            print("Skipping {}".format(disk.name))


def main():
    certificate_path = os.environ.get(AZURE_MANAGEMENT_CERTFILE)
    if not certificate_path or not certificate_path.endswith('.pem'):
        print("$AZURE_MANAGEMENT_CERTFILE is not a pem file.".format(
            certificate_path))
    subscription_id = os.environ.get(AZURE_MANAGEMENT_SUBSCRIPTIONID)
    if not subscription_id:
        print("$AZURE_MANAGEMENT_SUBSCRIPTIONID is not defined.".format(
            subscription_id))
    delete_unused_disks(subscription_id, certificate_path)
    return 0


if __name__ == '__main__':
    sys.exit(main())
