# Toolbox page

The four official plugins declare `contributes.toolbox.builtin` as `devtool`. FiveLeaf serves `devtool.zip` in an editor tab. `ToolPage.vue` displays DevTool’s components and feature tabs. **Switch Tool** and `Cmd+K` (`Ctrl+K` on Windows and Linux) open the command palette. The archive includes the icons, editor workers and dependency licenses. `source.json` records the upstream revision and selected features.

The page reads `Toolbox.Theme` on startup and subscribes to `toolbox-theme` for live updates. `theme.css` maps the host palette to DevTool controls, and `theme.js` applies the same palette and syntax colors to Monaco.

To refresh the bundle, install the dependencies in a local DevTool checkout using its lockfile, then run `node scripts/bundle-toolbox.mjs /path/to/DevTool` from the FiveLeaf root. Regular Go builds embed the checked-in archive directly.
