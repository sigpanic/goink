#!/usr/bin/env node
/**
 * i18n CI Check Script
 *
 * Checks:
 * 1. Semantic key consistency between en.json and zh-CN.json
 * 2. No duplicate keys in either JSON file (within same object scope)
 * 3. No empty translations in zh-CN.json and en.json
 * 4. No hardcoded Chinese in .tsx/.ts source files
 */

import { readFileSync, readdirSync } from 'fs'
import { resolve, dirname, join, relative } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const LOCALES_DIR = resolve(__dirname, '../src/i18n/locales')
const SRC_DIR = resolve(__dirname, '../src')

let hasErrors = false

function error(msg) {
  console.error(`❌ ${msg}`)
  hasErrors = true
}

// ── 1. Extract all leaf keys from a nested JSON object ──
function extractKeys(obj, prefix = '') {
  const keys = new Set()
  for (const [k, v] of Object.entries(obj)) {
    const fullKey = prefix ? `${prefix}.${k}` : k
    if (typeof v === 'object' && v !== null) {
      for (const subKey of extractKeys(v, fullKey)) {
        keys.add(subKey)
      }
    } else {
      keys.add(fullKey)
    }
  }
  return keys
}

// ── Plural key helpers ──
// Extract the base key from a plural key (strips _one / _other suffix)
function getBaseKey(key) {
  return key.replace(/_one$/, '').replace(/_other$/, '')
}

// Check if a key is a plural suffix key
function isPluralKey(key) {
  return key.endsWith('_one') || key.endsWith('_other')
}

// Get the set of "semantic" base keys from a set of raw keys.
// For plural keys (_one/_other), only the base key is included.
// For non-plural keys, the key itself is included.
function extractSemanticBaseKeys(keys) {
  const baseKeys = new Set()
  for (const key of keys) {
    if (isPluralKey(key)) {
      baseKeys.add(getBaseKey(key))
    } else {
      baseKeys.add(key)
    }
  }
  return baseKeys
}

// Given a set of raw keys, return the set of base keys that have
// BOTH _one and _other forms (i.e. complete plural pairs)
function extractCompletePluralBases(keys) {
  const oneKeys = new Set()
  const otherKeys = new Set()
  for (const key of keys) {
    if (key.endsWith('_one')) oneKeys.add(getBaseKey(key))
    if (key.endsWith('_other')) otherKeys.add(getBaseKey(key))
  }
  const complete = new Set()
  for (const base of oneKeys) {
    if (otherKeys.has(base)) complete.add(base)
  }
  return complete
}

// ── 2. Check for duplicate keys in raw JSON (within same object scope) ──
function findDuplicateKeys(filePath) {
  const content = readFileSync(filePath, 'utf-8')
  const lines = content.split('\n')
  const duplicates = []
  // Track keys at each nesting depth
  const depthKeys = new Map() // depth -> Map of key -> count
  let currentDepth = 0

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    // Match key-value pairs at current depth BEFORE processing braces,
    // because a line like "character": { has the key at the outer depth.
    const match = line.match(/^\s*"([^"]+)"\s*:/)
    if (match) {
      const key = match[1]
      const keysAtDepth = depthKeys.get(currentDepth)
      if (keysAtDepth) {
        const count = (keysAtDepth.get(key) || 0) + 1
        keysAtDepth.set(key, count)
        if (count > 1) {
          duplicates.push(key)
        }
      }
    }
    // Count braces to track depth
    for (const ch of line) {
      if (ch === '{' || ch === '[') {
        currentDepth++
        depthKeys.set(currentDepth, new Map())
      } else if (ch === '}' || ch === ']') {
        depthKeys.delete(currentDepth)
        currentDepth--
      }
    }
  }
  return duplicates
}

// ── Main checks ──

console.log('🔍 i18n consistency check\n')

// Load JSON files
const zhCNPath = resolve(LOCALES_DIR, 'zh-CN.json')
const enPath = resolve(LOCALES_DIR, 'en.json')

const zhCN = JSON.parse(readFileSync(zhCNPath, 'utf-8'))
const en = JSON.parse(readFileSync(enPath, 'utf-8'))

// Check 1: Semantic key structure consistency
// Rules:
//   - en _one/_other pairs → zh-CN must have the base key (no suffix)
//   - en non-suffix keys → zh-CN must have the same non-suffix key
//   - zh-CN non-suffix keys → en must have the same non-suffix key or _one/_other pair
//   - zh-CN should NOT have _one/_other suffix keys
console.log('📋 Checking semantic key structure consistency...')
const zhKeys = extractKeys(zhCN)
const enKeys = extractKeys(en)

// 1a. zh-CN should not have _one/_other keys
const zhPluralKeys = [...zhKeys].filter(k => isPluralKey(k))
if (zhPluralKeys.length > 0) {
  error(`zh-CN should not have _one/_other keys (${zhPluralKeys.length}):\n  ${zhPluralKeys.join('\n  ')}`)
}

// Compute semantic base keys for both languages
const enBaseKeys = extractSemanticBaseKeys(enKeys)
const enCompletePluralBases = extractCompletePluralBases(enKeys)
const zhBaseKeys = extractSemanticBaseKeys(zhKeys)

// 1b. en _one/_other pairs → zh-CN must have the base key
const enPluralMissingInZh = [...enCompletePluralBases].filter(k => !zhKeys.has(k))
if (enPluralMissingInZh.length > 0) {
  error(`en has _one/_other pair but zh-CN missing base key (${enPluralMissingInZh.length}):\n  ${enPluralMissingInZh.join('\n  ')}`)
}

// 1c. en non-suffix keys → zh-CN must have the same non-suffix key
//     (keys in en that are NOT part of a _one/_other pair and are not plural suffixes)
const enNonPluralBaseKeys = [...enBaseKeys].filter(k => !enCompletePluralBases.has(k))
const enNonPluralMissingInZh = enNonPluralBaseKeys.filter(k => !zhKeys.has(k))
if (enNonPluralMissingInZh.length > 0) {
  error(`en has non-plural key but zh-CN missing it (${enNonPluralMissingInZh.length}):\n  ${enNonPluralMissingInZh.join('\n  ')}`)
}

// 1d. zh-CN non-suffix keys → en must have the same non-suffix key or _one/_other pair
const zhMissingInEn = [...zhKeys].filter(k => {
  if (isPluralKey(k)) return false // already checked in 1a
  return !enKeys.has(k) && !enCompletePluralBases.has(k)
})
if (zhMissingInEn.length > 0) {
  error(`zh-CN has key but en missing it (no base key or _one/_other pair) (${zhMissingInEn.length}):\n  ${zhMissingInEn.join('\n  ')}`)
}

// 1e. en _one without _other or vice versa (incomplete plural pair)
const enOneOnly = [...enKeys].filter(k => k.endsWith('_one') && !enKeys.has(getBaseKey(k) + '_other'))
const enOtherOnly = [...enKeys].filter(k => k.endsWith('_other') && !enKeys.has(getBaseKey(k) + '_one'))
const incompletePlural = [...enOneOnly.map(k => k + ' (missing _other)'), ...enOtherOnly.map(k => k + ' (missing _one)')]
if (incompletePlural.length > 0) {
  error(`en has incomplete plural pairs (${incompletePlural.length}):\n  ${incompletePlural.join('\n  ')}`)
}

const check1Pass = zhPluralKeys.length === 0
  && enPluralMissingInZh.length === 0
  && enNonPluralMissingInZh.length === 0
  && zhMissingInEn.length === 0
  && incompletePlural.length === 0
if (check1Pass) {
  console.log('  ✅ zh-CN and en have semantically consistent key structures\n')
}

// Check 2: Duplicate keys
console.log('📋 Checking for duplicate keys...')
const zhDups = findDuplicateKeys(zhCNPath)
const enDups = findDuplicateKeys(enPath)

if (zhDups.length > 0) {
  error(`Duplicate keys in zh-CN.json (same parent object): ${zhDups.join(', ')}`)
}
if (enDups.length > 0) {
  error(`Duplicate keys in en.json (same parent object): ${enDups.join(', ')}`)
}
if (zhDups.length === 0 && enDups.length === 0) {
  console.log('  ✅ No duplicate keys found\n')
}

// Check 3: Empty translations (both zh-CN and en, including _one/_other plural keys)
console.log('📋 Checking for empty translations...')
function findEmptyKeys(keys, obj, label) {
  const emptyKeys = []
  for (const key of keys) {
    const parts = key.split('.')
    let val = obj
    for (const p of parts) {
      val = val?.[p]
    }
    if (typeof val === 'string' && val.trim() === '') {
      emptyKeys.push(key)
    }
  }
  if (emptyKeys.length > 0) {
    error(`Empty translations in ${label} (${emptyKeys.length}):\n  ${emptyKeys.join('\n  ')}`)
  } else {
    console.log(`  ✅ No empty translations in ${label}`)
  }
  return emptyKeys.length
}
const emptyErrors = findEmptyKeys(enKeys, en, 'en.json') + findEmptyKeys(zhKeys, zhCN, 'zh-CN.json')
if (emptyErrors === 0) {
  console.log()
}

// Check 4: Hardcoded Chinese in .tsx/.ts source files
console.log('📋 Checking for hardcoded Chinese in source files...')

// Recursively find all .tsx/.ts files, excluding i18n locale JSONs
function findSourceFiles(dir, exts = ['.tsx', '.ts']) {
  const results = []
  const entries = readdirSync(dir, { withFileTypes: true })
  for (const entry of entries) {
    const fullPath = join(dir, entry.name)
    if (entry.isDirectory()) {
      // Skip node_modules, .git, i18n/locales
      if (entry.name === 'node_modules' || entry.name === '.git') continue
      results.push(...findSourceFiles(fullPath, exts))
    } else if (exts.some(ext => entry.name.endsWith(ext))) {
      // 跳过测试文件：测试夹具中的中文是后端错误消息测例，非 UI 文案
      if (/\.(test|spec)\.(ts|tsx)$/.test(entry.name)) continue
      results.push(fullPath)
    }
  }
  return results
}

// Patterns to skip (not actual hardcoded Chinese)
const SKIP_PATTERNS = [
  /import\s/,                          // import statements
  /\/\/.*[\u4e00-\u9fff]/,             // single-line comments with Chinese
  /\/\*[\s\S]*?\*\//,                  // multi-line comments with Chinese
  /\.includes\(['"][\u4e00-\u9fff]/,   // msg.includes('中文') — backend string matching
  /console\.(log|warn|error|debug)/,   // console output
  /i18next/i,                          // i18n config references
  /label:\s*['"][\u4e00-\u9fff]/,      // language selector native names (e.g. label: '中文')
]

// Extract lines with hardcoded Chinese that are NOT in t() calls
function findHardcodedChinese(filePath) {
  const content = readFileSync(filePath, 'utf-8')
  const lines = content.split('\n')
  const findings = []

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const lineNum = i + 1
    const trimmed = line.trim()

    // Skip empty lines
    if (!trimmed) continue

    // Skip lines that are comments
    if (trimmed.startsWith('//') || trimmed.startsWith('*') || trimmed.startsWith('/*')) continue

    // Skip import lines
    if (trimmed.startsWith('import ')) continue

    // Check if line contains Chinese characters
    if (!/[\u4e00-\u9fff]/.test(line)) continue

    // Skip lines where Chinese is inside a t() call
    if (/t\(['"`][\s\S]*[\u4e00-\u9fff]/.test(line)) continue

    // Skip console.log/warn/error lines
    if (/console\.(log|warn|error|debug)\(/.test(line)) continue

    // Skip .includes() with Chinese (backend message matching)
    if (/\.includes\(['"`][\u4e00-\u9fff]/.test(line)) continue

    // Skip language selector native names (e.g. label: '中文')
    if (/label:\s*['"][\u4e00-\u9fff]/.test(line)) continue

    // Skip type declaration lines (TypeScript)
    if (trimmed.startsWith('type ') || trimmed.startsWith('interface ') || trimmed.startsWith('declare ')) continue

    // Extract the Chinese-containing string for reporting
    const chineseMatches = line.match(/['"`][^'"`]*[\u4e00-\u9fff][^'"`]*['"`]/g)
    if (chineseMatches) {
      findings.push({ lineNum, matches: chineseMatches, line: trimmed })
    }
  }
  return findings
}

const sourceFiles = findSourceFiles(SRC_DIR)
const hardcodedFindings = []

for (const file of sourceFiles) {
  const relPath = relative(resolve(__dirname, '..'), file)
  const findings = findHardcodedChinese(file)
  if (findings.length > 0) {
    hardcodedFindings.push({ file: relPath, findings })
  }
}

if (hardcodedFindings.length > 0) {
  const totalFindings = hardcodedFindings.reduce((sum, f) => sum + f.findings.length, 0)
  const details = hardcodedFindings.map(f =>
    `  ${f.file}:\n${f.findings.map(d => `    L${d.lineNum}: ${d.matches.join(', ')}`).join('\n')}`
  ).join('\n')
  error(`Hardcoded Chinese in source files (${totalFindings} occurrences in ${hardcodedFindings.length} files):\n${details}`)
} else {
  console.log('  ✅ No hardcoded Chinese found in source files\n')
}

// ── Summary ──
if (hasErrors) {
  console.log('\n❌ i18n check FAILED — fix the errors above before merging.')
  process.exit(1)
} else {
  console.log('\n✅ All i18n checks passed!')
  process.exit(0)
}
