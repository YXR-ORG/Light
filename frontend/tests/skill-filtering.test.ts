import assert from 'node:assert/strict'
import { enabledSkills, filterSelectedEnabledSkillIDs } from '../src/utils/skills'

const skills = [
  { id: 'a', name: 'A', enabled: true },
  { id: 'b', name: 'B', enabled: false },
  { id: 'c', name: 'C', enabled: true },
]

assert.deepEqual(enabledSkills(skills as any).map(s => s.id), ['a', 'c'])
assert.deepEqual(filterSelectedEnabledSkillIDs(['a', 'b', 'missing'], skills as any), ['a'])
