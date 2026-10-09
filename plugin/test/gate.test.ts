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

  test('leaves other commands to the rules', async ($, on) => {
    on('tool.check', () => ({ decision: 'allow' as const }))
    for (const command of ['punchcard stop', 'punchcard stop --dry-run', 'punchcard start ABC-1', 'ls']) {
      const v = await $.tool.check({ tool: 'Bash', input: { command } })
      expect(v.decision).toBe('allow')
    }
  })
})
