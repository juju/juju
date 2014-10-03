from contextlib import contextmanager
#from mock import patch
import json
import shutil
from tempfile import mkdtemp
from unittest import TestCase

from validate_streams import (
    parse_args,
    find_tools,
)


@contextmanager
def temp_dir():
    dirname = mkdtemp()
    try:
        yield dirname
    finally:
        shutil.rmtree(dirname)


def make_tool_data(version='1.20.7', release='trusty', arch='amd64'):
    name = '{}-{}-{}'.format(version, release, arch)
    sha = '3dd276c232f6fae2c547de7fe1aa6825e8a937487d6f6cda01a11f8b39580511'
    tool = {
        "release": "{}".format(release),
        "version": "{}".format(version),
        "arch": "{}".format(arch),
        "size": 8234578,
        "path": "releases/juju-{}.tgz".format(name),
        "ftype": "tar.gz",
        "sha256": "{}".format(sha)
    }
    return name, tool


def make_product_data(release='trusty', arch='amd64', versions=['1.20.7']):
    name = 'com.ubuntu.juju:{}:{}'.format(release, arch)
    items = [make_tool_data(v, release, arch) for v in versions]
    product = {
        "version": "{}".format(versions[0]),
        "arch": "{}".format(arch),
        "versions": {
            "20140919": {
                "items": items
            }
        }
    }
    return name, product


def make_products_data(versions=('1.20.7', '1.20.8')):
    products = {}
    for release, arch in (('trusty', 'amd64'), ('trusty', 'i386')):
        name, product = make_product_data(release, arch, versions)
        products[name] = product
    stream = {
        "products": products,
        "updated": "Fri, 19 Sep 2014 13:25:28 -0400",
        "format": "products:1.0",
        "content_id": "com.ubuntu.juju:released:tools"
    }
    return stream


class ValidateStreams(TestCase):

    def test_parge_args(self):
        # The purpose, release, old json and new json are required.
        required = ['proposed', '1.20.9', 'old/json', 'new/json']
        args = parse_args(required)
        self.assertEqual('proposed', args.purpose)
        self.assertEqual('1.20.9', args.release)
        self.assertEqual('old/json', args.old_json)
        self.assertEqual('new/json', args.new_json)
        # A bad release version can be retracted.
        args = parse_args(['--retracted', 'bad'] + required)
        self.assertEqual('bad', args.retracted)

    def test_find_tools(self):
        products = make_products_data()
        with temp_dir() as wd:
            file_path = '{}/json'.format(wd)
            with open(file_path, 'w') as f:
                f.write(json.dumps(products))
            tools = find_tools(file_path)
        expected = [
            '1.20.7-trusty-i386', '1.20.7-trusty-amd64',
            '1.20.8-trusty-amd64', '1.20.8-trusty-i386']
        self.assertEqual(expected, tools.keys())
