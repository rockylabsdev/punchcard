#!/usr/bin/env bash
# release.sh X.Y.Z[-rc.N]: pin the Claude Code plugin to the release about
# to be tagged, commit, and tag vX.Y.Z or vX.Y.Z-rc.N (ADR 0007).
#
# An -rc.N tag is a pre-release for testers: GoReleaser marks the GitHub
# release a pre-release and skips the Homebrew tap, so testers download the
# tarball with `gh release download` (docs/guide/testers.md).
#
# It bumps, together:
#   plugin/.claude-plugin/plugin.json       "version": "X.Y.Z"
#   .claude-plugin/marketplace.json         "ref": "vX.Y.Z"
# so an install or update of punchcard@punchcard gets the mod and skill from
# the same tag as the binary. It refuses a dirty tree, a branch other than
# main and an existing tag. It never pushes: it prints the push command, and
# pushing the tag runs .github/workflows/release.yml.
set -euo pipefail

die() { echo "release: $*" >&2; exit 1; }

[ $# -eq 1 ] || die "usage: scripts/release.sh X.Y.Z[-rc.N]"
version=${1#v}
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?$ ]] || die "version must be X.Y.Z or X.Y.Z-rc.N, got '$1'"
tag="v$version"

root=$(git rev-parse --show-toplevel) || die "not in a git repository"
cd "$root"

plugin=plugin/.claude-plugin/plugin.json
market=.claude-plugin/marketplace.json
[ -f "$plugin" ] || die "missing $plugin"
[ -f "$market" ] || die "missing $market"

[ -z "$(git status --porcelain)" ] || die "working tree is not clean; commit or stash first"
branch=$(git symbolic-ref --short -q HEAD || true)
[ "$branch" = main ] || die "release from main, not '${branch:-a detached HEAD}'"
if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  die "tag $tag already exists"
fi

# Targeted edits, so the files keep their layout. Each must hit exactly one
# key, which is checked afterwards.
perl -pi -e 's/("version"\s*:\s*")[^"]*(")/${1}'"$version"'${2}/' "$plugin"
perl -pi -e 's/("ref"\s*:\s*")[^"]*(")/${1}'"$tag"'${2}/' "$market"

[ "$(grep -c "\"version\": *\"$version\"" "$plugin")" = 1 ] || die "$plugin: version not set to $version"
[ "$(grep -c "\"ref\": *\"$tag\"" "$market")" = 1 ] || die "$market: ref not set to $tag"

if [ -z "$(git status --porcelain)" ]; then
  echo "release: plugin.json and marketplace.json already name $tag"
else
  git add "$plugin" "$market"
  git commit -q -m "Release $tag"
fi
git tag -a "$tag" -m "punchcard $tag"

echo "Tagged $tag at $(git rev-parse --short HEAD). Nothing is pushed yet; to publish:"
echo
echo "  git push --atomic origin main $tag"
