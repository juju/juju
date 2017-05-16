import argparse
import mock
import subprocess

import git_gate
import tests


class TestParseArgs(tests.TestCase):

    def test_project_and_url(self):
        args = git_gate.parse_args(
            ["--project", "git.testing/project",
             "--project-url", "https://git.testing/project"])
        self.assertEqual(args.project, "git.testing/project")
        self.assertEqual(args.project_url, "https://git.testing/project")
        self.assertEqual(args.keep, False)

    def test_keep(self):
        args = git_gate.parse_args(
            ["--project", "git.testing/project",
             "--project-url", "https://git.testing/project", "--keep"])
        self.assertEqual(args.project, "git.testing/project")
        self.assertEqual(args.keep, True)

    def test_project_and_ref(self):
        args = git_gate.parse_args(
            ["--project", "git.testing/project", "--go-get-all",
             "--project-ref", "v1"])
        self.assertEqual(args.project, "git.testing/project")
        self.assertEqual(args.project_ref, "v1")

    def test_merging_other(self):
        args = git_gate.parse_args(
            ["--project", "git.testing/project", "--go-get-all",
             "--merge-url", "https://git.testing/proposed"])
        self.assertEqual(args.project, "git.testing/project")
        self.assertEqual(args.merge_url, "https://git.testing/proposed")
        self.assertEqual(args.merge_ref, "HEAD")

    def test_merging_other_ref(self):
        args = git_gate.parse_args(
            ["--project", "git.testing/project", "--go-get-all",
             "--merge-url", "https://git.testing/proposed",
             "--merge-ref", "feature"])
        self.assertEqual(args.project, "git.testing/project")
        self.assertEqual(args.merge_url, "https://git.testing/proposed")
        self.assertEqual(args.merge_ref, "feature")

    def test_project_with_deps(self):
        args = git_gate.parse_args(
            ["--project", "git.testing/project",
             "--project-url", "https://git.testing/project",
             "--dependencies", "git.testing/a", "git.testing/b"])
        self.assertEqual(args.project, "git.testing/project")
        self.assertEqual(args.project_url, "https://git.testing/project")
        self.assertEqual(args.dependencies, ["git.testing/a", "git.testing/b"])
        self.assertEqual(args.go_get_all, False)
        self.assertEqual(args.tsv_path, None)

    def test_project_with_go_deps(self):
        args = git_gate.parse_args(
            ["--project", "git.testing/project", "--go-get-all"])
        self.assertEqual(args.project, "git.testing/project")
        self.assertEqual(args.dependencies, None)
        self.assertEqual(args.go_get_all, True)
        self.assertEqual(args.tsv_path, None)

    def test_project_with_tsv_path(self):
        args = git_gate.parse_args(
            ["--project", "git.testing/project",
             "--project-url", "https://git.testing/project",
             "--tsv-path", "/a/file.tsv"])
        self.assertEqual(args.project, "git.testing/project")
        self.assertEqual(args.project_url, "https://git.testing/project")
        self.assertEqual(args.dependencies, None)
        self.assertEqual(args.go_get_all, False)
        self.assertEqual(args.tsv_path, "/a/file.tsv")

    def test_error_on_project_url_missing(self):
        with tests.parse_error(self) as stderr:
            git_gate.parse_args(["--project", "git.testing/project"])
        self.assertIn(
            "Must supply either --project-url or --go-get-all",
            stderr.getvalue())

    def test_error_go_get_feature_branch(self):
        with tests.parse_error(self) as stderr:
            git_gate.parse_args(
                ["--project", "git.testing/project",
                 "--project-url", "https://git.testing/project",
                 "--go-get-all", "--feature-branch"])
        self.assertIn(
            "Cannot use --feature-branch and --go-get-all together",
            stderr.getvalue())


class TestSubcommandError(tests.TestCase):

    def test_subcommand_error(self):
        proc_error = subprocess.CalledProcessError(1, ["git"])
        err = git_gate.SubcommandError("git", "clone", proc_error)
        self.assertEqual(str(err), "Subprocess git clone failed with code 1")


class TestGoTest(tests.TestCase):
    """
    Tests for go_test function.

    Class has setup that patches out each operation with relevent side effects,
    running a command, printing output, or changing directory. Those are
    recorded in order in the actions list, so each test can supply a set of
    arguments then just match the actions.
    """

    maxDiff = None

    def setUp(self):
        super(TestGoTest, self).setUp()
        # Patch out and record actions run as part of go_test()
        self.actions = []
        self.patch_action("git_gate.print_now", lambda s: ("print", s))
        self.patch_action("git_gate.SubcommandRunner.__call__",
                          lambda self, *args: (self.command,) + args)
        self.patch_action("os.chdir", lambda d: ("chdir", d))

        # Verify go commands run with GOPATH overridden
        real_runner = git_gate.SubcommandRunner

        def _check(command, environ=None):
            if command in ("go", "godeps"):
                self.assertIsInstance(environ, dict)
                self.assertEquals(environ.get("GOPATH"), "/tmp/fake")
            return real_runner(command, environ)

        patcher = mock.patch("git_gate.SubcommandRunner", side_effect=_check)
        patcher.start()
        self.addCleanup(patcher.stop)

    def patch_action(self, target, func):
        """Patch target recording each call in actions as wrapped by func."""
        def _record(*args, **kwargs):
            self.actions.append(func(*args, **kwargs))
        patcher = mock.patch(target, _record)
        patcher.start()
        self.addCleanup(patcher.stop)

    args = frozenset("project project_url project_ref feature_branch merge_url"
                     " merge_ref go_get_all dependencies tsv_path".split())

    @classmethod
    def make_args(cls, project, **kwargs):
        """Gives args like parse_args with all values defaulted to None."""
        if not cls.args.issuperset(kwargs):
            raise ValueError("Invalid arguments given: {!r}".format(kwargs))
        kwargs["project"] = project
        return argparse.Namespace(**dict((k, kwargs.get(k)) for k in cls.args))

    def test_get_test(self):
        args = self.make_args("git.testing/project", go_get_all=True)
        git_gate.go_test(args, "/tmp/fake")
        self.assertEqual(self.actions, [
            ('print', 'Getting git.testing/project and dependencies using go'),
            ('go', 'get', '-v', '-d', '-t', 'git.testing/project/...'),
            ('chdir', '/tmp/fake/src/git.testing/project'),
            ('go', 'build', 'git.testing/project/...'),
            ('go', 'test', 'git.testing/project/...')
        ])

    def test_get_merge_test(self):
        args = self.make_args("git.testing/project", go_get_all=True,
                              merge_url="https://git.testing/proposed",
                              merge_ref="HEAD")
        git_gate.go_test(args, "/tmp/fake")
        self.assertEqual(self.actions, [
            ('print', 'Getting git.testing/project and dependencies using go'),
            ('go', 'get', '-v', '-d', '-t', 'git.testing/project/...'),
            ('chdir', '/tmp/fake/src/git.testing/project'),
            ('print', 'Merging https://git.testing/proposed ref HEAD'),
            ('git', 'fetch', 'https://git.testing/proposed', 'HEAD'),
            ('git', 'merge', '--no-ff', '-m', 'Merged HEAD', 'FETCH_HEAD'),
            ('print', 'Updating git.testing/project dependencies using go'),
            ('go', 'get', '-v', '-d', '-t', 'git.testing/project/...'),
            ('go', 'build', 'git.testing/project/...'),
            ('go', 'test', 'git.testing/project/...')
        ])

    def test_get_merge_other_test(self):
        args = self.make_args("git.testing/project", go_get_all=True,
                              project_url="https://git.testing/project",
                              project_ref="v1",
                              merge_url="https://git.testing/proposed",
                              merge_ref="feature")
        git_gate.go_test(args, "/tmp/fake")
        self.assertEqual(self.actions, [
            ('print', 'Cloning git.testing/project from'
             ' https://git.testing/project'),
            ('git', 'clone', 'https://git.testing/project',
             '/tmp/fake/src/git.testing/project'),
            ('chdir', '/tmp/fake/src/git.testing/project'),
            ('print', 'Switching repository to v1'),
            ('git', 'checkout', 'v1'),
            ('print', 'Merging https://git.testing/proposed ref feature'),
            ('git', 'fetch', 'https://git.testing/proposed', 'feature'),
            ('git', 'merge', '--no-ff', '-m', 'Merged feature', 'FETCH_HEAD'),
            ('print', 'Updating git.testing/project dependencies using go'),
            ('go', 'get', '-v', '-d', '-t', 'git.testing/project/...'),
            ('go', 'build', 'git.testing/project/...'),
            ('go', 'test', 'git.testing/project/...')
        ])

    def test_deps_test(self):
        args = self.make_args("git.testing/project",
                              project_url="https://git.testing/project",
                              dependencies=["git.testing/a", "git.testing/b"])
        git_gate.go_test(args, "/tmp/fake")
        self.assertEqual(self.actions, [
            ('print', 'Cloning git.testing/project from'
             ' https://git.testing/project'),
            ('git', 'clone', 'https://git.testing/project',
             '/tmp/fake/src/git.testing/project'),
            ('chdir', '/tmp/fake/src/git.testing/project'),
            ('print', 'Getting git.testing/a and dependencies using go'),
            ('go', 'get', '-v', '-d', 'git.testing/a'),
            ('print', 'Getting git.testing/b and dependencies using go'),
            ('go', 'get', '-v', '-d', 'git.testing/b'),
            ('go', 'build', 'git.testing/project/...'),
            ('go', 'test', 'git.testing/project/...')
        ])

    def test_tsv_test(self):
        args = self.make_args("git.testing/project",
                              project_url="https://git.testing/project",
                              tsv_path="dependencies.tsv")
        git_gate.go_test(args, "/tmp/fake")
        self.assertEqual(self.actions, [
            ('print', 'Getting and installing godeps'),
            ('go', 'get', '-v', '-d', 'github.com/rogpeppe/godeps/...'),
            ('go', 'install', 'github.com/rogpeppe/godeps/...'),
            ('print', 'Cloning git.testing/project from'
             ' https://git.testing/project'),
            ('git', 'clone', 'https://git.testing/project',
             '/tmp/fake/src/git.testing/project'),
            ('chdir', '/tmp/fake/src/git.testing/project'),
            ('print', 'Getting dependencies using godeps from'
             ' /tmp/fake/src/git.testing/project/dependencies.tsv'),
            ('/tmp/fake/bin/godeps', '-u',
             '/tmp/fake/src/git.testing/project/dependencies.tsv'),
            ('go', 'build', 'git.testing/project/...'),
            ('go', 'test', 'git.testing/project/...')
        ])

    def test_feature_branch(self):
        args = self.make_args("vgo.testing/project.v2.feature",
                              project_url="https://git.testing/project",
                              project_ref="v2.feature",
                              feature_branch=True, dependencies=[])
        git_gate.go_test(args, "/tmp/fake")
        self.assertEqual(self.actions, [
            ('print', 'Cloning vgo.testing/project.v2 from'
             ' https://git.testing/project'),
            ('git', 'clone', 'https://git.testing/project',
             '/tmp/fake/src/vgo.testing/project.v2'),
            ('chdir', '/tmp/fake/src/vgo.testing/project.v2'),
            ('print', 'Switching repository to v2.feature'),
            ('git', 'checkout', 'v2.feature'),
            ('go', 'build', 'vgo.testing/project.v2/...'),
            ('go', 'test', 'vgo.testing/project.v2/...')
        ])

    def test_no_magic_dots(self):
        args = self.make_args("vgo.testing/project.dot.love",
                              project_url="https://git.testing/project",
                              project_ref="dot.love", dependencies=[])
        git_gate.go_test(args, "/tmp/fake")
        self.assertEqual(self.actions, [
            ('print', 'Cloning vgo.testing/project.dot.love from'
             ' https://git.testing/project'),
            ('git', 'clone', 'https://git.testing/project',
             '/tmp/fake/src/vgo.testing/project.dot.love'),
            ('chdir', '/tmp/fake/src/vgo.testing/project.dot.love'),
            ('print', 'Switching repository to dot.love'),
            ('git', 'checkout', 'dot.love'),
            ('go', 'build', 'vgo.testing/project.dot.love/...'),
            ('go', 'test', 'vgo.testing/project.dot.love/...')
        ])


class TestFromFeatureDir(tests.TestCase):
    """Tests for from_feature_dir function."""

    def test_boring(self):
        directory = git_gate.from_feature_dir("github.com/juju/juju")
        self.assertEqual(directory, "github.com/juju/juju")

    def test_gopkg(self):
        directory = git_gate.from_feature_dir("gopkg.in/juju/charm.v6")
        self.assertEqual(directory, "gopkg.in/juju/charm.v6")

    def test_gopkg_feature(self):
        directory = git_gate.from_feature_dir("gopkg.in/juju/charm.v6.minver")
        self.assertEqual(directory, "gopkg.in/juju/charm.v6")
