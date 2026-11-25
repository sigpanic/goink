import { useCallback, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useMoveSkill } from "@/components/skill/useMoveSkill";
import { getEditorSaveQueue } from "./editorSaveQueue";
import { isSkillPath, skillNameFromPath, sourceFromPath } from "./types";
import { useEditorTabsStore } from "./useEditorTabsStore";

export interface MoveSkillTabRef {
  tabId: string;
  path: string;
}

// useMoveSkillTab：技能换层，并同步受影响的编辑器 tab。
//
// 职责：
// - 调 MoveSkill 落盘；
// - 成功后把发起 tab 的 path 切到新层级，避免后续保存/关闭/预览仍指向旧文件；
// - 关闭目标层同 path 的遗留 tab（干净时），避免同 path 出现两个 tab；
// - 源为用户级时，关闭其他小说里指向旧用户级路径的 tab。
//
// tab 由调用时传入而不是 hook 时固定：这样 hook 只依赖 novelId，
// 调用方（ContentPanel / 未来的 SkillList）在动作发生那一刻决定操作哪个 tab，
// 不会被 render 时的快照绑死。
//
// 目标 tab 或其他小说 tab 有未保存修改时抛错阻止移动；
// mutation 自身的 AppErr 原样向上抛，由调用方映射文案。
export function useMoveSkillTab(novelId: number) {
  const { t } = useTranslation();
  const { mutateAsync: moveSkill } = useMoveSkill(novelId);
  const queue = getEditorSaveQueue(useQueryClient());
  const movingRef = useRef(false);

  return useCallback(
    async ({ tabId, path }: MoveSkillTabRef, to: "novel" | "user") => {
      if (movingRef.current) return;
      if (!isSkillPath(path)) return;
      const from = sourceFromPath(path);
      if (from !== "novel" && from !== "user") return;
      if (from === to) return;

      const name = skillNameFromPath(path);
      const newPath =
        to === "user" ? `~/.goink/skills/${name}.md` : `skills/${name}.md`;
      const store = useEditorTabsStore.getState();

      // 目标层已有同名 tab：脏草稿先阻止（通常是复制出来、尚未落盘的草稿），
      // 干净 tab 等移动成功后再关闭，移动失败则不产生副作用。
      const targetTab = store.byNovel[String(novelId)]?.tabs.find(
        (tab) =>
          tab.type === "file" && tab.path === newPath && tab.id !== tabId,
      );
      if (
        targetTab &&
        (targetTab.isDirty || queue.isPending(novelId, newPath))
      ) {
        throw new Error(t("skill.moveTargetTabOpen"));
      }

      // 源是用户级时，收集其他小说里指向旧路径的 tab；有未保存修改就阻止，
      // 避免移动后它再保存把旧用户级文件重新创建一份。
      const staleUserTabs: { novelId: number; tabId: string }[] = [];
      if (from === "user") {
        for (const [key, entry] of Object.entries(store.byNovel)) {
          const otherNovelId = Number(key);
          if (otherNovelId === novelId) continue;
          for (const tab of entry.tabs) {
            if (tab.type !== "file" || tab.path !== path) continue;
            if (tab.isDirty || queue.isPending(otherNovelId, path)) {
              throw new Error(t("skill.moveOtherTabDirty"));
            }
            staleUserTabs.push({ novelId: otherNovelId, tabId: tab.id });
          }
        }
      }

      movingRef.current = true;
      try {
        await moveSkill({ novel_id: novelId, name, from, to });
        const latest = useEditorTabsStore.getState();
        if (targetTab) latest.closeTab(novelId, targetTab.id);
        for (const item of staleUserTabs) {
          latest.closeTab(item.novelId, item.tabId);
        }
        latest.updateTab(novelId, tabId, { path: newPath, readOnly: false });
      } finally {
        movingRef.current = false;
      }
    },
    [moveSkill, novelId, queue, t],
  );
}
