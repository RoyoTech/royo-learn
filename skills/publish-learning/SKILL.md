---
name: publish-learning
description: Publish only a curated and approved learning. Generate and show a preview first, obtain human approval for AGENTS.md, shared libraries or existing Skill changes, apply through royo-learn, verify, audit and rollback on failure.
license: MIT
metadata:
  author: RoyoTech
  version: "2.0.0"
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
4. For AGENTS.md, shared scope or existing Skill updates, call `learning_approve` only after explicit human approval.
5. Call `learning_publish` with the exact preview hash.
6. Verify result and registry.
7. If verification fails, ensure rollback occurred.
8. Report canonical target and publication ID.

## Rules

- never edit targets directly when royo-learn can apply the approved operation;
- never reuse approval for another preview;
- never place a long procedure in AGENTS.md;
- never guess a global path;
- never bypass a dirty-target block silently;
- never publish secrets or project-private assumptions into shared scope.
