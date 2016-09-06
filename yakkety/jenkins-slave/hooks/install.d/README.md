# hooks/install.d

This directory can be used to extend the function of the jenkins master
charm without changing any of the base hooks.

Files must be executable otherwise the install hook (which is also run
on upgrade-charm and config-changed hooks) will not execute them.
