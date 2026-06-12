export interface SkillLike { id: string; enabled: boolean }

export function enabledSkills<T extends SkillLike>(skills: T[]): T[] {
  return skills.filter(skill => skill.enabled)
}

export function filterSelectedEnabledSkillIDs(selectedIDs: string[], skills: SkillLike[]): string[] {
  const enabledIDs = new Set(enabledSkills(skills).map(skill => skill.id))
  return selectedIDs.filter(id => enabledIDs.has(id))
}
