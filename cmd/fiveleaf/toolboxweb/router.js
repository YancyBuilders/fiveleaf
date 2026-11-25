import { createRouter, createWebHashHistory } from 'vue-router'
import { toolRoutes } from '@/buildDataTemp'
import { getTool, tools } from '@/config'
import useOperate from '@/store/operate'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: () => tools[0]?.firstFeature().getRouter() ?? '/unavailable' },
    { path: '/unavailable', component: { render: () => null } },
    {
      path: '/tool', component: () => import('./ToolPage.vue'),
      children: [
        ...tools.filter(tool => !tool.isSimple()).map(tool => ({ path: `/tool/${tool.name}`, redirect: tool.firstFeature().getRouter() })),
        ...toolRoutes.map(({ tool, feature, component }) => ({
          path: getTool(tool).getFeature(feature).getRouter(),
          name: `${tool}_${feature}`, component, meta: { type: 'tool', tool, feature },
        })),
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.beforeResolve(to => {
  if (to.meta.tool) {
    const tool = getTool(to.meta.tool)
    useOperate().access(tool.name, to.meta.feature, tool.firstCategory().name)
  }
})

export default router
