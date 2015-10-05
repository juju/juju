#   Copyright (C) 2013, 2015 Canonical Ltd.
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
import json
import os
import sys

from simplestreams import util


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


def json_dump(data, filename):
    with open(filename, "w") as fp:
        sys.stderr.write("writing %s\n" % filename)
        fp.write(json.dumps(data, indent=2, sort_keys=True,
                 separators=(',', ': ')) + "\n")
