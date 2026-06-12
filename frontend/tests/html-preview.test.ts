import assert from 'node:assert/strict'
import { isPreviewableHtmlLanguage, extractHtmlPreviewSource, buildPreviewSrcdoc } from '../src/utils/htmlPreview'

assert.equal(isPreviewableHtmlLanguage('html'), true)
assert.equal(isPreviewableHtmlLanguage('HTML'), true)
assert.equal(isPreviewableHtmlLanguage('javascript'), false)

const rendered = '<pre><code class="language-html">&lt;h1&gt;Hi&lt;/h1&gt;</code></pre>'
assert.equal(extractHtmlPreviewSource(rendered), '<h1>Hi</h1>')

const srcdoc = buildPreviewSrcdoc('<script>fetch("https://example.com")</script>')
assert.match(srcdoc, /Content-Security-Policy/)
assert.match(srcdoc, /connect-src 'none'/)
