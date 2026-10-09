// The mod's logic as pure functions: no `$`, no engine, no Node. The hooks
// module (register.tsx) does the I/O and calls these; test/logic.test.mjs
// runs them under `node --test`.

/** A Jira ticket key anywhere in a branch name (feature/ABC-123-thing). */
export const KEY_PATTERN = /[A-Z][A-Z0-9]+-\d+/

/**
 * The ticket key a git branch names, or "" when it names none.
 * @param {string} branch
 * @returns {string}
 */
export function branchKey(branch) {
  const m = KEY_PATTERN.exec(String(branch ?? ''))
  return m ? m[0] : ''
}

/** The prompt origins that are the person's own submit (spec: composer, bridge). */
const PERSON = new Set(['composer', 'bridge'])

/**
 * The punchcard argv a submitted prompt runs, or null for none. The
 * person's own prompt is presence and focus from the session directory's
 * branch; a slash command is presence only, so `/punch pause` is never
 * undone by a focus from the same Enter.
 * @param {{ kind: string }} origin
 * @param {string} text
 * @param {string} cwd
 * @returns {string[] | null}
 */
export function submitTouch(origin, text, cwd) {
  if (!PERSON.has(origin?.kind)) return null
  if (String(text ?? '').trimStart().startsWith('/')) return ['touch']
  return ['touch', '--cwd', cwd]
}

/** How often typing in the prompt box counts as presence, at most. */
export const EDIT_GAP_MS = 30_000

/**
 * Whether a keystroke at nowMs should record presence, the last one having
 * been recorded at lastMs (0 for never).
 * @param {number} lastMs
 * @param {number} nowMs
 */
export function editPresenceDue(lastMs, nowMs) {
  return lastMs === 0 || nowMs - lastMs >= EDIT_GAP_MS
}

/**
 * h:mm, as the band and `status --short` show a timer.
 * @param {number} seconds
 */
export function clock(seconds) {
  const m = Math.floor(Math.max(0, seconds) / 60)
  return `${Math.floor(m / 60)}:${String(m % 60).padStart(2, '0')}`
}

/**
 * 1h30m, 45m, 2h: a worklog's time as `--duration` reads it.
 * @param {number} seconds
 */
export function worklogDuration(seconds) {
  const m = Math.floor(Math.max(0, seconds) / 60)
  const h = Math.floor(m / 60)
  if (h === 0) return `${m}m`
  return m % 60 === 0 ? `${h}h` : `${h}h${m % 60}m`
}

/**
 * The `--json` envelope in a command's stdout, or null when it is not one.
 * @param {string} stdout
 * @returns {any}
 */
export function parseEnvelope(stdout) {
  try {
    const env = JSON.parse(String(stdout ?? '').trim())
    return env && typeof env === 'object' && typeof env.ok === 'boolean' ? env : null
  } catch {
    return null
  }
}

/**
 * @typedef {{ tone: 'success' | 'warning' | 'error' | 'subtle', text: string }} Band
 * @typedef {{ plugin: string, binary: string }} Skew
 */

/**
 * A release version without its leading v, or "" for one that is not a
 * release (dev, unknown, empty): those never count as skew, as in setup.
 * @param {unknown} v
 */
function release(v) {
  const s = String(v ?? '').replace(/^v/, '')
  return /^\d/.test(s) ? s : ''
}

/**
 * The `version` in a plugin.json's text, or "" when it has none.
 * @param {string} text
 */
export function manifestVersion(text) {
  try {
    const v = JSON.parse(String(text ?? '')).version
    return typeof v === 'string' ? v : ''
  } catch {
    return ''
  }
}

/**
 * Whether this plugin and the punchcard binary on PATH come from different
 * releases (ADR 0007: the binary leads after `brew upgrade`, the plugin
 * follows through `punchcard setup`). plugin is plugin.json's version,
 * binary is `version --json`'s data.version.
 * @param {unknown} plugin
 * @param {unknown} binary
 * @returns {Skew | null}
 */
export function versionSkew(plugin, binary) {
  const p = release(plugin)
  const b = release(binary)
  return p && b && p !== b ? { plugin: p, binary: b } : null
}

/**
 * What the band above the prompt shows, or null for no band.
 *
 * poll is what the last `status --json` came to: `{ missing: true }` when
 * punchcard could not be started, `{ envelope }` when it answered (the
 * envelope null when its stdout was not one), `{ failed: message }` when
 * it timed out or similar. hereKey is this session's branch key ("" for
 * none).
 * @param {{ missing?: boolean, failed?: string, envelope?: any }} poll
 * @param {string} hereKey
 * @param {Skew | null} [skew] plugin and binary versions when they differ
 * @returns {Band | null}
 */
export function bandFrom(poll, hereKey, skew = null) {
  if (poll.missing) return { tone: 'error', text: 'punchcard not found on PATH' }
  if (poll.failed !== undefined) return { tone: 'error', text: `punchcard: ${poll.failed}` }
  const env = poll.envelope
  if (!env) return { tone: 'error', text: 'punchcard: status gave no JSON' }
  if (!env.ok) return { tone: 'error', text: `punchcard: ${env.error?.message ?? env.outcome}` }

  const d = env.data ?? {}
  const paused = Array.isArray(d.paused) ? d.paused : []
  const unsent = d.unsent?.count ?? 0
  // One slot for Jira, the worse wins: token rejected, then a Keychain
  // error (it names why Jira is not connected), then not connected.
  const authFailed = Boolean(d.authFailedAt)
  const keychainBroken = !authFailed && Boolean(d.keychainError)
  const notConnected = !authFailed && !keychainBroken && d.configured === false
  const shown = d.running ?? paused[0]
  if (!shown && unsent === 0 && !authFailed && !keychainBroken && !notConnected && !skew) return null

  /** @type {string[]} */
  const parts = []
  /** @type {Band['tone']} */
  let tone = 'subtle'
  if (shown) {
    const t = timerParts(shown)
    parts.push(...t.parts)
    tone = t.tone
    if (!d.running && paused.length > 1) parts.push(`+${paused.length - 1} paused`)
  }
  if (unsent > 0) {
    parts.push(`${unsent} unsent`)
    if (tone === 'subtle') tone = 'warning'
  }
  if (authFailed) {
    parts.push('Jira auth failed')
    tone = 'error'
  }
  if (keychainBroken) {
    parts.push(`Keychain error: ${String(d.keychainError).replace(/^macOS Keychain: /, '')}`)
    if (tone !== 'error') tone = 'warning'
  }
  if (notConnected) {
    parts.push('Jira not connected')
    if (tone === 'subtle') tone = 'warning'
  }
  const runningKey = d.running?.key ?? ''
  if (hereKey && hereKey !== runningKey) parts.push(`here: ${hereKey}`)
  if (skew) {
    parts.push(`plugin ${skew.plugin} ≠ punchcard ${skew.binary}`, 'run punchcard setup')
    if (tone === 'subtle') tone = 'warning'
  }
  return { tone, text: parts.join(' · ') }
}

/**
 * The band's words for one timer.
 * @param {any} t a Timer from `status --json`
 * @returns {{ tone: Band['tone'], parts: string[] }}
 */
function timerParts(t) {
  if (t.recovered) return { tone: 'error', parts: [`! ${t.key} recovered`, 'fix at stop'] }
  if (t.state === 'stale') return { tone: 'error', parts: [`! ${t.key} stale ${clock(t.elapsedSeconds)}`] }
  const parts = []
  if (t.state === 'paused') {
    parts.push(`⏸ ${t.key} ${clock(t.elapsedSeconds)}`)
  } else {
    parts.push(`▶ ${t.key} ${clock(t.elapsedSeconds)}`)
    if (t.held) parts.push('held')
  }
  if (t.summary) parts.push(t.summary)
  return { tone: t.state === 'paused' ? 'warning' : 'success', parts }
}

/**
 * One event from a command's envelope as a toast line.
 * @param {{ kind: string, key: string, message?: string }} e
 */
export function eventText(e) {
  switch (e.kind) {
    case 'added':
      return `+ ${e.key} added`
    case 'paused':
      return `⏸ ${e.key} paused`
    case 'held':
      return `${e.key} is now held: only you can move it`
    case 'started':
      return `▶ ${e.key} started`
    case 'recovered':
      return `! ${e.key} recovered: fix the duration at stop`
    case 'synced':
      return `${e.key} worklog sent`
    case 'syncFailed':
      return `${e.key} worklog not sent${e.message ? `: ${e.message}` : ''}`
    default:
      return `${e.kind} ${e.key}`
  }
}

/**
 * Whether a Bash command would send an agent's worklog: it names
 * punchcard, stop and --yes anywhere in its raw text. Deliberately loose
 * (any order, any segment of a compound command, any path to the binary,
 * any case): a false match costs one dialog, a miss sends time unread.
 * @param {unknown} command
 */
export function isGatedStop(command) {
  const s = String(command ?? '')
  return /punchcard/i.test(s) && /(^|[^a-z0-9_-])stop([^a-z0-9_-]|$)/i.test(s) && /--yes(?![a-z0-9_-])/i.test(s)
}

/**
 * What the tool.check gate answers for a call: `ask` for a gated stop,
 * null to leave the call to the chain. Never `allow`: a mod's allow would
 * override the user's ask rule.
 * @param {string} tool
 * @param {unknown} input
 * @returns {{ decision: 'ask', reason: string } | null}
 */
export function gateVerdict(tool, input) {
  if (tool !== 'Bash') return null
  const command = input && typeof input === 'object' ? /** @type {any} */ (input).command : undefined
  return isGatedStop(command) ? ASK : null
}

export const ASK = Object.freeze({
  decision: /** @type {const} */ ('ask'),
  reason: 'Punchcard: this stops the timer and sends the worklog to Jira. Approve only the worklog you were shown.',
})

/**
 * The gate's answer given the verdict beneath it (the user's rules and
 * mode): a deny stands, anything else becomes ask. The gate only ever
 * tightens.
 * @template {{ decision: string }} T
 * @param {T} below
 * @returns {T | typeof ASK}
 */
export function tighten(below) {
  return below?.decision === 'deny' ? below : ASK
}

/**
 * Splits /punch's arguments like a shell would, quotes kept together.
 * @param {string} args
 * @returns {string[]}
 */
export function tokenize(args) {
  /** @type {string[]} */
  const out = []
  const re = /"([^"]*)"|'([^']*)'|(\S+)/g
  let m
  while ((m = re.exec(String(args ?? ''))) !== null) out.push(m[1] ?? m[2] ?? m[3] ?? '')
  return out
}

export const PUNCH_USAGE = 'Usage: /punch start [KEY] | pause | resume [KEY] | stop [duration] [-m comment]'

/**
 * @typedef {{ verb: 'start' | 'pause' | 'resume', argv: string[] }} TimerAction
 * @typedef {{ verb: 'stop', duration: string, message: string }} StopAction
 * @typedef {{ error: string }} PunchError
 */

/**
 * What `/punch <args>` asks for. start with no key takes this branch's key
 * (hereKey); with neither it is refused, since an unkeyed session needs an
 * explicit key. stop takes an optional duration and `-m`, the comment
 * being the rest of the line (quotes optional).
 * @param {string} args
 * @param {string} hereKey
 * @returns {TimerAction | StopAction | PunchError}
 */
export function parsePunch(args, hereKey) {
  const raw = String(args ?? '').trim()
  const [verb = '', ...rest] = tokenize(raw)
  switch (verb) {
    case 'start': {
      if (rest.length > 1) return { error: PUNCH_USAGE }
      const key = rest[0] ?? hereKey
      if (!key) return { error: 'This branch names no ticket: use /punch start KEY.' }
      return { verb: 'start', argv: ['start', key] }
    }
    case 'pause':
      return rest.length ? { error: PUNCH_USAGE } : { verb: 'pause', argv: ['pause'] }
    case 'resume':
      return rest.length > 1 ? { error: PUNCH_USAGE } : { verb: 'resume', argv: ['resume', ...rest] }
    case 'stop': {
      const after = raw.slice(raw.indexOf('stop') + 4).trim()
      const at = after.search(/(^|\s)(-m|--message)(\s|$)/)
      const head = (at < 0 ? after : after.slice(0, at)).trim()
      let message = at < 0 ? '' : after.slice(at).trim().replace(/^(-m|--message)\s*/, '')
      message = message.replace(/^"(.*)"$/s, '$1').replace(/^'(.*)'$/s, '$1')
      const words = tokenize(head)
      if (words.length > 1) return { error: PUNCH_USAGE }
      return { verb: 'stop', duration: words[0] ?? '', message }
    }
    default:
      return { error: PUNCH_USAGE }
  }
}

/**
 * The argv that previews a /punch stop (exit 0 with data.preview).
 * @param {StopAction} a
 */
export function previewArgv(a) {
  return ['stop', '--dry-run', ...(a.duration ? ['--duration', a.duration] : []), ...(a.message ? ['-m', a.message] : [])]
}

/** The dialog's three answers. */
export const SUBMIT = 'Submit'
export const KEEP = 'Keep pending'
export const CANCEL = 'Cancel'

/**
 * The argv that performs the stop the person approved, pinned to the
 * previewed ticket and duration so what is sent is what they read.
 * @param {{ key: string, worklog: { timeSpentSeconds: number, comment?: string } }} preview
 * @param {string} choice SUBMIT or KEEP
 */
export function stopArgv(preview, choice) {
  const argv = ['stop', preview.key, '--yes', '--duration', String(Math.floor(preview.worklog.timeSpentSeconds / 60))]
  if (preview.worklog.comment) argv.push('-m', preview.worklog.comment)
  if (choice === KEEP) argv.push('--no-sync')
  return argv
}

/**
 * The dialog's question for a preview.
 * @param {{ key: string, recovered?: boolean, worklog: { timeSpentSeconds: number, comment?: string } }} preview
 */
export function stopQuestion(preview) {
  const w = preview.worklog
  const comment = w.comment ? ` with "${w.comment}"` : ' with no comment'
  const recovered = preview.recovered ? ' (recovered entry)' : ''
  return `Log ${worklogDuration(w.timeSpentSeconds)} to ${preview.key}${comment}${recovered}?`
}

/**
 * Whether a preview cannot go ahead without a duration typed by the
 * person: a recovered entry's measured time is a guess.
 * @param {{ recovered?: boolean }} preview
 * @param {StopAction} a
 */
export function needsDuration(preview, a) {
  return Boolean(preview.recovered) && !a.duration
}

/**
 * The transcript line for a /punch command's result.
 * @param {string} verb
 * @param {any} env the envelope, or null when stdout was not one
 * @param {number} exitCode
 */
export function replyText(verb, env, exitCode) {
  if (!env) return `punchcard ${verb} exited ${exitCode} without a JSON answer.`
  if (!env.ok) {
    const e = env.error ?? {}
    return `punchcard ${verb}: ${e.message ?? env.outcome}${e.hint ? ` (${e.hint})` : ''}`
  }
  if (verb === 'stop') {
    const d = env.data ?? {}
    const time = worklogDuration(d.worklog?.timeSpentSeconds ?? 0)
    if (exitCode === 7) {
      return `Stopped ${d.key}: ${time} saved but not sent (${env.error?.message ?? 'sync pending'}). Run \`punchcard sync\` later; never stop again.`
    }
    return d.worklog?.state === 'synced' ? `Logged ${time} to ${d.key}.` : `Stopped ${d.key}: ${time} kept pending.`
  }
  const band = bandFrom({ envelope: env }, '')
  return band ? band.text : 'Nothing running.'
}
