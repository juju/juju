(command-juju-agree)=
# `juju agree`
> See also: [agreements](#agreements)

## Summary
Agrees to terms.

## Usage
```juju agree [options] <term>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `-c`, `--controller` |  | Performs the operation in the specified controller. |
| `--yes` | false | Agrees to terms non-interactively. |

## Examples

Displays terms for somePlan revision 1 and prompts for agreement:

    juju agree somePlan/1

Displays the terms for revision 1 of somePlan, revision 2 of otherPlan, and prompts for agreement:

    juju agree somePlan/1 otherPlan/2

Agree to the terms without prompting:

    juju agree somePlan/1 otherPlan/2 --yes


## Details

Agrees to the terms required by a charm.

When deploying a charm that requires agreement to terms, use `juju agree` to
view the terms and agree to them. Then the charm may be deployed.

Once terms have been agreed to, the user will not be prompted to view them again.