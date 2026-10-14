import type { Result } from '../types'
import { DensityScale } from '../types'

const W = 520
const H = 260
const GRID = [0, 0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5]

function polyline(result: Result, values: number[]): string {
  return result.tuning
    .map((ql, i) => {
      const x = 40 + (ql / 1023) * (W - 48)
      const y = H - 20 - (values[i] / DensityScale / 3.5) * (H - 40)
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
}

export function Chart({ result }: { result: Result }) {
  const weightValues = result.weights.map((w) => Math.min(w, 1))
  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="chart">
      <rect width="100%" height="100%" fill="#0b0f14" />
      {GRID.map((d) => {
        const y = H - 20 - (d / 3.5) * (H - 40)
        return (
          <g key={d}>
            <line x1={40} y1={y} x2={W - 8} y2={y} stroke="#22303c" />
            <text x={6} y={y + 3} fill="#8aa0b4" fontSize={10}>
              {d.toFixed(1)}
            </text>
          </g>
        )
      })}
      <polyline
        points={polyline(result, result.densityWords)}
        fill="none"
        stroke="#5a7f96"
        strokeWidth={1.5}
      />
      <polyline
        points={polyline(result, result.changedWords)}
        fill="none"
        stroke="#4cc38a"
        strokeWidth={1.5}
      />
      <polyline
        points={polyline(result, weightValues)}
        fill="none"
        stroke="#ffb454"
        strokeWidth={1}
        strokeDasharray="3 3"
      />
      <text x={10} y={14} fill="#5a7f96" fontSize={10}>
        before
      </text>
      <text x={60} y={14} fill="#4cc38a" fontSize={10}>
        after
      </text>
      <text x={104} y={14} fill="#ffb454" fontSize={10}>
        shift weight
      </text>
    </svg>
  )
}
