---
applyTo: "**/*"
---

# Main Instructions for GitHub Copilot Agents

This document defines the global rules all Copilot agents MUST follow in this repository.

## 1. Terminology (RFC 2119)

We use the key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", 
"MAY", and "OPTIONAL" as defined in RFC 2119 (and clarified by RFC 8174 when in uppercase). In short:

- MUST / SHALL / REQUIRED: An absolute requirement.
- MUST NOT / SHALL NOT: An absolute prohibition.
- SHOULD / RECOMMENDED: A strong recommendation; valid reasons may exist to deviate, but the full implications MUST 
  be understood and weighed before doing so.
- SHOULD NOT / NOT RECOMMENDED: A strong discouragement; deviations MUST be justified and low risk.
- MAY / OPTIONAL: Truly optional; use judgment.

References:
- RFC 2119: https://www.rfc-editor.org/rfc/rfc2119
- RFC 8174 (context clarification): https://www.rfc-editor.org/rfc/rfc8174

## 2. Agent Responsibilities

- Follow repository policies in `README.md`, `CONTRIBUTING.md`, `CODING.md`, `STYLE.md`, `AGENTS.md`, and any 
  referenced docs.
- Prefer minimal, safe, incremental changes. Avoid broad refactors unless explicitly authorized.
- When instructions are ambiguous, ask for clarification rather than guessing, unless the action is clearly low 
  risk and reversible.


## 3. Decision Logging and Justification

When deviating from a SHOULD/RECOMMENDED guideline, briefly justify in the PR description and commit message why 
deviation is lower risk or necessary.
