#!/bin/bash
# SessionStart hook for Claude Code cloud sessions. A fresh clone has none
# of the agent skills (.agents/skills and .claude/skills are gitignored and
# restored from skills-lock.json), and no Go module cache.
set -euo pipefail

if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

cd "$CLAUDE_PROJECT_DIR"

# Skills: restore from the lock into .agents/skills, then link each one
# where Claude Code looks, .claude/skills. The CLI exits 1 if any one
# skill in the lock is missing upstream, so success is judged by what
# was restored, and each failure is named. One retry if nothing came
# back (a clone can fail transiently). The restore fetches the latest
# versions and rewrites their hashes in skills-lock.json, so the lock is
# put back afterwards: a session never leaves it modified. The session
# starts whatever happens; the log is in /tmp.
log=/tmp/skills-restore.log
restore() { npx -y skills experimental_install >"$log" 2>&1 || true; }
restore
if ! ls .agents/skills/*/SKILL.md >/dev/null 2>&1; then
  sleep 2
  restore
fi
git checkout -- skills-lock.json 2>/dev/null || true
if ls .agents/skills/*/SKILL.md >/dev/null 2>&1; then
  mkdir -p .claude/skills
  for dir in .agents/skills/*/; do
    name=$(basename "$dir")
    ln -sfn "../../.agents/skills/$name" ".claude/skills/$name"
  done
  echo "skills: $(ls .claude/skills | wc -l) linked into .claude/skills"
else
  echo "skills: none restored (see $log)" >&2
fi
grep -a 'Failed to install' "$log" | sed 's/^.*Failed/skills: failed/' >&2 || true

# Go: the module cache and the pinned staticcheck CI runs, so
# `go vet`, `go test` and staticcheck need no download mid-session.
go mod download
go install honnef.co/go/tools/cmd/staticcheck@v0.8.1
