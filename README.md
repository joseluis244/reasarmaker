# SAR MAKER for Win — reimplementación en Go + htmx

Reimplementación del binario **`ocsarmk.exe`** (PE32 GUI, MFC/C++,
`FujiPhotoFilm_MiyaD1_2002_10_21`), analizado con REA/desensamblado
estático de Capstone. El binario original es una herramienta de QA para
monitores médicos (DICOM GSDF) que lee tablas de calibración y genera un
archivo de datos `sar_savedata.txt` que luego postprocesa `SarmakerPost.exe`.

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

- **Puntos de interés**: 5 columnas (density/shift/contrast) como en el
  diálogo del binario. Cada punto cargado tiene un QL, una densidad, un
  shift (−100..100) y un contraste (10..400, 100 = neutro).
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

## Uso

```sh
go build -o sar-maker . && ./sar-maker   # http://localhost:8080
```

1. Sube `lpmod.prm` y `gamma2.prm` (botón *Load files*).
2. Elige GAMMA No. y parámetros, pulsa *Compute* (htmx actualiza la tabla,
   la curva SVG y el enlace de descarga).
3. *Download sar_savedata.txt* genera el archivo con la estructura original.
