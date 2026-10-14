// Run with: node --test plugin/test/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  ASK,
  KEEP,
  SUBMIT,
  bandFrom,
  clock,
  editPresenceDue,
  eventText,
  gateNeedsDialog,
  gateLabel,
  gateQuestion,
  gateVerdict,
  hereKey,
  isGatedStop,
  manifestVersion,
  needsDuration,
  parseEnvelope,
  parsePunch,
  previewArgv,
  replyText,
  stopArgv,
  stopOptions,
  stopQuestion,
  submitTouch,
  tighten,
  tokenize,
  trackerOf,
  versionSkew,
  worklogDuration,
} from '../hooks/logic.mjs'
import * as logic from '../hooks/logic.mjs'

describe('versionSkew', () => {
  const cases = [
    ['same version', '0.2.0', '0.2.0', null],
    ['a leading v on either side is the same version', '0.2.0', 'v0.2.0', null],
    ['binary upgraded, plugin not', '0.1.0', '0.2.0', { plugin: '0.1.0', binary: '0.2.0' }],
    ['plugin ahead of the binary', 'v0.3.0', '0.2.0', { plugin: '0.3.0', binary: '0.2.0' }],
    ['a dev binary never skews', '0.1.0', 'dev', null],
    ['an unknown binary never skews', '0.1.0', 'unknown', null],
    // go install ./cmd/punchcard stamps a pseudo-version: not a release.
    ['a go install pseudo-version never skews', '0.1.0', 'v0.0.0-20261009220046-aaacff0d8d07+dirty', null],
    ['a clean pseudo-version never skews', '0.1.0', 'v0.0.0-20261009220046-aaacff0d8d07', null],
    ['a pseudo-version after a tag never skews', '0.1.0', 'v0.1.1-0.20261009220046-aaacff0d8d07', null],
    ['a pseudo-version after a pre-release never skews', '0.1.0', 'v0.2.0-rc.1.0.20261009220046-aaacff0d8d07', null],
    ['a dirty build never skews', '0.1.0', 'v0.2.0+dirty', null],
    ['a pre-release tag is a release', '0.1.0', 'v0.2.0-rc.1', { plugin: '0.1.0', binary: '0.2.0-rc.1' }],
    ['no plugin version', '', '0.2.0', null],
    ['no binary version yet', '0.1.0', '', null],
    ['not a version', '0.1.0', undefined, null],
  ]
  for (const [name, plugin, binary, want] of cases) {
    test(name, () => assert.deepEqual(versionSkew(plugin, binary), want))
  }
})

test('manifestVersion', () => {
  assert.equal(manifestVersion('{ "name": "punchcard", "version": "0.3.1" }'), '0.3.1')
  assert.equal(manifestVersion('{ "name": "punchcard" }'), '')
  assert.equal(manifestVersion('not json'), '')
  assert.equal(manifestVersion('{ "version": 3 }'), '')
})

describe('bandFrom with version skew', () => {
  const skew = { plugin: '0.1.0', binary: '0.2.0' }
  const said = 'plugin 0.1.0 ≠ punchcard 0.2.0 · run punchcard setup'
  const idle = { envelope: { v: 1, ok: true, outcome: 'ok', data: { running: null, paused: [], unsent: { count: 0 }, authFailedAt: null } } }
  const running = {
    envelope: {
      v: 1, ok: true, outcome: 'ok',
      data: { running: { key: 'ABC-1', state: 'running', elapsedSeconds: 60 }, paused: [], unsent: { count: 0 }, authFailedAt: null },
    },
  }
  test('shows even when idle, as a warning', () =>
    assert.deepEqual(bandFrom(idle, skew), { tone: 'warning', text: said }))
  test('follows the timer without changing its tone', () =>
    assert.deepEqual(bandFrom(running, skew), { tone: 'success', text: `▶ ABC-1 0:01 · ${said}` }))
  test('no skew, no change', () => assert.equal(bandFrom(idle, null), null))
  test('a missing binary says only that', () =>
    assert.deepEqual(bandFrom({ missing: true }, skew), { tone: 'error', text: 'punchcard not found on PATH' }))
})

// The binary alone knows what a directory's key is (its .punchcard, else
// its branch); the mod has no key rule of its own to disagree with it.
test('the mod exports no key rule', () => {
  assert.equal(logic.KEY_PATTERN, undefined)
  assert.equal(logic.branchKey, undefined)
})

describe('submitTouch', () => {
  test('the person\'s prompt focuses from the session directory', () => {
    assert.deepEqual(submitTouch({ kind: 'composer' }, 'fix it', '/w'), ['touch', '--cwd', '/w'])
    assert.deepEqual(submitTouch({ kind: 'bridge' }, 'fix it', '/w'), ['touch', '--cwd', '/w'])
  })
  test('a slash command is presence only', () => {
    assert.deepEqual(submitTouch({ kind: 'composer' }, '  /punch pause', '/w'), ['touch'])
  })
  test('other origins touch nothing', () => {
    for (const kind of ['sdk', 'task-notification', 'scheduled-trigger', 'plugin', 'auto-continuation', 'peer']) {
      assert.equal(submitTouch({ kind }, 'x', '/w'), null, kind)
    }
  })
})

test('editPresenceDue rate-limits typing', () => {
  assert.equal(editPresenceDue(0, 5), true)
  assert.equal(editPresenceDue(1_000, 30_999), false)
  assert.equal(editPresenceDue(1_000, 31_000), true)
})

test('clock and worklogDuration', () => {
  assert.equal(clock(0), '0:00')
  assert.equal(clock(59), '0:00')
  assert.equal(clock(72 * 60 + 30), '1:12')
  assert.equal(worklogDuration(45 * 60), '45m')
  assert.equal(worklogDuration(7200), '2h')
  assert.equal(worklogDuration(90 * 60 + 59), '1h30m')
})

test('parseEnvelope', () => {
  assert.deepEqual(parseEnvelope('{"v":1,"ok":true}\n'), { v: 1, ok: true })
  assert.equal(parseEnvelope('punchcard: oops'), null)
  assert.equal(parseEnvelope('{"no":"ok"}'), null)
  assert.equal(parseEnvelope(''), null)
})

// Status envelopes as `punchcard status --json` prints them.
const timer = (over) => ({
  key: 'ABC-123',
  unverified: false,
  entryId: 'E1',
  state: 'running',
  byHand: false,
  origin: 'prompt',
  recovered: false,
  elapsedSeconds: 72 * 60,
  ...over,
})
const status = (data) => ({
  envelope: {
    v: 1,
    ok: true,
    outcome: 'ok',
    command: 'status',
    data: { running: null, paused: [], unsent: { count: 0, seconds: 0 }, authFailedAt: null, ...data },
    events: [],
    error: null,
  },
})

describe('bandFrom', () => {
  const cases = [
    ['idle with nothing owed shows no band', status({}), '', null],
    ['idle in a keyed directory names the ticket and the next action', status({}), 'DEF-9',
      { tone: 'subtle', text: 'DEF-9 not running · /punch start' }],
    ['running', status({ running: timer({ summary: 'Fix login' }) }), '',
      { tone: 'success', text: '▶ ABC-123 1:12 · Fix login' }],
    ['running and set by hand', status({ running: timer({ byHand: true, summary: 'Fix login' }) }), '',
      { tone: 'success', text: '▶ ABC-123 1:12 · by hand · Fix login' }],
    ['another ticket running: grey, says where the time goes and what this session is for', status({ running: timer() }), 'DEF-9',
      { tone: 'subtle', text: 'timer on ABC-123 1:12 · here: DEF-9' }],
    ['another ticket running and set by hand', status({ running: timer({ byHand: true, summary: 'Fix login' }) }), 'DEF-9',
      { tone: 'subtle', text: 'timer on ABC-123 1:12 · by hand · Fix login · here: DEF-9' }],
    ['another ticket running, worklogs owed: the debt still warns', status({ running: timer(), unsent: { count: 1, seconds: 60 } }), 'DEF-9',
      { tone: 'warning', text: 'timer on ABC-123 1:12 · 1 unsent · here: DEF-9' }],
    ['another ticket recovered: still red here', status({ running: timer({ recovered: true }) }), 'DEF-9',
      { tone: 'error', text: '! ABC-123 recovered · fix at stop · here: DEF-9' }],
    ['a directory with no ticket: green, nothing to compare', status({ running: timer() }), '',
      { tone: 'success', text: '▶ ABC-123 1:12' }],
    ['nothing extra in the running ticket\'s own directory', status({ running: timer() }), 'ABC-123',
      { tone: 'success', text: '▶ ABC-123 1:12' }],
    ['paused', status({ paused: [timer({ state: 'paused', elapsedSeconds: 600 })] }), '',
      { tone: 'warning', text: '⏸ ABC-123 0:10' }],
    ['several paused, the first the session\'s', status({ paused: [timer({ state: 'paused' }), timer({ key: 'X-2', state: 'paused' })] }), 'ABC-123',
      { tone: 'warning', text: '⏸ ABC-123 1:12 · +1 paused' }],
    ['paused on another ticket', status({ paused: [timer({ state: 'paused' })] }), 'DEF-9',
      { tone: 'warning', text: '⏸ ABC-123 1:12 · here: DEF-9' }],
    ['idle and owed in a keyed directory', status({ unsent: { count: 1, seconds: 300 } }), 'DEF-9',
      { tone: 'warning', text: '1 unsent · DEF-9 not running · /punch start' }],
    ['recovered', status({ running: timer({ recovered: true }) }), '',
      { tone: 'error', text: '! ABC-123 recovered · fix at stop' }],
    ['stale', status({ running: timer({ state: 'stale', elapsedSeconds: 3600 }) }), '',
      { tone: 'error', text: '! ABC-123 stale 1:00' }],
    ['unsent while running', status({ running: timer(), unsent: { count: 2, seconds: 900 } }), '',
      { tone: 'success', text: '▶ ABC-123 1:12 · 2 unsent' }],
    ['idle but owed', status({ unsent: { count: 1, seconds: 300 } }), '',
      { tone: 'warning', text: '1 unsent' }],
    ['auth failure', status({ running: timer(), unsent: { count: 1, seconds: 60 }, authFailedAt: '2026-10-09T09:00:00.000+0000' }), '',
      { tone: 'error', text: '▶ ABC-123 1:12 · 1 unsent · Jira auth failed' }],
    ['a broken Keychain shows even when idle, naming the cause', status({ configured: false, keychainError: 'macOS Keychain: timed out reading the token' }), '',
      { tone: 'warning', text: 'Keychain error: timed out reading the token' }],
    ['a broken Keychain while running', status({ running: timer(), configured: false, keychainError: 'macOS Keychain: x' }), '',
      { tone: 'warning', text: '▶ ABC-123 1:12 · Keychain error: x' }],
    ['a rejected token outranks a broken Keychain', status({ authFailedAt: '2026-10-09T09:00:00.000+0000', keychainError: 'macOS Keychain: x' }), '',
      { tone: 'error', text: 'Jira auth failed' }],
    ['not connected shows even when idle', status({ configured: false, notConfigured: 'not connected to Jira: no config.json' }), '',
      { tone: 'warning', text: 'Jira not connected' }],
    ['not connected while running', status({ running: timer(), configured: false, notConfigured: 'x' }), '',
      { tone: 'success', text: '▶ ABC-123 1:12 · Jira not connected' }],
    ['a broken Keychain is named instead of not connected', status({ configured: false, notConfigured: 'macOS Keychain: x', keychainError: 'macOS Keychain: x' }), '',
      { tone: 'warning', text: 'Keychain error: x' }],
    ['a rejected token outranks not connected', status({ authFailedAt: '2026-10-09T09:00:00.000+0000', configured: false }), '',
      { tone: 'error', text: 'Jira auth failed' }],
    ['connected says nothing', status({ configured: true }), '', null],
    ['Jira named: the band is as before', status({ tracker: 'jira', running: timer(), configured: false }), '',
      { tone: 'success', text: '▶ ABC-123 1:12 · Jira not connected' }],
    ['no tracker, idle: no band, and not connected is no warning', status({ tracker: 'none' }), '', null],
    ['no tracker, idle in a keyed directory', status({ tracker: 'none' }), 'payroll',
      { tone: 'subtle', text: 'payroll not running · /punch start' }],
    ['no tracker, running: the timer alone', status({ tracker: 'none', running: timer({ key: 'payroll' }) }), 'payroll',
      { tone: 'success', text: '▶ payroll 1:12' }],
    ['no tracker: never the Jira words, whatever else is set',
      status({ tracker: 'none', running: timer(), configured: false, notConfigured: 'x', keychainError: 'macOS Keychain: x',
        authFailedAt: '2026-10-09T09:00:00.000+0000', unsent: { count: 2, seconds: 600 } }), '',
      { tone: 'success', text: '▶ ABC-123 1:12' }],
    ['the day\'s total follows the clock and by hand, before the summary',
      status({ running: timer({ key: 'PUN-142', byHand: true, summary: 'Fix the login form' }), todaySeconds: 190 * 60 }), '',
      { tone: 'success', text: '▶ PUN-142 1:12 · by hand · today 3h 10m · Fix the login form' }],
    ['paused, with the day\'s total',
      status({ paused: [timer({ key: 'PUN-142', state: 'paused', elapsedSeconds: 40 * 60, summary: 'Fix the login form' })], todaySeconds: 190 * 60 }), '',
      { tone: 'warning', text: '⏸ PUN-142 0:40 · today 3h 10m · Fix the login form' }],
    ['another ticket running, with the day\'s total',
      status({ running: timer({ key: 'PUN-142' }), todaySeconds: 190 * 60 }), 'DEF-9',
      { tone: 'subtle', text: 'timer on PUN-142 1:12 · today 3h 10m · here: DEF-9' }],
    ['seconds are dropped from the total', status({ running: timer(), todaySeconds: 72 * 60 + 59 }), '',
      { tone: 'success', text: '▶ ABC-123 1:12 · today 1h 12m' }],
    ['whole hours today', status({ running: timer(), todaySeconds: 2 * 3600 }), '',
      { tone: 'success', text: '▶ ABC-123 1:12 · today 2h' }],
    ['minutes only today', status({ paused: [timer({ state: 'paused', elapsedSeconds: 600 })], todaySeconds: 600 }), '',
      { tone: 'warning', text: '⏸ ABC-123 0:10 · today 10m' }],
    ['an older binary without todaySeconds: the band as before', status({ running: timer({ byHand: true, summary: 'Fix login' }) }), '',
      { tone: 'success', text: '▶ ABC-123 1:12 · by hand · Fix login' }],
    ['nothing today: no total',status({ running: timer(), todaySeconds: 0 }), '',
      { tone: 'success', text: '▶ ABC-123 1:12' }],
    ['recovered leads with the fix, not the total', status({ running: timer({ recovered: true }), todaySeconds: 3600 }), '',
      { tone: 'error', text: '! ABC-123 recovered · fix at stop' }],
    ['stale leads with the fix, not the total', status({ running: timer({ state: 'stale', elapsedSeconds: 3600 }), todaySeconds: 3600 }), '',
      { tone: 'error', text: '! ABC-123 stale 1:00' }],
    ['idle in a keyed directory with time logged today', status({ todaySeconds: 190 * 60 }), 'DEF-9',
      { tone: 'subtle', text: 'DEF-9 not running · today 3h 10m · /punch start' }],
    ['idle in a keyed directory with nothing today', status({ todaySeconds: 0 }), 'DEF-9',
      { tone: 'subtle', text: 'DEF-9 not running · /punch start' }],
    ['idle elsewhere with time today shows no band', status({ todaySeconds: 3600 }), '', null],
    ['binary missing', { missing: true }, 'ABC-123', { tone: 'error', text: 'punchcard not found on PATH' }],
    ['refusal', { envelope: { ok: false, outcome: 'state_unreadable', error: { message: 'state is newer' } } }, '',
      { tone: 'error', text: 'punchcard: state is newer' }],
    ['not JSON', { envelope: null }, '', { tone: 'error', text: 'punchcard: status gave no JSON' }],
    ['timed out', { failed: 'timed out' }, '', { tone: 'error', text: 'punchcard: timed out' }],
  ]
  // The case's here is what status --cwd reported for the session.
  const at = (poll, here) =>
    poll.envelope?.data ? { envelope: { ...poll.envelope, data: { ...poll.envelope.data, here: here ? { key: here, source: 'file' } : null } } } : poll
  for (const [name, poll, here, want] of cases) {
    test(name, () => assert.deepEqual(bandFrom(at(poll, here)), want))
  }
})

describe('hereKey', () => {
  const cases = [
    ['from a .punchcard', status({ here: { key: 'payroll', source: 'file' } }), 'payroll'],
    ['from the branch', status({ here: { key: 'ABC-123', source: 'branch' } }), 'ABC-123'],
    ['the directory names none', status({ here: null }), ''],
    ['an older binary that does not say', status({}), ''],
    ['no JSON', { envelope: null }, ''],
    ['binary missing', { missing: true }, ''],
    ['timed out', { failed: 'timed out' }, ''],
  ]
  for (const [name, poll, want] of cases) {
    test(name, () => assert.equal(hereKey(poll), want))
  }
})

test('eventText', () => {
  assert.equal(eventText({ kind: 'started', key: 'A-1' }), '▶ A-1 started')
  assert.equal(eventText({ kind: 'paused', key: 'A-1' }), '⏸ A-1 paused')
  assert.equal(eventText({ kind: 'paused', key: 'A-1', at: '2026-08-09T10:00:00.000-0500' }), '⏸ A-1 paused at 10:00')
  assert.equal(eventText({ kind: 'started', key: 'A-1', at: '2026-08-09T10:45:00.000-0500' }), '▶ A-1 started at 10:45')
  assert.equal(
    eventText({ kind: 'paused', key: 'A-1', at: '2026-08-09T10:00:00.000-0500', message: 'the 30s from 09:59 was under a minute, so it was dropped' }),
    '⏸ A-1 paused at 10:00 (the 30s from 09:59 was under a minute, so it was dropped)',
  )
  assert.equal(eventText({ kind: 'byHand', key: 'A-1' }), "A-1 set by hand: agents can't move it")
  assert.equal(eventText({ kind: 'syncFailed', key: 'A-1', message: '500' }), 'A-1 worklog not sent: 500')
  assert.equal(eventText({ kind: 'novel', key: 'A-1' }), 'novel A-1')
})

describe('isGatedStop', () => {
  const gated = [
    'punchcard stop --yes',
    'punchcard stop --yes --duration 1h -m "done"',
    'punchcard stop ABC-1 --duration 90m --yes',
    'punchcard --json stop --yes',
    'punchcard stop --yes=true',
    '/opt/homebrew/bin/punchcard stop --yes',
    'env FOO=1 punchcard stop --yes',
    'cd /x && punchcard stop --yes',
    'punchcard status; punchcard stop --yes',
    'punchcard stop -m "a; b" --yes',
    'echo hi | punchcard stop --yes',
    '(punchcard stop --yes)',
    'sh -c "punchcard stop --yes"',
    'PUNCHCARD STOP --YES',
    'punchcard\tstop\t--yes',
    'punchcard stop \\\n  --yes',
  ]
  for (const c of gated) test(`asks: ${JSON.stringify(c)}`, () => assert.equal(isGatedStop(c), true))
  const open = [
    'punchcard stop',
    'punchcard stop --dry-run',
    'punchcard start ABC-1',
    'punchcard status --json',
    'punchcard sync',
    'git stop --yes',
    'punchcard stopwatch --yes',
    'punchcard stop --yesterday',
    '',
  ]
  for (const c of open) test(`passes: ${JSON.stringify(c)}`, () => assert.equal(isGatedStop(c), false))
  test('non-strings', () => {
    assert.equal(isGatedStop(undefined), false)
    assert.equal(isGatedStop(null), false)
  })
})

describe('gateVerdict', () => {
  test('asks for a gated Bash stop, never allows', () => {
    assert.equal(gateVerdict('Bash', { command: 'punchcard stop --yes' }), ASK)
    assert.equal(ASK.decision, 'ask')
  })
  test('leaves everything else to the chain', () => {
    assert.equal(gateVerdict('Bash', { command: 'ls' }), null)
    assert.equal(gateVerdict('Edit', { command: 'punchcard stop --yes' }), null)
    assert.equal(gateVerdict('Bash', null), null)
    assert.equal(gateVerdict('Bash', {}), null)
  })
})

describe('gateNeedsDialog', () => {
  const ask = { decision: 'ask' }
  const byRule = { decision: 'ask', rule: 'Bash(punchcard stop *--yes*)' }
  const cases = [
    ['auto mode: the classifier would decide, so the plugin asks', 'auto', ask, true],
    ['auto mode, allowed beneath: still asks', 'auto', { decision: 'allow' }, true],
    ["mode not seen yet: fails closed and asks", '', ask, true],
    ['default mode: the engine puts the ask to the person', 'default', ask, false],
    ['bypass mode: the ask verdict prompts there', 'bypassPermissions', ask, false],
    ["the user's own ask rule prompts in every mode", 'auto', byRule, false],
    ['a deny beneath is refused by the engine', 'auto', { decision: 'deny' }, false],
    ['no verdict at all, auto: asks', 'auto', null, true],
  ]
  for (const [name, mode, verdict, want] of cases) {
    test(name, () => assert.equal(gateNeedsDialog(mode, verdict), want))
  }
})

describe('the gate dialog says what the stop does', () => {
  const command = '  punchcard stop --yes --duration 1h  '
  test('no tracker: "Log it"', () => {
    assert.equal(gateLabel('none'), 'Log it')
    const q = gateQuestion(command, 'none')
    assert.match(q, /logs the worklog\. Log it\?/)
    assert.doesNotMatch(q, /Jira/)
    assert.match(q, /\npunchcard stop --yes --duration 1h$/)
  })
  test('Jira: "Send to Jira"', () => {
    assert.equal(gateLabel('jira'), 'Send to Jira')
    const q = gateQuestion(command, 'jira')
    assert.match(q, /sends the worklog to Jira\. Send it\?/)
    assert.match(q, /\npunchcard stop --yes --duration 1h$/)
  })
  // An older binary reports no tracker, and every older binary sent to
  // Jira; a poll that failed has no answer, and the stop it guards will
  // most likely fail the same way. Either way the words are Jira's, so
  // the label is fixed before the dialog opens.
  test('unknown: the Jira words', () => {
    assert.equal(gateLabel(''), 'Send to Jira')
    assert.equal(gateLabel(undefined), 'Send to Jira')
    assert.match(gateQuestion(command, ''), /Send it\?/)
  })
})

describe('trackerOf', () => {
  const cases = [
    ['none', status({ tracker: 'none' }), 'none'],
    ['jira', status({ tracker: 'jira' }), 'jira'],
    ['an older binary that does not say', status({}), ''],
    ['a refusal', { envelope: { ok: false, outcome: 'busy' } }, ''],
    ['no JSON', { envelope: null }, ''],
    ['binary missing', { missing: true }, ''],
  ]
  for (const [name, poll, want] of cases) {
    test(name, () => assert.equal(trackerOf(poll), want))
  }
})

describe('tighten', () => {
  test("an ask by the user's own rule stands, rule and all", () => {
    const below = { decision: 'ask', rule: 'Bash(punchcard stop *--yes*)', reason: 'rule' }
    assert.equal(tighten(below), below)
  })
  test('a deny beneath stands', () => {
    const deny = { decision: 'deny', reason: 'rule' }
    assert.equal(tighten(deny), deny)
  })
  test('allow and ask beneath become ask', () => {
    assert.equal(tighten({ decision: 'allow' }), ASK)
    assert.equal(tighten({ decision: 'ask' }), ASK)
    assert.equal(tighten(undefined), ASK)
  })
})

test('tokenize keeps quotes together', () => {
  assert.deepEqual(tokenize(`stop 1h -m "fixed it" 'x y'`), ['stop', '1h', '-m', 'fixed it', 'x y'])
  assert.deepEqual(tokenize('   '), [])
})

describe('parsePunch', () => {
  const cases = [
    ['start', 'ABC-1', { verb: 'start', argv: ['start', 'ABC-1'] }],
    ['start DEF-2', 'ABC-1', { verb: 'start', argv: ['start', 'DEF-2'] }],
    ['start', '', { error: 'No ticket here (no .punchcard, and the branch names none): use /punch start KEY.' }],
    ['pause', '', { verb: 'pause', argv: ['pause'] }],
    ['resume', 'ABC-1', { verb: 'resume', argv: ['resume'] }],
    ['resume DEF-2', '', { verb: 'resume', argv: ['resume', 'DEF-2'] }],
    ['stop', '', { verb: 'stop', duration: '', message: '' }],
    ['stop 1h30m', '', { verb: 'stop', duration: '1h30m', message: '' }],
    ['stop -m fixed the login bug', '', { verb: 'stop', duration: '', message: 'fixed the login bug' }],
    ['stop 45m -m "fixed it"', '', { verb: 'stop', duration: '45m', message: 'fixed it' }],
    ['stop 45m --message fixed -m twice', '', { verb: 'stop', duration: '45m', message: 'fixed -m twice' }],
    ['pause --at 10:00', '', { verb: 'pause', argv: ['pause', '--at', '10:00'] }],
    ['resume --at -5m', '', { verb: 'resume', argv: ['resume', '--at', '-5m'] }],
    ['resume DEF-2 --at=10:45', '', { verb: 'resume', argv: ['resume', 'DEF-2', '--at', '10:45'] }],
    ['start --at 09:00', 'ABC-1', { verb: 'start', argv: ['start', 'ABC-1', '--at', '09:00'] }],
    ['start DEF-2 --at 09:00', 'ABC-1', { verb: 'start', argv: ['start', 'DEF-2', '--at', '09:00'] }],
  ]
  for (const [args, here, want] of cases) {
    test(JSON.stringify(args), () => assert.deepEqual(parsePunch(args, here), want))
  }
  for (const bad of ['', 'frobnicate', 'start A-1 B-2', 'pause now', 'stop 1h 2h', 'pause --at', 'pause --at 10:00 11:00']) {
    test(`usage: ${JSON.stringify(bad)}`, () => assert.match(parsePunch(bad, 'ABC-1').error, /^Usage: \/punch/))
  }
})

describe('stop dialog', () => {
  const preview = { key: 'ABC-1', entryId: 'E1', recovered: false, worklog: { timeSpentSeconds: 72 * 60 + 40, comment: 'fixed it' } }
  test('previewArgv', () => {
    assert.deepEqual(previewArgv({ verb: 'stop', duration: '', message: '' }), ['stop', '--dry-run'])
    assert.deepEqual(previewArgv({ verb: 'stop', duration: '1h', message: 'x y' }), ['stop', '--dry-run', '--duration', '1h', '-m', 'x y'])
  })
  test('stopOptions: keeping a worklog pending means something only with a tracker', () => {
    assert.deepEqual(stopOptions('jira'), [SUBMIT, KEEP, 'Cancel'])
    assert.deepEqual(stopOptions(''), [SUBMIT, KEEP, 'Cancel'])
    assert.deepEqual(stopOptions('none'), [SUBMIT, 'Cancel'])
  })
  test('stopArgv sends exactly what was previewed', () => {
    assert.deepEqual(stopArgv(preview, SUBMIT), ['stop', 'ABC-1', '--yes', '--duration', '72', '-m', 'fixed it'])
    assert.deepEqual(stopArgv({ ...preview, worklog: { timeSpentSeconds: 600 } }, KEEP), ['stop', 'ABC-1', '--yes', '--duration', '10', '--no-sync'])
  })
  test('stopQuestion', () => {
    assert.equal(stopQuestion(preview), 'Log 1h12m to ABC-1 with "fixed it"?')
    assert.equal(stopQuestion({ ...preview, recovered: true, worklog: { timeSpentSeconds: 3600 } }), 'Log 1h to ABC-1 with no comment (recovered entry)?')
  })
  test('needsDuration: a recovered entry needs one typed', () => {
    assert.equal(needsDuration({ recovered: true }, { verb: 'stop', duration: '', message: '' }), true)
    assert.equal(needsDuration({ recovered: true }, { verb: 'stop', duration: '1h', message: '' }), false)
    assert.equal(needsDuration({ recovered: false }, { verb: 'stop', duration: '', message: '' }), false)
  })
})

describe('replyText', () => {
  test('refusal with hint', () => {
    const env = { ok: false, outcome: 'no_timer', error: { code: 'no_timer', message: 'nothing is running', hint: 'start one' } }
    // Claude Code already puts "punchcard:" before the reply.
    assert.equal(replyText('pause', env, 4), 'nothing is running (start one)')
  })
  test('no JSON', () => assert.equal(replyText('pause', null, 1), 'punchcard pause exited 1 without a JSON answer.'))
  test('a timer command answers with the band', () => {
    assert.equal(replyText('start', status({ running: timer({ byHand: true }) }).envelope, 0), '▶ ABC-123 1:12 · by hand')
    assert.equal(replyText('pause', status({}).envelope, 0), 'Nothing running.')
  })
  test('stop: synced, pending, local, owed', () => {
    const stop = (state, extra = {}) => ({ ok: true, outcome: 'ok', data: { key: 'A-1', worklog: { state, timeSpentSeconds: 3600 } }, ...extra })
    assert.equal(replyText('stop', stop('synced'), 0), 'Logged 1h to A-1.')
    assert.equal(replyText('stop', stop('pending'), 0), 'Stopped A-1: 1h kept pending.')
    assert.equal(replyText('stop', stop('local'), 0), 'Logged 1h to A-1.')
    assert.equal(replyText('stop', stop('pending', { outcome: 'sync_pending', error: { message: 'unreachable' } }), 7),
      'Stopped A-1: 1h saved but not sent (unreachable). Run `punchcard sync` later; never stop again.')
  })
})
