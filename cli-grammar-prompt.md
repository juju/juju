# Task: Standardize CLI Flag Descriptions in Juju

**Goal:** Refactor all CLI flag descriptions in the `juju` repository to follow a consistent grammatical pattern.

**Pattern Requirements:**
1.  **Grammar:** Use the **3rd person singular present tense** (sentential form).
    *   *Bad:* "Specify the output format" (Imperative)
    *   *Good:* "Specifies the output format" (3rd Person Sentential)
2.  **Punctuation:** Every description must end with a **full stop (period)**.

**Scope:**
*   Focus on Go files within `cmd/juju/` and `cmd/modelcmd/`.
*   Look for `SetFlags` methods and calls to `f.StringVar`, `f.BoolVar`, `f.IntVar`, `f.Var`, etc.

**Examples:**

| Flag | Old Description | New Description |
| :--- | :--- | :--- |
| `--output` | "Specify output format" | "Specifies the output format." |
| `--dry-run` | "Simulate the deployment" | "Simulates the deployment." |
| `--force` | "Allow unsafe operations" | "Allows unsafe operations." |
| `--client` | "Mark this as a client operation" | "Performs the operation on the local client." |
| `--controller` | "Specifies the controller to operate in" | "Performs the operation in the specified controller." |

**Instructions:**
1.  Scan the codebase for flag definitions.
2.  Identify descriptions starting with imperative verbs (e.g., "Specify", "Run", "Set", "Enable", "Disable", "Show", "List").
3.  Convert them to 3rd person singular ("Specifies", "Runs", "Sets", "Enables", "Disables", "Shows", "Lists").
4.  Ensure all descriptions (even those already in 3rd person) end with a period.
5.  Apply these changes to the files.
