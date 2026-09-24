export function useMarkdownRenderer() {
  function escapeHtml(text: string): string {
    return text
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
  }

  function processInline(text: string): string {
    return escapeHtml(text)
      .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
      .replace(/\*(.+?)\*/g, '<em>$1</em>')
      .replace(/`([^`]+)`/g, '<code>$1</code>')
  }

  function processInlineWithSyntax(text: string): string {
    let result = escapeHtml(text)
    
    result = result.replace(/\*\*(.+?)\*\*/g, '<span class="md-syntax">**</span><strong>$1</strong><span class="md-syntax">**</span>')
    result = result.replace(/(?<!\*)\*([^*]+?)\*(?!\*)/g, '<span class="md-syntax">*</span><em>$1</em><span class="md-syntax">*</span>')
    result = result.replace(/`([^`]+)`/g, '<span class="md-syntax">`</span><code>$1</code><span class="md-syntax">`</span>')
    
    return result
  }

  function renderHeader(line: string): string {
    if (line.startsWith('### ')) {
      return `<h3><span class="md-syntax">###</span> ${escapeHtml(line.slice(4))}</h3>`
    }
    if (line.startsWith('## ')) {
      return `<h2><span class="md-syntax">##</span> ${escapeHtml(line.slice(3))}</h2>`
    }
    if (line.startsWith('# ')) {
      return `<h1><span class="md-syntax">#</span> ${escapeHtml(line.slice(2))}</h1>`
    }
    return ''
  }

  function renderListItem(line: string): string {
    if (line.match(/^[\-\*]\s/)) {
      return `<li><span class="md-syntax">${line.slice(0, 1)}</span> ${processInline(line.slice(2))}</li>`
    }
    if (line.match(/^\d+\.\s/)) {
      const match = line.match(/^(\d+\.)\s(.*)$/)
      if (match) {
        return `<li><span class="md-syntax">${match[1]}</span> ${processInline(match[2])}</li>`
      }
    }
    return ''
  }

  function renderParagraph(line: string): string {
    return `<p>${processInlineWithSyntax(line)}</p>`
  }

  function wrapLists(html: string): string {
    html = html.replace(/(<li>.*?<\/li>)(?=<li>)/g, '<ul>$1</ul>')
    html = html.replace(/(<\/li>)(?=<ul>)/g, '$1</ul>')
    return html
  }

  function renderMarkdown(text: string): string {
    if (!text) {
      return '<span class="placeholder">Skriv dina regler här...</span>'
    }

    const html = text
      .split('\n')
      .map(line => {
        if (line.startsWith('### ') || line.startsWith('## ') || line.startsWith('# ')) {
          return renderHeader(line)
        }
        if (line.match(/^[\-\*]\s/) || line.match(/^\d+\.\s/)) {
          return renderListItem(line)
        }
        if (line.trim() === '') return ''
        return renderParagraph(line)
      })
      .join('')

    return wrapLists(html)
  }

  return { renderMarkdown, escapeHtml, processInline }
}
