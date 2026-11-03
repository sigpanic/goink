import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
  BookOpen,
  Files,
  Users,
  MapPin,
  MessageCircle,
  WandSparkles,
  Sparkles,
} from "lucide-react";
import type { AnnualReport } from "./useAnnualReport";

export function PageHeading({
  index,
  title,
  note,
}: {
  index: string;
  title: string;
  note: string;
}) {
  return (
    <header className="report-chapter-heading">
      <span className="report-chapter-index">{index}</span>
      <div>
        <h2>{title}</h2>
        <p>{note}</p>
      </div>
    </header>
  );
}

export function LeadStat({
  label,
  value,
  unit,
  note,
}: {
  label: string;
  value: string;
  unit: string;
  note?: string;
}) {
  return (
    <div className="report-lead-stat">
      <p>{label}</p>
      <div>
        <strong>{value}</strong>
        <span>{unit}</span>
      </div>
      {note && <small>{note}</small>}
    </div>
  );
}

export function AchievementPage({
  report,
  locale,
}: {
  report: AnnualReport;
  locale: string;
}) {
  const { t } = useTranslation();
  const number = (value: number) =>
    new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(value);
  const compact = (value: number) =>
    new Intl.NumberFormat(locale, {
      notation: "compact",
      maximumFractionDigits: 1,
    }).format(value);
  const seconds = report.Activity.CreativeSeconds;
  const inMinutes = seconds > 0 && seconds < 3600;
  const outcomes = [
    {
      icon: BookOpen,
      value: report.Activity.NovelsCreated,
      label: "annualReport.novels",
      note: "annualReport.novelsNote",
      color: "teal",
    },
    {
      icon: Files,
      value: report.Activity.ChaptersCreated,
      label: "annualReport.chapters",
      note: "annualReport.chaptersNote",
      color: "amber",
    },
    {
      icon: Users,
      value: report.Activity.CharactersCreated,
      label: "annualReport.characters",
      note: "annualReport.charactersNote",
      color: "purple",
    },
    {
      icon: MapPin,
      value: report.Activity.LocationsCreated,
      label: "annualReport.locations",
      note: "annualReport.locationsNote",
      color: "blue",
    },
  ];
  return (
    <>
      <PageHeading
        index="01"
        title={t("annualReport.worldTitle")}
        note={t("annualReport.worldNote")}
      />
      <section
        className="report-lead-stats"
        aria-label={t("annualReport.overview")}
      >
        <LeadStat
          label={t("annualReport.wordsAdded")}
          value={compact(report.Writing.WordsAdded)}
          unit={t("annualReport.wordsUnit")}
          note={t("annualReport.wordsNote")}
        />
        <LeadStat
          label={t("annualReport.creativeTime")}
          value={number(inMinutes ? seconds / 60 : seconds / 3600)}
          unit={t(inMinutes ? "annualReport.minutes" : "annualReport.hours")}
          note={t("annualReport.timeNote")}
        />
      </section>
      <section
        className="report-outcomes"
        aria-label={t("annualReport.worldTitle")}
      >
        {outcomes.map((item) => (
          <article
            className="report-outcome"
            key={item.label}
            data-color={item.color}
          >
            <div className="report-outcome-heading">
              <span>{t(item.label)}</span>
              <item.icon size={18} />
            </div>
            <strong>{number(item.value)}</strong>
            <p>{t(item.note)}</p>
          </article>
        ))}
      </section>
      <p className="report-page-note">{t("annualReport.creationHistory")}</p>
    </>
  );
}

export function NovelRanking({
  report,
  locale,
}: {
  report: AnnualReport;
  locale: string;
}) {
  const { t } = useTranslation();
  const number = (value: number) =>
    new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(value);
  const compact = (value: number) =>
    new Intl.NumberFormat(locale, {
      notation: "compact",
      maximumFractionDigits: 1,
    }).format(value);
  if (report.Novels.length === 0) return null;
  return (
    <section className="report-panel" aria-labelledby="report-books-title">
      <div className="report-panel-heading">
        <div>
          <span className="report-kicker">{t("annualReport.booksKicker")}</span>
          <h3 id="report-books-title">{t("annualReport.booksTitle")}</h3>
        </div>
        <BookOpen size={17} className="report-muted-icon" />
      </div>
      <div className="report-books">
        {report.Novels.slice(0, 5).map((book, index) => (
          <div className="report-book-row" key={book.NovelID}>
            <span
              className="report-book-cover"
              data-color={index % 3}
              aria-hidden="true"
            >
              {String(index + 1).padStart(2, "0")}
            </span>
            <div className="report-book-main">
              <div className="report-book-name">
                {book.Deleted ? t("annualReport.deletedNovel") : book.Title}
              </div>
              <span>
                {t("annualReport.bookShare", {
                  percent: number(
                    report.Writing.WordsAdded > 0
                      ? (book.WordsAdded / report.Writing.WordsAdded) * 100
                      : 0,
                  ),
                })}
              </span>
              <div className="report-track">
                <i
                  style={{
                    width: `${report.Writing.WordsAdded > 0 ? (book.WordsAdded / report.Writing.WordsAdded) * 100 : 0}%`,
                  }}
                />
              </div>
            </div>
            <strong>
              {compact(book.WordsAdded)}
              <small>{t("annualReport.wordsUnit")}</small>
            </strong>
          </div>
        ))}
      </div>
    </section>
  );
}

export function AIPage({
  report,
  locale,
}: {
  report: AnnualReport;
  locale: string;
}) {
  const { t } = useTranslation();
  const number = (value: number) =>
    new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(value);
  const compact = (value: number) =>
    new Intl.NumberFormat(locale, {
      notation: "compact",
      maximumFractionDigits: 1,
    }).format(value);
  const inputOutput = report.Usage.InputTokens + report.Usage.OutputTokens;
  return (
    <>
      <PageHeading
        index="04"
        title={t("annualReport.aiTitle")}
        note={t("annualReport.aiNote")}
      />
      <section
        className="report-ai-grid"
        aria-label={t("annualReport.aiTitle")}
      >
        <AIStat
          icon={<MessageCircle size={17} />}
          label={t("annualReport.turns")}
          value={number(report.Activity.ConversationTurns)}
          unit={t("annualReport.turnsUnit")}
          note={t("annualReport.turnsNote")}
        />
        <AIStat
          icon={<WandSparkles size={17} />}
          label={t("annualReport.tools")}
          value={number(report.Activity.ToolCalls)}
          unit={t("annualReport.toolsUnit")}
          note={t("annualReport.toolsNote")}
        />
        <article className="report-ai-card report-token-card">
          <div className="report-ai-label">
            <Sparkles size={17} />
            {t("annualReport.tokens")}
          </div>
          <div className="report-ai-value">
            {report.UsageCount > 0 ? compact(report.Usage.TotalTokens) : "—"}
            <small>tokens</small>
          </div>
          <div className="report-token-track" aria-hidden="true">
            <i
              style={{
                width: `${inputOutput > 0 ? (report.Usage.InputTokens / inputOutput) * 100 : 0}%`,
              }}
            />
            <i
              style={{
                width: `${inputOutput > 0 ? (report.Usage.OutputTokens / inputOutput) * 100 : 0}%`,
              }}
            />
          </div>
          <div className="report-token-labels">
            <span>
              {t("annualReport.input")}{" "}
              <b>{compact(report.Usage.InputTokens)}</b>
            </span>
            <span>
              {t("annualReport.output")}{" "}
              <b>{compact(report.Usage.OutputTokens)}</b>
            </span>
          </div>
        </article>
      </section>
      {report.Models.length > 0 ? (
        <section
          className="report-panel report-model-panel"
          aria-labelledby="report-model-title"
        >
          <div className="report-model-heading">
            <span className="report-model-symbol">
              <Sparkles size={20} />
            </span>
            <div>
              <span className="report-kicker">
                {t("annualReport.modelKicker")}
              </span>
              <h3 id="report-model-title">{t("annualReport.modelsTitle")}</h3>
              <p>{t("annualReport.modelsNote")}</p>
            </div>
          </div>
          <div className="report-models">
            {report.Models.slice(0, 5).map((model, index) => {
              const percent =
                report.UsageCount > 0
                  ? (model.UsageCount / report.UsageCount) * 100
                  : 0;
              return (
                <div
                  className="report-model"
                  key={JSON.stringify([model.Provider, model.Model])}
                >
                  <span className="report-model-dot" data-color={index % 3} />
                  <div className="report-model-name">
                    <strong>{model.Model}</strong>
                    <span>{model.Provider}</span>
                  </div>
                  <div className="report-model-meter">
                    <i style={{ width: `${percent}%` }} />
                  </div>
                  <span className="report-model-count">
                    {t("annualReport.recordedCalls", {
                      value: number(model.UsageCount),
                    })}
                    <small>{number(percent)}%</small>
                  </span>
                </div>
              );
            })}
            {report.Models.length > 5 && (
              <p className="report-model-more">
                {t("annualReport.otherModels", {
                  value: number(report.Models.length - 5),
                })}
              </p>
            )}
          </div>
        </section>
      ) : (
        <p className="report-no-models">{t("annualReport.noModelUsage")}</p>
      )}
    </>
  );
}

function AIStat({
  icon,
  label,
  value,
  unit,
  note,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  unit: string;
  note: string;
}) {
  return (
    <article className="report-ai-card">
      <div className="report-ai-label">
        {icon}
        {label}
      </div>
      <div className="report-ai-value">
        {value}
        <small>{unit}</small>
      </div>
      <p>{note}</p>
    </article>
  );
}
