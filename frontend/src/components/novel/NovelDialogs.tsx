import { useNovelStore } from "./useNovelStore";
import { usePanelStore } from "@/stores/usePanelStore";
import { useNovels } from "./useNovels";
import { useCreateNovel } from "./useCreateNovel";
import { useUpdateNovel } from "./useUpdateNovel";
import { useDeleteNovel } from "./useDeleteNovel";
import { ExportNovel } from "@/lib/wailsjs/go/app/App";
import NovelEditDialog from "./NovelEditDialog";
import NovelDeleteDialog from "./NovelDeleteDialog";
import ExportDialog from "@/components/export/ExportDialog";

// 3.6: 把 4 个小说对话框（创建/编辑/删除/导出）从 WorkspaceView 抽到独立组件。
// 内部订阅 useNovelStore 取对话框开关 + 4 个 setter；用 3.3-3.5 mutation 提交。
// 3.8 后续：删 switchToNovel prop，直接调 useNovelStore.switchNovel + usePanelStore.setActivePanel。
// reset 由 WorkspaceView effect 监听 activeNovelId 变化自动调。
export default function NovelDialogs() {
  const switchNovel = useNovelStore((s) => s.switchNovel);
  const setActivePanel = usePanelStore((s) => s.setActivePanel);
  // ExportDialog 显示标题用 novels 列表查找；与 WorkspaceView 共享 useNovels 缓存。
  const { data: novels = [] } = useNovels();
  const editingNovel = useNovelStore((s) => s.editingNovel);
  const deletingNovel = useNovelStore((s) => s.deletingNovel);
  const showCreateDialog = useNovelStore((s) => s.showCreateDialog);
  const exportNovelId = useNovelStore((s) => s.exportNovelId);
  const setEditingNovel = useNovelStore((s) => s.setEditingNovel);
  const setDeletingNovel = useNovelStore((s) => s.setDeletingNovel);
  const setShowCreateDialog = useNovelStore((s) => s.setShowCreateDialog);
  const setExportNovelId = useNovelStore((s) => s.setExportNovelId);

  const createNovel = useCreateNovel();
  const updateNovel = useUpdateNovel();
  const deleteNovel = useDeleteNovel();

  async function handleCreateNovelFromDialog(input: {
    title: string;
    description: string;
    genre: string;
  }) {
    try {
      // mutation 的 onSuccess 已失效 novelKeys.all；handler 只管关 dialog + 切到新小说。
      const n = await createNovel.mutateAsync(input);
      setShowCreateDialog(false);
      // 3.8 后续：删 switchToNovel wrapper，直接调 switchNovel + setActivePanel。
      await switchNovel(n.id);
      setActivePanel("chapters");
    } catch (err) {
      console.error(err);
      throw err;
    }
  }

  async function handleUpdateNovel(input: {
    title: string;
    description: string;
    genre: string;
  }) {
    if (!editingNovel) return;
    try {
      await updateNovel.mutateAsync({ id: editingNovel.id, input });
      setEditingNovel(null);
    } catch (err) {
      console.error(err);
      throw err;
    }
  }

  async function handleDeleteNovel() {
    if (!deletingNovel) return;
    try {
      // 删除当前 activeNovelId 时，refetch 后 WorkspaceView 的自动选小说 effect 接管。
      await deleteNovel.mutateAsync(deletingNovel.id);
      setDeletingNovel(null);
    } catch (err) {
      console.error(err);
      throw err;
    }
  }

  async function handleExportNovel(format: "epub" | "markdown" | "txt") {
    if (exportNovelId == null) return;
    try {
      await ExportNovel(exportNovelId, format);
    } catch (err) {
      console.error(err);
      throw err;
    }
  }

  return (
    <>
      <NovelEditDialog
        open={showCreateDialog}
        onClose={() => setShowCreateDialog(false)}
        onSave={handleCreateNovelFromDialog}
      />
      <NovelEditDialog
        open={!!editingNovel}
        novel={editingNovel}
        onClose={() => setEditingNovel(null)}
        onSave={handleUpdateNovel}
      />
      <NovelDeleteDialog
        open={!!deletingNovel}
        novelTitle={deletingNovel?.title ?? ""}
        onClose={() => setDeletingNovel(null)}
        onConfirm={handleDeleteNovel}
      />
      <ExportDialog
        open={exportNovelId !== null}
        novelTitle={novels.find((n) => n.id === exportNovelId)?.title ?? ""}
        onClose={() => setExportNovelId(null)}
        onExport={handleExportNovel}
      />
    </>
  );
}
