#!/usr/bin/python3
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


def write_streams(out_d, trees, updated, sign):
    streamdir = 'streams/v1'
    index = {"index": {}, 'format': 'index:1.0', 'updated': updated}

    to_write = [("%s/%s" % (streamdir, 'index.json'), index,)]

    not_copied_up = ['content_id']
    for content_id in trees:
        util.products_condense(trees[content_id],
                               sticky=['path', 'sha256', 'md5', 'size'])
        content = trees[content_id]
        index['index'][content_id] = {
            'path': "%s/%s.json" % (streamdir, content_id),
            'products': list(content['products'].keys()),
        }
        for k in util.stringitems(content):
            if k not in not_copied_up:
                index['index'][content_id][k] = content[k]

        to_write.append((index['index'][content_id]['path'], content,))

    for (outfile, data) in to_write:
        filef = os.path.join(out_d, outfile)
        util.mkdir_p(os.path.dirname(filef))
        with open(filef, "w") as fp:
            sys.stderr.write("writing %s\n" % filef)
            fp.write(json.dumps(data, indent=1) + "\n")

        if sign:
            sys.stderr.write("signing %s\n" % filef)
            toolutil.signjson_file(filef)


def main():
    items = [
        ('com.ubuntu.juju:released:tools',
         'com.ubuntu.juju:14.04:ppc64el',
         '20150928',
         '1.24-beta6-trusty-ppc64el', {
             'arch': 'ppc64el',
             'ftype': 'tar.gz',
             'path': 'releases/juju-1.24-beta6-trusty-ppc64el.tgz',
             'release': 'trusty',
             'sha256': ('a5ee6a753eef008418992e3105857ea69be78e9565ab1'
                        'f428203d2855b7f5522'),
             'size': 15114526,
             'version': '1.24-beta6',
             }),
    ]
    updated = util.timestamp()

    data = {'updated': updated, 'datatype': 'content-download'}
    trees = items2content_trees(items, data)
    write_streams('outdir', trees, updated, False)


if __name__ == '__main__':
    sys.exit(main())
