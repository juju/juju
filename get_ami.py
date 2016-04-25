#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import errno
import subprocess
import sys


STREAM_INDEX = "http://cloud-images.ubuntu.com/releases/streams/v1/index.json"
DAILY_INDEX = "http://cloud-images.ubuntu.com/daily/streams/v1/index.json"
ENDPOINT_TEMPLATE = "endpoint~ec2.{region}.amazonaws.com"

DEFAULT_PARAMS = {
    "label": None,
    "virt": "pv",
    "root_store": "ssd",
}


def query_ami(series, arch, region=None, stream='released', **kwargs):
    """Lookup newest ami for given series and arch, plus optional params."""
    if region is None:
        region = "us-east-1"
    if stream == 'daily':
        index = DAILY_INDEX
    else:
        index = STREAM_INDEX
    sstream_params = ["arch=" + arch, "release=" + series]
    for k in sorted(DEFAULT_PARAMS):
        v = kwargs.pop(k, DEFAULT_PARAMS[k])
        if v is None:
            v = DEFAULT_PARAMS[k]
        if v is not None:
            sstream_params.append("{}={}".format(k, v))
    if kwargs:
        raise ValueError("Unknown kwargs: {}".format(", ".join(kwargs)))
    endpoint_info = ENDPOINT_TEMPLATE.format(region=region)
    cmdline = ["sstream-query", index, endpoint_info]
    cmdline.extend(sstream_params)
    cmdline.extend(["--output-format", "%(id)s"])
    try:
        out = subprocess.check_output(cmdline)
    except OSError as err:
        if err.errno == errno.ENOENT:
            raise ValueError("sstream-query tool not found, is it installed?")
        raise
    ami_ids = out.split("\n")
    if not ami_ids or not ami_ids[0]:
        raise ValueError("No amis for {} in region={}".format(
                         " ".join(sstream_params), region))
    return ami_ids[0]


def parse_args(args=None):
    parser = ArgumentParser('Get an up to date ami.')
    parser.add_argument('series', help='Ubuntu series for image')
    parser.add_argument('arch', help='Architecture for image')
    parser.add_argument('--stream', choices=['released', 'daily'],
                        default='released',
                        help='The stream to select the image from')
    parser.add_argument('--region', help='Region to retrieve image for')
    parser.add_argument('--label')
    parser.add_argument('--root-store')
    parser.add_argument('--virt')
    return parser.parse_args(args)


def main():
    args = parse_args()
    try:
        print(query_ami(args.series, args.arch, region=args.region,
                        stream=args.stream, label=args.label,
                        root_store=args.root_store, virt=args.virt))
    except ValueError as err:
        print(err, file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
