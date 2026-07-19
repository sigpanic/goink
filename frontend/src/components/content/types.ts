export type EditorTab = {
  id: string
  type: 'file' | 'diff'
  path: string
  title: string
  // file tab
  content?: string
  outlineContent?: string
  isDirty?: boolean
  viewMode?: 'content' | 'outline' | 'preview' | 'edit'
  readOnly?: boolean
  // diff tab
  diff?: string
  original?: string
  modified?: string
  changeType?: string
  reason?: string
  toolId?: string
}

// 文件名格式 chapters/001.md，outlines/001.md 同理
export function chapterPath(num: number): string {
  return `chapters/${String(num).padStart(3, '0')}.md`
}

export function outlinePath(num: number): string {
  return `outlines/${String(num).padStart(3, '0')}.md`
}

export function goinkPath(): string {
  return 'goink.md'
}

export function isContentPath(p: string): boolean {
  return p.startsWith('chapters/') || p === 'goink.md'
}

export function isOutlinePath(p: string): boolean {
  return p.startsWith('outlines/')
}

export function isSkillPath(p: string): boolean {
  return p.startsWith('skills/') || p.startsWith('~/.goink/skills/') || p.startsWith('/builtin/skills/')
}

export type SkillSource = 'builtin' | 'user' | 'novel'

// sourceFromPath infers the skill layer from its tab path.
// Mirrors the path conventions in SkillList.skillPath.
export function sourceFromPath(p: string): SkillSource {
  if (p.startsWith('/builtin/skills/')) return 'builtin'
  if (p.startsWith('~/.goink/skills/')) return 'user'
  return 'novel'
}

export function skillNameFromPath(p: string): string {
  return p.replace(/.*\//, '').replace('.md', '')
}

// splitFrontmatter splits YAML frontmatter from markdown content.
export function splitFrontmatter(content: string): { meta: Record<string, string>; body: string } {
  if (!content.startsWith('---')) {
    return { meta: {}, body: content }
  }
  const end = content.indexOf('\n---', 3)
  if (end === -1) {
    return { meta: {}, body: content }
  }
  const fm = content.substring(3, end).trim()
  const body = content.substring(end + 4).trim()
  const meta: Record<string, string> = {}
  for (const line of fm.split('\n')) {
    const i = line.indexOf(':')
    if (i > 0) {
      meta[line.substring(0, i).trim()] = line.substring(i + 1).trim()
    }
  }
  return { meta, body }
}

export function chapterNumFromPath(p: string): number {
  let n = 0
  if (p.startsWith('chapters/')) {
    const s = p.replace('chapters/', '').replace('.md', '')
    n = parseInt(s, 10)
  } else if (p.startsWith('outlines/')) {
    const s = p.replace('outlines/', '').replace('.md', '')
    n = parseInt(s, 10)
  }
  return n || 0
}
