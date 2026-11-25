import { reactive } from 'vue'

const appearance = reactive({ raw: 'light', config: 'light' })
let current
let codeEditor

export function useTheme() { return { theme: appearance } }

export function bindEditorTheme(editor) {
  codeEditor = editor
  updateEditorTheme()
}

function updateEditorTheme() {
  if (!codeEditor || !current) return
  const { colors, dark } = current
  const syntax = {
    'string.key.json': colors.key, 'string.value.json': colors.string, string: colors.string, number: colors.number,
    keyword: colors.keyword, comment: colors.comment, delimiter: colors.muted,
  }
  codeEditor.defineTheme('fiveleaf', {
    base: dark ? 'vs-dark' : 'vs', inherit: true,
    rules: Object.entries(syntax).map(([token, color]) => ({ token, foreground: color.slice(1) })),
    colors: {
      focusBorder: colors.accent, 'input.background': colors.surface, 'input.foreground': colors.text,
      'editor.background': colors.editor, 'editor.foreground': colors.text,
      'editorGutter.background': colors.editor, 'editorLineNumber.foreground': colors.gutter,
      'editor.lineHighlightBackground': colors.line, 'editorCursor.foreground': colors.accent,
      'editorBracketMatch.border': colors.accent, 'editorBracketMatch.background': colors.active,
      'editor.selectionBackground': colors.selection, 'editorWidget.background': colors.surface,
      'editorWidget.foreground': colors.text, 'editorWidget.border': colors.border,
      'editorError.foreground': colors.danger, 'editorWarning.foreground': colors.warning,
    },
  })
  codeEditor.setTheme('fiveleaf')
}

function applyTheme(theme) {
  current = theme
  const root = document.documentElement
  for (const [role, value] of Object.entries(theme.colors)) root.style.setProperty(`--ff-${role}`, value)
  const mode = theme.dark ? 'dark' : 'light'
  root.dataset.theme = mode
  root.style.colorScheme = mode
  appearance.raw = appearance.config = mode
  updateEditorTheme()
}

export async function initializeTheme() {
  if (!window.mygo) return
  window.mygo.on('toolbox-theme', applyTheme)
  const initial = await window.mygo.call('Toolbox.Theme')
  if (!current) applyTheme(initial)
}

export function openTools() { return window.mygo?.call('Toolbox.OpenTools').catch(console.error) }
