import assert from 'node:assert/strict'
import { splitTaskArtifacts, type Artifact } from '../src/utils/artifacts'

const artifacts: Artifact[] = [
  { type: 'plan', title: '执行计划', plan_id: 'plan-1', steps: [{ content: '分析', status: 'done' }] },
  { type: 'file', action: 'read', title: 'notes.md', path: 'notes.md', abs_path: '/tmp/notes.md' },
  { type: 'url', title: '参考链接', url: 'https://example.com' },
]

const grouped = splitTaskArtifacts(artifacts)

assert.equal(grouped.plans.length, 1)
assert.equal(grouped.plans[0].title, '执行计划')
assert.equal(grouped.files.length, 1)
assert.equal(grouped.files[0].title, 'notes.md')
assert.equal(grouped.others.length, 1)
assert.equal(grouped.others[0].type, 'url')
