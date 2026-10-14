import { useEffect, useState } from 'react'
import type { EngineInfo, GammaTables, POI, Result } from './types'
import { getEngine, type Engine } from './wasm/bridge'
import { ResultsPanel } from './components/ResultsPanel'

interface PoiRow {
  density: string
  shift: string
  contrast: string
}

const emptyRow = (): PoiRow => ({ density: '', shift: '0', contrast: '100' })
const initialRows = (): PoiRow[] => Array.from({ length: 5 }, emptyRow)

// The original dialog exposes a tuning point selection, but the engine's
// recovered pipeline derives the pivot from each point of interest, so the
// value is not user-relevant: keep the original default (24) internally.
const DEFAULT_TUNING_IDX = 24

// The values last applied by Compute. GAMMA No. / Base Color changes
// recompute automatically, while the points of interest and Dmin/Dmax
// take effect only when Compute is pressed.
interface Applied {
  dmin: string
  dmax: string
  pois: PoiRow[]
}

type Status = { kind: 'ok' | 'error'; text: string }

function num(s: string, def: number): number {
  const v = Number.parseFloat(s.trim())
  return Number.isNaN(v) ? def : v
}

function int(s: string, def: number): number {
  const v = Number.parseInt(s.trim(), 10)
  return Number.isNaN(v) ? def : v
}

// The Dmin/Dmax inputs use the density×100 scale of the original dialog
// (300 = 3.00). Reflect a computed curve min/max back into that scale.
function densityToInput(v: number): string {
  return String(Math.round(v * 100))
}

export default function App() {
  const [engine, setEngine] = useState<Engine | null>(null)
  const [bootError, setBootError] = useState<string | null>(null)
  const [engineInfo, setEngineInfo] = useState<EngineInfo | null>(null)

  const [gammas, setGammas] = useState<GammaTables | null>(null)
  const [status, setStatus] = useState<Status | null>(null)

  const [gammaNo, setGammaNo] = useState(11)
  const [baseColor, setBaseColor] = useState<'clear' | 'blue'>('clear')
  const [dmin, setDmin] = useState('0')
  const [dmax, setDmax] = useState('300')
  const [pois, setPois] = useState<PoiRow[]>(initialRows)
  const [applied, setApplied] = useState<Applied>({
    dmin: '0',
    dmax: '300',
    pois: initialRows(),
  })

  const [result, setResult] = useState<Result | null>(null)

  useEffect(() => {
    let cancelled = false
    getEngine()
      .then((eng) => {
        if (cancelled) return
        setEngine(eng)
        try {
          setEngineInfo(eng.info())
        } catch {
          // info() is cosmetic; the engine itself is usable
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setBootError(err instanceof Error ? err.message : String(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [])

  // The sample calibration files are embedded in the engine build, so the
  // app starts with them already loaded — no upload required.
  useEffect(() => {
    if (!engine) return
    try {
      const defaults = engine.defaults()
      setGammas(defaults.gamma)
      setGammaNo((cur) =>
        defaults.gamma.numbers.includes(cur) ? cur : defaults.gamma.numbers[0],
      )
      setStatus({
        kind: 'ok',
        text: `Loaded: ${defaults.gamma.numbers.length} gamma tables (embedded defaults)`,
      })
    } catch (err) {
      setStatus({
        kind: 'error',
        text: err instanceof Error ? err.message : String(err),
      })
    }
  }, [engine])
  const busy = engine === null

  function runCompute(announce: boolean, use?: Applied) {
    if (!engine || !gammas) return
    const table = gammas.tables[String(gammaNo)]
    if (!table) {
      setStatus({
        kind: 'error',
        text: `Check GAMMA Table: G${gammaNo} not found. Load gamma2.prm first.`,
      })
      return
    }
    const p = use ?? applied
    // Only the density is required per row: rows without a density are
    // skipped, and the QL is derived from the gamma table (the curve point
    // whose density is closest to the entered density).
    const points: POI[] = p.pois
      .filter((r) => r.density.trim() !== '')
      .map((r) => {
        const density = num(r.density, 0)
        let best = table.length > 0 ? table[0] : { ql: 0, density: 0 }
        for (const pt of table) {
          if (Math.abs(pt.density - density) < Math.abs(best.density - density)) best = pt
        }
        return {
          ql: best.ql,
          density,
          shift: num(r.shift, 0),
          contrast: num(r.contrast, 100),
        }
      })
    try {
      const res = engine.compute({
        table: { number: gammaNo, points: table },
        params: {
          gammaNo,
          baseColor,
          tuningIdx: DEFAULT_TUNING_IDX,
          dmin: num(p.dmin, 0),
          dmax: num(p.dmax, 300),
          points,
        },
      })
      setResult(res)
      // Reflect the curve's actual min/max back into the Dmin/Dmax
      // inputs (density × 100), so they always match the displayed
      // `GAMMA NO : n — dmax X, dmin Y` line for the current curve.
      setDmin(densityToInput(res.dmin))
      setDmax(densityToInput(res.dmax))
      if (announce) setStatus({ kind: 'ok', text: 'save complete' })
    } catch (err) {
      setStatus({ kind: 'error', text: err instanceof Error ? err.message : String(err) })
    }
  }

  // GAMMA No. / Base Color changes recompute the curve automatically
  // (debounced), so the chart and its GAMMA NO / dmax / dmin line are
  // always in sync — G11 appears by default without pressing Compute.
  useEffect(() => {
    if (!engine || !gammas) return
    const timer = setTimeout(() => runCompute(false), 250)
    return () => clearTimeout(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [engine, gammas, gammaNo, baseColor, applied])

  // Compute applies the points of interest and Dmin/Dmax drafts.
  function onCompute(e: React.FormEvent) {
    e.preventDefault()
    const next: Applied = { dmin, dmax, pois }
    setApplied(next)
    runCompute(true, next)
  }

  function setPoi(i: number, key: keyof PoiRow, value: string) {
    setPois((rows) => rows.map((r, j) => (j === i ? { ...r, [key]: value } : r)))
  }

  return (
    <>
      <h1>
        SAR MAKER for Win{' '}
        <span className="subtitle">— React + WebAssembly reimplementation</span>
      </h1>

      {bootError && <div className="boot">{bootError}</div>}
      {status && <div className={`status ${status.kind}`}>{status.text}</div>}

      <div className="workspace">
        <div className="panel stack left">
          <h3>Parameters</h3>
          <div className="field">
            <label htmlFor="gamma_no">GAMMA No.</label>
            <select
              id="gamma_no"
              value={gammaNo}
              onChange={(e) => setGammaNo(int(e.target.value, 11))}
              disabled={busy || !gammas}
            >
              {(gammas?.numbers ?? [gammaNo]).map((n) => (
                <option key={n} value={n}>
                  G{n}
                </option>
              ))}
            </select>
          </div>
          <div className="field">
            <label htmlFor="base_color">Base Color</label>
            <select
              id="base_color"
              value={baseColor}
              onChange={(e) => setBaseColor(e.target.value as 'clear' | 'blue')}
              disabled={busy}
            >
              <option value="clear">Clear</option>
              <option value="blue">Blue</option>
            </select>
          </div>
        </div>

        <div className="panel right">
          {result ? (
            <ResultsPanel result={result} />
          ) : (
            <div className="placeholder">
              {busy ? 'Cargando engine…' : 'Ajusta los parámetros para ver la curva.'}
            </div>
          )}
        </div>
      </div>

      <form className="panel adjust" onSubmit={onCompute}>
        <div className="row">
          <div className="field">
            <label htmlFor="dmin">Dmin</label>
            <input
              type="number"
              step={0.01}
              id="dmin"
              value={dmin}
              onChange={(e) => setDmin(e.target.value)}
              disabled={busy}
            />
          </div>
          <div className="field">
            <label htmlFor="dmax">Dmax</label>
            <input
              type="number"
              step={0.01}
              id="dmax"
              value={dmax}
              onChange={(e) => setDmax(e.target.value)}
              disabled={busy}
            />
          </div>
        </div>

        <h3>Points of interest</h3>
        <table className="poi">
          <thead>
            <tr>
              <th>Point</th>
              <th>Density</th>
              <th>Shift</th>
              <th>Contrast</th>
            </tr>
          </thead>
          <tbody>
            {pois.map((r, i) => (
              <tr key={i}>
                <td>{i + 1}</td>
                <td>
                  <input
                    type="number"
                    step={0.0001}
                    id={`p${i}_density`}
                    placeholder="—"
                    value={r.density}
                    onChange={(e) => setPoi(i, 'density', e.target.value)}
                    disabled={busy}
                  />
                </td>
                <td>
                  <input
                    type="number"
                    step={0.01}
                    id={`p${i}_shift`}
                    value={r.shift}
                    onChange={(e) => setPoi(i, 'shift', e.target.value)}
                    disabled={busy}
                  />
                </td>
                <td>
                  <input
                    type="number"
                    step={0.01}
                    id={`p${i}_contrast`}
                    value={r.contrast}
                    onChange={(e) => setPoi(i, 'contrast', e.target.value)}
                    disabled={busy}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="hint">
          Solo la densidad es obligatoria: deja Density vacío para no usar un punto. El QL se
          deriva automáticamente del punto de la curva con la densidad más cercana; el punto
          aplica su shift a la curva alrededor de ese punto (smoothstep) y su contraste
          alrededor de su densidad.
        </p>
        <div className="compute-row">
          <button type="submit" className="btn" disabled={busy}>
            Compute
          </button>
        </div>
      </form>

      {engineInfo && (
        <p className="footer">
          engine: {engineInfo.engine} · source: {engineInfo.source} · binary:{' '}
          {engineInfo.binary} · sha256: {engineInfo.sha256}
        </p>
      )}
    </>
  )
}
