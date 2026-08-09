import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import { sessionMessagesKeys } from "@/lib/queryKeys";
import { useChatStore } from "./useChatStore";
import { useDeleteSession } from "./useDeleteSession";

interface Props {
  activeSessionId: string | null | undefined;
  onActiveSessionDeleted: () => void;
}

// DeleteSessionDialog: 会话删除集中确认对话框（ChatPanel 唯一入口）。
// 从 store 取 deletingSession，挂唯一 ConfirmDialog；
// SessionHistory/RecentSessions 点删除只 dispatch store，不执行、不挂 ConfirmDialog。
// 删除成功后：invalidate sessions 全前缀（mutation onSuccess）+
//   若删的是活跃会话则 invalidate messages + onActiveSessionDeleted 清空视图。
// 删除失败：调用方 try/catch + toastError（mutation 不挂 onError）。
export default function DeleteSessionDialog({
  activeSessionId,
  onActiveSessionDeleted,
}: Props) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const deletingSession = useChatStore((s) => s.deletingSession);
  const setDeletingSession = useChatStore((s) => s.setDeletingSession);
  const deleteMutation = useDeleteSession();

  const handleConfirm = async () => {
    if (!deletingSession) return;
    try {
      await deleteMutation.mutateAsync(deletingSession.session_id);
      // 若删的是活跃会话，invalidate messages + 清空活跃会话视图
      if (deletingSession.session_id === activeSessionId) {
        qc.invalidateQueries({
          queryKey: sessionMessagesKeys.detail(deletingSession.session_id),
        });
        onActiveSessionDeleted();
      }
      setDeletingSession(null);
    } catch (err) {
      toastError(t("chat.deleteSessionFailed") + ": " + toErrorMessage(err));
    }
  };

  return (
    <ConfirmDialog
      open={deletingSession !== null}
      title={t("chat.confirmDeleteSession")}
      message={
        deletingSession
          ? t("chat.confirmDeleteSessionMessage", {
              title: deletingSession.title || t("chat.newChat"),
            })
          : ""
      }
      danger
      loading={deleteMutation.isPending}
      confirmText={t("common.delete")}
      onClose={() => setDeletingSession(null)}
      onConfirm={handleConfirm}
    />
  );
}
