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
  Library,
  Network,
  Route,
  Lightbulb,
  MessagesSquare,
} from "lucide-react";
import type { AnnualReport } from "./useAnnualReport";
import ReportCalendar from "./ReportCalendar";
import MonthlyChart from "./MonthlyChart";

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
  return (
    <>
      <PageHeading
        index="01"
        title={t("annualReport.worksTitle")}
        note={t("annualReport.booksNote")}
      />
      <div className="report-works-layout">
        <section className="report-manuscript">
          <Files size={25} strokeWidth={1.2} />
          <span className="report-kicker">
            {t("annualReport.manuscriptKicker")}
          </span>
          <strong className="report-display-number">
            {number(report.Activity.ChaptersCreated)}
          </strong>
          <h3>{t("annualReport.chapters")}</h3>
          <p>{t("annualReport.manuscriptNote")}</p>
          <dl className="report-inline-counts">
            <Count
              label={t("annualReport.novels")}
              value={report.Activity.NovelsCreated}
              locale={locale}
            />
            <Count
              label={t("annualReport.volumes")}
              value={report.Activity.VolumesCreated}
              locale={locale}
            />
          </dl>
        </section>
        {report.Novels.length > 0 ? (
          <NovelRanking report={report} locale={locale} />
        ) : (
          <div className="report-unwritten">
            <BookOpen size={62} strokeWidth={0.7} />
            <p>{t("annualReport.unwrittenNote")}</p>
          </div>
        )}
      </div>
      <p className="report-page-note">{t("annualReport.creationHistory")}</p>
    </>
  );
}

function Count({
  label,
  value,
  locale,
}: {
  label: string;
  value: number;
  locale: string;
}) {
  return (
    <div className="report-count">
      <dt>{label}</dt>
      <dd>{value.toLocaleString(locale)}</dd>
    </div>
  );
}

export function WorldPage({
  report,
  locale,
}: {
  report: AnnualReport;
  locale: string;
}) {
  const { t } = useTranslation();
  const a = report.Activity;
  return (
    <>
      <PageHeading
        index="02"
        title={t("annualReport.worldTitle")}
        note={t("annualReport.worldNote")}
      />
      <div className="report-world-layout">
        <section className="report-cast">
          <div className="report-orbit" aria-hidden="true">
            <i />
            <i />
            <i />
            <Users size={48} strokeWidth={1} />
          </div>
          <span className="report-kicker">{t("annualReport.castKicker")}</span>
          <strong className="report-display-number">
            {a.CharactersCreated.toLocaleString(locale)}
          </strong>
          <h3>{t("annualReport.characters")}</h3>
          <p>{t("annualReport.castNote")}</p>
        </section>
        <div className="report-world-details">
          <section className="report-detail-group">
            <Network size={22} strokeWidth={1.2} />
            <h3>{t("annualReport.connectionsTitle")}</h3>
            <dl>
              <Count
                label={t("annualReport.characterGroups")}
                value={a.CharacterGroupsCreated}
                locale={locale}
              />
              <Count
                label={t("annualReport.characterRelations")}
                value={a.CharacterRelationsCreated}
                locale={locale}
              />
            </dl>
          </section>
          <section className="report-detail-group">
            <MapPin size={22} strokeWidth={1.2} />
            <h3>{t("annualReport.placesTitle")}</h3>
            <dl>
              <Count
                label={t("annualReport.locations")}
                value={a.LocationsCreated}
                locale={locale}
              />
              <Count
                label={t("annualReport.locationRelations")}
                value={a.LocationRelationsCreated}
                locale={locale}
              />
            </dl>
          </section>
        </div>
      </div>
      <p className="report-page-note">{t("annualReport.creationHistory")}</p>
    </>
  );
}

export function StoryPage({
  report,
  locale,
}: {
  report: AnnualReport;
  locale: string;
}) {
  const { t } = useTranslation();
  const a = report.Activity;
  return (
    <>
      <PageHeading
        index="03"
        title={t("annualReport.storyTitle")}
        note={t("annualReport.storyNote")}
      />
      <div className="report-story-layout">
        <section className="report-story-feature">
          <Route size={30} strokeWidth={1.1} />
          <span className="report-kicker">{t("annualReport.storyKicker")}</span>
          <strong className="report-display-number">
            {a.StoryArcsCreated.toLocaleString(locale)}
          </strong>
          <h3>{t("annualReport.storyArcs")}</h3>
          <p>{t("annualReport.storyFeatureNote")}</p>
          <svg
            viewBox="0 0 400 130"
            className="report-story-illustration"
            aria-hidden="true"
            fill="none"
          >
            <path
              d="M15 105C85 105 55 25 125 25S200 100 260 65s60-55 125-50"
              stroke="currentColor"
              strokeWidth="1.5"
            />
            {[
              [15, 105],
              [125, 25],
              [260, 65],
              [385, 15],
            ].map(([x, y]) => (
              <circle key={x} cx={x} cy={y} r="5" fill="currentColor" />
            ))}
          </svg>
        </section>
        <dl className="report-story-ledger">
          <Count
            label={t("annualReport.arcNodes")}
            value={a.ArcNodesCreated}
            locale={locale}
          />
          <Count
            label={t("annualReport.timelineEntries")}
            value={a.TimelineEntriesCreated}
            locale={locale}
          />
          <Count
            label={t("annualReport.readerPerspectives")}
            value={a.ReaderPerspectivesCreated}
            locale={locale}
          />
        </dl>
      </div>
      <p className="report-page-note">{t("annualReport.creationHistory")}</p>
    </>
  );
}

export function HabitsPage({
  report,
  locale,
}: {
  report: AnnualReport;
  locale: string;
}) {
  const { t } = useTranslation();
  return (
    <>
      <PageHeading
        index="04"
        title={t("annualReport.habitsTitle")}
        note={t("annualReport.habitsNote")}
      />
      <div className="report-habit-layout">
        <div className="report-habit-summary">
          <LeadStat
            label={t("annualReport.activeDays")}
            value={report.Writing.ActiveDays.toLocaleString(locale)}
            unit={t("annualReport.days")}
          />
          <LeadStat
            label={t("annualReport.longestStreak")}
            value={report.Writing.LongestStreak.toLocaleString(locale)}
            unit={t("annualReport.days")}
          />
          <dl className="report-edit-counts">
            <Count
              label={t("annualReport.wordsDeleted")}
              value={report.Writing.WordsDeleted}
              locale={locale}
            />
            <Count
              label={t("annualReport.wordsNet")}
              value={report.Writing.WordsNet}
              locale={locale}
            />
          </dl>
        </div>
        <MonthlyChart report={report} />
      </div>
      <ReportCalendar report={report} />
    </>
  );
}

export function InspirationPage({
  report,
  locale,
}: {
  report: AnnualReport;
  locale: string;
}) {
  const { t } = useTranslation();
  const a = report.Activity;
  return (
    <>
      <PageHeading
        index="05"
        title={t("annualReport.inspirationTitle")}
        note={t("annualReport.inspirationNote")}
      />
      <div className="report-inspiration-layout">
        <section className="report-notebook">
          <Lightbulb size={32} strokeWidth={1} />
          <span className="report-kicker">
            {t("annualReport.inspirationKicker")}
          </span>
          <strong className="report-display-number">
            {a.SettingsCreated.toLocaleString(locale)}
          </strong>
          <h3>{t("annualReport.settings")}</h3>
          <p>{t("annualReport.settingsNote")}</p>
          <dl className="report-inline-counts">
            <Count
              label={t("annualReport.preferences")}
              value={a.PreferencesCreated}
              locale={locale}
            />
          </dl>
        </section>
        <section className="report-collection">
          <Library size={26} strokeWidth={1} />
          <h3>{t("annualReport.collectionTitle")}</h3>
          <dl>
            <Count
              label={t("annualReport.styleSamples")}
              value={a.StyleSamplesCreated}
              locale={locale}
            />
            <Count
              label={t("annualReport.skills")}
              value={a.SkillsCreated}
              locale={locale}
            />
            <Count
              label={t("annualReport.images")}
              value={a.ImagesImported}
              locale={locale}
            />
          </dl>
          <p className="report-page-note">{t("annualReport.skillsNote")}</p>
        </section>
      </div>
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
        index="06"
        title={t("annualReport.aiTitle")}
        note={t("annualReport.aiNote")}
      />
      <section
        className="report-ai-grid"
        aria-label={t("annualReport.aiTitle")}
      >
        <AIStat
          icon={<MessagesSquare size={17} />}
          label={t("annualReport.sessions")}
          value={number(report.Activity.SessionsCreated)}
          unit={t("annualReport.sessionsUnit")}
          note={t("annualReport.sessionsNote")}
        />
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
