#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import datetime
import json
import os
import sys
import traceback


RELEASED = 'released'
PROPOSED = 'proposed'
DEVEL = 'devel'
TESTING = 'testing'
PURPOSES = (RELEASED, PROPOSED, DEVEL, TESTING)
AWS_MIRROR = {
    "mirror": "https://juju-dist.s3.amazonaws.com/tools",
    "path": "PRODUCT_FILE",
    "clouds": [
        {
            "endpoint": "https://ec2.us-east-1.amazonaws.com",
            "region": "us-east-1"
        },
        {
            "endpoint": "https://ec2.us-west-1.amazonaws.com",
            "region": "us-west-1"
        },
        {
            "endpoint": "https://ec2.us-west-2.amazonaws.com",
            "region": "us-west-2"
        },
        {
            "endpoint": "https://ec2.eu-west-1.amazonaws.com",
            "region": "eu-west-1"
        },
        {
            "endpoint": "https://ec2.ap-southeast-1.amazonaws.com",
            "region": "ap-southeast-1"
        },
        {
            "endpoint": "https://ec2.ap-southeast-2.amazonaws.com",
            "region": "ap-southeast-2"
        },
        {
            "endpoint": "https://ec2.ap-northeast-1.amazonaws.com",
            "region": "ap-northeast-1"
        },
        {
            "endpoint": "https://ec2.sa-east-1.amazonaws.com",
            "region": "sa-east-1"
        },
        {
            "endpoint": "https://ec2.eu-central-1.amazonaws.com",
            "region": "eu-central-1"
        }
    ]
}
AZURE_MIRROR = {
    "mirror": "https://jujutools.blob.core.windows.net/juju-tools/tools",
    "path": "streams/v1/com.ubuntu.juju:%s:tools.json",
    "clouds": [
        {
            "endpoint": "https://core.windows.net/",
            "region": "Japan East"
        },
        {
            "endpoint": "https://core.windows.net/",
            "region": "Japan West"
        },
        {
            "endpoint": "https://core.chinacloudapi.cn/",
            "region": "China East"
        },
        {
            "endpoint": "https://core.chinacloudapi.cn/",
            "region": "China North"
        },
        {
            "endpoint": "https://core.windows.net/",
            "region": "East Asia"
        },
        {
            "endpoint": "https://core.windows.net/",
            "region": "Southeast Asia"
        },
        {
            "endpoint": "https://core.windows.net/",
            "region": "North Europe"
        },
        {
            "endpoint": "https://core.windows.net/",
            "region": "West Europe"
        },
        {
            "endpoint": "https://core.windows.net/",
            "region": "East US"
        },
        {
            "endpoint": "https://core.windows.net/",
            "region": "East US 2"
        },
        {
            "endpoint": "https://core.windows.net/",
            "region": "Central US"
        },
        {
            "endpoint": "https://core.windows.net/",
            "region": "West US"
        },
        {
            "endpoint": "https://management.core.windows.net/",
            "region": "North Central US"
        },
        {
            "endpoint": "https://management.core.windows.net/",
            "region": "South Central US"
        },
        {
            "endpoint": "https://management.core.windows.net/",
            "region": "Australia Southeast"
        },
        {
            "endpoint": "https://management.core.windows.net/",
            "region": "Australia East"
        },
        {
            "endpoint": "https://management.core.windows.net/",
            "region": "Brazil South"
        }
    ]
}
HP_MIRROR = {
    "mirror": ("https://region-a.geo-1.objects.hpcloudsvc.com/"
               "v1/60502529753910/juju-dist/tools"),
    "path": "streams/v1/com.ubuntu.juju:released:tools.json",
    "clouds": [
        {
            "endpoint": ("https://region-a.geo-1.identity.hpcloudsvc.com:"
                         "35357/v2.0/"),
            "region": "region-a.geo-1"
        },
        {
            "endpoint": ("https://region-b.geo-1.identity.hpcloudsvc.com:"
                         "35357/v2.0/"),
            "region": "region-b.geo-1"
        }
    ]
}
JOYENT_MIRROR = {
    "mirror": ("https://us-east.manta.joyent.com/"
               "cpcjoyentsupport/public/juju-dist/tools"),
    "path": "streams/v1/com.ubuntu.juju:released:tools.json",
    "clouds": [
        {
            "region": "eu-ams-1",
            "endpoint": "https://eu-ams-1.api.joyentcloud.com"
        },
        {
            "region": "us-sw-1",
            "endpoint": "https://us-sw-1.api.joyentcloud.com"
        },
        {
            "region": "us-east-3",
            "endpoint": "https://us-east-3.api.joyentcloud.com"
        },
        {
            "region": "us-east-2",
            "endpoint": "https://us-east-2.api.joyentcloud.com"
        },
        {
            "region": "us-east-1",
            "endpoint": "https://us-east-1.api.joyentcloud.com"
        },
        {
            "region": "us-west-1",
            "endpoint": "https://us-west-1.api.joyentcloud.com"
        }
    ]
}


def generate_mirrors_file(updated, streams_path,
                          verbose=False, dry_run=False):
    if verbose:
        print('Creating mirrors.json')
    updated = updated.strftime('%Y%m%d')
    mirrors = {
        "mirrors": {}
    }
    for purpose in PURPOSES:
        product_name = "com.ubuntu.juju:%s:tools" % purpose
        if verbose:
            print("Adding %s at %s to mirrors.json" % (product_name, updated))
        mirrors['mirrors'][product_name] = [{
            "datatype": "content-download",
            "path": "streams/v1/cpc-mirrors.json",
            "updated": "%s" % updated,
            "format": "mirrors:1.0"
        }]
    data = json.dumps(mirrors)
    file_path = '%s/mirrors.json' % streams_path
    if not dry_run:
        with open(file_path, 'w') as mirror_file:
            mirror_file.write(data)


def generate_cpc_mirrors_file(updated, streams_path,
                              verbose=False, dry_run=False):
    if verbose:
        print('Creating cpc-mirrors.json')
    updated = updated.strftime('%a, %d %b %Y %H:%M:%S -0000')
    mirrors = {
        "mirrors": {
            "format": "mirrors:1.0",
            "updated": updated}
    }
    for purpose in PURPOSES:
        product_name = "com.ubuntu.juju:%s:tools" % purpose
        product_path = "streams/v1/%s.json" % product_name
        if verbose:
            print(
                "Adding %s at %s to cpc-mirrors.json" %
                (product_path, updated))
        aws_mirror = dict(AWS_MIRROR)
        aws_mirror['path'] = product_path
        azure_mirror = dict(AZURE_MIRROR)
        azure_mirror['path'] = product_path
        hp_mirror = dict(HP_MIRROR)
        hp_mirror['path'] = product_path
        joyent_mirror = dict(JOYENT_MIRROR)
        joyent_mirror['path'] = product_path
        mirrors['mirrors'][product_name] = [
            aws_mirror, azure_mirror, hp_mirror, joyent_mirror]
    data = json.dumps(mirrors)
    file_path = '%s/cpc-mirrors.json' % streams_path
    if not dry_run:
        with open(file_path, 'w') as mirror_file:
            mirror_file.write(data)


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("Compare old and new stream data.")
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
    """Verify that the new json has all the expected changes.

    An exit code of 1 will have a list of strings explaining the problems.
    An exit code of 0 is a pass and the explanation is None.
    """
    args = parse_args(argv[1:])
    try:
        streams_path = os.path.join(args.streams_path, 'streams', 'v1')
        updated = datetime.datetime.utcnow()
        generate_cpc_mirrors_file(
            updated, streams_path, args.verbose, args.dry_run)
        generate_mirrors_file(
            updated, streams_path, args.verbose, args.dry_run)
    except Exception as e:
        print(e)
        if args.verbose:
            traceback.print_tb(sys.exc_info()[2])
        return 2
    if args.verbose:
        print("Created mirror json.")
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
