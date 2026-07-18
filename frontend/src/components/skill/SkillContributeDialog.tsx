import { Heart, ExternalLink, FileText, GitFork } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { BrowserOpenURL } from '@/lib/wailsjs/runtime/runtime'

const REPO = 'sigpanic/goink-skills'
const BRANCH = 'main'

interface Props {
  open: boolean
  onClose: () => void
}

export default function SkillContributeDialog({ open, onClose }: Props) {
  const { t } = useTranslation()

  if (!open) return null

  const links = {
    template: `https://github.com/${REPO}/blob/${BRANCH}/.template/skill.md`,
    fork: `https://github.com/${REPO}/fork`,
    guide: `https://github.com/${REPO}/blob/${BRANCH}/README.md`,
  }

  const steps = [
    { icon: <GitFork className="w-5 h-5 shrink-0" />, text: t('skill.contributeStep1') },
    { icon: <FileText className="w-5 h-5 shrink-0" />, text: t('skill.contributeStep2') },
    { icon: <Heart className="w-5 h-5 shrink-0" />, text: t('skill.contributeStep3') },
  ]

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-background border rounded-xl shadow-xl w-[520px] max-h-[80vh] overflow-y-auto">
        {/* Header */}
        <div className="flex items-center gap-2 px-5 py-4 border-b">
          <Heart className="w-5 h-5 text-rose-500" />
          <h2 className="text-base font-semibold">{t('skill.contribute')}</h2>
          <button
            onClick={onClose}
            className="ml-auto text-muted-foreground hover:text-foreground transition-colors text-sm"
          >
            {t('common.close')}
          </button>
        </div>

        <div className="px-5 py-4 space-y-5">
          {/* Description */}
          <p className="text-sm text-muted-foreground leading-relaxed">
            {t('skill.contributeDesc')}
          </p>

          {/* Steps */}
          <div className="space-y-3">
            <h3 className="text-sm font-medium text-muted-foreground uppercase tracking-wider">
              {t('skill.contributeHowTo')}
            </h3>
            {steps.map((step, i) => (
              <div key={i} className="flex items-start gap-2.5">
                <span className="mt-0.5 flex items-center justify-center w-6 h-6 rounded-full bg-primary/10 text-primary text-xs font-bold shrink-0">
                  {i + 1}
                </span>
                <div className="flex items-center gap-1.5 text-sm text-foreground/80 pt-0.5">
                  {step.icon}
                  <span>{step.text}</span>
                </div>
              </div>
            ))}
          </div>

          {/* Format reference */}
          <div className="space-y-2">
            <h3 className="text-sm font-medium text-muted-foreground uppercase tracking-wider">
              {t('skill.contributeFormatTitle')}
            </h3>
            <pre className="text-xs font-mono bg-muted/60 rounded-lg p-3 leading-relaxed text-foreground/70 overflow-x-auto">
{`---
name: my-skill
description: ${t('skill.contributeFormatDesc')}
category: ${t('skill.contributeFormatCategory')}
mode: auto
author: ${t('skill.contributeFormatAuthor')}
version: 1
---`}
            </pre>
          </div>

          {/* Action buttons */}
          <div className="flex flex-col gap-2 pt-1">
            <button
              onClick={() => BrowserOpenURL(links.fork)}
              className="flex items-center justify-center gap-1.5 h-9 rounded-lg text-sm font-medium bg-primary text-primary-foreground hover:bg-primary/90 transition-colors cursor-pointer"
            >
              <GitFork className="w-4 h-4" />
              {t('skill.forkAndContribute')}
              <ExternalLink className="w-3.5 h-3.5 opacity-60" />
            </button>
            <div className="flex gap-2">
              <button
                onClick={() => BrowserOpenURL(links.template)}
                className="flex-1 flex items-center justify-center gap-1.5 h-8 rounded-lg text-sm font-medium border hover:bg-muted transition-colors cursor-pointer"
              >
                <FileText className="w-4 h-4" />
                {t('skill.viewTemplate')}
              </button>
              <button
                onClick={() => BrowserOpenURL(links.guide)}
                className="flex-1 flex items-center justify-center gap-1.5 h-8 rounded-lg text-sm font-medium border hover:bg-muted transition-colors cursor-pointer"
              >
                {t('skill.viewGuide')}
                <ExternalLink className="w-3.5 h-3.5 opacity-60" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
