#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import errno
import subprocess
import sys


STREAM_INDEX = "http://cloud-images.ubuntu.com/releases/streams/v1/index.json"


def query_ami(series, arch):
    try:
        out = subprocess.check_output([
            "sstream-query", STREAM_INDEX,
            "endpoint~ec2.us-east-1.amazonaws.com", "arch=" + arch,
            "release=" + series, "label=release", "virt=hvm", "root_store=ebs",
            "--output-format", "%(id)s"])
    except OSError as err:
        if err.errno == errno.ENOENT:
            raise ValueError("sstream-query tool not found, is it installed?")
        raise
    ami_ids = out.split("\n")
    if not ami_ids or not ami_ids[0]:
        raise ValueError("No amis for series=%s arch=%s" % (series, arch))
    return ami_ids[0]


def parse_args(args=None):
    parser = ArgumentParser('Get an up to date ami.')
    parser.add_argument('series', help='Ubuntu series for image')
    parser.add_argument('arch', help='Architecture for image')
    return parser.parse_args(args)


def main():
    args = parse_args()
    try:
        print(query_ami(args.series, args.arch))
    except ValueError as err:
        print(err, file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
