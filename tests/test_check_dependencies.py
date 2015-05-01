"""Tests for check_dependencies script used in tarball building."""

import mock
import os
import shutil
import StringIO
import tempfile
import unittest

from check_dependencies import (
    compare_dependencies,
    get_arg_parser,
    get_dependencies,
)


class TestArgParser(unittest.TestCase):

    def parse_args(self, args):
        parser = get_arg_parser()
        return parser.parse_args(args)

    def test_defaults(self):
        args = self.parse_args(["dep.tsv", "."])
        self.assertEqual("dep.tsv", args.depfile)
        self.assertEqual(".", args.srcdir)
        self.assertEqual([], args.ignore)
        self.assertEqual(False, args.delete_unknown)

    def test_ignore(self):
        args = self.parse_args(["-i", "pkg", "dep.tsv", "."])
        self.assertEqual("dep.tsv", args.depfile)
        self.assertEqual(".", args.srcdir)
        self.assertEqual(["pkg"], args.ignore)
        self.assertEqual(False, args.delete_unknown)

    def test_delete_unknown(self):
        args = self.parse_args(["--delete-unknown", "f", "."])
        self.assertEqual("f", args.depfile)
        self.assertEqual(".", args.srcdir)
        self.assertEqual([], args.ignore)
        self.assertEqual(True, args.delete_unknown)


class TestCompareDependencies(unittest.TestCase):

    def make_testing_packages(self, packages):
        tempdir = tempfile.mkdtemp()
        for package in packages:
            path = os.path.join(tempdir, package)
            os.makedirs(path)
            open(os.path.join(path, "fakesource"), "w").close()
        self.addCleanup(shutil.rmtree, tempdir)
        return tempdir

    def test_found_package(self):
        srcdir = self.make_testing_packages(["apkg"])
        deps = compare_dependencies(["apkg"], srcdir)
        self.assertEqual(deps, (["apkg"], []))

    def test_missing_package(self):
        # When juju requires a package that dependencies.tsv does not declare
        # expect build to fail so just omit missing packages from results.
        srcdir = self.make_testing_packages(["apkg"])
        deps = compare_dependencies(["apkg", "bpkg"], srcdir)
        self.assertEqual(deps, (["apkg"], []))

    def test_extra_package(self):
        srcdir = self.make_testing_packages(["apkg", "bpkg"])
        deps = compare_dependencies(["apkg"], srcdir)
        self.assertEqual(deps, (["apkg"], ["bpkg"]))

    def test_find_when_v1(self):
        # Package names with a dotted suffix can be included either with
        # the suffix or without. This may no longer be needed.
        srcdir = self.make_testing_packages(["apkg.v1", "bpkg.v2-unstable"])
        deps = compare_dependencies(["apkg.v1", "bpkg"], srcdir)
        self.assertEqual(deps, (["apkg.v1", "bpkg.v2-unstable"], []))


class TestGetDependencies(unittest.TestCase):

    def test_get_dependencies(self):
        m = mock.mock_open()
        m.return_value.__enter__.return_value = StringIO.StringIO(
            "gopkg.in/yaml.v1"
            "\tgit"
            "\t9f9df34309c04878acc86042b16630b0f696e1de"
            "\t2014-09-24T16:16:07Z\n"
            "launchpad.net/gnuflag"
            "\tbzr"
            "\troger.peppe@canonical.com-20140716064605-pk32dnmfust02yab"
            "\t13"
        )
        with mock.patch("check_dependencies.open", m, create=True):
            d = get_dependencies("a-file")
        m.assert_called_once_with("a-file")
        self.assertEqual(d, set(["gopkg.in/yaml.v1", "launchpad.net/gnuflag"]))
