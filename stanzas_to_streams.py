#!/usr/bin/env python3
#   Copyright (C) 2013, 2015 Canonical Ltd.

from argparse import ArgumentParser
import json
import os
import sys

from simplestreams import util

from generate_simplestreams import (
    FileNamer,
    Item,
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


def dict_to_item(item_dict):
    """Convert a dict into an Item, mutating input."""
    item_dict.pop('item_url', None)
    item_dict['size'] = int(item_dict['size'])
    content_id = item_dict.pop('content_id')
    product_name = item_dict.pop('product_name')
    version_name = item_dict.pop('version_name')
    item_name = item_dict.pop('item_name')
    return Item(content_id, product_name, version_name, item_name, item_dict)


def read_items_file(filename):
    with open(filename) as items_file:
        item_list = json.load(items_file)
    return (dict_to_item(item) for item in item_list)


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


def filenames_to_streams(filenames, updated, out_d):
    """Convert a list of filenames into simplestreams.

    File contents must be json simplestream stanzas.
    'updated' is the date to use for 'updated' in the streams.
    out_d is the directory to create streams in.
    """
    items = []
    for items_file in filenames:
        items.extend(read_items_file(items_file))

    data = {'updated': updated, 'datatype': 'content-download'}
    trees = items2content_trees(items, data)
    out_filenames = write_streams(out_d, trees, updated, JujuFileNamer)
    out_filenames.append(write_release_index(out_d))


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
    updated = util.timestamp()
    filenames_to_streams(args.items_file, updated, args.out_d)


if __name__ == '__main__':
    sys.exit(main())
