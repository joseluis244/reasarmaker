# SAR MAKER for Win — React + Vite con engine en WebAssembly

Reimplementación del binario **`ocsarmk.exe`** (PE32 GUI, MFC/C++,
`FujiPhotoFilm_MiyaD1_2002_10_21`), analizado con REA (SHA-256 del binario:
`4a6a413ceb603b9ba30fdd0921c3bf3c46050acd2ec9a7a61ab073c88338332a`). El
binario original es una herramienta de QA para monitores médicos
(DICOM GSDF) que lee tablas de calibración y genera un archivo de datos
`sar_savedata.txt` que luego postprocesa `SarmakerPost.exe`.

## Arquitectura

```
webapp/                  React 19 + Vite 8 + TypeScript (frontend)
  src/wasm/bridge.ts     carga sar.wasm y tipa la API del engine
  src/App.tsx            UI: cabecera/alertas, GAMMA No.+Base Color (izq),
                         curva automática (dcha), Dmin/Dmax + puntos de
                         interés + Compute (sección inferior)
  src/components/        curva SVG
  public/sar.wasm        ← engine compilado a WebAssembly (generado)

wasm/main.go             build js/wasm: expone el engine como global `sarWasm`
wasm/check/              CLI nativo que evalúa los mismos casos (diff-test)
wasm/smoke.mjs           test diferencial: Go nativo vs wasm, outputs idénticos

internal/sar/            port exacto de los algoritmos de ocsarmk.exe (Go puro)
internal/sarwire/        tipos JSON de intercambio JS ↔ engine (camelCase)
internal/defaults/       gamma2.prm y LPMOD.PRM embebidos con go:embed
```

El engine es el paquete Go `internal/sar` (el port recuperado del binario),
compilado con `GOOS=js GOARCH=wasm`. React lo invoca desde el navegador:
toda la lógica del `ocsarmk.exe` (parseo de `LPMOD.PRM`/`gamma2.prm`,
interpolación, smoothstep, contraste, clamp monótono y generación de
`sar_savedata.txt`) corre en WebAssembly, sin servidor. Los archivos de
calibración de ejemplo viajan **embebidos dentro del propio wasm**
(`internal/defaults`, export `defaults()`) y se cargan automáticamente al
arrancar la app: la UI solo expone las tablas gamma embebidas.

## Qué hace el original (evidencia del binario)

- Lee `LPMOD.PRM` (LUTs `$LP1_CLEAR`, `$LP1_BLUE`, `$LP2_CLEAR`, `$LP2_BLUE`,
  301 pares `QL -> L` cada una) y `gamma2.prm` (30 curvas `G11`..`G47` de 48
  puntos `(ql,density)`); también espera `GlatFmt.txt`.
- Parámetros de ajuste: GAMMA No., Base Color (Clear/Blue), Contrast, Shift,
  Density, Dmin, Dmax, Tuning Point.
- Guarda `sar_savedata.txt` (cabecera con fecha `%Y-%m-%d %H:%M:%S`,
  `GAMMA NO :n`, tabla `dmax dmin density shift contrast`, sección
  `Changed Density data (48 points)`) y lanza `SarmakerPost.exe` con
  `CreateProcessA`; usa `CreateMutexA` para instancia única.
- Modos: D3.0 / D3.3 / D3.6 × Clear/Blue.

## Algoritmos recuperados (port exacto en `internal/sar`)

| Original  | Go                    | Fórmula |
|-----------|-----------------------|---------|
| `sub_4018E0` | `sar.Interp`      | `v1 + (x-k1)*(v2-v1)/(k2-k1)` |
| `sub_4017A0` | `sar.BuildLUT`    | LUT de 1024 niveles por interpolación lineal a tramos sobre los 48 tuning points; `trunc(v+0.5)` |
| `sub_40A680` | `sar.ShiftWeights` | curva de pesos smoothstep (`3t²-2t³`) sobre los 48 puntos, centrada en el tuning point; 1 en el pivote, 0 en los bordes (branch izquierdo exacto) |
| `sub_40A870` | en `Compute`      | `changed[i] = trunc(orig[i] + shift*w[i] + 0.5)` |
| `sub_40A5D0` | en `Compute`      | contraste: `v' = trunc(v*c - (c-1)*anchor)`, `c = contrast/100`, `anchor` = densidad del punto de interés |
| `sub_40A940` | `enforceMonotonic`| clamp a [Dmin,Dmax] + curva monótona no decreciente |
| `0x40D110=10000` | `DensityScale` | densidad en punto fijo: `word = trunc(densidad*10000)` |

## Puntos de interés, tuning point y shift (flujo del original)

- **Puntos de interés**: 5 filas (density/shift/contrast) como en el
  diálogo del binario. Cada punto cargado tiene un QL, una densidad, un
  shift (−100..100) y un contraste (10..400, 100 = neutro). En la UI
  **solo la densidad es obligatoria**: el QL no aparece en la interfaz y
  se deriva automáticamente del punto de la curva con la densidad más
  cercana.
- **Tuning point**: selecciona el punto de la curva (índice 0..47 o QL);
  el shift se aplica con peso smoothstep centrado en ese punto: máximo en
  el pivote, decayendo suavemente a 0 en los extremos.
- **Validaciones** (sub_406710): densidad del punto dentro de
  (Dmin, Dmax), shift ∈ [−100, 100], contraste ∈ [10, 400],
  `densidad + shift ∈ [0, Dmax]`, Dmin ≤ Dmax ("Input Error" si no).

## Reconstrucción parcial (documentada)

- El branch derecho de `sub_40A680` (pivote ≥ 512) añade un offset +1.0
  con una discontinuidad 0→2; aquí se usa el espejo suave (1 en el pivote,
  0 en ambos bordes), preservando la semántica del branch izquierdo.
- El ancla de contraste (suma `a+b` de `sub_40A5D0`) se interpreta como la
  densidad del punto de interés; los argumentos exactos de la cadena
  `sub_4062C0 → 40A5D0/40AA10/40A680/40A870` no se trazaron campo a campo.
- El formato de fila exacto de `sar_savedata.txt` (whitespace/puntuación)
  no se reprodujo byte a byte; se mantiene la estructura y todos los valores.
- La generación de imagen SAR (sub_409120, buffers 1024×48) y el lanzamiento
  de `SarmakerPost.exe` no están incluidos.
- **Varianza de 1 ulp en flotantes**: el compilador de Go puede contraer
  mul+add en FMA en targets nativos (arm64) mientras el soft-float de
  js/wasm es IEEE estricto, por lo que valores de display (p. ej. los
  pesos smoothstep) pueden diferir en ~1 ulp entre ambas builds. Las
  salidas enteras del engine (density words, `sar_savedata.txt`) son
  exactas; `wasm/smoke.mjs` compara con tolerancia de 12 dígitos
  significativos solo para flotantes.

## Uso

Requisitos: Go 1.25+, Node 20.19+.

```sh
make wasm          # compila el engine Go → webapp/public/sar.wasm
cd webapp
npm install        # primera vez
npm run dev        # http://localhost:5173
```

1. Las tablas gamma de ejemplo (`gamma2.prm`) van embebidas en el wasm y
   **se cargan automáticamente al arrancar**; no hay selectores de
   archivos.
2. Cabecera con títulos y alertas; debajo, una sección de dos columnas
   del **mismo tamaño**: a la izquierda la tarjeta de parámetros (GAMMA
   No. y Base Color), a la derecha la tarjeta con la **curva**, que
   aparece por defecto (G11) y se **recalcula automáticamente** al cambiar
   GAMMA No. o Base Color, con su línea `GAMMA NO : n — dmax X, dmin Y`
   siempre visible y sincronizada con la curva.
3. Debajo, una sección a ancho completo con **Dmin y Dmax** lado a lado —
   sus valores se **sincronizan automáticamente con la curva** mostrada
   (mismo valor que la línea `dmax X, dmin Y`, multiplicado por 100: p. ej.
   `dmax 3.00, dmin 0.15` → Dmax 300, Dmin 15) —, los **puntos de interés**
   (solo densidad, shift y contraste; el QL se deriva automáticamente) y
   el botón **Compute** al final, de uso exclusivo de esta sección: aplica
   los cambios de Dmin/Dmax y de los puntos de interés.

### Builds y tests

| Comando | Qué hace |
|---|---|
| `make test` | tests del engine Go (`internal/sar`, `internal/sarwire`) |
| `make smoke` | test diferencial: las mismas llamadas en Go nativo y en el wasm deben producir outputs idénticos |
| `make e2e` | end-to-end en Chrome headless real: boot del wasm, gammas embebidas, compute y download |
| `make web-build` | build de producción (`webapp/dist`, incluye el wasm) |
| `make dev` | dev server de Vite |

> El `make e2e` usa el Chrome local (por defecto
> `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`; se puede
> cambiar con `CHROME_PATH`).

## Evidencia REA

El binario `data/ocsarmk.exe` está analizado con REA (skill
`reverse-engineer-anything`): `open_binary` confirma PE32 x86 con 695
procedimientos y las funciones clave del port existen en las direcciones
documentadas arriba (`sub_4017a0`, `sub_40a680`, `sub_40a5d0`,
`sub_40a940`, `sub_40a870`, `sub_406710`, `sub_409120`, `sub_4056f0`,
`sub_406580`). El frontend muestra el SHA-256 del binario analizado en el
pie de página (lo reporta el propio engine wasm vía `version()`).
