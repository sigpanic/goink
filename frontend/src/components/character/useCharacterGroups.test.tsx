import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { PropsWithChildren } from "react";
import {
  useCharacterGroups,
  useCharacterGroupMembers,
  useCharacterGroupMutations,
} from "./useCharacterGroups";
import { useDeleteCharacter } from "./useDeleteCharacter";
import { characterKeys } from "@/lib/queryKeys";

const api = vi.hoisted(() => ({
  GetCharacterGroups: vi.fn(),
  GetCharacterGroupMembers: vi.fn(),
  CreateCharacterGroup: vi.fn(),
  UpdateCharacterGroup: vi.fn(),
  DeleteCharacterGroup: vi.fn(),
  UpdateCharacterGroupMemberships: vi.fn(),
  DeleteCharacter: vi.fn(),
}));
vi.mock("@/lib/wailsjs/go/app/App", () => api);

function setup() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  function wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  }
  return { qc, wrapper };
}

describe("character group queries and cache updates", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    api.GetCharacterGroups.mockImplementation(({ novel_id }) =>
      Promise.resolve({
        items: [
          { id: novel_id * 10, novel_id, name: "Group", member_count: 1 },
        ],
      }),
    );
    api.GetCharacterGroupMembers.mockImplementation(({ novel_id }) =>
      Promise.resolve({
        items: [{ id: 1, novel_id, group_id: novel_id * 10, character_id: 1 }],
      }),
    );
  });

  it("shares group queries across consumers and isolates data by novel", async () => {
    const { qc, wrapper } = setup();
    const { result, rerender } = renderHook(
      ({ novelId }) => ({
        groups: useCharacterGroups(novelId),
        sidebarGroups: useCharacterGroups(novelId),
        members: useCharacterGroupMembers(novelId),
      }),
      { wrapper, initialProps: { novelId: 1 } },
    );
    await waitFor(() =>
      expect(
        result.current.groups.isSuccess && result.current.members.isSuccess,
      ).toBe(true),
    );
    expect(api.GetCharacterGroups).toHaveBeenCalledTimes(1);
    expect(api.GetCharacterGroups).toHaveBeenCalledWith({
      novel_id: 1,
      page: 1,
      size: -1,
      search: "",
    });
    expect(api.GetCharacterGroupMembers).toHaveBeenCalledWith({
      novel_id: 1,
      page: 1,
      size: -1,
      search: "",
      group_id: 0,
      character_id: 0,
    });
    rerender({ novelId: 2 });
    await waitFor(() =>
      expect(result.current.groups.data?.[0].novel_id).toBe(2),
    );
    expect(result.current.sidebarGroups.data?.[0].id).toBe(20);
    expect(qc.getQueryData(characterKeys.groups(1))).toEqual(
      expect.arrayContaining([expect.objectContaining({ id: 10 })]),
    );
  });

  it("refreshes memberships and group counts after a membership mutation", async () => {
    const { wrapper } = setup();
    const { result } = renderHook(
      () => ({
        groups: useCharacterGroups(1),
        members: useCharacterGroupMembers(1),
        actions: useCharacterGroupMutations(1),
      }),
      { wrapper },
    );
    await waitFor(() =>
      expect(
        result.current.groups.isSuccess && result.current.members.isSuccess,
      ).toBe(true),
    );
    api.UpdateCharacterGroupMemberships.mockImplementation(() => {
      api.GetCharacterGroups.mockResolvedValue({
        items: [
          { id: 10, member_count: 0 },
          { id: 20, member_count: 1 },
        ],
      });
      api.GetCharacterGroupMembers.mockResolvedValue({
        items: [{ character_id: 1, group_id: 20 }],
      });
      return Promise.resolve();
    });
    const input = {
      character_ids: [1],
      add_group_ids: [20],
      remove_group_ids: [10],
    };
    await act(async () => {
      await result.current.actions.memberships.mutateAsync(input);
    });
    expect(api.UpdateCharacterGroupMemberships).toHaveBeenCalledWith(1, input);
    await waitFor(() =>
      expect(result.current.members.data?.[0].group_id).toBe(20),
    );
    expect(result.current.groups.data?.[0].member_count).toBe(0);
    expect(result.current.groups.data?.[1].member_count).toBe(1);
  });

  it("refreshes group counts and removes cached memberships when a character is deleted", async () => {
    const { wrapper } = setup();
    const { result } = renderHook(
      () => ({
        groups: useCharacterGroups(1),
        members: useCharacterGroupMembers(1),
        remove: useDeleteCharacter(1),
      }),
      { wrapper },
    );
    await waitFor(() =>
      expect(
        result.current.groups.isSuccess && result.current.members.isSuccess,
      ).toBe(true),
    );
    api.DeleteCharacter.mockImplementation(() => {
      api.GetCharacterGroups.mockResolvedValue({
        items: [{ id: 10, member_count: 0 }],
      });
      api.GetCharacterGroupMembers.mockResolvedValue({ items: [] });
      return Promise.resolve();
    });
    await act(async () => {
      await result.current.remove.mutateAsync(1);
    });
    await waitFor(() => expect(result.current.members.data).toEqual([]));
    expect(result.current.groups.data?.[0].member_count).toBe(0);
  });

  it("clears cached membership rows after deleting a group", async () => {
    const { wrapper } = setup();
    const { result } = renderHook(
      () => ({
        groups: useCharacterGroups(1),
        members: useCharacterGroupMembers(1),
        actions: useCharacterGroupMutations(1),
      }),
      { wrapper },
    );
    await waitFor(() =>
      expect(
        result.current.groups.isSuccess && result.current.members.isSuccess,
      ).toBe(true),
    );
    api.DeleteCharacterGroup.mockImplementation(() => {
      api.GetCharacterGroups.mockResolvedValue({ items: [] });
      api.GetCharacterGroupMembers.mockResolvedValue({ items: [] });
      return Promise.resolve();
    });
    await act(async () => {
      await result.current.actions.remove.mutateAsync(10);
    });
    await waitFor(() => expect(result.current.groups.data).toEqual([]));
    expect(result.current.members.data).toEqual([]);
  });

  it("leaves cached group data intact when a mutation fails", async () => {
    const { wrapper } = setup();
    const { result } = renderHook(
      () => ({
        groups: useCharacterGroups(1),
        actions: useCharacterGroupMutations(1),
      }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.groups.isSuccess).toBe(true));
    api.UpdateCharacterGroup.mockRejectedValue(new Error("duplicate name"));
    await act(async () => {
      await expect(
        result.current.actions.update.mutateAsync({
          id: 10,
          input: { name: "Taken", description: "" },
        }),
      ).rejects.toThrow("duplicate name");
    });
    expect(api.GetCharacterGroups).toHaveBeenCalledTimes(1);
    expect(result.current.groups.data?.[0].name).toBe("Group");
  });
});
