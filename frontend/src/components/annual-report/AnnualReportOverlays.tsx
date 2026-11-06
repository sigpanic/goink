import { useState } from "react";
import AnnualReportPromptDialog from "./AnnualReportPromptDialog";
import AnnualReportDialog from "./AnnualReportDialog";
import type { useAnnualReportEntry } from "./useAnnualReportEntry";

// 临时无条件展示提醒供预览，发布前设为 false。
const FORCE_PROMPT_PREVIEW = false;

export default function AnnualReportOverlays({
  entry,
}: {
  entry: ReturnType<typeof useAnnualReportEntry>;
}) {
  const [previewOpen, setPreviewOpen] = useState(FORCE_PROMPT_PREVIEW);
  return (
    <>
      <AnnualReportPromptDialog
        open={previewOpen || entry.promptOpen}
        year={entry.season.reportYear}
        onConfirm={() => {
          setPreviewOpen(false);
          entry.openReport();
        }}
        onClose={() => {
          setPreviewOpen(false);
          entry.dismissPrompt();
        }}
      />
      <AnnualReportDialog open={entry.reportOpen} onClose={entry.closeReport} />
    </>
  );
}
