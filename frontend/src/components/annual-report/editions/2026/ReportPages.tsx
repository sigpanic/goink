import styles from "./report.module.css";
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
import type { AnnualReport } from "../../useAnnualReport";
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
    <header className={styles["report-chapter-heading"]}>
      <span className={styles["report-chapter-index"]}>{index}</span>
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
    <div className={styles["report-lead-stat"]}>
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
        title={t("annualReport.editions.2026.worksTitle")}
        note={t("annualReport.editions.2026.booksNote")}
      />
      <div className={styles["report-works-layout"]}>
        <section className={styles["report-manuscript"]}>
          <Files size={25} strokeWidth={1.2} />
          <span className={styles["report-kicker"]}>
            {t("annualReport.editions.2026.manuscriptKicker")}
          </span>
          <strong className={styles["report-display-number"]}>
            {number(report.Activity.ChaptersCreated)}
          </strong>
          <h3>{t("annualReport.editions.2026.chapters")}</h3>
          <p>{t("annualReport.editions.2026.manuscriptNote")}</p>
          <dl className={styles["report-inline-counts"]}>
            <Count
              label={t("annualReport.editions.2026.novels")}
              value={report.Activity.NovelsCreated}
              locale={locale}
            />
            <Count
              label={t("annualReport.editions.2026.volumes")}
              value={report.Activity.VolumesCreated}
              locale={locale}
            />
          </dl>
        </section>
        {report.Novels.length > 0 ? (
          <NovelRanking report={report} locale={locale} />
        ) : (
          <div className={styles["report-unwritten"]}>
            <BookOpen size={62} strokeWidth={0.7} />
            <p>{t("annualReport.editions.2026.unwrittenNote")}</p>
          </div>
        )}
      </div>
      <p className={styles["report-page-note"]}>
        {t("annualReport.editions.2026.creationHistory")}
      </p>
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
    <div className={styles["report-count"]}>
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
        title={t("annualReport.editions.2026.worldTitle")}
        note={t("annualReport.editions.2026.worldNote")}
      />
      <div className={styles["report-world-layout"]}>
        <section className={styles["report-cast"]}>
          <div className={styles["report-orbit"]} aria-hidden="true">
            <i />
            <i />
            <i />
            <Users size={48} strokeWidth={1} />
          </div>
          <span className={styles["report-kicker"]}>
            {t("annualReport.editions.2026.castKicker")}
          </span>
          <strong className={styles["report-display-number"]}>
            {a.CharactersCreated.toLocaleString(locale)}
          </strong>
          <h3>{t("annualReport.editions.2026.characters")}</h3>
          <p>{t("annualReport.editions.2026.castNote")}</p>
        </section>
        <div className={styles["report-world-details"]}>
          <section className={styles["report-detail-group"]}>
            <Network size={22} strokeWidth={1.2} />
            <h3>{t("annualReport.editions.2026.connectionsTitle")}</h3>
            <dl>
              <Count
                label={t("annualReport.editions.2026.characterGroups")}
                value={a.CharacterGroupsCreated}
                locale={locale}
              />
              <Count
                label={t("annualReport.editions.2026.characterRelations")}
                value={a.CharacterRelationsCreated}
                locale={locale}
              />
            </dl>
          </section>
          <section className={styles["report-detail-group"]}>
            <MapPin size={22} strokeWidth={1.2} />
            <h3>{t("annualReport.editions.2026.placesTitle")}</h3>
            <dl>
              <Count
                label={t("annualReport.editions.2026.locations")}
                value={a.LocationsCreated}
                locale={locale}
              />
              <Count
                label={t("annualReport.editions.2026.locationRelations")}
                value={a.LocationRelationsCreated}
                locale={locale}
              />
            </dl>
          </section>
        </div>
      </div>
      <p className={styles["report-page-note"]}>
        {t("annualReport.editions.2026.creationHistory")}
      </p>
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
        title={t("annualReport.editions.2026.storyTitle")}
        note={t("annualReport.editions.2026.storyNote")}
      />
      <div className={styles["report-story-layout"]}>
        <section className={styles["report-story-feature"]}>
          <Route size={30} strokeWidth={1.1} />
          <span className={styles["report-kicker"]}>
            {t("annualReport.editions.2026.storyKicker")}
          </span>
          <strong className={styles["report-display-number"]}>
            {a.StoryArcsCreated.toLocaleString(locale)}
          </strong>
          <h3>{t("annualReport.editions.2026.storyArcs")}</h3>
          <p>{t("annualReport.editions.2026.storyFeatureNote")}</p>
          <svg
            viewBox="0 0 400 130"
            className={styles["report-story-illustration"]}
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
        <dl className={styles["report-story-ledger"]}>
          <Count
            label={t("annualReport.editions.2026.arcNodes")}
            value={a.ArcNodesCreated}
            locale={locale}
          />
          <Count
            label={t("annualReport.editions.2026.timelineEntries")}
            value={a.TimelineEntriesCreated}
            locale={locale}
          />
          <Count
            label={t("annualReport.editions.2026.readerPerspectives")}
            value={a.ReaderPerspectivesCreated}
            locale={locale}
          />
        </dl>
      </div>
      <p className={styles["report-page-note"]}>
        {t("annualReport.editions.2026.creationHistory")}
      </p>
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
        title={t("annualReport.editions.2026.habitsTitle")}
        note={t("annualReport.editions.2026.habitsNote")}
      />
      <div className={styles["report-habit-layout"]}>
        <div className={styles["report-habit-summary"]}>
          <LeadStat
            label={t("annualReport.editions.2026.activeDays")}
            value={report.Writing.ActiveDays.toLocaleString(locale)}
            unit={t("annualReport.editions.2026.days")}
          />
          <LeadStat
            label={t("annualReport.editions.2026.longestStreak")}
            value={report.Writing.LongestStreak.toLocaleString(locale)}
            unit={t("annualReport.editions.2026.days")}
          />
          <dl className={styles["report-edit-counts"]}>
            <Count
              label={t("annualReport.editions.2026.wordsDeleted")}
              value={report.Writing.WordsDeleted}
              locale={locale}
            />
            <Count
              label={t("annualReport.editions.2026.wordsNet")}
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
        title={t("annualReport.editions.2026.inspirationTitle")}
        note={t("annualReport.editions.2026.inspirationNote")}
      />
      <div className={styles["report-inspiration-layout"]}>
        <section className={styles["report-notebook"]}>
          <Lightbulb size={32} strokeWidth={1} />
          <span className={styles["report-kicker"]}>
            {t("annualReport.editions.2026.inspirationKicker")}
          </span>
          <strong className={styles["report-display-number"]}>
            {a.SettingsCreated.toLocaleString(locale)}
          </strong>
          <h3>{t("annualReport.editions.2026.settings")}</h3>
          <p>{t("annualReport.editions.2026.settingsNote")}</p>
          <dl className={styles["report-inline-counts"]}>
            <Count
              label={t("annualReport.editions.2026.preferences")}
              value={a.PreferencesCreated}
              locale={locale}
            />
          </dl>
        </section>
        <section className={styles["report-collection"]}>
          <Library size={26} strokeWidth={1} />
          <h3>{t("annualReport.editions.2026.collectionTitle")}</h3>
          <dl>
            <Count
              label={t("annualReport.editions.2026.styleSamples")}
              value={a.StyleSamplesCreated}
              locale={locale}
            />
            <Count
              label={t("annualReport.editions.2026.skills")}
              value={a.SkillsCreated}
              locale={locale}
            />
            <Count
              label={t("annualReport.editions.2026.images")}
              value={a.ImagesImported}
              locale={locale}
            />
          </dl>
          <p className={styles["report-page-note"]}>
            {t("annualReport.editions.2026.skillsNote")}
          </p>
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
    <section
      className={styles["report-panel"]}
      aria-labelledby="report-books-title"
    >
      <div className={styles["report-panel-heading"]}>
        <div>
          <span className={styles["report-kicker"]}>
            {t("annualReport.editions.2026.booksKicker")}
          </span>
          <h3 id="report-books-title">
            {t("annualReport.editions.2026.booksTitle")}
          </h3>
        </div>
        <BookOpen size={17} className={styles["report-muted-icon"]} />
      </div>
      <div className={styles["report-books"]}>
        {report.Novels.slice(0, 5).map((book, index) => (
          <div className={styles["report-book-row"]} key={book.NovelID}>
            <span
              className={styles["report-book-cover"]}
              data-color={index % 3}
              aria-hidden="true"
            >
              {String(index + 1).padStart(2, "0")}
            </span>
            <div className={styles["report-book-main"]}>
              <div className={styles["report-book-name"]}>
                {book.Deleted
                  ? t("annualReport.editions.2026.deletedNovel")
                  : book.Title}
              </div>
              <span>
                {t("annualReport.editions.2026.bookShare", {
                  percent: number(
                    report.Writing.WordsAdded > 0
                      ? (book.WordsAdded / report.Writing.WordsAdded) * 100
                      : 0,
                  ),
                })}
              </span>
              <div className={styles["report-track"]}>
                <i
                  style={{
                    width: `${report.Writing.WordsAdded > 0 ? (book.WordsAdded / report.Writing.WordsAdded) * 100 : 0}%`,
                  }}
                />
              </div>
            </div>
            <strong>
              {compact(book.WordsAdded)}
              <small>{t("annualReport.editions.2026.wordsUnit")}</small>
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
        title={t("annualReport.editions.2026.aiTitle")}
        note={t("annualReport.editions.2026.aiNote")}
      />
      <section
        className={styles["report-ai-grid"]}
        aria-label={t("annualReport.editions.2026.aiTitle")}
      >
        <AIStat
          icon={<MessagesSquare size={17} />}
          label={t("annualReport.editions.2026.sessions")}
          value={number(report.Activity.SessionsCreated)}
          unit={t("annualReport.editions.2026.sessionsUnit")}
          note={t("annualReport.editions.2026.sessionsNote")}
        />
        <AIStat
          icon={<MessageCircle size={17} />}
          label={t("annualReport.editions.2026.turns")}
          value={number(report.Activity.ConversationTurns)}
          unit={t("annualReport.editions.2026.turnsUnit")}
          note={t("annualReport.editions.2026.turnsNote")}
        />
        <AIStat
          icon={<WandSparkles size={17} />}
          label={t("annualReport.editions.2026.tools")}
          value={number(report.Activity.ToolCalls)}
          unit={t("annualReport.editions.2026.toolsUnit")}
          note={t("annualReport.editions.2026.toolsNote")}
        />
        <article
          className={[
            styles["report-ai-card"],
            styles["report-token-card"],
          ].join(" ")}
        >
          <div className={styles["report-ai-label"]}>
            <Sparkles size={17} />
            {t("annualReport.editions.2026.tokens")}
          </div>
          <div className={styles["report-ai-value"]}>
            {report.UsageCount > 0 ? compact(report.Usage.TotalTokens) : "—"}
            <small>tokens</small>
          </div>
          <div className={styles["report-token-track"]} aria-hidden="true">
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
          <div className={styles["report-token-labels"]}>
            <span>
              {t("annualReport.editions.2026.input")}{" "}
              <b>{compact(report.Usage.InputTokens)}</b>
            </span>
            <span>
              {t("annualReport.editions.2026.output")}{" "}
              <b>{compact(report.Usage.OutputTokens)}</b>
            </span>
          </div>
        </article>
      </section>
      {report.Models.length > 0 ? (
        <section
          className={[
            styles["report-panel"],
            styles["report-model-panel"],
          ].join(" ")}
          aria-labelledby="report-model-title"
        >
          <div className={styles["report-model-heading"]}>
            <span className={styles["report-model-symbol"]}>
              <Sparkles size={20} />
            </span>
            <div>
              <span className={styles["report-kicker"]}>
                {t("annualReport.editions.2026.modelKicker")}
              </span>
              <h3 id="report-model-title">
                {t("annualReport.editions.2026.modelsTitle")}
              </h3>
              <p>{t("annualReport.editions.2026.modelsNote")}</p>
            </div>
          </div>
          <div className={styles["report-models"]}>
            {report.Models.slice(0, 5).map((model, index) => {
              const percent =
                report.UsageCount > 0
                  ? (model.UsageCount / report.UsageCount) * 100
                  : 0;
              return (
                <div
                  className={styles["report-model"]}
                  key={JSON.stringify([model.Provider, model.Model])}
                >
                  <span
                    className={styles["report-model-dot"]}
                    data-color={index % 3}
                  />
                  <div className={styles["report-model-name"]}>
                    <strong>{model.Model}</strong>
                    <span>{model.Provider}</span>
                  </div>
                  <div className={styles["report-model-meter"]}>
                    <i style={{ width: `${percent}%` }} />
                  </div>
                  <span className={styles["report-model-count"]}>
                    {t("annualReport.editions.2026.recordedCalls", {
                      value: number(model.UsageCount),
                    })}
                    <small>{number(percent)}%</small>
                  </span>
                </div>
              );
            })}
            {report.Models.length > 5 && (
              <p className={styles["report-model-more"]}>
                {t("annualReport.editions.2026.otherModels", {
                  value: number(report.Models.length - 5),
                })}
              </p>
            )}
          </div>
        </section>
      ) : (
        <p className={styles["report-no-models"]}>
          {t("annualReport.editions.2026.noModelUsage")}
        </p>
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
    <article className={styles["report-ai-card"]}>
      <div className={styles["report-ai-label"]}>
        {icon}
        {label}
      </div>
      <div className={styles["report-ai-value"]}>
        {value}
        <small>{unit}</small>
      </div>
      <p>{note}</p>
    </article>
  );
}
