import { describe, expect, it } from "vitest";
import type { chapter } from "@/lib/wailsjs/go/models";
import { buildChapterReferenceMap } from "./chapterReferenceMap";

describe("buildChapterReferenceMap", () => {
  it("keeps stable IDs separate from reading numbers", () => {
    const references = buildChapterReferenceMap([
      { id: 42, reading_number: 1 },
      { id: 7, reading_number: 2 },
    ] as chapter.Chapter[]);

    expect(references.chapterIDByReadingNumber.get(1)).toBe(42);
    expect(references.readingNumberByChapterID.get(42)).toBe(1);
  });
});
