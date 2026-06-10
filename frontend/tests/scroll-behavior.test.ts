import assert from 'node:assert/strict'
import { isNearBottom, shouldAutoScroll } from '../src/utils/scroll'

assert.equal(isNearBottom(1000, 350, 600), true)
assert.equal(isNearBottom(1000, 200, 600), false)

assert.equal(shouldAutoScroll(false, false), true)
assert.equal(shouldAutoScroll(false, true), false)
assert.equal(shouldAutoScroll(true, true), true)
