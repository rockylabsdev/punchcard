#!/usr/bin/env bash
#
# Slice 0 of the Punchcard build order — prove the Jira REST assumptions
# before any Swift is written against them. See docs/wayfinder/issues/05-jira-api-curl-spike.md
#
# Usage:
#   1. cp docs/jira-api/spike-env.example docs/jira-api/.spike-env
#   2. fill in the four values (the file is gitignored)
#   3. bash docs/jira-api/spike.sh            # everything
#      bash docs/jira-api/spike.sh search     # identity + the /search/jql probes only
#
# Every response body lands in docs/jira-api/responses/ as the fixture the
# Codable types get transcribed from at slice 2. The token is never written
# to disk by this script.
#
# `search` mode (wayfinder #80) skips the worklog write/delete round so a
# re-run for the search fixtures neither posts to the ticket nor churns the
# worklog fixtures. It still re-captures myself/serverinfo/issue-summary:
# redact.py derives the real site, account and ticket from those files, and
# the committed copies are already placeholders, so the probes must be raw.

set -uo pipefail

cd "$(dirname "$0")"
OUT=responses

MODE="${1:-all}"
case "$MODE" in
  all|search) ;;
  *) echo "usage: spike.sh [all|search]" >&2; exit 64 ;;
esac

if [ ! -f .spike-env ]; then
  echo "missing docs/jira-api/.spike-env — copy spike-env.example and fill it in" >&2
  exit 1
fi
# shellcheck disable=SC1091
source .spike-env

: "${JIRA_SITE:?}" "${JIRA_EMAIL:?}" "${JIRA_TOKEN:?}" "${JIRA_TICKET:?}"

# Normalize: accept company.atlassian.net or https://company.atlassian.net/
BASE="${JIRA_SITE#http://}"
BASE="${BASE#https://}"
BASE="https://${BASE%%/}"
echo "base url  : $BASE"
echo "ticket    : $JIRA_TICKET"
echo

AUTH=(-u "$JIRA_EMAIL:$JIRA_TOKEN")
JSON=(-H "Accept: application/json")

# $1 = label, rest = curl args. Writes body to $OUT/$1.json, sets LAST_STATUS.
LAST_STATUS=
probe() {
  local label="$1"; shift
  LAST_STATUS=$(curl -sS -o "$OUT/$label.json" -w '%{http_code}' "${AUTH[@]}" "${JSON[@]}" "$@")
  printf '%-28s HTTP %s\n' "$label" "$LAST_STATUS"
}

# ---------------------------------------------------------------- 1. identity
echo "== 1. credentials + deployment type =="
probe myself "$BASE/rest/api/3/myself"
# deploymentType here is the Cloud vs Server/DC answer
probe serverinfo "$BASE/rest/api/3/serverInfo"
echo

# ------------------------------------------------------------------- 2. read
echo "== 2. ticket read =="
probe issue-summary "$BASE/rest/api/3/issue/$JIRA_TICKET?fields=summary"
if [ "$MODE" = all ]; then
  # full issue, unfiltered — shows what we are choosing NOT to ask for
  probe issue-full "$BASE/rest/api/3/issue/$JIRA_TICKET"
  # 404 shape for a key that cannot exist — the "bad ticket key" error path
  probe issue-404 "$BASE/rest/api/3/issue/ZZZZ-99999?fields=summary"
fi
echo

if [ "$MODE" = all ]; then
# ------------------------------------------------------------- 3. worklog POST
echo "== 3. worklog write =="

# The started format Jira demands: yyyy-MM-dd'T'HH:mm:ss.SSSZ  (offset WITHOUT a colon).
# Recorded here as a literal because getting it wrong is the classic 400.
STARTED=$(date -u '+%Y-%m-%dT%H:%M:%S.000+0000')
echo "started   : $STARTED"

cat > "$OUT/request-worklog.json" <<EOF
{
  "timeSpentSeconds": 300,
  "started": "$STARTED",
  "comment": {
    "type": "doc",
    "version": 1,
    "content": [
      { "type": "paragraph", "content": [ { "type": "text", "text": "Punchcard slice-0 spike — delete me" } ] }
    ]
  }
}
EOF

probe worklog-post \
  -X POST \
  -H "Content-Type: application/json" \
  --data @"$OUT/request-worklog.json" \
  "$BASE/rest/api/3/issue/$JIRA_TICKET/worklog?adjustEstimate=leave"
POST_STATUS=$LAST_STATUS

# Probe the wrong-format hypothesis: RFC3339 with a colon in the offset, which is
# what a naive Swift ISO8601DateFormatter emits. Expect 400 if the theory holds.
BAD_STARTED=$(date -u '+%Y-%m-%dT%H:%M:%S.000+00:00')
cat > "$OUT/request-worklog-badstarted.json" <<EOF
{ "timeSpentSeconds": 60, "started": "$BAD_STARTED" }
EOF
probe worklog-post-badstarted \
  -X POST \
  -H "Content-Type: application/json" \
  --data @"$OUT/request-worklog-badstarted.json" \
  "$BASE/rest/api/3/issue/$JIRA_TICKET/worklog?adjustEstimate=leave"
BAD_POST_STATUS=$LAST_STATUS

# list, so we can see the collection shape and confirm what actually landed
probe worklog-list "$BASE/rest/api/3/issue/$JIRA_TICKET/worklog"
echo

# ----------------------------------------------------------- 4. cleanup
echo "== 4. cleanup (map's test strategy: delete the worklogs) =="

# $1 = label of the POST response file, $2 = its status. Deletes if it created one.
cleanup() {
  local file="$OUT/$1.json" status="$2" id del
  if [ "$status" != "201" ]; then
    printf '%-28s nothing to delete (HTTP %s)\n' "delete:$1" "$status"
    return
  fi
  id=$(/usr/bin/python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$file" 2>/dev/null)
  if [ -z "$id" ]; then
    printf '%-28s !! created but no id in response — delete by hand\n' "delete:$1"
    return
  fi
  del=$(curl -sS -o /dev/null -w '%{http_code}' -X DELETE "${AUTH[@]}" \
    "$BASE/rest/api/3/issue/$JIRA_TICKET/worklog/$id?adjustEstimate=leave")
  printf '%-28s HTTP %s  (id %s)\n' "delete:$1" "$del" "$id"
}

cleanup worklog-post "$POST_STATUS"
cleanup worklog-post-badstarted "$BAD_POST_STATUS"

# whatever survives cleanup, so a stray worklog is visible rather than silent
probe worklog-list-after "$BASE/rest/api/3/issue/$JIRA_TICKET/worklog"
echo
fi  # MODE = all

# ------------------------------------------------------------- 5. JQL search
# Wayfinder #80: the assigned pool (#55) is one GET /rest/api/3/search/jql call.
# Written from docs/research/jira-search-jql.md (research/jira-search-jql branch),
# not from memory. Expectations recorded there, to be checked against the wire:
#   - `fields` defaults to `id`, so fields=summary is required to get summaries
#   - 200 body is {issues:[{id,key,self,fields:{summary}}], isLast, nextPageToken?}
#     with NO `total`; nextPageToken is absent or null on the last page
#   - "first fifty" is maxResults=50 and no nextPageToken, nothing else
#   - a JQL with no restriction (unbounded) is a documented 400 on this endpoint
#   - the old /rest/api/3/search answers 410 since CHANGE-2046 (not probed: the
#     client never calls it, and a plain-text body is not a JSON fixture)
echo "== 5. assigned pool via /search/jql =="

# The spec's default (#55 §Jira), with the research ruling `resolution IS EMPTY`
# (the keyword form Atlassian documents; `= EMPTY` is the accepted synonym) and
# an explicit ORDER BY because no default order is documented.
ASSIGNED_JQL='assignee = currentUser() AND resolution IS EMPTY ORDER BY updated DESC'
echo "jql       : $ASSIGNED_JQL"

# -G puts --data-urlencode pairs on the query string of a GET, so curl does the
# percent-encoding of spaces, `=` and `()` in the JQL rather than this script.
probe search-jql -G \
  --data-urlencode "jql=$ASSIGNED_JQL" \
  --data-urlencode "fields=summary" \
  --data-urlencode "maxResults=50" \
  "$BASE/rest/api/3/search/jql"

# Deliberately malformed JQL (unclosed parenthesis): the 400 envelope and the
# parser's message. This is the shape a user's bad `assignedJql` produces.
probe search-jql-badjql -G \
  --data-urlencode "jql=assignee = currentUser( AND resolution IS EMPTY" \
  --data-urlencode "fields=summary" \
  --data-urlencode "maxResults=50" \
  "$BASE/rest/api/3/search/jql"

# Syntactically valid but unbounded JQL: the documented second 400 cause, and the
# other way a user's `assignedJql` override can be refused.
probe search-jql-unbounded -G \
  --data-urlencode "jql=ORDER BY updated DESC" \
  --data-urlencode "fields=summary" \
  --data-urlencode "maxResults=50" \
  "$BASE/rest/api/3/search/jql"

# Bad token: the 401 body on this endpoint (envelope, or empty with
# WWW-Authenticate), so the jira client's ErrUnauthorized mapping is from the wire.
GOOD_AUTH=("${AUTH[@]}")
AUTH=(-u "$JIRA_EMAIL:not-a-token")
probe search-jql-401 -G \
  --data-urlencode "jql=$ASSIGNED_JQL" \
  --data-urlencode "fields=summary" \
  --data-urlencode "maxResults=50" \
  "$BASE/rest/api/3/search/jql"
AUTH=("${GOOD_AUTH[@]}")

# A quick read of the shape so the author can eyeball it before redacting.
/usr/bin/python3 - "$OUT/search-jql.json" <<'PY' || echo "search-jql.json is not JSON — see the status above"
import json, sys
d = json.load(open(sys.argv[1]))
issues = d.get("issues", [])
print(f"search-jql.json: top-level keys {sorted(d)}; {len(issues)} issue(s); "
      f"isLast={d.get('isLast')!r}; nextPageToken present={'nextPageToken' in d}; "
      f"total present={'total' in d}")
if issues:
    print(f"  per-issue keys {sorted(issues[0])}; fields keys {sorted(issues[0].get('fields', {}))}")
PY
echo

echo "responses in docs/jira-api/$OUT/"
[ "$MODE" = all ] && echo "check worklog-list-after is empty before you walk away"
echo
echo "!! these are RAW responses carrying the real site, account and ticket."
echo "   before committing:  python3 docs/jira-api/redact.py"
