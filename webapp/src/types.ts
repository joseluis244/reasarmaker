// Wire types shared with the Go WebAssembly engine (internal/sarwire).
// Field names mirror the JSON tags of the Go wire types.

export interface EngineInfo {
  engine: string
  source: string
  binary: string
  sha256: string
  toolchain: string
}

export interface GammaPoint {
  ql: number
  density: number
}

export interface GammaTables {
  numbers: number[]
  tables: Record<string, GammaPoint[]>
}

export interface POI {
  ql: number
  density: number
  shift: number
  contrast: number
}

export interface Params {
  gammaNo: number
  baseColor: 'clear' | 'blue'
  tuningIdx: number
  dmin: number
  dmax: number
  points: POI[]
}

export interface GammaTable {
  number: number
  points: GammaPoint[]
}

export interface ComputeInput {
  table: GammaTable
  params: Params
}

export interface Result {
  gammaNo: number
  baseColor: string
  tuning: number[]
  densityWords: number[]
  changedWords: number[]
  lut: number[]
  weights: number[]
  dmax: number
  dmin: number
  modeLabel: string
}

export const DensityScale = 10000
