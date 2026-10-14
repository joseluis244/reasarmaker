// End-to-end test: boots the Vite dev server, drives the app in a real
// Chrome (headless), checks the embedded gammas load, computes, and checks
// the results. Requires the `wasm` build (`npm run wasm`) and a local
// Chrome.
//
// Usage (from webapp/):  npm run e2e
import { spawn } from 'node:child_process'
import { existsSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import puppeteer from 'puppeteer-core'

const here = path.dirname(fileURLToPath(import.meta.url))

const CHROME = process.env.CHROME_PATH ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
if (!existsSync(CHROME)) {
  console.error(`Chrome not found at ${CHROME} (set CHROME_PATH)`)
  process.exit(1)
}
if (!existsSync(path.join(here, '..', 'public', 'sar.wasm'))) {
  console.error('public/sar.wasm missing — run `npm run wasm` first')
  process.exit(1)
}

const port = 5299
const vite = spawn('npx', ['vite', '--port', String(port), '--strictPort'], {
  cwd: path.join(here, '..'),
  stdio: 'ignore',
})

const waitForServer = async (url) => {
  for (let i = 0; i < 60; i++) {
    try {
      const resp = await fetch(url)
      if (resp.ok) return
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 250))
  }
  throw new Error('dev server did not start')
}

const fail = (msg) => {
  console.error('E2E FAIL:', msg)
  process.exitCode = 1
}

let browser = null
try {
  await waitForServer(`http://localhost:${port}/`)
  browser = await puppeteer.launch({
    executablePath: CHROME,
    headless: true,
    args: ['--no-first-run', '--disable-gpu'],
  })
  const page = await browser.newPage()
  await page.setViewport({ width: 1280, height: 900 })
  const errors = []
  page.on('pageerror', (err) => errors.push(String(err)))
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      const loc = msg.location()
      errors.push(`console: ${msg.text()} @ ${loc?.url ?? '?'}`)
    }
  })
  page.on('response', (resp) => {
    if (resp.status() >= 400) errors.push(`HTTP ${resp.status()}: ${resp.url()}`)
  })

  const waitStatus = async (predicate, label) => {
    const deadline = Date.now() + 10000
    for (;;) {
      const text = await page
        .evaluate(() => {
          const el = document.querySelector('.status.ok')
          return el ? (el.textContent ?? '') : ''
        })
        .catch(() => '')
      if (text !== '' && predicate(text)) return text
      if (Date.now() > deadline) {
        const dump = await page.evaluate(() => ({
          body: document.body.innerHTML.slice(0, 1500),
          statuses: [...document.querySelectorAll('.status')].map((el) => el.textContent),
        }))
        fail(`${label} timed out: ${dump.statuses.join(' | ')}\nPAGE:\n${dump.body}`)
        throw new Error(`${label} timed out`)
      }
      await new Promise((r) => setTimeout(r, 250))
    }
  }

  await page.goto(`http://localhost:${port}/`, { waitUntil: 'networkidle0' })

  // 1. engine boots and registers sarWasm
  const info = await page.evaluate(() => {
    const footer = document.querySelector('.footer')?.textContent ?? ''
    return footer
  })
  if (!info.includes('sar-wasm')) fail(`engine footer missing: ${info}`)

  // 2. embedded gammas load automatically
  const autoLoaded = await waitStatus((t) => t.includes('embedded defaults'), 'defaults auto-load')

  // 2b. the G11 curve appears by default, without pressing Compute, with
  //     its GAMMA NO / dmax / dmin line always present
  await page.waitForSelector('#results svg polyline', { timeout: 10000 })
  const defaultInfo = await page.$eval('#results .gamma-info', (el) => el.textContent)
  if (!defaultInfo.includes('GAMMA NO : 11')) fail(`default curve info wrong: ${defaultInfo}`)

  // the Dmin/Dmax inputs mirror the curve's min/max (×100)
  const checkInputs = async () => {
    const infoText = await page.$eval('#results .gamma-info', (el) => el.textContent)
    const m = infoText.match(/dmax ([\d.]+), dmin ([\d.]+)/)
    const dmaxVal = await page.$eval('#dmax', (el) => el.value)
    const dminVal = await page.$eval('#dmin', (el) => el.value)
    const expDmax = String(Math.round(parseFloat(m[1]) * 100))
    const expDmin = String(Math.round(parseFloat(m[2]) * 100))
    if (dmaxVal !== expDmax || dminVal !== expDmin) {
      return `mismatch: inputs ${dminVal}/${dmaxVal}, expected ${expDmin}/${expDmax}`
    }
    return 'ok'
  }
  const sync0 = await checkInputs()
  if (sync0 !== 'ok') fail(`Dmin/Dmax not synced to curve: ${sync0}`)

  // 2c. changing the gamma updates the curve and its info line live
  await page.select('#gamma_no', '12')
  await page.waitForFunction(
    () => document.querySelector('#results .gamma-info')?.textContent?.includes('GAMMA NO : 12'),
    { timeout: 10000 },
  )
  const sync12 = await checkInputs()
  if (sync12 !== 'ok') fail(`Dmin/Dmax not synced to G12 curve: ${sync12}`)
  await page.select('#gamma_no', '11')
  await page.waitForFunction(
    () => document.querySelector('#results .gamma-info')?.textContent?.includes('GAMMA NO : 11'),
    { timeout: 10000 },
  )

  // 3. no Tuning Point input and no QL column in the points-of-interest table
  const removed = await page.evaluate(() => {
    if (document.querySelector('#tuning_idx')) return 'tuning-idx-present'
    if (document.querySelector('#p0_ql')) return 'poi-ql-present'
    const headers = [...document.querySelectorAll('table.poi th')].map((th) =>
      th.textContent?.trim(),
    )
    if (headers.some((h) => h === 'QL')) return `ql-header: ${headers.join(',')}`
    return 'ok'
  })
  if (removed !== 'ok') fail(`unexpected controls: ${removed}`)

  // 4. the adjust section (Dmin/Dmax + points of interest + Compute) sits
  //    below the two-column workspace, with Dmin/Dmax side by side above
  //    the Points of interest heading and Compute at the bottom
  const adjustLayout = await page.evaluate(() => {
    const ws = document.querySelector('.workspace')?.getBoundingClientRect()
    const box = document.querySelector('.adjust')
    const sec = box?.getBoundingClientRect()
    if (!ws || !sec) return 'missing'
    if (!(sec.top >= ws.bottom - 1)) {
      return `adjust-not-below (ws.bottom=${ws.bottom}, sec.top=${sec.top})`
    }
    const dmin = box.querySelector('#dmin')?.getBoundingClientRect()
    const dmax = box.querySelector('#dmax')?.getBoundingClientRect()
    const h3 = box.querySelector('h3')?.getBoundingClientRect()
    const table = box.querySelector('table.poi')?.getBoundingClientRect()
    const btn = box.querySelector('button[type="submit"]')?.getBoundingClientRect()
    if (!dmin || !dmax || !h3 || !table || !btn) return 'missing-elements'
    if (Math.abs(dmin.top - dmax.top) > 2) {
      return `dmin/dmax not side by side (${dmin.top} vs ${dmax.top})`
    }
    if (!(dmax.bottom <= h3.top + 1)) return 'dmin/dmax not above points-of-interest'
    if (!(table.bottom <= btn.top)) return 'compute not below the table'
    return 'ok'
  })
  if (adjustLayout !== 'ok') fail(`adjust section layout: ${adjustLayout}`)

  // 5. fill POI row 1 with a density only (QL stays derived) and a shift of 20
  await page.click('#p0_density')
  await page.type('#p0_density', '1.0')
  await page.click('#p0_shift')
  await page.type('#p0_shift', '20')

  // the curve must NOT change until Compute is pressed (the button is
  // exclusive to the points of interest / Dmin / Dmax section)
  const beforePoints = await page.$$eval('#results svg polyline', (els) =>
    els.map((e) => e.getAttribute('points') ?? ''),
  )
  await new Promise((r) => setTimeout(r, 600)) // longer than the auto-recompute debounce
  const draftPoints = await page.$$eval('#results svg polyline', (els) =>
    els.map((e) => e.getAttribute('points') ?? ''),
  )
  if (beforePoints[1] !== draftPoints[1]) {
    fail('curve changed before pressing Compute')
  }

  // 6. Compute applies the points of interest / Dmin / Dmax drafts
  await page.click('form.panel button[type="submit"]')
  await page.waitForFunction(
    () => {
      const els = document.querySelectorAll('#results svg polyline')
      return (
        els.length >= 2 && els[0].getAttribute('points') !== els[1].getAttribute('points')
      )
    },
    { timeout: 10000 },
  )
  const gammaInfo = await page.$eval('#results .gamma-info', (el) => el.textContent)
  if (!gammaInfo.includes('GAMMA NO : 11')) fail(`gamma info missing after compute: ${gammaInfo}`)
  const gammaValue = await page.$eval('#gamma_no', (el) => el.value)

  // the results show only the curve: no mode heading, no table, no button
  const noExtras = await page.evaluate(() => {
    if (document.querySelector('#results h2')) return 'mode-heading-present'
    if (document.querySelector('#results table')) return 'table-present'
    if (document.querySelector('#results button')) return 'download-present'
    return 'ok'
  })
  if (noExtras !== 'ok') fail(`unexpected results controls: ${noExtras}`)

  // the derived QL pivot must have applied the shift: the "after" curve
  // differs from the "before" curve
  const polylines = await page.$$eval('#results svg polyline', (els) =>
    els.map((e) => e.getAttribute('points') ?? ''),
  )
  if (polylines.length < 2) fail(`expected before/after polylines, got ${polylines.length}`)
  if (polylines[0] === polylines[1]) fail('density-only POI with shift had no effect (before == after)')

  // the right column must actually sit to the right of the params column
  const geometry = await page.evaluate(() => {
    const left = document.querySelector('.workspace .left')?.getBoundingClientRect()
    const right = document.querySelector('.workspace .right')?.getBoundingClientRect()
    if (!left || !right) return 'missing'
    if (!(right.left >= left.right - 1)) {
      return `not-right-side (left.right=${left.right}, right.left=${right.left})`
    }
    // both cards must be the same size for a uniform look
    if (Math.abs(left.width - right.width) > 2) {
      return `widths differ (${left.width} vs ${right.width})`
    }
    if (Math.abs(left.height - right.height) > 2) {
      return `heights differ (${left.height} vs ${right.height})`
    }
    return 'ok'
  })
  if (geometry !== 'ok') fail(`layout geometry wrong: ${geometry}`)

  // 7. download content sanity check (via the wasm saveData path)
  const saved = await page.evaluate(async () => {
    const engine = window.sarWasm
    const res = JSON.parse(engine.compute(JSON.stringify({
      table: { number: 11, points: [{ ql: 0, density: 0.5 }] },
      params: { gammaNo: 11, baseColor: 'clear', tuningIdx: 24, dmin: 0, dmax: 300, points: [] },
    })))
    return engine.saveData(JSON.stringify(res.data), '2026-10-13T12:00:00Z')
  })

  if (!saved.includes('GAMMA NO :11') || !saved.includes('Changed Density data (48 points)')) {
    fail('saveData output structure wrong')
  }
  if (errors.length > 0) fail(`page errors: ${errors.join(' | ')}`)

  console.log('E2E OK')
  console.log('  defaults:', autoLoaded)
  console.log('  gamma# :', gammaValue)
  console.log('  info   :', gammaInfo)
  console.log('  chart  :', polylines[0]?.slice(0, 60) + '…')
  console.log('  saved  :', saved.split('\n').slice(0, 2).join(' / '))
} catch (err) {
  fail(String(err))
} finally {
  if (browser) await browser.close().catch(() => {})
  vite.kill()
}
