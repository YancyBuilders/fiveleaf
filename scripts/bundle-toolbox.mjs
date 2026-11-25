import { execFileSync } from 'node:child_process'
import { copyFile, mkdtemp, readFile, readdir, rm, utimes, writeFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const repo = fileURLToPath(new URL('../', import.meta.url))
const source = path.resolve(process.argv[2] ?? path.join(repo, '../DevTool'))
const requireSource = createRequire(path.join(source, 'package.json'))
const library = async name => import(pathToFileURL(requireSource.resolve(name)))
const factory = async name => {
  const module = await library(name)
  return module.default?.default ?? module.default
}
const original = file => path.join(source, 'src', file)
const toolbox = path.join(repo, 'cmd/fiveleaf/toolboxweb')
const features = { time: ['timestamp', 'timezone', 'calculator'], base64: ['encoder', 'decoder'], json: ['common', 'io', 'object'], url: ['encoder', 'decoder'] }
const categories = { conversion: ['time', 'json'], encoder_decoder: ['base64', 'url'] }
const routeCode = Object.entries(features).flatMap(([tool, entries]) => entries.map(feature => {
  const component = tool === 'json' ? 'json/Json' : `${tool}/${feature[0].toUpperCase()}${feature.slice(1)}`
  return `{tool:'${tool}',feature:'${feature}',component:()=>import('@/tools/${component}.vue')}`
})).join(',\n')

function replaceOnce(text, before, after) {
  if (!text.includes(before)) throw Error(`DevTool integration point has changed: ${before}`)
  return text.replace(before, after)
}

const integration = {
  name: 'fiveleaf-toolbox', enforce: 'pre',
  resolveId(id) {
    if (id === 'virtual:devtool-capsule.css') return '\0devtool-capsule.css'
  },
  transformIndexHtml(html) {
    const policy = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; worker-src 'self' blob:; connect-src 'self' blob: data:; object-src 'none'; base-uri 'none'; form-action 'none'"
    return html.replace('</head>', `<meta http-equiv="Content-Security-Policy" content="${policy}"></head>`)
  },
  async load(id) {
    if (id === original('store/setting.ts')) {
      const sourceText = await readFile(id, 'utf8')
      const themeStore = /export const useTheme = defineStore\('theme',[\s\S]*?(?=export default useSetting)/.exec(sourceText)
      if (!themeStore) throw Error('DevTool theme store missing')
      return sourceText.replace(themeStore[0], `export { useTheme } from 'fiveleaf-toolbox-theme';\n`)
    }
    if (id === original('components/editor/monaco.ts')) {
      return `import { bindEditorTheme } from 'fiveleaf-toolbox-theme';\n` + await readFile(id, 'utf8') + `\nbindEditorTheme(monaco.editor);\n`
    }
    if ([original('components/editor/Editor.vue'), original('components/editor/Diff.vue')].includes(id)) {
      return replaceOnce(await readFile(id, 'utf8'), 'storeTheme.theme.raw === "dark" ? "vs-dark" : "vs"', '"fiveleaf"')
    }
    if (id === '\0devtool-capsule.css') {
      const filename = original('block/layout/sidebar/Header.vue')
      const { parse, compileStyle } = requireSource('vue/compiler-sfc')
      const { descriptor } = parse(await readFile(filename, 'utf8'), { filename })
      const rules = descriptor.styles.flatMap(style => {
        const { errors, rawResult } = compileStyle({ source: style.content, filename, id: 'toolbox' })
        if (errors.length) throw errors[0]
        return rawResult.root.nodes
      })
        .filter(rule => rule.type === 'rule' && rule.selector.startsWith('.devtool-feature-tab'))
      if (!rules.some(rule => rule.selector === '.devtool-feature-tab-slider')) throw Error('DevTool feature-tab styles missing')
      return rules.map(rule => rule.toString()).join('\n')
    }
    if (id === original('config/config.ts')) return `
      const allowed = new Set(new URLSearchParams(location.search).get('tools')?.split(','));
      const features = ${JSON.stringify(features)};
      export const _common = Object.keys(features).filter(name => allowed.has(name));
      export const _tools = Object.fromEntries(_common.map(name => [name, { feature: features[name], parent_directory: '' }]));
      export const _categoryTool = Object.fromEntries(Object.entries(${JSON.stringify(categories)}).map(([key, tools]) => [key, tools.filter(name => allowed.has(name))]).filter(([, tools]) => tools.length));`
    if (id === original('buildDataTemp.ts')) {
      const sourceText = await readFile(id, 'utf8')
      const icons = sourceText.indexOf('export type iconType')
      if (icons < 0) throw Error('DevTool icon data missing')
      return `import { _tools } from '@/config/config';\nexport const toolRoutes = [${routeCode}].filter(route => route.tool in _tools);\nexport const toolKeywords = [];\n` + sourceText.slice(icons)
    }
    if (id === original('helper/stat.ts')) return 'export function report() {}'
    if (id === original('helper/plugin.ts')) {
      return replaceOnce(await readFile(id, 'utf8'), 'if (navigator.clipboard) {', `if (window.mygo) { window.mygo.call('Toolbox.Copy', text).then(successCallback, () => fallbackCopy(text, successCallback)); return; }\n    if (navigator.clipboard) {`)
    }
    if (id === original('main.ts')) return `
      import { createApp } from 'vue';
      import { RouterView } from 'vue-router';
      import { pinia } from '@/helper/pinia';
      import devtool from '@/helper/plugin';
      import router from '@/helper/router';
      import '@/assets/style.css';
      import 'fiveleaf-toolbox-style';
      import { initializeTheme } from 'fiveleaf-toolbox-theme';
      initializeTheme().catch(console.error).finally(() => createApp(RouterView).use(pinia).use(router).use(devtool).mount('#app'));`
  },
}

const stage = await mkdtemp(path.join(tmpdir(), 'fiveleaf-devtool-'))
const output = path.join(stage, 'site')
const zip = path.join(stage, 'toolbox.zip')
process.chdir(source)
try {
  const { build } = await library('vite')
  const polyfills = await library('vite-plugin-node-polyfills')
  await build({
    configFile: false, root: source, base: './', publicDir: false, logLevel: 'warn',
    plugins: [integration, (await factory('@tailwindcss/vite'))(),
      (polyfills.nodePolyfills ?? polyfills.default.nodePolyfills)({ globals: { Buffer: true, global: true, process: true } }),
      (await factory('@vitejs/plugin-vue'))(), (await factory('@vue-macros/reactivity-transform/vite'))(), (await factory('vite-svg-loader'))()],
    resolve: { dedupe: ['vue', 'vue-router'], alias: { 'fiveleaf-toolbox-theme': path.join(toolbox, 'theme.js'), 'fiveleaf-toolbox-style': path.join(toolbox, 'theme.css'), '@/helper/router': path.join(toolbox, 'router.js'), '@/constants': original('helper/constants.ts'), '@': original(''), 'devtool-config': original('config'),
      path: original('helper/empty.ts'), fs: original('helper/empty.ts'), 'yargs-parser': original('helper/empty.ts') } },
    build: { outDir: output, target: 'es2020', license: { fileName: 'licenses.txt' } },
  })
  await copyFile(path.join(source, 'public/favicon.svg'), path.join(output, 'favicon.svg'))
  const revision = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: source, encoding: 'utf8' }).trim()
  await writeFile(path.join(output, 'source.json'), JSON.stringify({ revision, features }, null, 2) + '\n')
  const files = (await readdir(output, { recursive: true, withFileTypes: true })).filter(entry => entry.isFile())
    .map(entry => path.relative(output, path.join(entry.parentPath, entry.name))).sort()
  for (const name of files) await utimes(path.join(output, name), 946684800, 946684800)
  execFileSync('zip', ['-q', '-9', '-X', zip, ...files], { cwd: output, env: { ...process.env, TZ: 'UTC' } })
  await copyFile(zip, path.join(repo, 'cmd/fiveleaf/toolboxweb/devtool.zip'))
  console.log(`Packaged ${files.length} files from DevTool ${revision}.`)
} finally {
  await rm(stage, { recursive: true, force: true })
}
