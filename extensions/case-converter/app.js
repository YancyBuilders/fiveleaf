const source = document.querySelector('#source')
const result = document.querySelector('#result')
const buttons = [...document.querySelectorAll('[data-case]')]
let mode = 'upper'

function titleCase(text) {
  return text.toLowerCase().replace(/(^|[\s.!?])(\p{L})/gu, (_, before, letter) => before + letter.toUpperCase())
}

function sentenceCase(text) {
  const lower = text.toLowerCase()
  return lower.replace(/(^|[.!?]\s+)(\p{L})/gu, (_, before, letter) => before + letter.toUpperCase())
}

function convert(text) {
  switch (mode) {
    case 'lower': return text.toLowerCase()
    case 'title': return titleCase(text)
    case 'sentence': return sentenceCase(text)
    default: return text.toUpperCase()
  }
}

function render() {
  result.textContent = convert(source.value)
}

for (const button of buttons) {
  button.addEventListener('click', () => {
    mode = button.dataset.case
    for (const other of buttons) other.setAttribute('aria-pressed', String(other === button))
    render()
  })
}
source.addEventListener('input', render)
document.querySelector('#copy').addEventListener('click', () => {
  window.mygo?.call('Toolbox.Copy', result.textContent).catch(console.error)
})

const theme = await window.mygo?.call('Toolbox.Theme').catch(() => null)
if (theme) {
  for (const [role, value] of Object.entries(theme.colors)) {
    document.documentElement.style.setProperty(`--ff-${role}`, value)
  }
  document.documentElement.dataset.theme = theme.dark ? 'dark' : 'light'
}
window.mygo?.call('Toolbox.SetDetail', true).catch(console.error)
render()
