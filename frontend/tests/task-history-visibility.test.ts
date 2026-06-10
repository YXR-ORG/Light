import assert from 'node:assert/strict'
import { shouldShowTaskHistory } from '../src/utils/taskHistory'

assert.equal(shouldShowTaskHistory(0), false)
assert.equal(shouldShowTaskHistory(2), true)

// Existing DB history must remain visible after the current task is completed.
assert.equal(shouldShowTaskHistory(2, 1), true)
