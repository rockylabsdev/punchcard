// The punchcard mod: presence and focus from the person's prompts, the band
// above the prompt, /punch, and the gate on an agent's `stop --yes`.
// Everything goes through the bare `punchcard` on PATH with an explicit
// --origin (ADR 0005, 0007). The decisions live in ./logic.mjs.
import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { Band } from '../types'
type Skew = { plugin: string; binary: string } // logic.mjs's Skew
import {
  ASK,
  CANCEL,
  DENIED_BY_USER,
  DENIED_GATE_FAILED,
  DENIED_UNATTENDED,
  HOLD,
  KEEP,
  SEND,
  SUBMIT,
  bandFrom,
  editPresenceDue,
  eventText,
  gateNeedsDialog,
  gateQuestion,
  gateVerdict,
  hereKey,
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
  versionSkew,
} from './logic.mjs'

const band = atom({ plugin: 'punchcard', key: 'band' } as const, null as Band | null)

const POLL_MS = 10_000
const QUICK_MS = 5_000 // reads and presence
const SEND_MS = 30_000 // a stop POSTs within a 10 s budget
const VERSION_MS = 300_000 // how often to re-read the binary's version (brew upgrade mid-session)

type Origin = 'prompt' | 'hand'
type Ran = { envelope: any; exitCode: number }

// Module state: a reload resets these, which is harmless.
let lastEdit = 0 // when typing last counted as presence
let permissionMode = '' // the session's, from the classic hook inputs; '' until seen
let polling = false // a status poll is in flight
let pluginVersion: string | null = null // this plugin's plugin.json version, read once
let binaryVersion = '' // `punchcard version --json`, '' until known
let binaryCheckedAt = 0

// run calls punchcard with --json and an explicit --origin. It rejects
// only when the binary could not be started or timed out.
async function run($: EngineInterface, argv: string[], origin: Origin, timeoutMs = QUICK_MS): Promise<Ran> {
  const cwd = await $.session.cwd()
  const r = await $.process.run(['punchcard', ...argv, '--json', '--origin', origin], { cwd, timeoutMs })
  return { envelope: parseEnvelope(r.stdout), exitCode: r.exitCode }
}

// toastEvents shows the side effects of the mod's own calls, once each.
function toastEvents($: EngineInterface, r: Ran) {
  for (const e of r.envelope?.events ?? []) $.ui.toast(eventText(e))
}

// status asks for the session directory: the timers and, as here, the
// directory's key (its .punchcard, else its branch). The binary owns the
// key rule; the mod never works one out itself.
async function status($: EngineInterface): Promise<Ran> {
  return run($, ['status', '--cwd', await $.session.cwd()], 'prompt')
}

// here is the session directory's key, asked fresh ("" for none, or when
// punchcard did not answer), so a /punch start right after a cd or a
// checkout uses the new answer rather than the last poll's.
async function here($: EngineInterface): Promise<string> {
  try {
    return hereKey({ envelope: (await status($)).envelope })
  } catch {
    return ''
  }
}

// ownVersion is this plugin's plugin.json version, "" when unreadable.
async function ownVersion($: EngineInterface): Promise<string> {
  if (pluginVersion !== null) return pluginVersion
  pluginVersion = ''
  for (const p of [`${$.plugin.root}/.claude-plugin/plugin.json`, `${$.plugin.root}/plugin.json`]) {
    try {
      pluginVersion = manifestVersion(await $.fs.read(p))
      if (pluginVersion) break
    } catch {
      // not here; try the next place
    }
  }
  return pluginVersion
}

// skew compares this plugin's version with the binary's (ADR 0007). The
// binary's is re-read every VERSION_MS, so a brew upgrade during the
// session shows up; "" (unknown) never counts as skew.
async function skew($: EngineInterface): Promise<Skew | null> {
  const now = await $.clock.now()
  if (!binaryVersion || now - binaryCheckedAt >= VERSION_MS) {
    binaryCheckedAt = now
    try {
      const r = await run($, ['version'], 'prompt')
      binaryVersion = r.envelope?.ok ? String(r.envelope.data?.version ?? '') : ''
    } catch {
      binaryVersion = '' // not found or timed out: the status poll says so
    }
  }
  return versionSkew(await ownVersion($), binaryVersion)
}

// refresh polls status and redraws the band.
async function refresh($: EngineInterface) {
  if (polling) return
  polling = true
  try {
    let next: Band | null
    try {
      const r = await status($)
      next = bandFrom({ envelope: r.envelope }, await skew($))
    } catch (err) {
      const msg = String((err as Error)?.message ?? err)
      next = /time ?out|timed out/i.test(msg) ? bandFrom({ failed: 'status timed out' }) : bandFrom({ missing: true })
    }
    await update($, band, () => next)
  } finally {
    polling = false
  }
}

// touch records presence (and focus, given --cwd), then redraws the band.
async function touch($: EngineInterface, argv: string[]) {
  try {
    toastEvents($, await run($, argv, 'prompt'))
  } catch {
    // could not start punchcard: the refresh puts that in the band
  }
  await refresh($)
}

// presence records typing as presence only, never waiting on it.
async function presence($: EngineInterface) {
  try {
    await run($, ['touch'], 'prompt')
  } catch {
    // presence is best effort
  }
}

// stop previews, asks Submit / Keep pending / Cancel, then sends exactly
// the worklog shown.
async function stop($: EngineInterface, a: { verb: 'stop'; duration: string; message: string }): Promise<string> {
  const p = await run($, previewArgv(a), 'hand')
  const preview = p.envelope?.ok ? p.envelope.data?.preview : undefined
  if (!preview) return replyText('stop', p.envelope, p.exitCode)
  if (needsDuration(preview, a)) {
    return `${preview.key} was recovered after a gap, so its measured time is a guess. Run /punch stop <duration> (e.g. 1h30m).`
  }
  let choice = CANCEL
  try {
    choice = await $.ui.ask(stopQuestion(preview), { header: 'Punchcard', options: [SUBMIT, KEEP, CANCEL] })
  } catch {
    choice = CANCEL // dismissed, or no one to ask
  }
  if (choice !== SUBMIT && choice !== KEEP) return `Not stopped: the timer on ${preview.key} is untouched.`
  const r = await run($, stopArgv(preview, choice), 'hand', SEND_MS)
  toastEvents($, r)
  return replyText('stop', r.envelope, r.exitCode)
}

// punch answers /punch <args> with origin hand, so a start holds.
async function punch($: EngineInterface, args: string): Promise<string> {
  const a = parsePunch(args, await here($))
  if ('error' in a) return a.error
  try {
    if (a.verb === 'stop') return await stop($, a)
    const r = await run($, a.argv, 'hand')
    toastEvents($, r)
    return replyText(a.verb, r.envelope, r.exitCode)
  } catch {
    return 'punchcard did not answer: not found on PATH, or timed out.'
  } finally {
    void refresh($)
  }
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    await $.command.register({
      name: 'punch',
      description: 'Punchcard timer: start, pause, resume or stop by hand',
      argumentHint: 'start [KEY] | pause | resume [KEY] | stop [duration] [-m comment]',
    })
    $.clock.every(POLL_MS, () => void refresh($))
    void refresh($)
    return next(e)
  })

  on('prompt.submit', async ($, e, next) => {
    const argv = submitTouch(e.origin, e.text, await $.session.cwd())
    if (argv) void touch($, argv)
    return next(e)
  })

  on('prompt.edit', async ($, e, next) => {
    const now = await $.clock.now()
    if (editPresenceDue(lastEdit, now)) {
      lastEdit = now
      void presence($)
    }
    return next(e)
  })

  // The gate, in two layers (ADR 0005).
  //
  // 1. tool.check: ask for any Bash command that would send an agent's
  //    worklog, unless the user's own rules already deny it or ask by
  //    rule. Never allow (a mod's allow overrides the user's ask rule); a
  //    failure asks too, so the gate fails closed. At tool.check, next(e)
  //    only reads the verdict beneath.
  on('tool.check', { tool: 'Bash' }, async ($, e, next) => {
    if (!gateVerdict(e.tool, e.input)) return next(e)
    return tighten(await next(e))
  }).catch(() => ({ ...ASK }))

  // 2. tool.call: an `ask` verdict goes to the mode's decider, and in auto
  //    mode that is a classifier, not the person. So where the engine will
  //    not put the call to a person (auto mode, or a mode not yet seen),
  //    the plugin raises its own dialog and sends only on "Send to Jira".
  //    With nobody to ask (claude -p) the dialog rejects and the call is
  //    refused with the timer running. Judged before next, so a failure
  //    refuses rather than sends.
  on('tool.call', { tool: 'Bash' }, async ($, e, next) => {
    if (!gateVerdict(e.tool, { command: e.command })) return next(e)
    const verdict = await $.tool.check({ tool: 'Bash', input: { command: e.command } })
    if (!gateNeedsDialog(permissionMode, verdict)) return next(e)
    let answer: string
    try {
      answer = await $.ui.ask(gateQuestion(e.command), { header: 'Punchcard', options: [SEND, HOLD] })
    } catch {
      return { deny: DENIED_UNATTENDED }
    }
    if (answer !== SEND) return { deny: DENIED_BY_USER }
    return next(e)
  }).catch(($, e, next) => (next.called ? next(e) : { deny: DENIED_GATE_FAILED }))

  // The permission mode reaches a mod only on the classic hook inputs.
  on('classic.SessionStart', ($, e, next) => {
    permissionMode = e.permission_mode ?? permissionMode
    return next(e)
  })
  on('classic.UserPromptSubmit', ($, e, next) => {
    permissionMode = e.permission_mode ?? permissionMode
    return next(e)
  })

  on('command.run', { command: 'punch' }, async ($, e) => ({ text: await punch($, e.args) }))

  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    const b = await read($, band)
    if (e.props.hasSurvey || b === null) return next(e)
    const { Box, Text } = $.ui.resolve(e)
    return (
      <Box>
        <Text color={b.tone} wrap="truncate-end">
          {b.text}
        </Text>
      </Box>
    )
  })
}
