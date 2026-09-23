- **Agent instructions no longer carry dated or contradictory guidance**:
  `AGENTS.md` and the `bootstrap` skill now ask agents to read the DESIGN.md
  and SPEC.md sections a task touches instead of both documents in full
  (~150K tokens) before every task, and the unfollowable "read the current
  plan" line was removed from the spec-kit block. The `wizard` skill now
  points at `AGENTS.md` instead of a nonexistent `CLAUDE.md`, defers git
  writes to the user as the `guidelines` skill requires, exempts
  trial-disabled flags from dead-code removal, replaces generic
  exception/test-class advice with Go package-level test guidance, and drops
  all-caps pressure lines. The `cli-design` skill defers to the exit codes in
  SPEC.md §11.2 instead of prescribing 0/1 only.
