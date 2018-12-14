import platform


def get_platform():
    """Return the current OS platform.

    For example: if current os platform is Ubuntu then a string "ubuntu"
    will be returned (which is the name of the module).
    This string is used to decide which platform module should be imported.
    """
    # linux_distribution is deprecated and will be removed in Python 3.7
    # Warings *not* disabled, as we certainly need to fix this.
    tuple_platform = platform.linux_distribution()
    current_platform = tuple_platform[0]
    if "Ubuntu" in current_platform:
        return "ubuntu"
    elif "CentOS" in current_platform:
        return "centos"
    elif "debian" in current_platform:
        # Stock Python does not detect Ubuntu and instead returns debian.
        # Or at least it does in some build environments like Travis CI
        return "ubuntu"
    else:
        raise RuntimeError("This module is not supported on {}."
                           .format(current_platform))
