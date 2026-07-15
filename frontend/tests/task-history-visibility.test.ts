import assert from 'node:assert/strict'
import { shouldShowTaskHistory, filterOverlappingTaskHistory } from '../src/utils/taskHistory'

assert.equal(shouldShowTaskHistory(0), false)
assert.equal(shouldShowTaskHistory(2), true)

// Existing DB history must remain visible after the current task is completed.
assert.equal(shouldShowTaskHistory(2, 1), true)

// completedRounds 覆盖的尾部轮次应从 DB 历史中去掉，避免切换会话后双显
const history = [
  { id: '1', role: 'user', content: '旧问题' },
  { id: '2', role: 'assistant', content: '旧回答' },
  { id: '3', role: 'user', content: '新问题' },
  { id: '4', role: 'assistant', content: '新回答' },
]
const rounds = [
  { userContent: '新问题', assistantContent: '新回答' },
]
const filtered = filterOverlappingTaskHistory(history, rounds)
assert.equal(filtered.length, 2)
assert.equal(filtered[0].id, '1')
assert.equal(filtered[1].id, '2')

// 流式中：DB 已写入 user，但当前流式块也在显示同一条 user
const streamingHistory = [
  { id: '1', role: 'user', content: '旧问题' },
  { id: '2', role: 'assistant', content: '旧回答' },
  { id: '3', role: 'user', content: '进行中' },
]
const streamingFiltered = filterOverlappingTaskHistory(streamingHistory, [], '进行中')
assert.equal(streamingFiltered.length, 2)
assert.equal(streamingFiltered[1].id, '2')

// 无重叠时原样返回
const untouched = filterOverlappingTaskHistory(history, [
  { userContent: '不存在', assistantContent: '不存在' },
])
assert.equal(untouched.length, 4)

// 多轮 completedRounds 全部覆盖时历史应为空
const allCovered = filterOverlappingTaskHistory(history, [
  { userContent: '旧问题', assistantContent: '旧回答' },
  { userContent: '新问题', assistantContent: '新回答' },
])
assert.equal(allCovered.length, 0)

console.log('task-history-visibility tests passed')
