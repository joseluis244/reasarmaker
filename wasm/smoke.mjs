// Differential smoke test: runs the wasm smoke-test cases through BOTH
// the native Go engine (`go run ./wasm/check`) and the compiled WebAssembly
// module, and asserts the outputs are identical.
//
// Usage (from the repo root):  make smoke
//   (builds webapp/public/sar.wasm first, then runs this script)
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'
import { execFileSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const require = createRequire(import.meta.url)
const here = path.dirname(fileURLToPath(import.meta.url))
const root = path.join(here, '..')

const goroot = execFileSync('go', ['env', 'GOROOT'], { encoding: 'utf8' }).trim()

// Mirror the setup of Go 1.25's wasm_exec_node.js launcher, but as a
// library so we can drive the module programmatically.
globalThis.require = require
globalThis.fs = require('node:fs')
globalThis.path = require('node:path')
globalThis.TextEncoder = require('node:util').TextEncoder
globalThis.TextDecoder = require('node:util').TextDecoder
globalThis.performance ??= require('node:perf_hooks').performance
globalThis.crypto ??= require('node:crypto')
require(path.join(goroot, 'lib', 'wasm', 'wasm_exec.js'))

const cases = JSON.parse(readFileSync(path.join(here, 'testdata', 'cases.json'), 'utf8'))

// --- native side ---
const nativeOut = execFileSync('go', ['run', './wasm/check'], { cwd: root, encoding: 'utf8' })
const native = JSON.parse(nativeOut)

// --- wasm side ---
const wasmPath = path.join(root, 'webapp', 'public', 'sar.wasm')
const go = new globalThis.Go()
go.argv = ['node', 'sar.wasm']
go.env = Object.assign({ TMPDIR: require('node:os').tmpdir() }, process.env)
go.exit = process.exit
const inst = await WebAssembly.instantiate(
  await WebAssembly.compile(readFileSync(wasmPath)),
  go.importObject,
)
go.run(inst) // main() registers globalThis.sarWasm before blocking
const api = globalThis.sarWasm
if (!api) throw new Error('sarWasm was not registered on globalThis')

const wasm = []
for (const c of cases) {
  switch (c.op) {
    case 'defaults':
      wasm.push(api.defaults())
      break
    case 'parseGamma':
    case 'parseLpmod': {
      const text = readFileSync(path.join(root, c.file), 'utf8')
      wasm.push(c.op === 'parseGamma' ? api.parseGamma(text) : api.parseLpmod(text))
      break
    }
    case 'compute':
      wasm.push(api.compute(JSON.stringify(c.input)))
      break
    case 'saveData': {
      const env = JSON.parse(wasm[c.useResult])
      wasm.push(api.saveData(JSON.stringify(env.data), c.now))
      break
    }
    default:
      throw new Error(`unknown op ${c.op}`)
  }
}

// normalize walks the parsed JSON and rounds every finite float to 12
// significant digits. Go's compiler may contract mul+add into FMA on
// native targets (arm64) while the js/wasm soft-float path is strict
// IEEE, so display-only floats such as the shift weights can differ by
// 1 ulp between the two builds; integer engine outputs are exact and
// remain compared bit-for-bit (12 significant digits never alters an
// integer).
const normalize = (v) => {
  if (typeof v === 'number') {
    if (!Number.isFinite(v)) return v
    if (v === 0) return 0
    const exp = Math.floor(Math.log10(Math.abs(v)))
    const factor = 10 ** (12 - exp)
    return Math.round(v * factor) / factor
  }
  if (Array.isArray(v)) return v.map(normalize)
  if (v && typeof v === 'object') {
    const out = {}
    for (const [k, val] of Object.entries(v)) out[k] = normalize(val)
    return out
  }
  return v
}

const parseIfJSON = (v) => {
  if (typeof v !== 'string') return v
  try {
    return JSON.parse(v)
  } catch {
    return v
  }
}

let failed = false
for (let i = 0; i < native.length; i++) {
  const got = JSON.stringify(normalize(parseIfJSON(wasm[i])))
  const want = JSON.stringify(normalize(parseIfJSON(native[i])))
  if (got !== want) {
    failed = true
    console.error(`case ${i} (${cases[i].op}) MISMATCH:`)
    console.error('  native:', want.slice(0, 500))
    console.error('  wasm  :', got.slice(0, 500))
  }
}
if (failed) process.exit(1)
console.log(`smoke OK: ${cases.length} calls identical between native Go and wasm`)
