# Cloud Playbook Debrief Template

Status: Draft
Audience: SREs, operators, platform engineers
Purpose: Provide a single cloud-specific execution view by composing generic how-to steps with cloud-specific reference requirements.

## How to use this template

1. Pick one cloud target (for example AWS, Azure, GCE, OpenStack, EKS, AKS, GKE).
2. Fill all Required fields.
3. Keep normative command details in source how-to pages; summarize here and link out.
4. Keep cloud-specific constraints in source reference pages; summarize deltas here and link out.
5. Keep this page actionable for two major paths:
   - New controller path.
   - Existing controller path.

## 1. Debrief Summary

Required:
- Cloud target: <cloud-name>
- Cloud type: <machine|kubernetes>
- Primary operator goal: <one sentence>
- Typical completion time: <estimate>
- Preconditions confidence: <high|medium|low>

Optional:
- Change ticket or incident context: <link>
- Owner: <team/person>

## 2. Decision Gates

Required:
- Do you already have a suitable controller?
  - Yes: follow Existing controller path.
  - No: follow New controller path.
- Is this a public cloud or private cloud?
- Which auth pattern is preferred for this cloud?
- Is a secretless credential path available and acceptable?

Output:
- Chosen path: <new-controller|existing-controller>
- Chosen auth type: <auth-type>
- Fallback auth type: <auth-type>

## 3. Prerequisites and Readiness

Required:
- Account-side prerequisites.
- API endpoint and region/zone readiness.
- Identity and permission prerequisites.
- Local tool prerequisites.
- Any snap confinement or CLI interoperability constraints.

Checklist:
- [ ] API reachable
- [ ] Identity permissions validated
- [ ] Region/zone selected
- [ ] Local CLI/tools available
- [ ] Required files present (for example kubeconfig)

Source links:
- Generic how-to source: <link>
- Cloud-specific reference source: <link>

## 4. Cloud Profile Snapshot

Required:
- Supported auth types for this cloud.
- Recommended auth type.
- Explicitly discouraged auth patterns, if any.
- Secretless path availability and limits.
- Known caveats and compatibility notes.

Format:
- Auth matrix:
  - <auth-type-1>: <when to use>
  - <auth-type-2>: <when to use>
- Caveats:
  - <caveat-1>
  - <caveat-2>

## 5. Path A: New Controller

Goal: Add cloud, add credentials if needed, bootstrap, verify readiness.

Step block format:
1. Step name.
2. Why this step matters.
3. Command(s).
4. Expected output signal.
5. Failure indicators.
6. Recovery action.

Minimum sequence:
1. Verify cloud visibility.
2. Add cloud definition.
3. Add credentials if required.
4. Bootstrap controller.
5. Verify cloud/controller/model state.

## 6. Path B: Existing Controller

Goal: Add cloud and credentials to an existing controller without bootstrap.

Minimum sequence:
1. Confirm target controller context.
2. Add cloud (client or controller scope as needed).
3. Add credentials (client or controller scope as needed).
4. Attach credential to model where relevant.
5. Validate operational readiness.

## 7. Command Deck

Provide copy-paste command blocks in execution order per path.

Pattern:
- Command:

```text
<command>
```

- Expected success cues:
  - <cue-1>
  - <cue-2>

- If it fails:
  - Symptom: <text>
  - Likely cause: <text>
  - Fix: <text>

## 8. Verification and Exit Criteria

Required checks:
- Cloud appears as expected.
- Credentials appear as expected.
- Region defaults and overrides are correct.
- Controller/model target is correct.
- No blocking errors remain.

Exit criteria:
- [ ] Cloud configured
- [ ] Credential validated
- [ ] Controller objective met (bootstrapped or existing path complete)
- [ ] Team handoff notes captured

## 9. Failure Modes and Recovery

Top incidents table:
- Incident: <name>
- Detection: <signal>
- Immediate action: <action>
- Root cause checks: <list>
- Recovery command path: <link or snippet>

Include at least:
- Auth failure
- Endpoint/connectivity failure
- Invalid region/zone
- Permission denied
- Tooling or confinement mismatch

## 10. Security and Secrets Handling

Required:
- Where secrets are entered or loaded.
- Which paths avoid secret material persistence.
- Rotation guidance and cadence.
- Redaction expectations for logs and runbooks.

## 11. Operational Follow-through

Required:
- Update cloud definitions.
- Update or rotate credentials.
- Remove cloud and cleanup steps.
- Rollback strategy for partial configuration.

## 12. Source Traceability

List every source used to assemble this debrief.

Format:
- Generic how-to source:
  - <doc and anchor>
- Cloud reference source:
  - <doc and anchor>
- Command reference source:
  - <doc and anchor>

Rule:
- Every debrief step must map back to at least one source anchor.

## 13. Maintenance Notes

When updating this debrief:
1. Revalidate all source anchors.
2. Re-run commands in a test context where possible.
3. Update changed command flags and output cues.
4. Note date and version tested.

Metadata block:
- Last reviewed: <yyyy-mm-dd>
- Reviewed against Juju version: <version>
- Reviewer: <name>

## 14. Optional Automation Spec (for generated debriefs)

If you later automate composition, store these fields per cloud profile:
- cloud_id
- cloud_type
- recommended_auth
- fallback_auth
- supports_secretless_path
- requires_bootstrap_for_primary_path
- required_prereq_ids
- command_step_ids_new_controller
- command_step_ids_existing_controller
- known_failure_mode_ids

Composition rules:
1. Always emit Decision Gates.
2. Always emit both paths when both are valid.
3. If bootstrap is optional, never imply strict order.
4. Promote cloud-specific caveats near the first affected step.
5. Fail build if required source anchors are missing.

---

## Example Debrief Skeleton (to copy)

Title: Cloud Debrief: <cloud-name>

Summary:
- Goal: <goal>
- Scope: <new-controller|existing-controller|both>

Decision:
- Path selected: <path>
- Auth selected: <auth>

Do this:
1. <step>
2. <step>
3. <step>

Validate:
- <check>
- <check>

If blocked:
- <symptom> -> <action>

Sources:
- <source>
- <source>
