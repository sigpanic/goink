import { useTranslation } from 'react-i18next'
import Markdown from '@/components/Markdown'
import { splitFrontmatter, type SkillSource } from './types'

interface Props {
  content: string
  source?: SkillSource
}

export default function SkillPreview({ content, source }: Props) {
  const { t } = useTranslation()
  if (!content) {
    return (
      <div className="flex items-center justify-center h-full">
        <p className="text-sm text-muted-foreground">{t('content.noContent')}</p>
      </div>
    )
  }

  const { meta, body } = splitFrontmatter(content)

  return (
    <div className="overflow-auto h-full">
      {Object.keys(meta).length > 0 && (
        <div className="px-6 pt-6 pb-4">
          <table className="border bg-muted/20 w-full text-sm">
            <tbody>
              {meta.name && (
                <tr className="border-b">
                  <td className="px-4 py-2.5 text-muted-foreground whitespace-nowrap w-20">{t('content.name')}</td>
                  <td className="px-4 py-2.5 text-foreground font-semibold">{meta.name}</td>
                </tr>
              )}
              {meta.description && (
                <tr className="border-b">
                  <td className="px-4 py-2.5 text-muted-foreground whitespace-nowrap w-20">{t('content.summary')}</td>
                  <td className="px-4 py-2.5 text-foreground">{meta.description}</td>
                </tr>
              )}
              {meta.category && (
                <tr>
                  <td className="px-4 py-2.5 text-muted-foreground whitespace-nowrap w-20">{t('content.category')}</td>
                  <td className="px-4 py-2.5 text-foreground">{meta.category}</td>
                </tr>
              )}
              {meta.mode && (
                <tr>
                  <td className="px-4 py-2.5 text-muted-foreground whitespace-nowrap w-20">{t('content.mode')}</td>
                  <td className="px-4 py-2.5">
                    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium ${
                      meta.mode === 'manual' ? 'bg-tag-blue text-tag-blue-foreground' :
                      meta.mode === 'always' ? 'bg-tag-green text-tag-green-foreground' :
                      'bg-tag-amber text-tag-amber-foreground'
                    }`}>
                      {meta.mode === 'manual' ? t('content.command') : meta.mode === 'always' ? t('content.permanent') : t('content.smart')}
                    </span>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
      {meta.mode === 'always' && (source === 'user' || source === 'novel') && (
        <div className="px-6 pb-2">
          <p className="text-xs text-muted-foreground">
            {source === 'user' ? t('skill.alwaysScopeUser') : t('skill.alwaysScopeNovel')}
          </p>
        </div>
      )}
      <div className="px-6 py-4">
        <Markdown content={body} />
      </div>
    </div>
  )
}
