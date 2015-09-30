#!/usr/bin/env python3
#   Copyright (C) 2013, 2015 Canonical Ltd.

from argparse import ArgumentParser
import json
import os
import sys

from simplestreams import util

from generate_simplestreams import (
    FileNamer,
    items2content_trees,
    json_dump,
    write_streams,
    )


class JujuFileNamer(FileNamer):

    @classmethod
    def get_index_path(cls):
        return "%s/%s" % (cls.streamdir, 'index2.json')

    @classmethod
    def get_content_path(cls, content_id):
        return "%s/%s.json" % (cls.streamdir, content_id.replace(':', '-'))


def read_items_file(filename):
    with open(filename) as items_file:
        item_list = json.load(items_file)
    for item in item_list:
        item.pop('item_url', None)
        item['size'] = int(item['size'])
        content_id = item.pop('content_id')
        product_name = item.pop('product_name')
        version_name = item.pop('version_name')
        item_name = item.pop('item_name')
        yield (content_id, product_name, version_name, item_name, item)


def write_release_index(out_d):
    in_path = os.path.join(out_d, JujuFileNamer.get_index_path())
    with open(in_path) as in_file:
        full_index = json.load(in_file)
    full_index['index'] = dict(
        (k, v) for k, v in list(full_index['index'].items())
        if k == 'com.ubuntu.juju:released:tools')
    out_path = os.path.join(out_d, FileNamer.get_index_path())
    json_dump(full_index, out_path)
    return out_path


def parse_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument(
        'items_file', metavar='items-file', help='File to read items from',
        nargs='+')
    parser.add_argument(
        'out_d', metavar='output-dir',
        help='The directory to write stream files to.')
    return parser.parse_args(argv)


def main():
    args = parse_args()
    items = []
    for items_file in args.items_file:
        items.extend(read_items_file(items_file))
    updated = util.timestamp()

    data = {'updated': updated, 'datatype': 'content-download'}
    trees = items2content_trees(items, data)
    out_filenames = write_streams(args.out_d, trees, updated, JujuFileNamer)
    out_filenames.append(write_release_index(args.out_d))


if __name__ == '__main__':
    sys.exit(main())
