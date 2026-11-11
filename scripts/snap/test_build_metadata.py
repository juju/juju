#!/usr/bin/env python3
"""Unit tests for scripts/snap/build_metadata.py."""

import contextlib
import importlib.util
import io
import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

MODULE_PATH = Path(__file__).with_name("build_metadata.py")
SPEC = importlib.util.spec_from_file_location("build_metadata", MODULE_PATH)
build_metadata = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(build_metadata)

EDGE_URL = "https://launchpad.net/~owner/+snap/juju-edge-2.9/+build/42"
RELEASE_URL = (
    "https://launchpad.net/~owner/+snap/juju-release-2.9/+build/42"
)
COMMIT = "c63bc5954eb4e29d900b199a25107ee214f1493d"


class DetectModeTest(unittest.TestCase):
    def test_local_without_image_info(self):
        self.assertEqual(build_metadata.detect_mode({}), "local")

    def test_local_without_build_url(self):
        self.assertEqual(
            build_metadata.detect_mode({"build-request-id": "lp-42"}),
            "local",
        )

    def test_edge_recipe(self):
        self.assertEqual(
            build_metadata.detect_mode({"build_url": EDGE_URL}), "edge"
        )

    def test_release_recipes(self):
        for recipe in ("juju-release", "juju-release-2.9"):
            url = f"https://launchpad.net/~owner/+snap/{recipe}/+build/42"
            with self.subTest(recipe=recipe):
                self.assertEqual(
                    build_metadata.detect_mode({"build_url": url}),
                    "release",
                )

    def test_unrecognised_recipe_fails(self):
        url = "https://launchpad.net/~owner/+snap/juju-2.9/+build/42"
        with self.assertRaisesRegex(SystemExit, "juju-2.9"):
            build_metadata.detect_mode({"build_url": url})

    def test_non_launchpad_build_url_is_local(self):
        url = "https://github.com/owner/repo/actions/runs/42"
        self.assertEqual(
            build_metadata.detect_mode({"build_url": url}), "local"
        )


class BuildNumberTest(unittest.TestCase):
    def test_non_edge_modes_have_no_build_number(self):
        info = {"build-request-id": "lp-42"}
        for mode in ("local", "release"):
            with self.subTest(mode=mode):
                self.assertEqual(
                    build_metadata.build_number(info, mode), ""
                )

    def test_edge_build_number(self):
        for value, expected in (
            ("lp-42", "42"),
            ("42", "42"),
            (42, "42"),
        ):
            with self.subTest(value=value):
                self.assertEqual(
                    build_metadata.build_number(
                        {"build-request-id": value}, "edge"
                    ),
                    expected,
                )

    def test_edge_invalid_build_number(self):
        for info in (
            {},
            {"build-request-id": ""},
            {"build-request-id": "lp-abc"},
        ):
            with self.subTest(info=info):
                self.assertEqual(
                    build_metadata.build_number(info, "edge"), ""
                )


class ImageInfoTest(unittest.TestCase):
    def test_invalid_json_fails(self):
        with mock.patch.dict(
            os.environ, {"SNAPCRAFT_IMAGE_INFO": "{"}
        ), self.assertRaisesRegex(SystemExit, "invalid SNAPCRAFT_IMAGE_INFO"):
            build_metadata.image_info()

    def test_non_object_json_fails(self):
        with mock.patch.dict(
            os.environ, {"SNAPCRAFT_IMAGE_INFO": '["not", "an", "object"]'}
        ), self.assertRaisesRegex(SystemExit, "expected a JSON object"):
            build_metadata.image_info()


class BaseVersionTest(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.project_dir = self._tmp.name

    def _write_version(self, relative, content):
        path = Path(self.project_dir) / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)

    def test_version_file(self):
        self._write_version("version/version.go", 'const version = "3.6.1"\n')
        self.assertEqual(
            build_metadata.base_version(self.project_dir), "3.6.1"
        )

    def test_core_version_fallback(self):
        self._write_version(
            "core/version/version.go",
            '// Copyright\n\nconst version = "3.6.1"\n',
        )
        self.assertEqual(
            build_metadata.base_version(self.project_dir), "3.6.1"
        )

    def test_version_file_takes_precedence(self):
        self._write_version("version/version.go", 'const version = "3.6.1"\n')
        self._write_version(
            "core/version/version.go", 'const version = "4.0.0"\n'
        )
        self.assertEqual(
            build_metadata.base_version(self.project_dir), "3.6.1"
        )

    def test_missing_version_fails(self):
        with self.assertRaisesRegex(
            SystemExit, "could not find the Juju version"
        ):
            build_metadata.base_version(self.project_dir)


class GitTreeStateTest(unittest.TestCase):
    def test_no_commit_is_archive(self):
        self.assertEqual(
            build_metadata.git_tree_state("/does/not/matter", ""), "archive"
        )

    def test_clean_tree(self):
        with mock.patch.object(
            build_metadata.subprocess, "check_output", return_value=""
        ):
            self.assertEqual(
                build_metadata.git_tree_state("/repo", COMMIT), "clean"
            )

    def test_dirty_tree(self):
        with mock.patch.object(
            build_metadata.subprocess,
            "check_output",
            return_value=" M version/version.go\n",
        ):
            self.assertEqual(
                build_metadata.git_tree_state("/repo", COMMIT), "dirty"
            )

    def test_git_failure_is_archive(self):
        with mock.patch.object(
            build_metadata.subprocess, "check_output", side_effect=OSError
        ):
            self.assertEqual(
                build_metadata.git_tree_state("/repo", COMMIT), "archive"
            )


class MetadataFileTest(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.path = Path(self._tmp.name) / "metadata.json"

    def test_input_skips_computation(self):
        self.path.write_text(
            json.dumps({"version": "2.9.61", "grade": "stable"})
        )
        with mock.patch(
            "sys.argv",
            [
                "build_metadata.py",
                "--input",
                str(self.path),
                "--format",
                "shell",
            ],
        ), mock.patch.object(
            build_metadata,
            "metadata",
            side_effect=AssertionError("should not be computed"),
        ), contextlib.redirect_stdout(io.StringIO()) as out:
            build_metadata.main()
        self.assertEqual(
            out.getvalue(), "version=2.9.61\ngrade=stable\n"
        )

    def test_output_writes_computed_values(self):
        values = {"version": "2.9.61", "grade": "stable"}
        with mock.patch(
            "sys.argv",
            ["build_metadata.py", "--output", str(self.path)],
        ), mock.patch.object(
            build_metadata, "metadata", return_value=values
        ), contextlib.redirect_stdout(io.StringIO()):
            build_metadata.main()
        self.assertEqual(json.loads(self.path.read_text()), values)

    def _run_input(self, values):
        self.path.write_text(json.dumps(values))
        with mock.patch(
            "sys.argv",
            ["build_metadata.py", "--input", str(self.path)],
        ), contextlib.redirect_stdout(io.StringIO()):
            build_metadata.main()

    def test_input_missing_version_fails(self):
        with self.assertRaisesRegex(SystemExit, "missing key 'version'"):
            self._run_input({"grade": "stable"})

    def test_input_missing_grade_fails(self):
        with self.assertRaisesRegex(SystemExit, "missing key 'grade'"):
            self._run_input({"version": "2.9.61"})

    def test_input_non_object_fails(self):
        with self.assertRaisesRegex(SystemExit, "expected a JSON object"):
            self._run_input(["version", "grade"])


class MetadataTest(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.project_dir = self._tmp.name
        version_dir = Path(self.project_dir) / "version"
        version_dir.mkdir()
        (version_dir / "version.go").write_text(
            'const version = "2.9.61"\n'
        )

    def _metadata(self, info, commit=COMMIT):
        with mock.patch.object(
            build_metadata, "image_info", return_value=info
        ), mock.patch.object(
            build_metadata, "git_commit", return_value=commit
        ), mock.patch.object(
            build_metadata, "git_tree_state", return_value="clean"
        ):
            return build_metadata.metadata(self.project_dir)

    def test_local(self):
        values = self._metadata({}, commit="")
        self.assertEqual(values["mode"], "local")
        self.assertEqual(values["version"], "2.9.61")
        self.assertEqual(values["grade"], "devel")
        self.assertEqual(values["build"], "")
        self.assertEqual(values["oci_namespace"], "")

    def test_edge(self):
        values = self._metadata(
            {"build_url": EDGE_URL, "build-request-id": "lp-42"}
        )
        self.assertEqual(values["mode"], "edge")
        self.assertEqual(values["version"], "2.9.61-c63bc59")
        self.assertEqual(values["grade"], "devel")
        self.assertEqual(values["build"], "42")
        self.assertEqual(
            values["oci_namespace"], build_metadata.EDGE_OCI_NAMESPACE
        )

    def test_release(self):
        values = self._metadata(
            {"build_url": RELEASE_URL, "build-request-id": "lp-42"}
        )
        self.assertEqual(values["mode"], "release")
        self.assertEqual(values["version"], "2.9.61")
        self.assertEqual(values["grade"], "stable")
        self.assertEqual(values["build"], "")
        self.assertEqual(
            values["oci_namespace"], build_metadata.RELEASE_OCI_NAMESPACE
        )

    def test_edge_requires_git_checkout(self):
        with self.assertRaisesRegex(SystemExit, "requires a git checkout"):
            self._metadata(
                {"build_url": EDGE_URL, "build-request-id": "lp-42"},
                commit="",
            )

    def test_edge_requires_build_number(self):
        with self.assertRaisesRegex(SystemExit, "build-request-id"):
            self._metadata({"build_url": EDGE_URL})


if __name__ == "__main__":
    unittest.main()
