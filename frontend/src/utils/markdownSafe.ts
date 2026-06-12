const dangerousBlockRe = /<(script|iframe|object|embed|style|link|meta|base|svg|math)\b[\s\S]*?<\/\1>/gi
const dangerousSelfClosingRe = /<(script|iframe|object|embed|style|link|meta|base|svg|math)\b[^>]*\/?>/gi
const eventAttrRe = /\s+on[a-z]+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)/gi
const styleAttrRe = /\s+style\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)/gi
const javascriptUrlRe = /\s+(href|src)\s*=\s*("\s*javascript:[^"]*"|'\s*javascript:[^']*'|\s*javascript:[^\s>]+)/gi

export function sanitizeRenderedMarkdown(html: string): string {
  return html
    .replace(dangerousBlockRe, '')
    .replace(dangerousSelfClosingRe, '')
    .replace(eventAttrRe, '')
    .replace(styleAttrRe, '')
    .replace(javascriptUrlRe, '')
}
