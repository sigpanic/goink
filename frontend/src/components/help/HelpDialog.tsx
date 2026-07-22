import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  BookOpen,
  Wrench,
  Bot,
  Wand2,
  Cpu,
  Zap,
  ShieldCheck,
  Heart,
} from "lucide-react";
import SkillContributeDialog from "@/components/skill/SkillContributeDialog";

type Tab =
  | "quickstart"
  | "tools"
  | "subagents"
  | "skills"
  | "llm"
  | "context"
  | "approval";

interface Props {
  open: boolean;
  onClose: () => void;
}

// ── 工具参考（i18n key） ──────────────────────────

interface ToolEntry {
  name: string;
  desc: string;
}

const toolGroups: { label: string; tools: ToolEntry[] }[] = [
  {
    label: "help.toolGroupNovel",
    tools: [
      { name: "get_chapter_list", desc: "help.toolRef_get_chapter_list" },
      { name: "read", desc: "help.toolRef_read" },
      { name: "get_characters", desc: "help.toolRef_get_characters" },
      { name: "create_character", desc: "help.toolRef_create_character" },
      { name: "update_character", desc: "help.toolRef_update_character" },
      { name: "get_locations", desc: "help.toolRef_get_locations" },
      { name: "create_location", desc: "help.toolRef_create_location" },
      { name: "update_location", desc: "help.toolRef_update_location" },
      { name: "delete_record", desc: "help.toolRef_delete_record" },
    ],
  },
  {
    label: "help.toolGroupMemory",
    tools: [
      { name: "get_preferences", desc: "help.toolRef_get_preferences" },
      {
        name: "get_character_relations",
        desc: "help.toolRef_get_character_relations",
      },
      { name: "get_timeline", desc: "help.toolRef_get_timeline" },
      { name: "get_story_arcs", desc: "help.toolRef_get_story_arcs" },
      {
        name: "get_reader_perspective",
        desc: "help.toolRef_get_reader_perspective",
      },
      { name: "search_story_memory", desc: "help.toolRef_search_story_memory" },
    ],
  },
  {
    label: "help.toolGroupWriting",
    tools: [
      { name: "create_preference", desc: "help.toolRef_create_preference" },
      { name: "update_preference", desc: "help.toolRef_update_preference" },
      {
        name: "update_character_relationship",
        desc: "help.toolRef_update_character_relationship",
      },
      {
        name: "create_location_relation",
        desc: "help.toolRef_create_location_relation",
      },
      {
        name: "update_location_relation",
        desc: "help.toolRef_update_location_relation",
      },
      {
        name: "create_timeline_entry",
        desc: "help.toolRef_create_timeline_entry",
      },
      {
        name: "update_timeline_entry",
        desc: "help.toolRef_update_timeline_entry",
      },
      { name: "update_chapter_plan", desc: "help.toolRef_update_chapter_plan" },
      { name: "create_story_arc", desc: "help.toolRef_create_story_arc" },
      { name: "update_story_arc", desc: "help.toolRef_update_story_arc" },
      { name: "create_arc_node", desc: "help.toolRef_create_arc_node" },
      { name: "update_arc_node", desc: "help.toolRef_update_arc_node" },
      {
        name: "create_reader_perspective_entry",
        desc: "help.toolRef_create_reader_perspective_entry",
      },
      {
        name: "update_reader_perspective_entry",
        desc: "help.toolRef_update_reader_perspective_entry",
      },
      { name: "edit", desc: "help.toolRef_edit" },
      { name: "run_subagent", desc: "help.toolRef_run_subagent" },
      { name: "web_search", desc: "help.toolRef_web_search" },
      { name: "web_fetch", desc: "help.toolRef_web_fetch" },
    ],
  },
];

// ── 子代理介绍（i18n key） ──────────────────────────────────

const subAgentCards = [
  {
    type: "memory",
    name: "help.subAgentMemory",
    desc: "help.subagent_memoryDesc",
    example: "help.subagent_memoryExample",
  },
  {
    type: "review",
    name: "help.subAgentReview",
    desc: "help.subagent_reviewDesc",
    example: "help.subagent_reviewExample",
  },
];

// ── Tab 定义 ─────────────────────────────────────────────

export default function HelpDialog({ open, onClose }: Props) {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState<Tab>("quickstart");

  const tabs: { id: Tab; label: string; icon: React.ReactNode }[] = [
    {
      id: "quickstart",
      label: t("help.quickStart"),
      icon: <BookOpen className="w-4 h-4" />,
    },
    {
      id: "tools",
      label: t("help.toolReference"),
      icon: <Wrench className="w-4 h-4" />,
    },
    {
      id: "subagents",
      label: t("help.subagents"),
      icon: <Bot className="w-4 h-4" />,
    },
    {
      id: "skills",
      label: t("help.skillSystem"),
      icon: <Wand2 className="w-4 h-4" />,
    },
    {
      id: "llm",
      label: t("help.modelConfig"),
      icon: <Cpu className="w-4 h-4" />,
    },
    {
      id: "context",
      label: t("help.contextAndCache"),
      icon: <Zap className="w-4 h-4" />,
    },
    {
      id: "approval",
      label: t("help.approvalMode"),
      icon: <ShieldCheck className="w-4 h-4" />,
    },
  ];

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />

      <div className="relative bg-background rounded-xl shadow-2xl border flex w-[960px] h-[680px] max-w-[95vw] max-h-[90vh]">
        {/* 左侧导航 */}
        <nav className="w-[160px] border-r py-4 px-2 flex flex-col gap-1 shrink-0">
          <div className="text-sm font-medium px-3 pb-3 text-foreground">
            {t("help.help")}
          </div>
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-2 px-3 py-2 rounded-lg text-sm transition-colors w-full text-left ${
                activeTab === tab.id
                  ? "bg-primary/10 text-primary font-medium"
                  : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
              }`}
            >
              {tab.icon}
              {tab.label}
            </button>
          ))}
        </nav>

        {/* 右侧内容区 */}
        <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
          {/* 关闭按钮 */}
          <button
            onClick={onClose}
            className="absolute top-3 right-3 w-7 h-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors z-10"
          >
            ✕
          </button>

          <div className="flex-1 overflow-y-auto p-6">
            {activeTab === "quickstart" && <QuickStartTab />}
            {activeTab === "tools" && <ToolsTab />}
            {activeTab === "subagents" && <SubAgentsTab />}
            {activeTab === "skills" && <SkillsTab />}
            {activeTab === "llm" && <LLMConfigTab />}
            {activeTab === "context" && <ContextCacheTab />}
            {activeTab === "approval" && <ApprovalTab />}
          </div>
        </div>
      </div>
    </div>
  );
}

// ── 快速入门 ─────────────────────────────────────────────

function QuickStartTab() {
  const { t } = useTranslation();
  return (
    <div className="space-y-6 max-w-none">
      <section>
        <h2 className="text-lg font-semibold mb-2">
          {t("help.quickStart_welcomeTitle")}
        </h2>
        <p className="text-muted-foreground leading-relaxed">
          {t("help.quickStart_welcomeDesc")}
        </p>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">
          {t("help.quickStart_uiOverview")}
        </h3>
        <div className="space-y-3 text-sm text-muted-foreground leading-relaxed">
          <div>
            <span className="text-foreground font-medium">
              {t("help.quickStart_leftBar")}
            </span>
            —— {t("help.quickStart_leftBarDesc")}
          </div>
          <div>
            <span className="text-foreground font-medium">
              {t("help.quickStart_centerContent")}
            </span>
            —— {t("help.quickStart_centerContentDesc")}
          </div>
          <div>
            <span className="text-foreground font-medium">
              {t("help.quickStart_rightChat")}
            </span>
            —— {t("help.quickStart_rightChatDesc")}
          </div>
          <div>
            <span className="text-foreground font-medium">
              {t("help.quickStart_bottomStatus")}
            </span>
            —— {t("help.quickStart_bottomStatusDesc")}
          </div>
        </div>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">
          {t("help.quickStart_workflow")}
        </h3>
        <div className="space-y-2 text-sm text-muted-foreground leading-relaxed">
          <p>
            1.{" "}
            <span className="text-foreground font-medium">
              {t("help.quickStart_step1")}
            </span>{" "}
            —— {t("help.quickStart_step1Desc")}
          </p>
          <p>
            2.{" "}
            <span className="text-foreground font-medium">
              {t("help.quickStart_step2")}
            </span>{" "}
            —— {t("help.quickStart_step2Desc")}
          </p>
          <p>
            3.{" "}
            <span className="text-foreground font-medium">
              {t("help.quickStart_step3")}
            </span>{" "}
            —— {t("help.quickStart_step3Desc")}
          </p>
        </div>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">
          {t("help.quickStart_coreConcepts")}
        </h3>
        <div className="space-y-3 text-sm text-muted-foreground leading-relaxed">
          <p>
            <span className="text-foreground font-medium">
              {t("help.quickStart_conceptTools")}
            </span>{" "}
            —— {t("help.quickStart_conceptToolsDesc")}
          </p>
          <p>
            <span className="text-foreground font-medium">
              {t("help.quickStart_conceptSubagents")}
            </span>{" "}
            —— {t("help.quickStart_conceptSubagentsDesc")}
          </p>
          <p>
            <span className="text-foreground font-medium">
              {t("help.quickStart_conceptSkills")}
            </span>{" "}
            —— {t("help.quickStart_conceptSkillsDesc")}
          </p>
          <p>
            <span className="text-foreground font-medium">
              {t("help.quickStart_conceptApproval")}
            </span>{" "}
            —— {t("help.quickStart_conceptApprovalDesc")}
          </p>
        </div>
      </section>
    </div>
  );
}

// ── 工具参考 ─────────────────────────────────────────────

function ToolsTab() {
  const { t } = useTranslation();
  return (
    <div className="space-y-6">
      <p className="text-sm text-muted-foreground">{t("help.toolRef_intro")}</p>
      {toolGroups.map((group) => (
        <section key={group.label}>
          <h3 className="text-base font-semibold mb-3">{t(group.label)}</h3>
          <div className="space-y-2">
            {group.tools.map((tool) => (
              <div
                key={tool.name}
                className="rounded-lg border bg-card px-4 py-3"
              >
                <code className="text-sm font-medium text-primary">
                  {tool.name}
                </code>
                <p className="text-sm text-muted-foreground mt-1">
                  {t(tool.desc)}
                </p>
              </div>
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

// ── 子代理 ───────────────────────────────────────────────

function SubAgentsTab() {
  const { t } = useTranslation();
  return (
    <div className="space-y-6 max-w-none">
      <section>
        <h2 className="text-lg font-semibold mb-2">
          {t("help.subagent_whatIsTitle")}
        </h2>
        <p className="text-muted-foreground leading-relaxed">
          {t("help.subagent_whatIsDesc")}
        </p>
      </section>

      <section>
        <h3 className="text-base font-medium mb-3">
          {t("help.subagent_availableTypes")}
        </h3>
        <div className="space-y-4">
          {subAgentCards.map((sa) => (
            <div key={sa.type} className="rounded-lg border bg-card p-5">
              <div className="flex items-center gap-2 mb-2">
                <Bot className="w-4 h-4 text-primary" />
                <h4 className="font-semibold">{t(sa.name)}</h4>
                <code className="text-xs bg-muted px-1.5 py-0.5 rounded text-muted-foreground">
                  {sa.type}
                </code>
              </div>
              <p className="text-sm text-muted-foreground leading-relaxed mb-2">
                {t(sa.desc)}
              </p>
              <div className="text-sm text-muted-foreground bg-muted/50 rounded px-3 py-2">
                {t(sa.example)}
              </div>
            </div>
          ))}
        </div>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">
          {t("help.subagent_howToUse")}
        </h3>
        <p className="text-sm text-muted-foreground leading-relaxed">
          {t("help.subagent_howToUseDesc")}
        </p>
      </section>
    </div>
  );
}

// ── 技能系统 ─────────────────────────────────────────────

function SkillsTab() {
  const { t } = useTranslation();
  const [showContribute, setShowContribute] = useState(false);
  return (
    <div className="space-y-6 max-w-none">
      <section>
        <h2 className="text-lg font-semibold mb-2">
          {t("help.skill_whatIsTitle")}
        </h2>
        <p className="text-muted-foreground leading-relaxed">
          {t("help.skill_whatIsDesc")}
        </p>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">
          {t("help.skill_threeLayers")}
        </h3>
        <div className="space-y-2 text-sm text-muted-foreground leading-relaxed">
          <p>
            <span className="text-foreground font-medium">
              {t("help.skill_builtin")}
            </span>{" "}
            —— {t("help.skill_builtinDesc")}
          </p>
          <p>
            <span className="text-foreground font-medium">
              {t("help.skill_userLevel")}
            </span>{" "}
            —— {t("help.skill_userLevelDesc")}
          </p>
          <p>
            <span className="text-foreground font-medium">
              {t("help.skill_novelLevel")}
            </span>{" "}
            —— {t("help.skill_novelLevelDesc")}
          </p>
        </div>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">
          {t("help.skill_howToUse")}
        </h3>
        <p className="text-sm text-muted-foreground leading-relaxed">
          {t("help.skill_howToUseDesc")}
        </p>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">
          {t("help.skill_howToCreate")}
        </h3>
        <div className="text-sm text-muted-foreground leading-relaxed space-y-2">
          <p>{t("help.skill_howToCreateIntro")}</p>
          <div className="bg-muted/50 rounded-lg p-4 font-mono text-xs leading-relaxed">
            <p className="text-foreground/60">---</p>
            <p>
              name:{" "}
              <span className="text-foreground">{t("help.skill_mdName")}</span>
            </p>
            <p>
              description:{" "}
              <span className="text-foreground">
                {t("help.skill_mdDescription")}
              </span>
            </p>
            <p>
              category:{" "}
              <span className="text-foreground">
                {t("help.skill_mdCategory")}
              </span>
            </p>
            <p className="text-foreground/60">---</p>
            <p className="mt-2 text-foreground/60">
              {t("help.skill_mdBodyTitle")}
            </p>
            <p className="mt-1 text-foreground/80">
              {t("help.skill_mdBodyContent")}
            </p>
          </div>
          <p className="mt-3">{t("help.skill_howToCreateAfter")}</p>
        </div>
      </section>

      <section className="pt-2 border-t">
        <button
          onClick={() => setShowContribute(true)}
          className="flex items-center gap-2 text-sm font-medium text-primary hover:text-primary/80 transition-colors cursor-pointer"
        >
          <Heart className="w-4 h-4" />
          {t("skill.contribute")}
        </button>
      </section>

      <SkillContributeDialog
        open={showContribute}
        onClose={() => setShowContribute(false)}
      />
    </div>
  );
}

// ── 模型配置 ─────────────────────────────────────────────

function LLMConfigTab() {
  const { t } = useTranslation();
  return (
    <div className="space-y-6 max-w-none">
      <section>
        <h2 className="text-lg font-semibold mb-2">
          {t("help.model_whyConfigTitle")}
        </h2>
        <p className="text-muted-foreground leading-relaxed">
          {t("help.model_whyConfigDesc")}
        </p>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">
          {t("help.model_providerTypes")}
        </h3>
        <div className="space-y-3 text-sm text-muted-foreground leading-relaxed">
          <div className="rounded-lg border bg-card px-4 py-3">
            <h4 className="text-foreground font-medium mb-1">
              {t("help.model_builtinProvider")}
            </h4>
            <p>{t("help.model_builtinProviderDesc")}</p>
          </div>
          <div className="rounded-lg border bg-card px-4 py-3">
            <h4 className="text-foreground font-medium mb-1">
              {t("help.model_customProvider")}
            </h4>
            <p>{t("help.model_customProviderDesc")}</p>
          </div>
        </div>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">
          {t("help.model_management")}
        </h3>
        <div className="space-y-3 text-sm text-muted-foreground leading-relaxed">
          <p>{t("help.model_managementIntro")}</p>
          <div className="space-y-2">
            <p>
              <span className="text-foreground font-medium">
                {t("help.model_autoDiscover")}
              </span>{" "}
              —— {t("help.model_autoDiscoverDesc")}
            </p>
            <p>
              <span className="text-foreground font-medium">
                {t("help.model_manualAdd")}
              </span>{" "}
              —— {t("help.model_manualAddDesc")}
            </p>
          </div>
          <p className="mt-3">{t("help.model_paramsIntro")}</p>
          <div className="space-y-1">
            <p>
              <span className="text-foreground">
                {t("help.model_paramContext")}
              </span>{" "}
              —— {t("help.model_paramContextDesc")}
            </p>
            <p>
              <span className="text-foreground">
                {t("help.model_paramMaxOutput")}
              </span>{" "}
              —— {t("help.model_paramMaxOutputDesc")}
            </p>
            <p>
              <span className="text-foreground">
                {t("help.model_paramThinking")}
              </span>{" "}
              —— {t("help.model_paramThinkingDesc")}
            </p>
            <p>
              <span className="text-foreground">
                {t("help.model_paramVision")}
              </span>{" "}
              —— {t("help.model_paramVisionDesc")}
            </p>
          </div>
        </div>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">
          {t("help.model_temperature")}
        </h3>
        <p className="text-sm text-muted-foreground leading-relaxed">
          {t("help.model_temperatureDesc")}
        </p>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">
          {t("help.model_testAndSave")}
        </h3>
        <div className="space-y-2 text-sm text-muted-foreground leading-relaxed">
          <p>{t("help.model_testStep1")}</p>
          <p>{t("help.model_testStep2")}</p>
          <p>{t("help.model_testStep3")}</p>
        </div>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">{t("help.model_faq")}</h3>
        <div className="space-y-3 text-sm text-muted-foreground leading-relaxed">
          <div>
            <p className="text-foreground font-medium">
              {t("help.model_faqTestFail")}
            </p>
            <p>{t("help.model_faqTestFailDesc")}</p>
          </div>
          <div>
            <p className="text-foreground font-medium">
              {t("help.model_faqApiKeySafe")}
            </p>
            <p>{t("help.model_faqApiKeySafeDesc")}</p>
          </div>
          <div>
            <p className="text-foreground font-medium">
              {t("help.model_faqMultipleProviders")}
            </p>
            <p>{t("help.model_faqMultipleProvidersDesc")}</p>
          </div>
        </div>
      </section>
    </div>
  );
}

// ── 上下文与缓存 ─────────────────────────────────────────

function ContextCacheTab() {
  const { t } = useTranslation();
  return (
    <div className="space-y-6 max-w-none">
      <section>
        <h2 className="text-lg font-semibold mb-2">
          {t("help.context_whatIsTitle")}
        </h2>
        <p className="text-muted-foreground leading-relaxed">
          {t("help.context_whatIsDesc")}
        </p>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">
          {t("help.context_cacheHitRate")}
        </h3>
        <p className="text-sm text-muted-foreground leading-relaxed">
          {t("help.context_cacheHitRateDesc")}
        </p>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">
          {t("help.context_dropReasons")}
        </h3>
        <div className="space-y-3 text-sm text-muted-foreground leading-relaxed">
          <div className="rounded-lg border bg-card px-4 py-3">
            <h4 className="text-foreground font-medium mb-1">
              {t("help.context_switchModel")}
            </h4>
            <p>{t("help.context_switchModelDesc")}</p>
          </div>
          <div className="rounded-lg border bg-card px-4 py-3">
            <h4 className="text-foreground font-medium mb-1">
              {t("help.context_contextCompress")}
            </h4>
            <p>{t("help.context_contextCompressDesc")}</p>
          </div>
          <div className="rounded-lg border bg-card px-4 py-3">
            <h4 className="text-foreground font-medium mb-1">
              {t("help.context_newSession")}
            </h4>
            <p>{t("help.context_newSessionDesc")}</p>
          </div>
          <div className="rounded-lg border bg-card px-4 py-3">
            <h4 className="text-foreground font-medium mb-1">
              {t("help.context_cacheTTL")}
            </h4>
            <p>{t("help.context_cacheTTLDesc")}</p>
          </div>
        </div>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">
          {t("help.context_compressTitle")}
        </h3>
        <p className="text-sm text-muted-foreground leading-relaxed mb-3">
          {t("help.context_compressIntro")}
        </p>

        <div className="space-y-3 text-sm text-muted-foreground leading-relaxed">
          <div className="rounded-lg border bg-card px-4 py-3">
            <h4 className="text-foreground font-medium mb-1">
              {t("help.context_autoCompress")}
            </h4>
            <p>{t("help.context_autoCompressDesc")}</p>
          </div>
          <div className="rounded-lg border bg-card px-4 py-3">
            <h4 className="text-foreground font-medium mb-1">
              {t("help.context_manualCompress")}
            </h4>
            <p>{t("help.context_manualCompressDesc")}</p>
          </div>
        </div>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">{t("help.context_tips")}</h3>
        <div className="space-y-2 text-sm text-muted-foreground leading-relaxed">
          <p>
            •{" "}
            <span className="text-foreground font-medium">
              {t("help.context_tip1Label")}
            </span>
            。{t("help.context_tip1Desc")}
          </p>
          <p>
            •{" "}
            <span className="text-foreground font-medium">
              {t("help.context_tip2Label")}
            </span>
            。{t("help.context_tip2Desc")}
          </p>
          <p>
            •{" "}
            <span className="text-foreground font-medium">
              {t("help.context_tip3Label")}
            </span>
            。{t("help.context_tip3Desc")}
          </p>
          <p>
            •{" "}
            <span className="text-foreground font-medium">
              {t("help.context_tip4Label")}
            </span>
            ，{t("help.context_tip4Desc")}
          </p>
        </div>
      </section>
    </div>
  );
}

// ── 审批模式 ─────────────────────────────────────────────

function ApprovalTab() {
  const { t } = useTranslation();
  return (
    <div className="space-y-6 max-w-none">
      <section>
        <h2 className="text-lg font-semibold mb-2">
          {t("help.approval_title")}
        </h2>
        <p className="text-muted-foreground leading-relaxed">
          {t("help.approval_intro")}
        </p>
      </section>

      <section>
        <h3 className="text-base font-medium mb-3">
          {t("help.approval_autoMode")}
        </h3>
        <div className="rounded-lg border bg-card px-4 py-3 text-sm text-muted-foreground leading-relaxed">
          <p>{t("help.approval_autoModeDesc")}</p>
        </div>
      </section>

      <section>
        <h3 className="text-base font-medium mb-3">
          {t("help.approval_manualMode")}
        </h3>
        <div className="rounded-lg border bg-card px-4 py-3 text-sm text-muted-foreground leading-relaxed space-y-3">
          <p>{t("help.approval_manualModeIntro")}</p>
          <div className="space-y-2">
            <p>
              <span className="text-foreground font-medium">
                {t("help.approval_approve")}
              </span>{" "}
              —— {t("help.approval_approveDesc")}
            </p>
            <p>
              <span className="text-foreground font-medium">
                {t("help.approval_reject")}
              </span>{" "}
              —— {t("help.approval_rejectDesc")}
            </p>
            <p>
              <span className="text-foreground font-medium">
                {t("help.approval_modifyApprove")}
              </span>{" "}
              —— {t("help.approval_modifyApproveDesc")}
            </p>
          </div>
          <p className="text-muted-foreground/80">
            {t("help.approval_manualModeNote")}
          </p>
        </div>
      </section>

      <section>
        <h3 className="text-base font-medium mb-2">
          {t("help.approval_howToSwitch")}
        </h3>
        <p className="text-sm text-muted-foreground leading-relaxed">
          {t("help.approval_howToSwitchDesc")}
        </p>
      </section>
    </div>
  );
}
