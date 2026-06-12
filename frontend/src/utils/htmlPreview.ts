export function isPreviewableHtmlLanguage(lang?: string): boolean {
  return (lang || '').trim().toLowerCase() === 'html'
}

function decodeHtml(s: string): string {
  return s
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
}

function stripTags(s: string): string {
  return s.replace(/<[^>]*>/g, '')
}

export function extractHtmlPreviewSource(renderedHtml: string): string {
  const match = renderedHtml.match(/<pre><code[^>]*class="[^"]*language-html[^"]*"[^>]*>([\s\S]*?)<\/code><\/pre>/i)
  return match ? decodeHtml(stripTags(match[1])) : ''
}

export function decorateHtmlPreviewBlocks(renderedHtml: string): string {
  return renderedHtml.replace(
    /<pre><code([^>]*)class="([^"]*language-html[^"]*)"([^>]*)>([\s\S]*?)<\/code><\/pre>/gi,
    '<div class="html-preview-block"><pre><code$1class="$2"$3>$4</code></pre><button type="button" class="html-preview-run" title="运行预览">运行预览</button></div>',
  )
}

export function buildPreviewSrcdoc(html: string): string {
  const csp = `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; connect-src 'none'; media-src data: blob:; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'">`
  if (/<head[\s>]/i.test(html)) {
    return html.replace(/<head([^>]*)>/i, `<head$1>${csp}`)
  }
  if (/<html[\s>]/i.test(html)) {
    return html.replace(/<html([^>]*)>/i, `<html$1><head>${csp}</head>`)
  }
  return `<!doctype html><html><head>${csp}</head><body>${html}</body></html>`
}

export function getHtmlPreviewSourceFromButton(button: HTMLElement): string {
  const block = button.closest('.html-preview-block')
  const code = block?.querySelector('code')
  return code?.textContent || ''
}
