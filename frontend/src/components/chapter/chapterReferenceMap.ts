import type { chapter } from "@/lib/wailsjs/go/models";

export type ChapterReferenceMap = {
  chapterIDByReadingNumber: Map<number, number>;
  readingNumberByChapterID: Map<number, number>;
};

export function buildChapterReferenceMap(
  chapters: chapter.Chapter[],
): ChapterReferenceMap {
  const chapterIDByReadingNumber = new Map<number, number>();
  const readingNumberByChapterID = new Map<number, number>();

  for (const chapter of chapters) {
    chapterIDByReadingNumber.set(chapter.reading_number, chapter.id);
    readingNumberByChapterID.set(chapter.id, chapter.reading_number);
  }

  return { chapterIDByReadingNumber, readingNumberByChapterID };
}
