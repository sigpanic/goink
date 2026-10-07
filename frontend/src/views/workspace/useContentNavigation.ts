import type { RefObject } from "react";
import { flushSync } from "react-dom";
import { useTranslation } from "react-i18next";
import type { chapter } from "@/lib/wailsjs/go/models";
import type { ContentPanelHandle } from "@/components/content/ContentPanel";
import { useEditorStore } from "@/stores/useEditorStore";
import type { PanelId } from "@/types/panel";
import { describeFile } from "@/components/content/fileDescription";

interface Options {
  contentRef: RefObject<ContentPanelHandle | null>;
  setActivePanel: (panel: PanelId) => void;
  setActiveSkillName: (name: string) => void;
}

export function useContentNavigation({
  contentRef,
  setActivePanel,
  setActiveSkillName,
}: Options) {
  const { t } = useTranslation();

  function handleSelectChapter(ch: chapter.Chapter) {
    const chTitle = describeFile(ch.file_path, t, [ch]).title;
    // 3.8 后续：tabTarget 迁 useEditorStore，写方调 getState().setTabTarget。
    useEditorStore
      .getState()
      .setTabTarget({ path: ch.file_path, title: chTitle });
    contentRef.current?.openFile(ch.file_path, chTitle);
  }

  function handleSelectGoink() {
    useEditorStore.getState().setTabTarget({
      path: "goink.md",
      title: describeFile("goink.md", t).title,
    });
    contentRef.current?.openFile("goink.md", describeFile("goink.md", t).title);
  }

  function handleSelectVolumeOutline(path: string, volumeName: string) {
    const title = describeFile(
      path,
      t,
      [],
      [{ name: volumeName, outline_file_path: path }],
    ).title;
    useEditorStore.getState().setTabTarget({ path, title });
    contentRef.current?.openFile(path, title);
  }

  function handleSelectSkill(path: string, title: string, readOnly: boolean) {
    setActiveSkillName(title);
    contentRef.current?.openFile(path, title, readOnly);
  }

  function handleEditSkill(path: string, title: string, readOnly: boolean) {
    setActiveSkillName(title);
    contentRef.current?.openFile(path, title, readOnly, "edit");
  }

  function handleNewSkill(name: string) {
    setActiveSkillName(`${t("workspace.skillLabel")}${name}`);
    contentRef.current?.openFile(
      `skills/${name}.md`,
      `${t("workspace.skillLabel")}${name}`,
      false,
      "edit",
    );
  }

  function handleSearchNavigateChapter(
    filePath: string,
    title: string,
    _chapterNum: number,
    matchPos: number,
    matchLen: number,
  ) {
    flushSync(() => setActivePanel("chapters"));
    if (matchPos >= 0 && matchLen > 0) {
      contentRef.current?.openFileWithHighlight(
        filePath,
        title,
        matchPos,
        matchLen,
      );
    } else {
      contentRef.current?.openFile(filePath, title);
    }
  }

  return {
    handleSelectChapter,
    handleSelectGoink,
    handleSelectVolumeOutline,
    handleSelectSkill,
    handleEditSkill,
    handleNewSkill,
    handleSearchNavigateChapter,
  };
}
