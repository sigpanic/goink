import { useState, useEffect } from "react";
import {
  Star,
  Heart,
  ExternalLink,
  Info,
  Package,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { BrowserOpenURL } from "@/lib/wailsjs/runtime/runtime";
import { GetVersion } from "@/lib/wailsjs/go/app/App";
import GitHubIcon from "@/components/ui/GitHubIcon";
import Logo from "@/components/Logo";

const REPO_URL = "https://github.com/sigpanic/goink";
const SKILLS_REPO_URL = "https://github.com/sigpanic/goink-skills";
const AUTHOR_URL = "https://github.com/sigpanic";
const ISSUES_URL = "https://github.com/sigpanic/goink/issues";

export default function AboutTab() {
  const { t } = useTranslation();
  const [appVersion, setAppVersion] = useState("dev");

  // GetVersion: 与 GeneralConfigTab 一致，低频 GET，保留命令式。
  useEffect(() => {
    GetVersion()
      .then((v) => setAppVersion(v || "dev"))
      .catch(() => {});
  }, []);

  const features = [
    t("settings.aboutFeature1"),
    t("settings.aboutFeature2"),
    t("settings.aboutFeature3"),
    t("settings.aboutFeature4"),
    t("settings.aboutFeature5"),
    t("settings.aboutFeature6"),
  ];

  // 随安装包打包的第三方运行时组件（仅分发部分，不含源码级依赖）。
  const deps: { name: string; version?: string; license: string; desc: string }[] = [
    {
      name: "ONNX Runtime",
      version: "1.26.0",
      license: "MIT",
      desc: t("settings.aboutDepOnnx"),
    },
    {
      name: "BGE Embedding (bge-small-zh-v1.5 int8)",
      license: "MIT",
      desc: t("settings.aboutDepBge"),
    },
    {
      name: "Git (Windows: MinGit)",
      license: "GPL-2.0",
      desc: t("settings.aboutDepGit"),
    },
    {
      name: "SQLite + sqlite-vec",
      license: "Public Domain / Apache-2.0 / MIT",
      desc: t("settings.aboutDepSqlite"),
    },
    {
      name: "VC++ Runtime (Windows)",
      license: "Microsoft",
      desc: t("settings.aboutDepVcrt"),
    },
  ];

  return (
    <div className="flex-1 overflow-y-auto pr-1">
      {/* 头部：应用名 + 版本 */}
      <div className="flex items-center gap-3">
        <div className="w-12 h-12 rounded-xl bg-primary/10 flex items-center justify-center">
          <Logo className="w-7 h-7" />
        </div>
        <div>
          <div className="text-base font-semibold">Goink</div>
          <div className="text-xs text-muted-foreground">
            {t("update.currentVersion")} v{appVersion}
          </div>
        </div>
      </div>

      {/* 简介 */}
      <p className="mt-4 text-xs leading-5 text-muted-foreground">
        {t("settings.aboutIntro")}
      </p>

      {/* 核心特性 */}
      <h4 className="mt-6 flex items-center gap-1.5 text-xs font-medium text-foreground">
        <Info className="w-3.5 h-3.5 text-muted-foreground" />
        {t("settings.aboutFeaturesTitle")}
      </h4>
      <ul className="mt-2 space-y-1.5">
        {features.map((f, i) => (
          <li key={i} className="flex items-start gap-2 text-xs text-muted-foreground">
            <span className="mt-1.5 w-1 h-1 rounded-full bg-primary/60 shrink-0" />
            <span className="leading-5">{f}</span>
          </li>
        ))}
      </ul>

      {/* 开源信息 */}
      <h4 className="mt-6 flex items-center gap-1.5 text-xs font-medium text-foreground">
        <GitHubIcon className="w-3.5 h-3.5 text-muted-foreground" />
        {t("settings.aboutOpenSourceTitle")}
      </h4>
      <div className="mt-2 space-y-1.5">
        <button
          onClick={() => BrowserOpenURL(REPO_URL)}
          className="flex items-center gap-1.5 text-xs text-primary hover:underline cursor-pointer"
        >
          {t("settings.aboutRepo")}
          <ExternalLink className="w-3 h-3" />
        </button>
        <button
          onClick={() => BrowserOpenURL(SKILLS_REPO_URL)}
          className="flex items-center gap-1.5 text-xs text-primary hover:underline cursor-pointer"
        >
          {t("settings.aboutSkillsRepo")}
          <ExternalLink className="w-3 h-3" />
        </button>
        <button
          onClick={() => BrowserOpenURL(AUTHOR_URL)}
          className="flex items-center gap-1.5 text-xs text-primary hover:underline cursor-pointer"
        >
          {t("settings.aboutAuthor")}: sigpanic
          <ExternalLink className="w-3 h-3" />
        </button>
        <button
          onClick={() => BrowserOpenURL(ISSUES_URL)}
          className="flex items-center gap-1.5 text-xs text-primary hover:underline cursor-pointer"
        >
          {t("settings.aboutIssues")}
          <ExternalLink className="w-3 h-3" />
        </button>
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <Star className="w-3 h-3" />
          {t("settings.aboutStar")}
        </p>
        <p className="text-xs text-muted-foreground">
          {t("settings.aboutLicense")}: AGPL-3.0
        </p>
      </div>

      {/* 赞赏 */}
      <h4 className="mt-6 flex items-center gap-1.5 text-xs font-medium text-foreground">
        <Heart className="w-3.5 h-3.5 text-muted-foreground" />
        {t("settings.aboutDonateTitle")}
      </h4>
      <p className="mt-2 text-[11px] leading-4 text-muted-foreground">
        {t("settings.aboutDonateDesc")}
      </p>
      <div className="mt-3 flex gap-4">
        <div className="flex flex-col items-center gap-1">
          <img
            src="/wechat_qrcode.png"
            alt={t("settings.aboutWechat")}
            className="h-36 w-auto rounded-lg border"
          />
          <span className="text-[11px] text-muted-foreground">
            {t("settings.aboutWechat")}
          </span>
        </div>
        <div className="flex flex-col items-center gap-1">
          <img
            src="/zfb_qrcode.png"
            alt={t("settings.aboutAlipay")}
            className="h-36 w-auto rounded-lg border"
          />
          <span className="text-[11px] text-muted-foreground">
            {t("settings.aboutAlipay")}
          </span>
        </div>
      </div>

      {/* 第三方依赖 */}
      <h4 className="mt-6 flex items-center gap-1.5 text-xs font-medium text-foreground">
        <Package className="w-3.5 h-3.5 text-muted-foreground" />
        {t("settings.aboutDepsTitle")}
      </h4>
      <p className="mt-2 text-[11px] leading-4 text-muted-foreground">
        {t("settings.aboutDepsDesc")}
      </p>
      <ul className="mt-2 space-y-1.5">
        {deps.map((d, i) => (
          <li key={i} className="flex items-start justify-between gap-3 text-[11px]">
            <span className="text-muted-foreground flex-1 leading-4">
              <span className="text-foreground">{d.name}</span>
              <span className="text-muted-foreground"> · {d.desc}</span>
            </span>
            <span className="text-muted-foreground shrink-0">
              {d.version ? `v${d.version} · ` : ""}
              {d.license}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
