#!/usr/bin/env python3
#   Copyright (C) 2013,2015 Canonical Ltd.
#
#   Author: Scott Moser <scott.moser@canonical.com>,
#           Aaron Bentley <aaron.bentley@canonical.com>
#
#   Simplestreams is free software: you can redistribute it and/or modify it
#   under the terms of the GNU Affero General Public License as published by
#   the Free Software Foundation, either version 3 of the License, or (at your
#   option) any later version.
#
#   Simplestreams is distributed in the hope that it will be useful, but
#   WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY
#   or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU Affero General Public
#   License for more details.
#
from argparse import ArgumentParser
import json
import os
import sys

from simplestreams import util


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


def items2content_trees(itemslist, exdata):
    # input is a list with each item having:
    #   (content_id, product_name, version_name, item_name, {data})
    ctrees = {}
    for (content_id, prodname, vername, itemname, data) in itemslist:
        if content_id not in ctrees:
            ctrees[content_id] = {'content_id': content_id,
                                  'format': 'products:1.0', 'products': {}}
            ctrees[content_id].update(exdata)

        ctree = ctrees[content_id]
        if prodname not in ctree['products']:
            ctree['products'][prodname] = {'versions': {}}

        prodtree = ctree['products'][prodname]
        if vername not in prodtree['versions']:
            prodtree['versions'][vername] = {'items': {}}

        vertree = prodtree['versions'][vername]

        if itemname in vertree['items']:
            raise ValueError("%s: already existed" %
                             str([content_id, prodname, vername, itemname]))

        vertree['items'][itemname] = data
    return ctrees


class FileNamer:

    streamdir = 'streams/v1'

    @classmethod
    def get_index_path(cls):
        return "%s/%s" % (cls.streamdir, 'index.json')

    @classmethod
    def get_content_path(cls, content_id):
        return "%s/%s.json" % (cls.streamdir, content_id)


class JujuFileNamer(FileNamer):

    @classmethod
    def get_index_path(cls):
        return "%s/%s" % (cls.streamdir, 'index2.json')

    @classmethod
    def get_content_path(cls, content_id):
        return "%s/%s.json" % (cls.streamdir, content_id.replace(':', '-'))


def write_streams(out_d, trees, updated, namer=None):
    if namer is None:
        namer = FileNamer

    index = {"index": {}, 'format': 'index:1.0', 'updated': updated}

    to_write = [(namer.get_index_path(), index,)]

    not_copied_up = ['content_id']
    for content_id in trees:
        util.products_condense(trees[content_id],
                               sticky=['path', 'sha256', 'md5', 'size'])
        content = trees[content_id]
        index['index'][content_id] = {
            'products': list(content['products'].keys()),
            'path': namer.get_content_path(content_id),
        }
        for k in util.stringitems(content):
            if k not in not_copied_up:
                index['index'][content_id][k] = content[k]

        to_write.append((index['index'][content_id]['path'], content,))

    out_filenames = []
    for (outfile, data) in to_write:
        filef = os.path.join(out_d, outfile)
        util.mkdir_p(os.path.dirname(filef))
        json_dump(data, filef)
        out_filenames.append(filef)
    return out_filenames

def parse_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument(
        'items_file', metavar='items-file', help='File to read items from',
        nargs='+')
    parser.add_argument(
        'out_d', metavar='output-dir',
        help='The directory to write stream files to.')
    return parser.parse_args(argv)


def json_dump(data, filename):
    with open(filename, "w") as fp:
        sys.stderr.write("writing %s\n" % filename)
        fp.write(json.dumps(data, indent=2, sort_keys=True) + "\n")


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
