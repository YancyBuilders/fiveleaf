package main

import (
	"sort"

	"github.com/egoist/mygo/ui"
)

type liveTableRows struct {
	heights  []float32
	tops     []float32
	handles  []ui.Handle
	mounted  []int
	measured []bool
	sampled  bool
}

const liveTableWindowRows = 40

func (a *app) liveTableRows(c *ui.Context, d *editor, view *liveView, block *liveBlock, y, width float32, font ui.Font, setup func(int, int, ui.Element) func()) {
	preview := view.v.markdown
	count := len(preview.blocks)
	rows := view.rows
	if rows == nil || len(rows.heights) != count {
		rows = &liveTableRows{
			heights: make([]float32, count), tops: make([]float32, count+1),
			handles: make([]ui.Handle, count), measured: make([]bool, count),
		}
		view.rows = rows
		lineHeight := font.Size*a.settings.LineHeight + font.Size*0.8 + 1
		for index := range rows.heights {
			bottom, outside := a.markdownBelow(preview, index)
			rows.heights[index] = lineHeight + bottom + outside
			rows.tops[index+1] = rows.tops[index] + rows.heights[index]
		}
	} else {
		changed := false
		var sample float32
		for _, index := range rows.mounted {
			height := rows.handles[index].Bounds(c).H
			_, outside := a.markdownBelow(preview, index)
			if height <= 0 {
				continue
			}
			rows.measured[index] = true
			if index != 0 && index != count-1 && (sample == 0 || height < sample) {
				sample = height
			}
			if !near(rows.heights[index], height+outside) {
				rows.heights[index] = height + outside
				changed = true
			}
		}
		if !rows.sampled && sample > 0 {
			rows.sampled = true
			for index := 1; index < count-1; index++ {
				if !rows.measured[index] {
					rows.heights[index] = sample
				}
			}
			changed = true
		}
		if changed {
			for index, height := range rows.heights {
				rows.tops[index+1] = rows.tops[index] + height
			}
			a.invalidate()
		}
	}
	rows.mounted = rows.mounted[:0]
	pad := font.Size * a.settings.LineHeight * 2
	first := sort.Search(count, func(index int) bool { return rows.tops[index+1] > -y-pad })
	last := sort.Search(count, func(index int) bool { return rows.tops[index] >= d.lastView.H-y+pad })
	active := -1
	if edit := d.tableCell; edit != nil && edit.block.key == block.key {
		active = edit.row
		if edit.focus && active >= 0 && active < count {
			top, bottom := y+rows.tops[active], y+rows.tops[active+1]
			if top < 0 || bottom > d.lastView.H {
				delta := max(0, bottom-d.lastView.H)
				if top < 0 {
					delta = top
				}
				d.vscroll.Y = clamp(d.vscroll.Y+delta, 0, d.vscroll.MaxY)
				a.invalidate()
			}
		}
	} else if requested := d.tableFocusAt - 1; d.tableFocusAt > 0 && requested >= block.start && requested <= block.end {
		snap := d.buf.Snapshot()
		active = min(max(0, snap.LineAt(requested)-snap.LineAt(block.start)-1), count-1)
	}
	previous := 0
	spacer := func(end int) {
		if end > previous {
			ui.Box(c).Height(rows.tops[end] - rows.tops[previous]).Shrink(0)
		}
	}
	show := func(index int) {
		spacer(index)
		a.markdownRow(c.Key(index), view.v, index, width, font, &rows.handles[index], setup)
		rows.mounted = append(rows.mounted, index)
		previous = index + 1
	}
	if active >= 0 && active < first {
		show(active)
	}
	for index := first; index < last; index++ {
		show(index)
	}
	if active >= last && active < count {
		show(active)
	}
	spacer(count)
}
