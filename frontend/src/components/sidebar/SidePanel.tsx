import { useState, useCallback, useRef, useEffect } from "react";
import { useTranslation } from "react-i18next";
import type { novel, chapter } from "@/hooks/useApp";
import NovelList from "./NovelList";
import ChapterList from "@/components/chapter/ChapterList";
import CharacterList from "@/components/character/CharacterList";
import LocationList from "@/components/location/LocationList";
import SkillList from "@/components/skill/SkillList";
import SearchPanel from "@/components/search/SearchPanel";
import TimelineList from "@/components/timeline/TimelineList";
import ArcList from "@/components/storyarc/ArcList";
import ReaderList from "@/components/reader/ReaderList";
import PreferenceList from "@/components/preference/PreferenceList";
import NovelSettingList from "@/components/novel-setting/NovelSettingList";
import StyleSampleList from "@/components/style/StyleSampleList";
import GitHistoryList from "@/components/git/GitHistoryList";
import type { git } from "@/lib/wailsjs/go/models";
import type { PanelId } from "@/types/panel";
import { usePanelStore } from "@/stores/usePanelStore";

interface Props {
  novels: novel.Novel[];
  novelId: number;
  onSelectNovel: (n: novel.Novel) => void;
  onSelectChapter: (ch: chapter.Chapter) => void;
  onSelectGoink: () => void;
  onExportNovel: (novelId: number) => void;
  target: { path: string; title: string } | null;
  showCreate: boolean;
  setShowCreate: (v: boolean) => void;
  title: string;
  setTitle: (v: string) => void;
  description: string;
  setDescription: (v: string) => void;
  onCreateNovel: () => void;
  activeSkillName: string | null;
  onSelectSkill: (path: string, title: string, readOnly: boolean) => void;
  onEditSkill: (path: string, title: string, readOnly: boolean) => void;
  onNewSkill: (name: string) => void;
  // 4b: 透传 type（storyarc 区分 arc/node 全局跳转，其他领域 undefined）。
  onSearchNavigateEntity: (
    panelId: PanelId,
    entityId: number,
    type?: "arc" | "node",
  ) => void;
  onSearchNavigateChapter: (
    filePath: string,
    title: string,
    chapterNum: number,
    matchPos: number,
    matchLen: number,
  ) => void;
  searchQuery: string;
  onSearchChange: (query: string) => void;
  onSelectGitFile: (file: git.FileDiff) => void;
  onSelectStyleSample: (id: number) => void;
  sidePanelWidth: number;
  onSidePanelResize: (w: number) => void;
}

export default function SidePanel({
  novels,
  novelId,
  onSelectNovel,
  onSelectChapter,
  onSelectGoink,
  onExportNovel,
  target,
  showCreate,
  setShowCreate,
  title,
  setTitle,
  description,
  setDescription,
  onCreateNovel,
  activeSkillName,
  onSelectSkill,
  onEditSkill,
  onNewSkill,
  onSearchNavigateEntity,
  onSearchNavigateChapter,
  searchQuery,
  onSearchChange,
  onSelectGitFile,
  onSelectStyleSample,
  sidePanelWidth,
  onSidePanelResize,
}: Props) {
  const { t } = useTranslation();
  const activePanel = usePanelStore((s) => s.sidebarPanel ?? s.activePanel);
  const [isDragging, setIsDragging] = useState(false);
  const startXRef = useRef(0);
  const startWidthRef = useRef(sidePanelWidth);

  const handleResizeMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      setIsDragging(true);
      startXRef.current = e.clientX;
      startWidthRef.current = sidePanelWidth;
    },
    [sidePanelWidth],
  );

  useEffect(() => {
    if (!isDragging) return;
    const onMove = (e: MouseEvent) => {
      const delta = e.clientX - startXRef.current;
      onSidePanelResize(startWidthRef.current + delta);
    };
    const onUp = () => setIsDragging(false);
    document.addEventListener("mousemove", onMove);
    document.addEventListener("mouseup", onUp);
    return () => {
      document.removeEventListener("mousemove", onMove);
      document.removeEventListener("mouseup", onUp);
    };
  }, [isDragging, onSidePanelResize]);

  return (
    <aside
      className="shrink-0 flex flex-col bg-sidebar border-r relative select-none cursor-default"
      style={{ width: sidePanelWidth }}
    >
      {isDragging && (
        <div className="fixed inset-0 z-50 cursor-col-resize select-none" />
      )}
      {activePanel === "search" ? (
        <SearchPanel
          novelId={novelId}
          query={searchQuery}
          onQueryChange={onSearchChange}
          onNavigateEntity={onSearchNavigateEntity}
          onNavigateChapter={onSearchNavigateChapter}
        />
      ) : activePanel === "skills" ? (
        <SkillList
          novelId={novelId}
          activeSkillName={activeSkillName}
          onSelectSkill={onSelectSkill}
          onEditSkill={onEditSkill}
          onNewSkill={onNewSkill}
        />
      ) : activePanel === "novels" ? (
        <NovelList
          novels={novels}
          novelId={novelId}
          onSelectNovel={onSelectNovel}
          showCreate={showCreate}
          setShowCreate={setShowCreate}
          title={title}
          setTitle={setTitle}
          description={description}
          setDescription={setDescription}
          onCreateNovel={onCreateNovel}
        />
      ) : activePanel === "chapters" ? (
        <ChapterList
          novelId={novelId}
          target={target}
          onSelectChapter={onSelectChapter}
          onSelectGoink={onSelectGoink}
          onExportNovel={() => onExportNovel(novelId)}
        />
      ) : activePanel === "characters" ? (
        <CharacterList novelId={novelId} />
      ) : activePanel === "locations" ? (
        <LocationList novelId={novelId} />
      ) : activePanel === "storyarcs" ? (
        <ArcList novelId={novelId} />
      ) : activePanel === "timeline" ? (
        <TimelineList novelId={novelId} />
      ) : activePanel === "reader" ? (
        <ReaderList novelId={novelId} />
      ) : activePanel === "preferences" ? (
        <PreferenceList novelId={novelId} />
      ) : activePanel === "novel-settings" ? (
        <NovelSettingList novelId={novelId} />
      ) : activePanel === "git" ? (
        <GitHistoryList novelId={novelId} onSelectFile={onSelectGitFile} />
      ) : activePanel === "style-samples" ? (
        <StyleSampleList
          onSelectSample={onSelectStyleSample}
          novelId={novelId}
        />
      ) : (
        <div className="flex-1 flex items-center justify-center">
          <p className="text-xs text-muted-foreground">
            {t("sidebar.comingSoon")}
          </p>
        </div>
      )}
      <div
        className="absolute right-0 top-0 bottom-0 w-1 cursor-col-resize hover:bg-primary/30 transition-colors z-10 select-none"
        style={{ marginRight: -2 }}
        onMouseDown={handleResizeMouseDown}
      />
    </aside>
  );
}
