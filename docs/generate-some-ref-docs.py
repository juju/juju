import os
import re
import shutil
import subprocess

###################################################################
# Auto-generation of some documentations in the reference directory
###################################################################

def _major_minor_from_version_string(version):
    """Get a tuple of version from a juju version string.

    Note that release of juju use Major.Minor.Patch (eg, 3.6.0) but releases will use Major.Minor-betaBeta.
    This returns a tuple of either (3,6,0) or (3, 6, 'beta5').
    If neither can be found, this returns None
    """
    version_re = re.compile(r'(?P<major>\d+)\.(?P<minor>\d+)(\.(?P<patch>\d+)|-(?P<beta>beta\d+))(-.*)?')
    m = version_re.match(version)
    if m is None:
        return None
    return (int(m['major']), int(m['minor']))


def _extract_version_from_version_go(version_file):
    """Extract the version string from a juju version.go file"""
    # Note that this assumes Major and Minor are integers, but patch might be an integer or something like beta5.
    version_re = re.compile(r'const version = "(?P<version>[^"]*)"')
    for line in version_file:
        m = version_re.match(line)
        if m is None:
            continue
        version = m['version']
        return _major_minor_from_version_string(version), version
    return None, None


def get_tree_juju_version():
    """Read the version of juju as reported by the juju branch that we're building."""
    try:
        # This is the location in Juju 3.6
        with open('../version/version.go', 'rt') as version_file:
            major_minor, version = _extract_version_from_version_go(version_file)
    except FileNotFoundError:
        # This is the location in Juju 4.0
        with open('../core/version/version.go', 'rt') as version_file:
            major_minor, version = _extract_version_from_version_go(version_file)
    if major_minor is None:
        raise RuntimeError("could not determine the version of Juju for this directory")
    return major_minor, version


def get_juju_version():
    """Check to see what version of Juju we are running."""
    # There is probably more that could be done here about all the possible
    # version strings juju spits out, but this should cover stripping things
    # like 'genericlinux' and the architecture out.
    result = subprocess.run(['juju', 'version'], capture_output=True, text=True)
    version = result.stdout.rstrip()
    major_minor = _major_minor_from_version_string(version)
    if major_minor is None:
        raise RuntimeError('could not determine version from `juju version`: {}'.format(version))
    return major_minor, version


def generate_cli_docs():
    cli_dir = "reference/juju-cli/"
    generated_cli_docs_dir = cli_dir + "list-of-juju-cli-commands/"
    cli_index_header = cli_dir + 'cli_index'

    tree_major_minor, tree_version = get_tree_juju_version()
    juju_major_minor, juju_version = get_juju_version()
    if tree_major_minor != juju_major_minor:
        warning = ("refusing to rebuild docs with a mismatched minor juju version.\n" +
                "Found juju {} in $PATH, but the tree reports version {}".format(juju_version, tree_version))
        print(warning)
        raise RuntimeError(warning)
    else:
        print("generating cli command docs using juju version found in path: {} for tree version {}".format(juju_version, tree_version))

    # Remove existing cli folder to regenerate it
    if os.path.exists(generated_cli_docs_dir):
        shutil.rmtree(generated_cli_docs_dir)

    # Generate the CLI docs using "juju documentation" command.
    subprocess.run(['juju', 'documentation', '--split', '--no-index', '--out', generated_cli_docs_dir],
                   check=True)

    for page in os.listdir(generated_cli_docs_dir):
        title = "`juju " + page[:-3]+ "`"
        anchor = "command-juju-" + page[:-3]
        # Add sphinx names to each file.
        with open(os.path.join(generated_cli_docs_dir, page), 'r+') as mdfile:
            content = mdfile.read()
            # Remove trailing seperated (e.g. ----)
            content = content.rstrip(" -\n")
            mdfile.seek(0, 0)
            mdfile.write('(' + anchor + ')=\n' +
                         '# ' + title + '\n' +
                         content)

    # Add in the index file containing the command list.
    subprocess.run(['cp', cli_index_header, generated_cli_docs_dir + 'index.md'])


def generate_controller_config_docs():
    config_reference_dir = 'reference/configuration/'
    controller_config_file = config_reference_dir + 'list-of-controller-configuration-keys.md'
    controller_config_header = config_reference_dir + 'list-of-controller-configuration-keys.header'

    # Generate the controller config using script. The first argument of the script
    # is the root directory of the juju source code. This is the parent directory
    # so use pass '..'.
    result = subprocess.run(['go', 'run', '../scripts/md-gen/controller-config/main.go', '..'],
                            capture_output=True, text=True)
    if result.returncode != 0:
        raise Exception("error auto-generating controller config: " + result.stderr)

    # Remove existing controller config
    if os.path.exists(controller_config_file):
        os.remove(controller_config_file)

    # Copy header for the controller config doc page in.
    subprocess.run(['cp', controller_config_header, controller_config_file])

    # Append autogenerated docs.
    with open(controller_config_file, 'a') as f:
        f.write(result.stdout)

    print("generated controller config key list")


def generate_hook_command_docs():
    hook_commands_reference_dir = 'reference/hook-command/'
    generated_hook_commands_dir = hook_commands_reference_dir + 'list-of-hook-commands/'
    hook_index_header = hook_commands_reference_dir + 'hook_index'

    # Remove existing hook command folder to regenerate it
    if os.path.exists(generated_hook_commands_dir):
        shutil.rmtree(generated_hook_commands_dir)

    # Generate the hook commands doc using script.
    result = subprocess.run(['go', 'run', '../scripts/md-gen/hook-commands/main.go', generated_hook_commands_dir],
                            check=True)
    if result.returncode != 0:
        raise Exception("error auto-generating hook commands: " + result.stderr)

    # Remove 'help' and 'documentation' files as they are not needed.
    if os.path.exists(generated_hook_commands_dir + 'help.md'):
        os.remove(generated_hook_commands_dir + 'help.md')
    if os.path.exists(generated_hook_commands_dir + 'documentation.md'):
        os.remove(generated_hook_commands_dir + 'documentation.md')

    for page in os.listdir(generated_hook_commands_dir):
        title = "`" + page[:-3] + "`"
        anchor = "hook-command-" + page[:-3]
        # Add sphinx names to each file.
        with open(os.path.join(generated_hook_commands_dir, page), 'r+') as mdfile:
            content = mdfile.read()
            # Remove trailing seperated (e.g. ----)
            content = content.rstrip(" -\n")
            mdfile.seek(0, 0)
            mdfile.write('(' + anchor + ')=\n' +
                         '# ' + title + '\n' +
                         content)

    # Add in the index file containing the command list.
    subprocess.run(['cp', hook_index_header, generated_hook_commands_dir + 'index.md'])

    print("generated hook command list")

generate_cli_docs()
generate_controller_config_docs()
generate_hook_command_docs()
