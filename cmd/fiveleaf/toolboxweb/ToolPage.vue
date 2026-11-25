<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getTool } from '@/config'
import Icon from '@/components/ui/Icon.vue'
import { refreshMainToolHeight } from '@/helper/event'
import 'virtual:devtool-capsule.css'
import { openTools } from './theme.js'

const route = useRoute()
const router = useRouter()
const tabs = ref(null)
const features = computed(() => getTool(route.meta.tool)?.features ?? [])
const active = computed(() => features.value.findIndex(item => item.name === route.meta.feature))
const searchKeys = /Mac|iPhone|iPad/.test(navigator.platform) ? '⌘K' : 'Ctrl+K'

function searchShortcut(event) {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k' && !event.isComposing) {
    event.preventDefault()
    openTools()
  }
}
function moveTab(event) {
  const count = features.value.length
  let index = active.value
  switch (event.key) {
    case 'ArrowLeft': index = (index + count - 1) % count; break
    case 'ArrowRight': index = (index + 1) % count; break
    case 'Home': index = 0; break
    case 'End': index = count - 1; break
    default: return
  }
  event.preventDefault()
  tabs.value.querySelectorAll('[role=tab]')[index].focus()
  router.replace(features.value[index].getRouter())
}
onMounted(() => {
  tabs.value?.querySelector('[aria-selected="true"]')?.focus({ preventScroll: true })
  window.addEventListener('keydown', searchShortcut)
})
onUnmounted(() => window.removeEventListener('keydown', searchShortcut))
watch(() => route.name, () => nextTick(refreshMainToolHeight), { immediate: true })
</script>

<template>
  <main class="tool-page" :data-tool="route.meta.tool">
    <nav class="tool-controls" aria-label="工具导航">
      <div v-if="features.length > 1" ref="tabs" class="devtool-feature-tabs" role="tablist" aria-label="工具功能"
           :style="{ '--feature-count': features.length, '--feature-idx': active }" @keydown="moveTab">
        <span class="devtool-feature-tab-slider is-ready" aria-hidden="true"></span>
        <button v-for="(item, index) in features" :id="`feature-${item.name}`" :key="item.name" type="button"
                role="tab" class="devtool-feature-tab" :class="{ 'is-active': active === index }"
                :tabindex="active === index ? 0 : -1" :aria-selected="active === index" aria-controls="tool-body"
                @click="router.replace(item.getRouter())">{{ $t(`tool_${route.meta.tool}_${item.name}`) }}</button>
      </div>
      <button type="button" class="tool-back" aria-label="切换工具" :title="`切换工具 (${searchKeys})`" @click="openTools">
        <Icon name="search" :size="14" aria-hidden="true" /><span>切换工具</span><kbd aria-hidden="true">{{ searchKeys }}</kbd>
      </button>
    </nav>
    <section :key="route.name" id="tool-body" class="devtool-main-tool" role="tabpanel" :aria-labelledby="`feature-${route.meta.feature}`">
      <RouterView v-slot="{ Component }">
        <Suspense :timeout="0" @resolve="refreshMainToolHeight">
          <component :is="Component" />
          <template #fallback><p class="tool-loading" role="status">正在打开工具…</p></template>
        </Suspense>
      </RouterView>
    </section>
  </main>
</template>

<style scoped>
.tool-page { height: 100dvh; display: grid; grid-template-rows: auto minmax(0, 1fr); background: var(--devtool-background-color); }
.tool-controls { display: flex; align-items: center; justify-content: space-between; gap: 8px; width: 100%; margin: 0 auto; padding: 14px 16px 0; box-sizing: border-box; }
.tool-page[data-tool=time] .tool-controls { max-width: 860px; padding-inline: 20px; }
.tool-page[data-tool=json] .tool-controls { max-width: 1440px; }
.tool-controls .devtool-feature-tabs { margin: 0; }
.tool-back { display: inline-flex; align-items: center; justify-content: center; flex: none; gap: 8px; width: auto; min-width: 32px; min-height: 32px; margin: 0 0 0 auto; padding: 0 7px; border: 0; border-radius: 7px; background: transparent; box-shadow: none; color: var(--devtool-color-primary); font: inherit; font-size: 12px; font-weight: 550; cursor: pointer; }
.tool-back kbd { padding: 3px 5px; border: 1px solid var(--devtool-border-color); border-radius: 5px; color: inherit; background: var(--devtool-background-color); font: 11px/1 system-ui, sans-serif; }
.tool-back:hover { background: var(--devtool-block-title-bg-color); color: var(--devtool-primary); }
.tool-controls button:focus-visible { outline: 2px solid var(--devtool-primary); outline-offset: 2px; }
.devtool-main-tool { min-width: 0; min-height: 0; overflow: auto; }
.tool-page[data-tool=time] .devtool-main-tool > :deep(div) {
  height: auto !important;
  overflow: visible !important;
}
.tool-loading { margin: 24px; color: var(--devtool-color-primary); }
@media (max-width: 720px) {
  .devtool-main-tool :deep(.time-calc-wrapper.time-calc-wrapper) { justify-content: flex-start !important; }
  .devtool-main-tool :deep(.calc-card-body) { height: auto; max-height: none; }
  .devtool-main-tool :deep(.converter-connector) { justify-self: center; }
  .devtool-main-tool :deep(.diff-arrow) { justify-self: center; width: 24px; }
}
@media (max-width: 520px) {
  .tool-controls { padding: 10px 12px 0; }
  .tool-controls .devtool-feature-tabs { grid-auto-columns: minmax(0, 1fr); min-width: 0; }
  .tool-controls .devtool-feature-tab { padding: 0 9px; }
  .tool-back kbd { display: none; }
  .devtool-main-tool :deep(.hero-time-input-wrap input) { font-size: clamp(1rem, 5vw, 1.5rem) !important; }
  .devtool-main-tool :deep(.diff-result-main), .devtool-main-tool :deep(.poor-unit-segment) { flex-wrap: wrap; }
  .devtool-main-tool :deep(.poor-unit-segment) { max-width: 100%; }
  .devtool-main-tool :deep(.op-row-between) { flex-wrap: wrap !important; }
  .devtool-main-tool :deep(.calc-tabs) { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); width: 100%; }
  .devtool-main-tool :deep(.calc-tab-btn) { gap: 4px; padding-inline: 4px; white-space: normal; }
}
@media (max-width: 400px) {
  .tool-back span { display: none; }
  :global(.devtool-extend-drawer:has(.tz-drawer-wrapper)) { width: 100%; min-width: 0; }
}
@media (prefers-reduced-motion: reduce) {
  .devtool-feature-tab, .devtool-feature-tab-slider.is-ready { transition: none; }
}
</style>
