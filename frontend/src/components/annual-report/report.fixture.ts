import { report } from "@/lib/wailsjs/go/models";

export function makeReport(
  year = new Date().getFullYear(),
): report.AnnualReport {
  return new report.AnnualReport({
    Year: year,
    TrackingStartedAt: `${year - 1}-01-01T00:00:00Z`,
    TrackingStartDate: `${year - 1}-01-01`,
    Coverage: { Start: `${year}-01-01`, End: `${year}-07-02` },
    Activity: {
      NovelsCreated: 2,
      VolumesCreated: 3,
      ChaptersCreated: 4,
      CharactersCreated: 6,
      CharacterGroupsCreated: 2,
      CharacterRelationsCreated: 12,
      LocationsCreated: 3,
      LocationRelationsCreated: 5,
      StoryArcsCreated: 4,
      ArcNodesCreated: 18,
      TimelineEntriesCreated: 9,
      ReaderPerspectivesCreated: 7,
      SettingsCreated: 15,
      PreferencesCreated: 3,
      StyleSamplesCreated: 8,
      SkillsCreated: 2,
      ImagesImported: 6,
      SessionsCreated: 3,
      CreativeSeconds: 10800,
      ConversationTurns: 4,
      ToolCalls: 9,
    },
    Writing: {
      WordsAdded: 900,
      WordsDeleted: 200,
      WordsNet: 700,
      ActiveDays: 2,
      LongestStreak: 2,
    },
    Usage: {
      InputTokens: 1000,
      OutputTokens: 50,
      TotalTokens: 999,
      CacheHitTokens: 10,
      CacheMissTokens: 990,
      ReasoningTokens: 5,
    },
    UsageCount: 4,
    DailyActivity: [
      { Date: `${year}-01-03`, CreativeSeconds: 3600 },
      { Date: `${year}-02-03`, CreativeSeconds: 7200 },
    ],
    DailyWriting: [
      {
        date: `${year}-01-03`,
        words_added: 400,
        words_deleted: 200,
        words_net: 200,
      },
      {
        date: `${year}-01-04`,
        words_added: 500,
        words_deleted: 0,
        words_net: 500,
      },
    ],
    DailyLLMUsage: [],
    Monthly: Array.from({ length: 12 }, (_, index) => ({
      Month: index + 1,
      WordsAdded: index === 0 ? 900 : 0,
      CreativeSeconds: index === 0 ? 3600 : index === 1 ? 7200 : 0,
    })),
    Novels: [
      { NovelID: 1, Title: "Night Voyage", Deleted: false, WordsAdded: 700 },
      { NovelID: 2, Title: "", Deleted: true, WordsAdded: 200 },
    ],
    Models: [
      {
        Provider: "provider-a",
        Model: "model-a",
        UsageCount: 3,
        TotalTokens: 799,
      },
      {
        Provider: "provider-b",
        Model: "model-b",
        UsageCount: 1,
        TotalTokens: 200,
      },
    ],
  });
}
