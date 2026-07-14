---
name: publish-learning
description: Publish only a curated and approved learning. Generate and show a preview first, obtain human approval for AGENTS.md, shared libraries or existing Skill changes, apply through royo-learn, verify, audit and rollback on failure.
license: MIT
metadata:
  author: RoyoTech
  version: "3.0.0"
  mcp_profile: admin
---

# Publish Learning

## Preconditions

- status is approved;
- curation specifies target and action;
- acceptance checks exist;
- no unresolved conflict;
- preview is current;
- required approval exists.

## Workflow

1. Call `learning_publication_preview`.
2. Inspect targets, diff, risk and verification.
3. Show high-impact previews to the user.
4. For AGENTS.md, shared scope or existing Skill updates, stop and obtain explicit human approval before continuing. See "Approval step" below.
5. Call `learning_publish` with the exact preview hash.
6. Verify result and registry.
7. If verification fails, ensure rollback occurred.
8. Report canonical target and publication ID.

## Approval step

There is no approval tool yet. The publication approval bound to a preview hash
lands in Recorrido C of the contract recovery, together with the destination-based
approval policies. Until it ships, this Skill deliberately cites no approval tool,
because a Skill must never cite a tool that does not exist (decision D15).

Consequences you must respect in the meantime:

- approval is obtained from the user in conversation, not through royo-learn;
- royo-learn does not yet enforce it, so the burden is entirely on you;
- never treat a preview as an authorization: a preview describes, it does not permit.

When Recorrido C lands, step 4 becomes a call to the approval tool and this section
is removed.

## Rules

- never edit targets directly when royo-learn can apply the approved operation;
- never reuse approval for another preview;
- never place a long procedure in AGENTS.md;
- never guess a global path;
- never bypass a dirty-target block silently;
- never publish secrets or project-private assumptions into shared scope.
