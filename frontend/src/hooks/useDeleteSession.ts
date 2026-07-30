import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { app } from "@/hooks/useApp";
import { useApp } from "@/hooks/useApp";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";

// useDeleteSession 封装会话删除的通用逻辑：删除目标、loading 状态、调用 DeleteSession、
// 错误提示（toastError + console.error）。删除成功后调用 onDeleted 回调，由调用方负责
// 更新各自的数据源——SessionHistory 自管分页 state，RecentSessions 通知父组件。
//
// 抽出此 hook 的原因：SessionHistory 与 RecentSessions 两处删除逻辑除"删除成功后更新哪个
// 数据源"外完全一致，统一到此 hook，后续改文案/行为只改一处。
export function useDeleteSession(onDeleted: (sessionId: string) => void) {
  const { t } = useTranslation();
  const app = useApp();
  const [deleteTarget, setDeleteTarget] = useState<app.SessionMeta | null>(
    null,
  );
  const [deleting, setDeleting] = useState(false);

  async function handleDeleteSession() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await app.DeleteSession(deleteTarget.session_id);
      onDeleted(deleteTarget.session_id);
      setDeleteTarget(null);
    } catch (err) {
      toastError(t("chat.deleteSessionFailed") + ": " + toErrorMessage(err));
      console.error(err);
    } finally {
      setDeleting(false);
    }
  }

  return {
    deleteTarget,
    deleting,
    setDeleteTarget,
    handleDeleteSession,
  };
}
