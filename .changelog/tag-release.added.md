- Add `make tag-release` to run the maintainer release end to end. It opens
  the `chore(release): prepare <tag>` changelog-assembly PR, waits for it to
  be merged, then signs and pushes the tag on the merge commit. The tag comes
  from `VERSION` or a single prompt and is validated first; a rerun with the
  same tag resumes from the PR's state.
