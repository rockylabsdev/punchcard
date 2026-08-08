#!/usr/bin/env python3
"""Scrub work-identifying values out of the captured Jira fixtures.

`spike.sh` talks to a real corporate Jira, so its raw responses carry a real
site, account, and ticket. This rewrites them to placeholders **in place**,
preserving every format exactly — id-as-string, the colon in accountId, the
offset-without-colon in timestamps — so the files stay valid slice-2 fixtures.

The real values are read out of the captured responses at runtime and never
appear in this file. Run it after every `spike.sh` run, before committing:

    python3 docs/jira-api/redact.py

Idempotent: running it on already-redacted files is a no-op.
"""

import json
import re
import sys
from pathlib import Path

HERE = Path(__file__).parent
OUT = HERE / "responses"

# Placeholders. Shapes deliberately mirror the originals.
FAKE_HOST = "example.atlassian.net"
FAKE_EMAIL = "user@example.com"
FAKE_NAME = "Example User"
FAKE_UUID = "11111111-2222-3333-4444-555555555555"
FAKE_GRAVATAR = "a1b2c3d4e5f60718293a4b5c6d7e8f90"
FAKE_KEY = "EX-1234"
FAKE_ISSUE_ID = "100001"
FAKE_WORKLOG_ID = "110001"
FAKE_SUMMARY = "Example ticket summary for fixture purposes"


def load(name):
    p = OUT / name
    return json.loads(p.read_text()) if p.exists() else None


def build_mapping():
    """Derive real -> placeholder from whatever fixtures are present."""
    m = {}

    info = load("serverinfo.json")
    if info and info.get("baseUrl"):
        host = info["baseUrl"].split("://", 1)[-1].strip("/")
        m[host] = FAKE_HOST

    me = load("myself.json")
    if me:
        if me.get("emailAddress"):
            m[me["emailAddress"]] = FAKE_EMAIL
        if me.get("displayName"):
            m[me["displayName"]] = FAKE_NAME
        if me.get("accountId"):
            # "<prefix>:<uuid>" — keep the prefix and the colon, swap the uuid.
            uuid = me["accountId"].split(":")[-1]
            m[uuid] = FAKE_UUID
            # Prose often truncates the uuid ("<prefix>:<first-block>-…"), which the full
            # value would never match. Map the leading block on its own too.
            if "-" in uuid:
                m[uuid.split("-")[0]] = FAKE_UUID.split("-")[0]
        avatar = (me.get("avatarUrls") or {}).get("48x48", "")
        # gravatar hash
        for h in re.findall(r"/avatar/([0-9a-f]{32})", avatar):
            m[h] = FAKE_GRAVATAR
        # Initials fallback image, e.g. XY-6.png — these are the user's initials.
        # The URL is percent-encoded inside the ?d= parameter, so match the bare
        # filename token rather than a path that only exists once decoded.
        for ini in re.findall(r"initials%2F([A-Z]{1,3})-|initials/([A-Z]{1,3})-", avatar):
            token = ini[0] or ini[1]
            m[f"{token}-6.png"] = "EU-6.png"
            m[f"{token}-"] = "EU-"

    issue = load("issue-summary.json")
    if issue:
        if issue.get("key"):
            m[issue["key"]] = FAKE_KEY
        if issue.get("id"):
            m[issue["id"]] = FAKE_ISSUE_ID
        # The summary is free text describing real work — the one field whose
        # content, not just its format, has to go.
        summary = (issue.get("fields") or {}).get("summary")
        if summary:
            m[summary] = FAKE_SUMMARY

    wl = load("worklog-post.json")
    if wl and wl.get("id"):
        m[wl["id"]] = FAKE_WORKLOG_ID

    # Drop identity mappings whose source is already a placeholder (idempotency)
    # and any no-op pairs.
    return {k: v for k, v in m.items() if k and k != v}


def apply(mapping, paths):
    # Longest first, so a value that contains another is replaced whole.
    keys = sorted(mapping, key=len, reverse=True)
    touched = []
    for p in paths:
        if not p.exists() or p.is_dir():
            continue
        original = p.read_text()
        text = original
        for k in keys:
            text = text.replace(k, mapping[k])
            # Percent-encoded forms — values appear inside URL query params
            # (accountId as 712020%3A..., avatar paths as ...%2Finitials%2F...).
            enc = k.replace(":", "%3A").replace("/", "%2F")
            if enc != k:
                text = text.replace(enc, mapping[k].replace(":", "%3A").replace("/", "%2F"))
        if text != original:
            p.write_text(text)
            touched.append(p)
    return touched


def main():
    mapping = build_mapping()
    if not mapping:
        print("nothing to redact — fixtures are absent or already redacted")
        return 0

    targets = sorted(OUT.glob("*.json"))
    # Extra files carrying the same values (ticket write-ups, map entries).
    targets += [Path(a) for a in sys.argv[1:]]

    touched = apply(mapping, targets)
    print(f"mapped {len(mapping)} value(s); rewrote {len(touched)} file(s):")
    for p in touched:
        print("  ", p.relative_to(Path.cwd()) if p.is_absolute() else p)

    def variants(s):
        return {s, s.replace(":", "%3A").replace("/", "%2F")}

    bodies = [p.read_text() for p in targets if p.exists() and not p.is_dir()]
    leaked = [k for k in mapping
              if any(v in b for b in bodies for v in variants(k))]
    if leaked:
        print("\n!! still present after redaction — investigate:", len(leaked), "value(s)")
        return 1
    print("\nverified: no source value remains in any target file")
    return 0


if __name__ == "__main__":
    sys.exit(main())
