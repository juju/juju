from mock import patch
import json
from unittest import TestCase

from utils import temp_dir
from validate_streams import (
    check_devel_not_stable,
    check_expected_tools,
    check_tools_content,
    compare_tools,
    find_tools,
    parse_args,
)


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
        required = ['--added', '1.20.9', 'proposed', 'old/json', 'new/json']
        args = parse_args(required)
        self.assertEqual('proposed', args.purpose)
        self.assertEqual('old/json', args.old_json)
        self.assertEqual('new/json', args.new_json)
        self.assertEqual('1.20.9', args.added)
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

    def test_check_devel_not_stable(self):
        # devel tools cannot ever got to proposed and release.
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.21-alpha1'])
        # Devel versions can go to testing
        message = check_devel_not_stable(old_tools, new_tools, 'testing')
        self.assertIs(None, message)
        # Devel versions can go to devel
        message = check_devel_not_stable(old_tools, new_tools, 'devel')
        self.assertIs(None, message)
        # Devel versions cannot be proposed.
        message = check_devel_not_stable(old_tools, new_tools, 'proposed')
        self.assertEqual(
            "Devel versions in proposed stream: ['1.21-alpha1-trusty-amd64']",
            message)
        # Devel versions cannot be release.
        message = check_devel_not_stable(old_tools, new_tools, 'release')
        self.assertEqual(
            "Devel versions in release stream: ['1.21-alpha1-trusty-amd64']",
            message)

    def test_check_tools_content(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools['1.20.7-trusty-amd64']['sha256'] = 'bad_sum'
        message = check_tools_content(old_tools, new_tools)
        self.assertEqual(
            ('Tool 1.20.7-trusty-amd64 sha256 changed from '
             'valid_sum to bad_sum'),
            message)

    def test_compare_tools_identical(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        message = compare_tools(
            old_tools, new_tools, 'proposed', added=None, retracted=None)
        self.assertIs(None, message)

    def test_check_expected_tools_no_added_no_retraced(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        tools = check_expected_tools(old_tools, new_tools, None, None)
        new_expected, extra_errors, old_expected, missing_errors = tools
        self.assertEqual(
            ['1.20.7-trusty-amd64', '1.20.8-trusty-amd64'],
            new_expected.keys())
        self.assertIs(None, extra_errors)
        self.assertEqual(
            ['1.20.7-trusty-amd64', '1.20.8-trusty-amd64'],
            old_expected.keys())
        self.assertIs(None, missing_errors)

    def test_check_expected_tools_added_new(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.20.9'])
        tools = check_expected_tools(old_tools, new_tools, '1.20.9', None)
        new_expected, extra_errors, old_expected, missing_errors = tools
        self.assertEqual(
            ['1.20.7-trusty-amd64', '1.20.8-trusty-amd64'],
            new_expected.keys())
        self.assertIs(None, extra_errors)
        self.assertEqual(
            ['1.20.7-trusty-amd64', '1.20.8-trusty-amd64'],
            old_expected.keys())
        self.assertIs(None, missing_errors)

    def test_check_expected_tools_retracted_old(self):
        old_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.20.9'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        # revert to 1.20.8 as the newest, remove 1.20.9.
        tools = check_expected_tools(old_tools, new_tools, None, '1.20.9')
        new_expected, extra_errors, old_expected, missing_errors = tools
        self.assertEqual(
            ['1.20.7-trusty-amd64', '1.20.8-trusty-amd64'],
            new_expected.keys())
        self.assertIs(None, extra_errors)
        self.assertEqual(
            ['1.20.7-trusty-amd64', '1.20.8-trusty-amd64'],
            old_expected.keys())
        self.assertIs(None, missing_errors)

    def test_check_expected_tools_retracted_old_and_added_new(self):
        old_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.9'])
        # Remove 1.20.8, leep to 1.20.9.
        tools = check_expected_tools(old_tools, new_tools, '1.20.9', '1.20.8')
        new_expected, extra_errors, old_expected, missing_errors = tools
        self.assertEqual(['1.20.7-trusty-amd64'], new_expected.keys())
        self.assertIs(None, extra_errors)
        self.assertEqual(['1.20.7-trusty-amd64'], old_expected.keys())
        self.assertIs(None, missing_errors)

    def test_check_expected_tools_missing_from_new(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.8'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.9'])
        tools = check_expected_tools(old_tools, new_tools, '1.20.9', None)
        new_expected, extra_errors, old_expected, missing_errors = tools
        self.assertEqual([], new_expected.keys())
        self.assertIs(None, extra_errors)
        self.assertEqual(['1.20.8-trusty-amd64'], old_expected.keys())
        self.assertEqual(
            "Missing versions: ['1.20.8-trusty-amd64']", missing_errors)

    def test_check_expected_tools_missing_version(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.8'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.8'])
        tools = check_expected_tools(old_tools, new_tools, '1.20.9', None)
        new_expected, extra_errors, old_expected, missing_errors = tools
        self.assertEqual(['1.20.8-trusty-amd64'], new_expected.keys())
        self.assertIs(None, extra_errors)
        self.assertEqual(['1.20.8-trusty-amd64'], old_expected.keys())
        self.assertEqual(
            "Missing versions: ['1.20.9']", missing_errors)

    def test_check_expected_tools_extra_new(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7'])
        new_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.20.9'])
        tools = check_expected_tools(old_tools, new_tools, '1.20.9')
        new_expected, extra_errors, old_expected, missing_errors = tools
        self.assertEqual(
            ['1.20.7-trusty-amd64', '1.20.8-trusty-amd64'],
            new_expected.keys())
        self.assertEqual(
            "Extra versions: ['1.20.8-trusty-amd64']", extra_errors)
        self.assertEqual(['1.20.7-trusty-amd64'], old_expected.keys())
        self.assertIs(None, missing_errors)

    def test_check_expected_tools_failed_retracted_old(self):
        old_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.20.9'])
        new_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.20.9'])
        # revert to 1.20.8 as the newest, remove 1.20.9.
        tools = check_expected_tools(old_tools, new_tools, None, '1.20.9')
        new_expected, extra_errors, old_expected, missing_errors = tools
        self.assertEqual(
            ['1.20.7-trusty-amd64', '1.20.8-trusty-amd64',
             '1.20.9-trusty-amd64'],
            sorted(new_expected.keys()))
        self.assertEqual(
            "Extra versions: ['1.20.9-trusty-amd64']", extra_errors)
        self.assertEqual(
            ['1.20.7-trusty-amd64', '1.20.8-trusty-amd64'],
            old_expected.keys())
        self.assertIs(None, missing_errors)

    def test_compare_tools_changed_tool(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.20.9'])
        new_tools['1.20.7-trusty-amd64']['sha256'] = 'bad_sum'
        message = compare_tools(
            old_tools, new_tools, 'proposed', '1.20.9', retracted=None)
        self.assertEqual(
            ['Tool 1.20.7-trusty-amd64 sha256 changed from '
             'valid_sum to bad_sum'],
            message)

    def test_compare_tools_called_check_expected_tools_called(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        with patch("validate_streams.check_expected_tools",
                   return_value=(new_tools, 'foo', old_tools, None)) as mock:
            message = compare_tools(
                old_tools, new_tools, 'proposed', '1.20.9', retracted=None)
            mock.assert_called_with(old_tools, new_tools, '1.20.9', None)
        self.assertEqual(['foo'], message)

    def test_compare_tools_added_devel_version(self):
        # devel tools cannot ever got to proposed and release.
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.21-alpha1'])
        # Devel versions can go to devel
        message = compare_tools(
            old_tools, new_tools, 'devel', '1.21-alpha1', retracted=None)
        self.assertIs(None, message)
        # Devel versions cannot be proposed.
        message = compare_tools(
            old_tools, new_tools, 'proposed', '1.21-alpha1', retracted=None)
        expected = (
            "Devel versions in proposed stream: ['1.21-alpha1-trusty-amd64']")
        self.assertEqual([expected], message)
        # Devel versions cannot be release.
        message = compare_tools(
            old_tools, new_tools, 'release', '1.21-alpha1', retracted=None)
        expected = (
            "Devel versions in release stream: ['1.21-alpha1-trusty-amd64']")
        self.assertEqual([expected], message)
