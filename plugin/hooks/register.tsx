// The punchcard mod: presence and focus from the person's prompts, the band
// above the prompt, /punch, and the gate on an agent's `stop --yes`.
// Everything goes through the bare `punchcard` on PATH with an explicit
// --origin (ADR 0005, 0007). The decisions live in ./logic.mjs.
import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { Band } from '../types'
import {
  ASK,
  CANCEL,
  KEEP,
  SUBMIT,
  bandFrom,
  branchKey,
  editPresenceDue,
  eventText,
  gateVerdict,
  needsDuration,
  parseEnvelope,
  parsePunch,
  previewArgv,
  replyText,
  stopArgv,
  stopQuestion,
  submitTouch,
  tighten,
} from './logic.mjs'

const band = atom({ plugin: 'punchcard', key: 'band' } as const, null as Band | null)

const POLL_MS = 10_000
const QUICK_MS = 5_000 // reads and presence
const SEND_MS = 30_000 // a stop POSTs within a 10 s budget

type Origin = 'prompt' | 'hand'
type Ran = { envelope: any; exitCode: number }

// Module state: a reload resets both, which is harmless.
let lastEdit = 0 // when typing last counted as presence
let polling = false // a status poll is in flight

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

// hereKey is the ticket key in the session directory's git branch, or "".
async function hereKey($: EngineInterface): Promise<string> {
  try {
    const cwd = await $.session.cwd()
    const r = await $.process.run(['git', '-C', cwd, 'symbolic-ref', '--short', '-q', 'HEAD'], { timeoutMs: QUICK_MS })
    return r.exitCode === 0 ? branchKey(r.stdout.trim()) : ''
  } catch {
    return ''
  }
}

// refresh polls status --json and redraws the band.
async function refresh($: EngineInterface) {
  if (polling) return
  polling = true
  try {
    const here = await hereKey($)
    let next: Band | null
    try {
      const r = await run($, ['status'], 'prompt')
      next = bandFrom({ envelope: r.envelope }, here)
    } catch (err) {
      const msg = String((err as Error)?.message ?? err)
      next = /time ?out|timed out/i.test(msg) ? bandFrom({ failed: 'status timed out' }, here) : bandFrom({ missing: true }, here)
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
  const a = parsePunch(args, await hereKey($))
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

  // The gate: ask for any Bash command that would send an agent's worklog,
  // unless the user's own rules already deny it. Never allow (a mod's allow
  // overrides the user's ask rule); a failure asks too, so the gate fails
  // closed. At tool.check, next(e) only reads the verdict beneath.
  on('tool.check', { tool: 'Bash' }, async ($, e, next) => {
    if (!gateVerdict(e.tool, e.input)) return next(e)
    return tighten(await next(e))
  }).catch(() => ({ ...ASK }))

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
