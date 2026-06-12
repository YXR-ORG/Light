import assert from 'node:assert/strict'
import { sanitizeRenderedMarkdown } from '../src/utils/markdownSafe'

const malicious = '<p>ok</p><img src="x" style="position:fixed;inset:0;width:9999px" onerror="alert(1)"><script>alert(1)</script><iframe src="https://example.com"></iframe>'
const safe = sanitizeRenderedMarkdown(malicious)

assert.equal(safe.includes('position:fixed'), false)
assert.equal(safe.includes('onerror'), false)
assert.equal(safe.includes('<script'), false)
assert.equal(safe.includes('<iframe'), false)
assert.equal(safe.includes('<img src="x"'), true)
