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
  stopQuestion,
  submitTouch,
  tighten,
  tokenize,
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
  held: false,
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
    ['running and held', status({ running: timer({ held: true, summary: 'Fix login' }) }), '',
      { tone: 'success', text: '▶ ABC-123 1:12 · held · Fix login' }],
    ['another ticket running: grey, says where the time goes and what this session is for', status({ running: timer() }), 'DEF-9',
      { tone: 'subtle', text: 'timer on ABC-123 1:12 · here: DEF-9' }],
    ['another ticket running and held', status({ running: timer({ held: true, summary: 'Fix login' }) }), 'DEF-9',
      { tone: 'subtle', text: 'timer on ABC-123 1:12 · held · Fix login · here: DEF-9' }],
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

test('gateQuestion names the command', () => {
  const q = gateQuestion('  punchcard stop --yes --duration 1h  ')
  assert.match(q, /Send it\?/)
  assert.match(q, /\npunchcard stop --yes --duration 1h$/)
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
  ]
  for (const [args, here, want] of cases) {
    test(JSON.stringify(args), () => assert.deepEqual(parsePunch(args, here), want))
  }
  for (const bad of ['', 'frobnicate', 'start A-1 B-2', 'pause now', 'stop 1h 2h']) {
    test(`usage: ${JSON.stringify(bad)}`, () => assert.match(parsePunch(bad, 'ABC-1').error, /^Usage: \/punch/))
  }
})

describe('stop dialog', () => {
  const preview = { key: 'ABC-1', entryId: 'E1', recovered: false, worklog: { timeSpentSeconds: 72 * 60 + 40, comment: 'fixed it' } }
  test('previewArgv', () => {
    assert.deepEqual(previewArgv({ verb: 'stop', duration: '', message: '' }), ['stop', '--dry-run'])
    assert.deepEqual(previewArgv({ verb: 'stop', duration: '1h', message: 'x y' }), ['stop', '--dry-run', '--duration', '1h', '-m', 'x y'])
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
    assert.equal(replyText('pause', env, 4), 'punchcard pause: nothing is running (start one)')
  })
  test('no JSON', () => assert.equal(replyText('pause', null, 1), 'punchcard pause exited 1 without a JSON answer.'))
  test('a timer command answers with the band', () => {
    assert.equal(replyText('start', status({ running: timer({ held: true }) }).envelope, 0), '▶ ABC-123 1:12 · held')
    assert.equal(replyText('pause', status({}).envelope, 0), 'Nothing running.')
  })
  test('stop: synced, pending, owed', () => {
    const stop = (state, extra = {}) => ({ ok: true, outcome: 'ok', data: { key: 'A-1', worklog: { state, timeSpentSeconds: 3600 } }, ...extra })
    assert.equal(replyText('stop', stop('synced'), 0), 'Logged 1h to A-1.')
    assert.equal(replyText('stop', stop('pending'), 0), 'Stopped A-1: 1h kept pending.')
    assert.equal(replyText('stop', stop('pending', { outcome: 'sync_pending', error: { message: 'unreachable' } }), 7),
      'Stopped A-1: 1h saved but not sent (unreachable). Run `punchcard sync` later; never stop again.')
  })
})
