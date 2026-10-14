// Loads the Go WebAssembly build of the SAR MAKER engine (the ocsarmk.exe
// reimplementation) and exposes its typed API. The engine registers the
// global `sarWasm` object; every export except saveData answers with a
// JSON envelope {"ok":true,"data":...} or {"ok":false,"error":"..."}.
import type {
  ComputeInput,
  EngineInfo,
  GammaTables,
  Result,
} from '../types'

// @ts-ignore -- wasm_exec.js is a plain side-effect script defining globalThis.Go
import './wasm_exec.js'

interface GoInstance {
  importObject: WebAssembly.Imports
  run(inst: WebAssembly.Instance): Promise<void>
}

interface RawWasmApi {
  version(): string
  defaults(): string
  parseGamma(text: string): string
  parseLpmod(text: string): string
  compute(input: string): string
  saveData(result: string, isoNow: string): string
}

declare global {
  interface Window {
    Go?: new () => GoInstance
    sarWasm?: RawWasmApi
  }
}

export interface Engine {
  info(): EngineInfo
  defaults(): { gamma: GammaTables; lpmod: string[] }
  parseGamma(text: string): GammaTables
  parseLpmod(text: string): string[]
  compute(input: ComputeInput): Result
  saveData(result: Result, isoNow: string): string
}

type Envelope<T> = { ok: true; data: T } | { ok: false; error: string }

function unwrap<T>(raw: string): T {
  let env: Envelope<T>
  try {
    env = JSON.parse(raw) as Envelope<T>
  } catch {
    throw new Error(`engine returned invalid JSON: ${raw.slice(0, 120)}`)
  }
  if (!env.ok) throw new Error(env.error)
  return env.data
}

let enginePromise: Promise<RawWasmApi> | null = null

function loadEngine(): Promise<RawWasmApi> {
  if (!enginePromise) {
    enginePromise = boot()
  }
  return enginePromise
}

async function boot(): Promise<RawWasmApi> {
  const GoCtor = window.Go
  if (!GoCtor) {
    throw new Error('wasm_exec.js missing (run `make wasm` or `npm run wasm`)')
  }
  const go = new GoCtor()
  const url = import.meta.env.BASE_URL + 'sar.wasm'
  let module: WebAssembly.Module
  try {
    module = await WebAssembly.compileStreaming(fetch(url))
  } catch {
    const resp = await fetch(url)
    if (!resp.ok) {
      throw new Error(`failed to fetch ${url} (HTTP ${resp.status})`)
    }
    module = await WebAssembly.compile(await resp.arrayBuffer())
  }
  const inst = await WebAssembly.instantiate(module, go.importObject)
  // go.run() resolves only when the Go program exits; the engine's main()
  // registers window.sarWasm synchronously before blocking forever, so we
  // must not await it.
  go.run(inst).catch((err: unknown) => {
    console.error('wasm engine exited unexpectedly:', err)
  })
  let api = window.sarWasm
  if (!api) {
    // Some runtimes defer the entrypoint by a tick.
    await new Promise((resolve) => setTimeout(resolve, 0))
    api = window.sarWasm
  }
  if (!api) throw new Error('the wasm engine did not register window.sarWasm')
  return api
}

export async function getEngine(): Promise<Engine> {
  const api = await loadEngine()
  return {
    info: () => unwrap<EngineInfo>(api.version()),
    defaults: () => unwrap<{ gamma: GammaTables; lpmod: string[] }>(api.defaults()),
    parseGamma: (text) => unwrap<GammaTables>(api.parseGamma(text)),
    parseLpmod: (text) => unwrap<string[]>(api.parseLpmod(text)),
    compute: (input) => unwrap<Result>(api.compute(JSON.stringify(input))),
    saveData: (result, isoNow) => api.saveData(JSON.stringify(result), isoNow),
  }
}
