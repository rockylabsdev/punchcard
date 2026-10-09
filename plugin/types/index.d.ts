// The punchcard mod's $.state contract.

/** What the band above the prompt shows; null for no band. */
export type Band = { tone: 'success' | 'warning' | 'error' | 'subtle'; text: string }

declare module 'claude-code' {
  interface PluginState {
    punchcard: { band: Band | null }
  }
}
