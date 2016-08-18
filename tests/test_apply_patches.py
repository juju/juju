"""Tests for apply_patches script used in tarball building."""

import contextlib
import mock
import os
import StringIO
import sys
import unittest

from apply_patches import (
    apply_patch,
    get_arg_parser,
    main,
)
from utils import (
    temp_dir,
)


class TestArgParser(unittest.TestCase):

    def parse_args(self, args):
        parser = get_arg_parser()
        return parser.parse_args(args)

    def test_defaults(self):
        args = self.parse_args(["patches/", "."])
        self.assertEqual("patches/", args.patchdir)
        self.assertEqual(".", args.srctree)
        self.assertEqual(False, args.dry_run)
        self.assertEqual(False, args.verbose)

    def test_dry_run(self):
        args = self.parse_args(["--dry-run", "patches/", "."])
        self.assertEqual("patches/", args.patchdir)
        self.assertEqual(".", args.srctree)
        self.assertEqual(True, args.dry_run)
        self.assertEqual(False, args.verbose)

    def test_verbose(self):
        args = self.parse_args(["patches/", ".", "--verbose"])
        self.assertEqual("patches/", args.patchdir)
        self.assertEqual(".", args.srctree)
        self.assertEqual(False, args.dry_run)
        self.assertEqual(True, args.verbose)


class TestApplyPatches(unittest.TestCase):

    def assert_open_only(self, open_mock, filename):
        self.assertEqual(
            open_mock.mock_calls,
            [
                mock.call(filename),
                mock.call().__enter__(),
                mock.call().__exit__(None, None, None),
            ]
        )

    def test_with_defaults(self):
        open_mock = mock.mock_open()
        with mock.patch("subprocess.call", autospec=True) as call_mock:
            with mock.patch("apply_patches.open", open_mock, create=True):
                apply_patch("some.patch", "a/tree")
        self.assert_open_only(open_mock, "some.patch")
        call_mock.assert_called_once_with(
            ["patch", "-f", "-u", "-p1", "-r-"],
            stdin=open_mock(),
            cwd="a/tree",
            )

    def test_dry_run_verbose(self):
        open_mock = mock.mock_open()
        with mock.patch("subprocess.call", autospec=True) as call_mock:
            with mock.patch("apply_patches.open", open_mock, create=True):
                apply_patch("trial.diff", "a/tree", True, True)
        self.assert_open_only(open_mock, "trial.diff")
        call_mock.assert_called_once_with(
            ["patch", "-f", "-u", "-p1", "-r-", "--dry-run", "--verbose"],
            stdin=open_mock(),
            cwd="a/tree",
            )

    def test_verbose(self):
        open_mock = mock.mock_open()
        with mock.patch("subprocess.call", autospec=True) as call_mock:
            with mock.patch("apply_patches.open", open_mock, create=True):
                apply_patch("scary.patch", "a/tree", verbose=True)
        self.assert_open_only(open_mock, "scary.patch")
        call_mock.assert_called_once_with(
            ["patch", "-f", "-u", "-p1", "-r-", "--verbose"],
            stdin=open_mock(),
            cwd="a/tree",
            )


class TestMain(unittest.TestCase):

    @contextlib.contextmanager
    def patch_output(self):
        messages = []

        def fake_print(message, file=None):
            self.assertEqual(file, sys.stderr)
            messages.append(message)

        with mock.patch("apply_patches.print", fake_print, create=True):
            yield messages

    def test_no_patchdir(self):
        stream = StringIO.StringIO()
        with temp_dir() as basedir:
            with self.assertRaises(SystemExit):
                with mock.patch("sys.stderr", stream):
                    main(["test", os.path.join(basedir, "missing"), basedir])
        self.assertRegexpMatches(
            stream.getvalue(), "Could not list patch directory: .*/missing")

    def test_no_srctree(self):
        stream = StringIO.StringIO()
        with temp_dir() as basedir:
            with self.assertRaises(SystemExit):
                with mock.patch("sys.stderr", stream):
                    main(["test", basedir, os.path.join(basedir, "missing")])
        self.assertRegexpMatches(
            stream.getvalue(), "Source tree '.*/missing' not a directory")

    def test_one_patch(self):
        with temp_dir() as basedir:
            patchdir = os.path.join(basedir, "patches")
            os.mkdir(patchdir)
            patchfile = os.path.join(patchdir, "sample.diff")
            open(patchfile, "w").close()
            with self.patch_output() as messages:
                with mock.patch(
                        "apply_patches.apply_patch", return_value=0,
                        autospec=True) as ap_mock:
                    main(["test", patchdir, basedir])
            ap_mock.assert_called_once_with(patchfile, basedir, False, False)
            self.assertEqual(messages, [
                u"Applying 1 patch", u"Applied patch 'sample.diff'"
            ])

    def test_bad_patches(self):
        with temp_dir() as basedir:
            patchdir = os.path.join(basedir, "patches")
            os.mkdir(patchdir)
            patch_a = os.path.join(patchdir, "a.patch")
            open(patch_a, "w").close()
            patch_b = os.path.join(patchdir, "b.patch")
            open(patch_b, "w").close()
            with self.patch_output() as messages:
                with mock.patch(
                        "apply_patches.apply_patch", return_value=1,
                        autospec=True) as ap_mock:
                    main(["test", "--verbose", patchdir, basedir])
            ap_mock.assert_called_once_with(patch_a, basedir, False, True)
            self.assertEqual(messages, [
                u"Applying 2 patches", u"Failed to apply patch 'a.patch'"
            ])

    def test_non_patch(self):
        with temp_dir() as basedir:
            patchdir = os.path.join(basedir, "patches")
            os.mkdir(patchdir)
            patch_a = os.path.join(patchdir, "readme.txt")
            open(patch_a, "w").close()
            with self.patch_output() as messages:
                with mock.patch(
                        "apply_patches.apply_patch", autospec=True) as ap_mock:
                    main(["test", "--verbose", patchdir, basedir])
            self.assertEqual(ap_mock.mock_calls, [])
            self.assertEqual(messages, [u"Applying 0 patches"])
