from mock import patch
import json
from unittest import TestCase

from utils import temp_dir
from validate_streams import (
    check_devel_not_stable,
    check_expected_changes,
    check_expected_unchanged,
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
        # A bad release version can be removed.
        args = parse_args(['--removed', 'bad'] + required)
        self.assertEqual('bad', args.removed)

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
        self.assertEqual([], message)
        # Devel versions can go to devel
        message = check_devel_not_stable(old_tools, new_tools, 'devel')
        self.assertEqual([], message)
        # Devel versions cannot be proposed.
        message = check_devel_not_stable(old_tools, new_tools, 'proposed')
        self.assertEqual(
            ["Devel versions in proposed stream: "
             "['1.21-alpha1-trusty-amd64']"],
            message)
        # Devel versions cannot be release.
        message = check_devel_not_stable(old_tools, new_tools, 'release')
        self.assertEqual(
            ["Devel versions in release stream: ['1.21-alpha1-trusty-amd64']"],
            message)

    def test_check_tools_content(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools['1.20.7-trusty-amd64']['sha256'] = 'bad_sum'
        message = check_tools_content(old_tools, new_tools)
        self.assertEqual(
            (['Tool 1.20.7-trusty-amd64 sha256 changed from '
              'valid_sum to bad_sum']),
            message)

    def test_compare_tools_identical(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        message = compare_tools(
            old_tools, new_tools, 'proposed', added=None, removed=None)
        self.assertIs(None, message)

    def test_check_expected_changes_with_no_changes(self):
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        errors = check_expected_changes(new_tools, added=None, removed=None)
        self.assertEqual([], errors)

    def test_check_expected_changes_with_changes(self):
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.9'])
        errors = check_expected_changes(
            new_tools, added='1.20.9', removed='1.20.8')
        self.assertEqual([], errors)

    def test_check_expected_changes_with_found_errors(self):
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        errors = check_expected_changes(
            new_tools, added=None, removed='1.20.8')
        self.assertEqual(
            ["1.20.8 agents were not removed: ['1.20.8-trusty-amd64']"],
            errors)

    def test_check_expected_changes_with_missing_errors(self):
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7'])
        errors = check_expected_changes(
            new_tools, added='1.20.8', removed=None)
        self.assertEqual(['1.20.8 agents were not added'], errors)

    def test_check_expected_unchanged_without_changes(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        errors = check_expected_unchanged(
            old_tools, new_tools, added=None, removed=None)
        self.assertEqual([], errors)

    def test_check_expected_unchanged_without_changes_and_added_removed(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.9'])
        errors = check_expected_unchanged(
            old_tools, new_tools, added='1.20.9', removed='1.20.8')
        self.assertEqual([], errors)

    def test_check_expected_unchanged_with_missing_errors(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.9'])
        errors = check_expected_unchanged(
            old_tools, new_tools, added='1.20.9', removed=None)
        self.assertEqual(
            ["These agents are missing: ['1.20.8-trusty-amd64']"],
            errors)

    def test_check_expected_unchanged_with_found_errors(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.9'])
        errors = check_expected_unchanged(
            old_tools, new_tools, added=None, removed=None)
        self.assertEqual(
            ["These unknown agents were found: ['1.20.9-trusty-amd64']"],
            errors)

    def test_compare_tools_changed_tool(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.20.9'])
        new_tools['1.20.7-trusty-amd64']['sha256'] = 'bad_sum'
        errors = compare_tools(
            old_tools, new_tools, 'proposed', '1.20.9', removed=None)
        self.assertEqual(
            ['Tool 1.20.7-trusty-amd64 sha256 changed from '
             'valid_sum to bad_sum'],
            errors)

    def test_compare_tools_called_check_expected_tools_called(self):
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7'])
        new_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        with patch("validate_streams.check_expected_changes",
                   return_value=(['foo'])) as cec_mock:
            with patch("validate_streams.check_expected_unchanged",
                       return_value=(['bar'])) as ceu_mock:
                errors = compare_tools(
                    old_tools, new_tools, 'proposed', '1.20.9', removed=None)
                cec_mock.assert_called_with(new_tools, '1.20.9', None)
                ceu_mock.assert_called_with(
                    old_tools, new_tools, '1.20.9', None)
        self.assertEqual(['foo', 'bar'], errors)

    def test_compare_tools_added_devel_version(self):
        # devel tools cannot ever got to proposed and release.
        old_tools = make_tools_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_tools = make_tools_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.21-alpha1'])
        # Devel versions can go to devel
        message = compare_tools(
            old_tools, new_tools, 'devel', '1.21-alpha1', removed=None)
        self.assertIs(None, message)
        # Devel versions cannot be proposed.
        errors = compare_tools(
            old_tools, new_tools, 'proposed', '1.21-alpha1', removed=None)
        expected = (
            "Devel versions in proposed stream: ['1.21-alpha1-trusty-amd64']")
        self.assertEqual([expected], errors)
        # Devel versions cannot be release.
        errors = compare_tools(
            old_tools, new_tools, 'release', '1.21-alpha1', removed=None)
        expected = (
            "Devel versions in release stream: ['1.21-alpha1-trusty-amd64']")
        self.assertEqual([expected], errors)
