import type { Result } from '../types'
import { Chart } from './Chart'

export function ResultsPanel({ result }: { result: Result }) {
  return (
    <div id="results">
      <p className="gamma-info">
        GAMMA NO : {result.gammaNo} — dmax {result.dmax.toFixed(2)}, dmin{' '}
        {result.dmin.toFixed(2)}
      </p>
      <Chart result={result} />
    </div>
  )
}
