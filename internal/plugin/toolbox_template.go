package plugin

import (
	"encoding/json"
	"fmt"
)

func toolboxTemplateManifest(name, title, author, page string) string {
	manifest := Manifest{Name: name, DisplayName: title, Author: author, Version: "0.1.0", Category: "tools",
		Contributes: Contributes{Toolbox: &ToolboxContrib{ID: name + ".tool", Title: title, Page: page}}}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	return string(data) + "\n"
}

func scaffoldToolboxFramework(kind ScaffoldKind, name, title string) map[string]string {
	files := map[string]string{"style.css": `:root { font: 16px/1.5 system-ui, sans-serif; color-scheme: light dark; }
body { margin: 0; }
main { display: grid; gap: .75rem; max-width: 38rem; padding: 1.5rem; }
textarea { box-sizing: border-box; width: 100%; padding: .5rem; font: inherit; }
output { font-variant-numeric: tabular-nums; }
`}
	entry := "main.jsx"
	dependencies := map[string]string{"react": "^19.0.0", "react-dom": "^19.0.0"}
	build := map[string]string{"vite": "^7.0.0"}
	if kind == ScaffoldVueTool {
		entry = "main.js"
		dependencies = map[string]string{"vue": "^3.5.0"}
		build["@vitejs/plugin-vue"] = "^6.0.0"
		files["vite.config.js"] = `import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({ plugins: [vue()] })
`
		files[entry] = `import { createApp } from 'vue'
import App from './App.vue'
import './style.css'

createApp(App).mount('#app')
`
		files["App.vue"] = `<script setup>
import { ref } from 'vue'
const text = ref('')
</script>

<template>
  <main>
    <label for="text">Text</label>
    <textarea id="text" v-model="text" rows="5"></textarea>
    <output for="text">Characters: {{ Array.from(text).length }}</output>
  </main>
</template>
`
	} else {
		files[entry] = `import React, { useState } from 'react'
import { createRoot } from 'react-dom/client'
import './style.css'

function App() {
  const [text, setText] = useState('')
  return <main>
    <label htmlFor="text">Text</label>
    <textarea id="text" rows={5} value={text} onChange={event => setText(event.target.value)} />
    <output htmlFor="text">Characters: {Array.from(text).length}</output>
  </main>
}

createRoot(document.getElementById('app')).render(<App />)
`
	}
	files["index.html"] = fmt.Sprintf(`<!doctype html>
<html lang="en"><head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
</head><body>
  <div id="app"></div>
  <script type="module" src="./%s"></script>
</body></html>
`, title, entry)
	metadata := map[string]any{"name": name, "private": true, "type": "module", "scripts": map[string]string{"build": "vite build --base ./"}, "dependencies": dependencies, "devDependencies": build}
	data, _ := json.MarshalIndent(metadata, "", "  ")
	files["package.json"] = string(data) + "\n"
	return files
}
