import type { character } from "@/lib/wailsjs/go/models";

export function indexCharacterMemberships(members: character.GroupMember[]) {
  const byCharacter = new Map<number, Set<number>>();
  for (const member of members) {
    let groups = byCharacter.get(member.character_id);
    if (!groups) {
      groups = new Set();
      byCharacter.set(member.character_id, groups);
    }
    groups.add(member.group_id);
  }
  return byCharacter;
}

export function filterCharactersByGroup(
  characters: character.Character[],
  memberships: Map<number, Set<number>>,
  groupId: number | null,
) {
  if (groupId === null) return characters;
  return characters.filter((item) =>
    groupId === 0
      ? !memberships.get(item.id)?.size
      : memberships.get(item.id)?.has(groupId),
  );
}
