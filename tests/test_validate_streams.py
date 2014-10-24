from mock import patch
import json
from unittest import TestCase

from utils import temp_dir
from validate_streams import (
    check_devel_not_stable,
    check_expected_changes,
    check_expected_unchanged,
    check_agents_content,
    compare_agents,
    find_agents,
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


def make_agents_data(release='trusty', arch='amd64', versions=['1.20.7']):
    return dict(make_tool_data(v, release, arch) for v in versions)


def make_product_data(release='trusty', arch='amd64', versions=['1.20.7']):
    name = 'com.ubuntu.juju:{}:{}'.format(release, arch)
    items = make_agents_data(release, arch, versions)
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
        "content_id": "com.ubuntu.juju:released:agents"
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

    def test_find_agents(self):
        products = make_products_data(['1.20.7', '1.20.8'])
        with temp_dir() as wd:
            file_path = '{}/json'.format(wd)
            with open(file_path, 'w') as f:
                f.write(json.dumps(products))
            agents = find_agents(file_path)
        expected = [
            '1.20.7-trusty-i386', '1.20.7-trusty-amd64',
            '1.20.8-trusty-amd64', '1.20.8-trusty-i386']
        self.assertEqual(expected, agents.keys())

    def test_check_devel_not_stable(self):
        # devel agents cannot ever got to proposed and release.
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.21-alpha1'])
        # Devel versions can go to testing
        message = check_devel_not_stable(old_agents, new_agents, 'testing')
        self.assertEqual([], message)
        # Devel versions can go to devel
        message = check_devel_not_stable(old_agents, new_agents, 'devel')
        self.assertEqual([], message)
        # Devel versions cannot be proposed.
        message = check_devel_not_stable(old_agents, new_agents, 'proposed')
        self.assertEqual(
            ["Devel versions in proposed stream: "
             "['1.21-alpha1-trusty-amd64']"],
            message)
        # Devel versions cannot be release.
        message = check_devel_not_stable(old_agents, new_agents, 'release')
        self.assertEqual(
            ["Devel versions in release stream: ['1.21-alpha1-trusty-amd64']"],
            message)

    def test_check_agents_content(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents['1.20.7-trusty-amd64']['sha256'] = 'bad_sum'
        message = check_agents_content(old_agents, new_agents)
        self.assertEqual(
            (['Tool 1.20.7-trusty-amd64 sha256 changed from '
              'valid_sum to bad_sum']),
            message)

    def test_compare_agents_identical(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        message = compare_agents(
            old_agents, new_agents, 'proposed', added=None, removed=None)
        self.assertIs(None, message)

    def test_check_expected_changes_with_no_changes(self):
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        errors = check_expected_changes(new_agents, added=None, removed=None)
        self.assertEqual([], errors)

    def test_check_expected_changes_with_changes(self):
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.9'])
        errors = check_expected_changes(
            new_agents, added='1.20.9', removed='1.20.8')
        self.assertEqual([], errors)

    def test_check_expected_changes_with_found_errors(self):
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        errors = check_expected_changes(
            new_agents, added=None, removed='1.20.8')
        self.assertEqual(
            ["1.20.8 agents were not removed: ['1.20.8-trusty-amd64']"],
            errors)

    def test_check_expected_changes_with_missing_errors(self):
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7'])
        errors = check_expected_changes(
            new_agents, added='1.20.8', removed=None)
        self.assertEqual(['1.20.8 agents were not added'], errors)

    def test_check_expected_unchanged_without_changes(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        errors = check_expected_unchanged(
            old_agents, new_agents, added=None, removed=None)
        self.assertEqual([], errors)

    def test_check_expected_unchanged_without_changes_and_added_removed(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.9'])
        errors = check_expected_unchanged(
            old_agents, new_agents, added='1.20.9', removed='1.20.8')
        self.assertEqual([], errors)

    def test_check_expected_unchanged_with_missing_errors(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.9'])
        errors = check_expected_unchanged(
            old_agents, new_agents, added='1.20.9', removed=None)
        self.assertEqual(
            ["These agents are missing: ['1.20.8-trusty-amd64']"],
            errors)

    def test_check_expected_unchanged_with_found_errors(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7'])
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.9'])
        errors = check_expected_unchanged(
            old_agents, new_agents, added=None, removed=None)
        self.assertEqual(
            ["These unknown agents were found: ['1.20.9-trusty-amd64']"],
            errors)

    def test_compare_agents_changed_tool(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.20.9'])
        new_agents['1.20.7-trusty-amd64']['sha256'] = 'bad_sum'
        errors = compare_agents(
            old_agents, new_agents, 'proposed', '1.20.9', removed=None)
        self.assertEqual(
            ['Tool 1.20.7-trusty-amd64 sha256 changed from '
             'valid_sum to bad_sum'],
            errors)

    def test_compare_agents_called_check_expected_agents_called(self):
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7'])
        new_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        with patch("validate_streams.check_expected_changes",
                   return_value=(['foo'])) as cec_mock:
            with patch("validate_streams.check_expected_unchanged",
                       return_value=(['bar'])) as ceu_mock:
                errors = compare_agents(
                    old_agents, new_agents, 'proposed', '1.20.9', removed=None)
                cec_mock.assert_called_with(new_agents, '1.20.9', None)
                ceu_mock.assert_called_with(
                    old_agents, new_agents, '1.20.9', None)
        self.assertEqual(['foo', 'bar'], errors)

    def test_compare_agents_added_devel_version(self):
        # devel agents cannot ever got to proposed and release.
        old_agents = make_agents_data('trusty', 'amd64', ['1.20.7', '1.20.8'])
        new_agents = make_agents_data(
            'trusty', 'amd64', ['1.20.7', '1.20.8', '1.21-alpha1'])
        # Devel versions can go to devel
        message = compare_agents(
            old_agents, new_agents, 'devel', '1.21-alpha1', removed=None)
        self.assertIs(None, message)
        # Devel versions cannot be proposed.
        errors = compare_agents(
            old_agents, new_agents, 'proposed', '1.21-alpha1', removed=None)
        expected = (
            "Devel versions in proposed stream: ['1.21-alpha1-trusty-amd64']")
        self.assertEqual([expected], errors)
        # Devel versions cannot be release.
        errors = compare_agents(
            old_agents, new_agents, 'release', '1.21-alpha1', removed=None)
        expected = (
            "Devel versions in release stream: ['1.21-alpha1-trusty-amd64']")
        self.assertEqual([expected], errors)
