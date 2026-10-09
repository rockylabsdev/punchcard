// The gate against the engine itself: run with `claude plugin test plugin`.
// The test's own tool.check hook stands for the user's rules and mode.
import { describe, expect, test } from 'claude-code/testing'

const STOP = 'punchcard stop --yes --duration 1h -m "done"'

describe('tool.check gate', () => {
  test('asks for an agent stop even when the mode would allow it (skip-permissions)', async ($, on) => {
    on('tool.check', () => ({ decision: 'allow' as const }))
    const v = await $.tool.check({ tool: 'Bash', input: { command: STOP } })
    expect(v.decision).toBe('ask')
  })

  test('asks inside a compound command', async ($, on) => {
    on('tool.check', () => ({ decision: 'allow' as const }))
    const v = await $.tool.check({ tool: 'Bash', input: { command: `cd /w && ${STOP} | cat` } })
    expect(v.decision).toBe('ask')
  })

  test("keeps the user's deny", async ($, on) => {
    on('tool.check', () => ({ decision: 'deny' as const, reason: 'rule' }))
    const v = await $.tool.check({ tool: 'Bash', input: { command: STOP } })
    expect(v.decision).toBe('deny')
  })

  test("keeps the user's own ask rule visible, rather than replacing it", async ($, on) => {
    on('tool.check', () => ({ decision: 'ask' as const, rule: 'Bash(punchcard stop *--yes*)', reason: 'rule' }))
    const v = await $.tool.check({ tool: 'Bash', input: { command: STOP } })
    expect(v.decision).toBe('ask')
    expect(v.rule).toBe('Bash(punchcard stop *--yes*)')
  })

  test('leaves other commands to the rules', async ($, on) => {
    on('tool.check', () => ({ decision: 'allow' as const }))
    for (const command of ['punchcard stop', 'punchcard stop --dry-run', 'punchcard start ABC-1', 'ls']) {
      const v = await $.tool.check({ tool: 'Bash', input: { command } })
      expect(v.decision).toBe('allow')
    }
  })
})

// The second layer: in auto mode the plugin raises its own dialog on the
// call itself, since the mode's decider is a classifier. The test's hooks
// stand for the engine: its Bash hook is "the command ran", its
// AskUserQuestion hook is the person at the dialog.
describe('tool.call gate', () => {
  const ran = { result: { stdout: 'sent', stderr: '' } }
  // The person at the dialog: answers are keyed by the question's text.
  const person = (label: string) => (_: unknown, e: any) => ({
    result: { questions: e.questions, answers: { [e.questions[0].question]: label } },
  })

  test('auto mode: asks, and "Send to Jira" lets the command run', async ($, on) => {
    on('tool.check', () => ({ decision: 'ask' as const }))
    on('tool.call', { tool: 'Bash' }, () => ran)
    let asked = 0
    on('tool.call', { tool: 'AskUserQuestion' }, ($, e) => {
      asked++
      return person('Send to Jira')($, e)
    })
    on('classic.SessionStart', () => ({}))
    await $.classic.SessionStart({ source: 'startup', permission_mode: 'auto' })
    const r = await $.tool.call({ tool: 'Bash', command: STOP })
    expect(asked).toBe(1)
    expect(r.deny).toBeUndefined()
  })

  test('auto mode: "Keep the timer running" refuses the command', async ($, on) => {
    on('tool.check', () => ({ decision: 'ask' as const }))
    on('tool.call', { tool: 'Bash' }, () => ran)
    on('tool.call', { tool: 'AskUserQuestion' }, person('Keep the timer running'))
    on('classic.SessionStart', () => ({}))
    await $.classic.SessionStart({ source: 'startup', permission_mode: 'auto' })
    const r = await $.tool.call({ tool: 'Bash', command: STOP })
    expect(r.deny).toMatch(/did not approve/)
  })

  test("auto mode with the user's ask rule: no second dialog", async ($, on) => {
    on('tool.check', () => ({ decision: 'ask' as const, rule: 'Bash(punchcard stop *--yes*)' }))
    on('tool.call', { tool: 'Bash' }, () => ran)
    let asked = 0
    on('tool.call', { tool: 'AskUserQuestion' }, ($, e) => {
      asked++
      return person('Send to Jira')($, e)
    })
    on('classic.SessionStart', () => ({}))
    await $.classic.SessionStart({ source: 'startup', permission_mode: 'auto' })
    const r = await $.tool.call({ tool: 'Bash', command: STOP })
    expect(asked).toBe(0)
    expect(r.deny).toBeUndefined()
  })

  test('default mode: the engine asks, the plugin does not', async ($, on) => {
    on('tool.check', () => ({ decision: 'ask' as const }))
    on('tool.call', { tool: 'Bash' }, () => ran)
    let asked = 0
    on('tool.call', { tool: 'AskUserQuestion' }, ($, e) => {
      asked++
      return person('Send to Jira')($, e)
    })
    on('classic.SessionStart', () => ({}))
    await $.classic.SessionStart({ source: 'startup', permission_mode: 'default' })
    const r = await $.tool.call({ tool: 'Bash', command: STOP })
    expect(asked).toBe(0)
    expect(r.deny).toBeUndefined()
  })

  // The status the gate reads for the tracker: the test's process.run
  // hook stands for the punchcard binary.
  const statusSays = (tracker: string) => (_: unknown, e: any) => ({
    value: {
      exitCode: 0,
      stdout: e.argv.includes('status')
        ? JSON.stringify({ v: 1, ok: true, outcome: 'ok', command: 'status', data: { running: null, paused: [], tracker, here: null }, events: [], error: null })
        : '',
      stderr: '',
      isStdoutTruncated: false,
      isStderrTruncated: false,
    },
  })

  for (const [tracker, label, other] of [
    ['none', 'Log it', 'Send to Jira'],
    ['jira', 'Send to Jira', 'Log it'],
  ] as const) {
    test(`auto mode, tracker ${tracker}: the dialog offers "${label}", and it runs the command`, async ($, on) => {
      on('tool.check', () => ({ decision: 'ask' as const }))
      on('tool.call', { tool: 'Bash' }, () => ran)
      on('session.cwd', () => ({ value: '/work' }))
      on('process.run', statusSays(tracker))
      let options: string[] = []
      on('tool.call', { tool: 'AskUserQuestion' }, ($, e: any) => {
        options = e.questions[0].options.map((o: any) => o.label ?? o)
        return person(label)($, e)
      })
      on('classic.SessionStart', () => ({}))
      await $.classic.SessionStart({ source: 'startup', permission_mode: 'auto' })
      const r = await $.tool.call({ tool: 'Bash', command: STOP })
      expect(options).toContain(label)
      expect(options).not.toContain(other)
      expect(r.deny).toBeUndefined()
    })
  }

  test('a dialog nobody answers refuses the command with the timer running', async ($, on) => {
    on('tool.check', () => ({ decision: 'ask' as const }))
    on('tool.call', { tool: 'Bash' }, () => ran)
    on('tool.call', { tool: 'AskUserQuestion' }, () => ({ deny: 'nobody here' }))
    on('classic.SessionStart', () => ({}))
    await $.classic.SessionStart({ source: 'startup', permission_mode: 'auto' })
    const r = await $.tool.call({ tool: 'Bash', command: STOP })
    expect(r.deny).toMatch(/nobody is here/)
  })

  test('other commands never see the dialog', async ($, on) => {
    on('tool.check', () => ({ decision: 'allow' as const }))
    on('tool.call', { tool: 'Bash' }, () => ran)
    let asked = 0
    on('tool.call', { tool: 'AskUserQuestion' }, ($, e) => {
      asked++
      return person('Send to Jira')($, e)
    })
    on('classic.SessionStart', () => ({}))
    await $.classic.SessionStart({ source: 'startup', permission_mode: 'auto' })
    const r = await $.tool.call({ tool: 'Bash', command: 'punchcard stop --dry-run' })
    expect(asked).toBe(0)
    expect(r.deny).toBeUndefined()
  })
})
