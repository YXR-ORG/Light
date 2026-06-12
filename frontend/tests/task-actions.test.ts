import assert from 'node:assert/strict'
import { shouldShowTaskActions, taskCopyText } from '../src/utils/taskActions'

assert.equal(shouldShowTaskActions('assistant', false, true), true)
assert.equal(shouldShowTaskActions('assistant', true, true), false)
assert.equal(shouldShowTaskActions('user', false, true), false)

assert.equal(taskCopyText('final text', [{ type: 'thinking', content: 'hidden' } as any]), 'final text')
assert.equal(taskCopyText('', [{ type: 'content', content: 'hello' } as any, { type: 'tool_result', tool_result: 'hidden' } as any]), 'hello')
