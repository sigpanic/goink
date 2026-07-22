#!/usr/bin/env node
/**
 * Theme Rules CI Check Script
 *
 * Checks that frontend source files follow the theme-rules.md conventions:
 * 1. No Tailwind palette colors in business components (slate, gray, blue-500, etc.)
 * 2. No hex hardcoded colors in className/style (bg-[#xxx], text-[#xxx])
 * 3. No oklch() absolute values in TSX/TS files (except PALETTE definitions)
 * 4. No `dark:` Tailwind prefix in business components (excluding ui/ directory)
 * 5. No `dark ? ... : ...` ternary theme logic in TSX/TS files
 * 6. No `background: white` or hex colors in CSS files (except index.css)
 *
 * Exemptions (per theme-design.md §7 "不动的东西"):
 * - text-red-500/600, text-rose-500: functional colors for errors/love
 * - bg-black/XX with opacity: overlay masks, not theme colors
 * - text-white on image overlays: reasonable for contrast on images
 * - graphColors.ts, index.css: theme token definitions
 * - components/ui/: shadcn/ui generated code
 * - useTheme.ts: theme hook itself
 * - Markdown.tsx: dark ternary for markdown rendering
 */

import { readFileSync, readdirSync, statSync } from 'fs'
import { resolve, dirname, join, relative } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const SRC_DIR = resolve(__dirname, '../src')

let hasErrors = false
let totalErrors = 0
let totalWarnings = 0

function error(msg) {
  console.error(`❌ ${msg}`)
  hasErrors = true
  totalErrors++
}

function warn(msg) {
  console.warn(`⚠️  ${msg}`)
  totalWarnings++
}

// ── File discovery ──

const SKIP_DIRS = new Set([
  'node_modules', '.git', 'dist', 'build',
])

const SKIP_FILES = new Set([
  'index.css',           // Theme token definitions
  'graphColors.ts',      // Canvas palette, uses Record<Theme, Colors>
  'arcColors.ts',        // Story arc palette, oklch absolute values (theme-design.md §6)
  'useTheme.ts',         // Theme hook itself, contains dark: intentionally
])

const SKIP_DIR_PREFIXES = [
  join(SRC_DIR, 'components', 'ui'),  // shadcn/ui generated code
]

function shouldSkipDir(dirPath) {
  const name = dirPath.split('/').pop() || dirPath.split('\\').pop() || ''
  if (SKIP_DIRS.has(name)) return true
  return SKIP_DIR_PREFIXES.some(p => dirPath.startsWith(p))
}

function shouldSkipFile(filePath) {
  const name = filePath.split('/').pop() || filePath.split('\\').pop() || ''
  if (SKIP_FILES.has(name)) return true
  // Markdown.tsx dark ternary is exempted
  if (name === 'Markdown.tsx') return true
  return false
}

function walkDir(dir, exts) {
  let files = []
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    const stat = statSync(full)
    if (stat.isDirectory()) {
      if (!shouldSkipDir(full)) {
        files = files.concat(walkDir(full, exts))
      }
    } else if (stat.isFile()) {
      const ext = entry.split('.').pop()
      if (exts.includes(ext) && !shouldSkipFile(full)) {
        files.push(full)
      }
    }
  }
  return files
}

// ── Check patterns ──

// 1. Tailwind palette colors in className
// Matches: bg-slate-*, text-blue-*, border-emerald-*, etc.
const PALETTE_RE = /\b(bg|text|border|ring|shadow|from|to|via)-(slate|gray|zinc|neutral|stone|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d{2,3}\b/g

// Standalone bg-white, text-white, bg-black, text-black (without opacity)
const STANDALONE_COLOR_RE = /\b(bg|text|border)-(white|black)\b/g

// bg-black/XX with opacity (overlay masks) - allowed
const OVERLAY_RE = /\b(bg|text|border)-(white|black)\/\d+/g

// Functional colors exempted per theme-design.md §7
const FUNCTIONAL_COLORS = ['text-red-500', 'text-red-600', 'text-rose-500']

// 2. Hex hardcoded colors in className/style: bg-[#xxx], text-[#xxx]
const HEX_IN_CLASS_RE = /\b(bg|text|border|ring|shadow|from|to|via)-\[(#[0-9a-fA-F]{3,8})\]/g

// 3. oklch() absolute values in TSX/TS files (not in variable definitions)
const OKLCH_RE = /oklch\([^)]*\)/g

// 4. dark: Tailwind prefix in TSX/TS files
const DARK_PREFIX_RE = /\bdark:/g

// 5. dark ternary: dark ? ... : ...
const DARK_TERNARY_RE = /\bdark\s*\?\s*/g

// ── CSS file specific checks ──
const CSS_BG_WHITE_RE = /background:\s*white\b/g
const CSS_HEX_RE = /(?:color|background|border-color|border):\s*#[0-9a-fA-F]{3,8}\b/g
const CSS_DARK_RE = /@apply[^;]*dark:/g

// ── Helpers ──

function isSkippableLine(line) {
  const trimmed = line.trimStart()
  return trimmed.startsWith('//') || trimmed.startsWith('*') || trimmed.startsWith('/*') || trimmed.startsWith('import ')
}

function isFunctionalColor(matchStr) {
  return FUNCTIONAL_COLORS.some(fc => matchStr.startsWith(fc))
}

function isOverlayMask(matchStr) {
  return OVERLAY_RE.test(matchStr)
}

function isImageOverlay(line, prevLine = '', nextLine = '') {
  // text-white on image overlays: must be white for contrast on dark overlays
  // Patterns: bg-black/XX on same line, or Camera/text-white combo on overlay containers
  if (/bg-black\/\d+/.test(line)) return true
  if (/absolute/.test(line) && /overlay/.test(line)) return true
  if (/group-hover/.test(line) && /absolute/.test(line)) return true
  // Camera icon with text-white on dark overlay (pattern: bg-black/40 on parent line)
  if (/text-white/.test(line) && /Camera/.test(line)) return true
  return false
}

// ── Run checks ──

const tsxFiles = walkDir(SRC_DIR, ['tsx', 'ts'])
const cssFiles = walkDir(SRC_DIR, ['css'])

console.log(`\n🔍 Theme Rules Check`)
console.log(`   Scanning ${tsxFiles.length} TS/TSX files, ${cssFiles.length} CSS files\n`)

// ── Check 1: Tailwind palette colors ──
console.log('── Check 1: Tailwind palette colors ──')
let check1Errors = 0
for (const file of tsxFiles) {
  const content = readFileSync(file, 'utf8')
  const rel = relative(SRC_DIR, file)
  const lines = content.split('\n')

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    if (isSkippableLine(line)) continue

    let match
    PALETTE_RE.lastIndex = 0
    while ((match = PALETTE_RE.exec(line)) !== null) {
      // Exempt functional colors (red, rose for errors/love)
      if (isFunctionalColor(match[0])) {
        warn(`${rel}:${i + 1}: Functional color "${match[0]}" (exempted per theme-design.md §7)`)
        continue
      }
      error(`${rel}:${i + 1}: Found "${match[0]}" — use semantic class instead`)
      check1Errors++
    }

    STANDALONE_COLOR_RE.lastIndex = 0
    while ((match = STANDALONE_COLOR_RE.exec(line)) !== null) {
      // Exempt bg-black/XX overlay masks
      const fullMatch = line.substring(match.index, match.index + 20)
      if (/\b(bg|text|border)-(white|black)\/\d+/.test(fullMatch)) continue

      // Exempt text-white/bg-black on image overlays
      if ((match[0] === 'text-white' || match[0] === 'bg-black') && isImageOverlay(line)) {
        warn(`${rel}:${i + 1}: "${match[0]}" on image overlay (exempted)`)
        continue
      }

      // bg-black/40 style overlay masks in the same line
      if (match[0] === 'bg-black' && /bg-black\/\d+/.test(line)) continue

      error(`${rel}:${i + 1}: Found "${match[0]}" — use semantic class instead`)
      check1Errors++
    }
  }
}
if (check1Errors === 0) console.log('   ✅ No blocking palette color violations\n')

// ── Check 2: Hex hardcoded colors in className ──
console.log('── Check 2: Hex hardcoded colors (bg-[#xxx], text-[#xxx]) ──')
let check2Errors = 0
for (const file of tsxFiles) {
  const content = readFileSync(file, 'utf8')
  const rel = relative(SRC_DIR, file)
  const lines = content.split('\n')

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    if (isSkippableLine(line)) continue

    let match
    HEX_IN_CLASS_RE.lastIndex = 0
    while ((match = HEX_IN_CLASS_RE.exec(line)) !== null) {
      // Skip PALETTE definitions
      if (line.includes('PALETTE') || line.includes('fill:') || line.includes('stroke:')) continue
      error(`${rel}:${i + 1}: Found hex color "${match[0]}" — use semantic variable instead`)
      check2Errors++
    }
  }
}
if (check2Errors === 0) console.log('   ✅ No hex hardcoded colors found\n')

// ── Check 3: oklch() absolute values in TSX/TS ──
console.log('── Check 3: oklch() absolute values in TS/TSX ──')
let check3Errors = 0
for (const file of tsxFiles) {
  const content = readFileSync(file, 'utf8')
  const rel = relative(SRC_DIR, file)
  const lines = content.split('\n')

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    if (isSkippableLine(line)) continue

    let match
    OKLCH_RE.lastIndex = 0
    while ((match = OKLCH_RE.exec(line)) !== null) {
      // Allow in PALETTE/graph definitions
      if (line.includes('PALETTE') || line.includes('fill:') || line.includes('stroke:')) continue
      error(`${rel}:${i + 1}: Found oklch absolute value — use CSS variable instead`)
      check3Errors++
    }
  }
}
if (check3Errors === 0) console.log('   ✅ No oklch absolute values found\n')

// ── Check 4: dark: Tailwind prefix ──
console.log('── Check 4: dark: Tailwind prefix in business components ──')
let check4Errors = 0
for (const file of tsxFiles) {
  const content = readFileSync(file, 'utf8')
  const rel = relative(SRC_DIR, file)
  const lines = content.split('\n')

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    if (isSkippableLine(line)) continue
    // Skip TS object key patterns: { light: ..., dark: ... } or Record<Theme, ...>
    if (/\b(?:light|dark)\s*:/.test(line) && !/className|class=/.test(line)) continue
    // Skip TS/JSX object values: dark: <JSX>, dark: 'str', dark: t(...)
    if (/\bdark:\s*(?:<|'|t\()/.test(line)) continue

    let match
    DARK_PREFIX_RE.lastIndex = 0
    while ((match = DARK_PREFIX_RE.exec(line)) !== null) {
      error(`${rel}:${i + 1}: Found "dark:" prefix — use semantic variables or [data-theme="dark"] CSS selector instead`)
      check4Errors++
    }
  }
}
if (check4Errors === 0) console.log('   ✅ No dark: prefixes found\n')

// ── Check 5: dark ternary ──
console.log('── Check 5: dark ternary (dark ? ... : ...) ──')
let check5Errors = 0
for (const file of tsxFiles) {
  const content = readFileSync(file, 'utf8')
  const rel = relative(SRC_DIR, file)
  const lines = content.split('\n')

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    if (isSkippableLine(line)) continue

    let match
    DARK_TERNARY_RE.lastIndex = 0
    while ((match = DARK_TERNARY_RE.exec(line)) !== null) {
      error(`${rel}:${i + 1}: Found "dark ? ... : ..." ternary — use Record<Theme, T> dictionary instead`)
      check5Errors++
    }
  }
}
if (check5Errors === 0) console.log('   ✅ No dark ternaries found\n')

// ── Check 6: CSS file violations ──
console.log('── Check 6: CSS file violations (background: white, hex colors) ──')
let check6Errors = 0
for (const file of cssFiles) {
  const content = readFileSync(file, 'utf8')
  const rel = relative(SRC_DIR, file)
  const lines = content.split('\n')

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]

    let match
    CSS_BG_WHITE_RE.lastIndex = 0
    while ((match = CSS_BG_WHITE_RE.exec(line)) !== null) {
      error(`${rel}:${i + 1}: Found "background: white" — use var(--card) instead`)
      check6Errors++
    }

    CSS_HEX_RE.lastIndex = 0
    while ((match = CSS_HEX_RE.exec(line)) !== null) {
      error(`${rel}:${i + 1}: Found hex color "${match[0]}" — use CSS variable or oklch() instead`)
      check6Errors++
    }

    CSS_DARK_RE.lastIndex = 0
    while ((match = CSS_DARK_RE.exec(line)) !== null) {
      error(`${rel}:${i + 1}: Found "dark:" in @apply — use [data-theme="dark"] selector instead`)
      check6Errors++
    }
  }
}
if (check6Errors === 0) console.log('   ✅ No CSS violations found\n')

// ── Summary ──
console.log('═'.repeat(50))
if (totalWarnings > 0) {
  console.log(`⚠️  ${totalWarnings} warning(s) (exempted patterns)`)
}
if (hasErrors) {
  console.error(`\n❌ Theme rules check failed with ${totalErrors} error(s)`)
  process.exit(1)
} else {
  console.log(`\n✅ All theme rules checks passed`)
  process.exit(0)
}
