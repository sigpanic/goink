import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import SkillEditForm from "./SkillEditForm";

const original = "---\nname: 旧技能\ndescription: 说明\nmode: auto\nversion: 1\ncustom: 保留\n---\n\n旧正文";

describe("SkillEditForm", () => {
  it("未保存稿件在父组件更新及重挂载后保持，采用磁盘版时才重置", async () => {
    const onDraftChange = vi.fn();
    const props = {
      source: "novel" as const,
      readOnly: false,
      onDraftChange,
      onSave: vi.fn().mockResolvedValue(undefined),
      onCancel: vi.fn(),
    };
    const view = render(
      <SkillEditForm {...props} content={original} isDirty={false} />,
    );

    fireEvent.change(screen.getByPlaceholderText("skill.contentPlaceholder"), {
      target: { value: "\n本地未保存正文\n\n" },
    });
    const draft = onDraftChange.mock.lastCall?.[0] as string;
    expect(draft).toContain("---\n\n\n本地未保存正文\n\n");
    expect(draft).toContain("custom: 保留");

    view.rerender(
      <SkillEditForm {...props} content="AI 的新正文" isDirty={true} />,
    );
    expect(screen.getByPlaceholderText("skill.contentPlaceholder")).toHaveValue(
      "\n本地未保存正文\n\n",
    );

    view.unmount();
    const reopened = render(
      <SkillEditForm {...props} content={draft} isDirty={true} />,
    );
    expect(screen.getByPlaceholderText("skill.contentPlaceholder")).toHaveValue(
      "\n本地未保存正文\n\n",
    );

    reopened.rerender(
      <SkillEditForm {...props} content={original} isDirty={false} />,
    );
    await waitFor(() =>
      expect(screen.getByPlaceholderText("skill.contentPlaceholder")).toHaveValue(
        "旧正文",
      ),
    );
  });

  it("保存期间禁用输入；失败后保留稿件和编辑状态", async () => {
    let rejectSave!: (error: Error) => void;
    const onSave = vi.fn(
      () => new Promise<void>((_resolve, reject) => { rejectSave = reject; }),
    );
    const onDraftChange = vi.fn();
    render(
      <SkillEditForm
        content={original}
        isDirty={false}
        onDraftChange={onDraftChange}
        onSave={onSave}
        onCancel={vi.fn()}
      />,
    );
    fireEvent.change(screen.getByPlaceholderText("skill.contentPlaceholder"), {
      target: { value: "不能丢的正文" },
    });
    fireEvent.click(screen.getByRole("button", { name: "skill.save" }));

    expect(screen.getByPlaceholderText("skill.contentPlaceholder")).toBeDisabled();
    expect(onSave).toHaveBeenCalledWith(expect.stringContaining("不能丢的正文"));
    await act(async () => rejectSave(new Error("磁盘写入失败")));

    expect(screen.getByText("磁盘写入失败")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("skill.contentPlaceholder")).toHaveValue(
      "不能丢的正文",
    );
    expect(screen.getByPlaceholderText("skill.contentPlaceholder")).toBeEnabled();
  });
});
