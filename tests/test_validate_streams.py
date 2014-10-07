from contextlib import contextmanager
#from mock import patch
import json
import shutil
from tempfile import mkdtemp
from unittest import TestCase

from validate_streams import (
    compare_tools,
    find_tools,
    parse_args,
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
    tool = {
        "release": "{}".format(release),
        "version": "{}".format(version),
        "arch": "{}".format(arch),
        "size": 8234578,
        "path": "releases/juju-{}.tgz".format(name),
        "ftype": "tar.gz",
        "sha256": "valid_sum"
    }
    return name, tool


def make_tools_data(release='trusty', arch='amd64', versions=['1.20.7']):
    return dict(make_tool_data(v, release, arch) for v in versions)


def make_product_data(release='trusty', arch='amd64', versions=['1.20.7']):
    name = 'com.ubuntu.juju:{}:{}'.format(release, arch)
    items = make_tools_data(release, arch, versions)
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


def make_products_data(versions):
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
        self.assertEqual('1.20.9', args.version)
        self.assertEqual('old/json', args.old_json)
        self.assertEqual('new/json', args.new_json)
        # A bad release version can be retracted.
        args = parse_args(['--retracted', 'bad'] + required)
        self.assertEqual('bad', args.retracted)

    def test_find_tools(self):
        products = make_products_data(['1.20.7', '1.20.8'])
        with temp_dir() as wd:
            file_path = '{}/json'.format(wd)
            with open(file_path, 'w') as f:
                f.write(json.dumps(products))
            tools = find_tools(file_path)
        expected = [
            '1.20.7-trusty-i386', '1.20.7-trusty-amd64',
            '1.20.8-trusty-amd64', '1.20.8-trusty-i386']
        self.assertEqual(expected, tools.keys())

    def test_compare_tools_identical(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        code, info = compare_tools(
            old_tools, new_tools, 'proposed', 'IGNORE', retracted=None)
        self.assertIs(None, info)
        self.assertEqual(0, code)

    def test_compare_tools_added_new(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.20.9'])
        code, info = compare_tools(
            old_tools, new_tools, 'proposed', '1.20.9', retracted=None)
        self.assertIs(None, info)
        self.assertEqual(0, code)

    def test_compare_tools_retracted_old(self):
        old_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.20.9'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        # revert to 1.20.8 as the newest, remove 1.20.9.
        code, info = compare_tools(
            old_tools, new_tools, 'proposed', '1.20.8', retracted='1.20.9')
        self.assertIs(None, info)
        self.assertEqual(0, code)

    def test_compare_tools_missing_from_new(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.8'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.9'])
        code, info = compare_tools(
            old_tools, new_tools, 'proposed', '1.20.9', retracted=None)
        self.assertEqual(["Missing versions: ['1.20.8-trusty-amd64']"], info)
        self.assertEqual(1, code)

    def test_compare_tools_extra_added_new(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7'])
        new_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.20.9'])
        code, info = compare_tools(
            old_tools, new_tools, 'proposed', '1.20.9', retracted=None)
        self.assertEqual(["Extra versions: ['1.20.8-trusty-amd64']"], info)
        self.assertEqual(1, code)

    def test_compare_tools_failed_retraction_old(self):
        old_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.20.9'])
        new_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.20.9'])
        # revert to 1.20.8 as the newest, remove 1.20.9.
        code, info = compare_tools(
            old_tools, new_tools, 'proposed', '1.20.8', retracted='1.20.9')
        self.assertEqual(["Extra versions: ['1.20.9-trusty-amd64']"], info)
        self.assertEqual(1, code)

    def test_compare_tools_changed_tool(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.20.9'])
        new_tools['1.20.7-trusty-amd64']['sha256'] = 'bad_sum'
        code, info = compare_tools(
            old_tools, new_tools, 'proposed', '1.20.9', retracted=None)
        self.assertEqual(
            [('1.20.7-trusty-amd64', 'sha256', 'valid_sum', 'bad_sum')], info)
        self.assertEqual(1, code)

    def test_compare_tools_added_devel_version(self):
        # devel tools cannot ever got to proposed and release.
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.21-alpha1'])
        # Devel versions can go to testing
        code, info = compare_tools(
            old_tools, new_tools, 'testing', '1.21-alpha1', retracted=None)
        self.assertIs(None, info)
        self.assertEqual(0, code)
        # Devel versions can go to devel
        code, info = compare_tools(
            old_tools, new_tools, 'devel', '1.21-alpha1', retracted=None)
        self.assertIs(None, info)
        self.assertEqual(0, code)
        # Devel versions cannot be proposed.
        code, info = compare_tools(
            old_tools, new_tools, 'proposed', '1.21-alpha1', retracted=None)
        expected = (
            "Devel versions in proposed stream: ['1.21-alpha1-trusty-amd64']")
        self.assertEqual([expected], info)
        self.assertEqual(1, code)
        # Devel versions cannot be release.
        code, info = compare_tools(
            old_tools, new_tools, 'release', '1.21-alpha1', retracted=None)
        expected = (
            "Devel versions in release stream: ['1.21-alpha1-trusty-amd64']")
        self.assertEqual([expected], info)
        self.assertEqual(1, code)
