package main

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"fiveleaf/internal/settings"
)

type splitDirection int

const (
	splitLeft splitDirection = iota
	splitRight
	splitUp
	splitDown
)

func (direction splitDirection) vertical() bool {
	return direction == splitUp || direction == splitDown
}
func (direction splitDirection) after() bool {
	return direction == splitRight || direction == splitDown
}

type paneTile struct {
	parent   *paneTile
	children []*paneTile
	leaf     *column
	vertical bool
	weight   float32
}

func (tile *paneTile) insert(index int, child *paneTile) {
	tile.children = slices.Insert(tile.children, index, child)
	child.parent = tile
}

func (tile *paneTile) outerLeaf(first bool) *column {
	for tile.leaf == nil {
		index := len(tile.children) - 1
		if first {
			index = 0
		}
		tile = tile.children[index]
	}
	return tile.leaf
}

func (tile *paneTile) lockedPane() bool {
	if tile.leaf != nil {
		return tile.leaf.locked
	}
	return slices.ContainsFunc(tile.children, (*paneTile).lockedPane)
}

func (a *app) detachPane(col *column) {
	if a.splitPicker.owner == col {
		a.splitPicker = splitPickerState{}
	}
	parent := col.parent
	index := slices.Index(parent.children, &col.paneTile)
	parent.children = slices.Delete(parent.children, index, index+1)
	col.parent = nil
	if len(parent.children) == 1 && parent.parent != nil {
		outer, remaining := parent.parent, parent.children[0]
		at := slices.Index(outer.children, parent)
		if remaining.leaf == nil && remaining.vertical == outer.vertical {
			total := remaining.totalWeight()
			for _, child := range remaining.children {
				child.parent, child.weight = outer, parent.weight*(child.weight/total)
			}
			outer.children = slices.Replace(outer.children, at, at+1, remaining.children...)
		} else {
			remaining.parent, remaining.weight = outer, parent.weight
			outer.children[at] = remaining
		}
		return
	}
	if parent.lockedPane() {
		for _, sibling := range parent.children {
			if sibling.leaf == a.fileCol && a.fileCol != nil && !a.fileCol.locked {
				sibling.weight += col.weight
				return
			}
		}
		for _, sibling := range parent.children {
			if !sibling.lockedPane() {
				sibling.weight += col.weight
				return
			}
		}
	}
}

func (a *app) flatPaneTree() {
	a.panes = paneTile{}
	for _, col := range a.cols {
		a.panes.insert(len(a.panes.children), &col.paneTile)
	}
}

func (a *app) undoPaneSplit(source, empty *column) {
	source.weight += empty.weight
	empty.weight = 0
	a.removeColumn(empty)
}

func (tile *paneTile) savedLayout() []settings.PaneTile {
	if !slices.ContainsFunc(tile.children, func(child *paneTile) bool { return child.leaf == nil }) {
		return nil
	}
	return tile.savedChildren()
}

func (tile *paneTile) savedChildren() []settings.PaneTile {
	saved := make([]settings.PaneTile, len(tile.children))
	for index, child := range tile.children {
		if child.leaf == nil {
			saved[index] = settings.PaneTile{Vertical: child.vertical, Weight: child.weight, Children: child.savedChildren()}
		}
	}
	return saved
}

func (a *app) restorePaneLayout(layout []settings.PaneTile, cols []*column) {
	if len(layout) == 0 || len(cols) == 0 {
		return
	}
	var next int
	var decode func([]settings.PaneTile) ([]*paneTile, bool)
	decode = func(saved []settings.PaneTile) ([]*paneTile, bool) {
		children := make([]*paneTile, 0, len(saved))
		for _, item := range saved {
			if len(item.Children) == 0 {
				if next == len(cols) {
					return nil, false
				}
				children = append(children, &cols[next].paneTile)
				next++
				continue
			}
			if len(item.Children) < 2 || item.Weight <= 0 {
				return nil, false
			}
			group, ok := decode(item.Children)
			if !ok {
				return nil, false
			}
			children = append(children, &paneTile{vertical: item.Vertical, weight: item.Weight, children: group})
		}
		return children, true
	}
	children, ok := decode(layout)
	if !ok || next != len(cols) {
		return
	}
	start := slices.Index(a.panes.children, &cols[0].paneTile)
	a.panes.children = append(a.panes.children[:start], children...)
	a.panes.linkChildren()
}

func (tile *paneTile) linkChildren() {
	for _, child := range tile.children {
		child.parent = tile
		child.linkChildren()
	}
}

func (tile *paneTile) balance() {
	var weight float32
	var count int
	for _, child := range tile.children {
		if !child.lockedPane() {
			weight += child.weight
			count++
		}
		child.balance()
	}
	if count > 1 {
		for _, child := range tile.children {
			if !child.lockedPane() {
				child.weight = weight / float32(count)
			}
		}
	}
}

func (group *paneTile) sliceBounds(region ui.Rect, start, end float32) ui.Rect {
	if group.vertical {
		region.Y, region.H = region.Y+region.H*start, region.H*(end-start)
	} else {
		region.X, region.W = region.X+region.W*start, region.W*(end-start)
	}
	return region
}

func (group *paneTile) totalWeight() float32 {
	var total float32
	for _, child := range group.children {
		total += child.weight
	}
	return total
}

func (tile *paneTile) minimumExtent(vertical bool) float32 {
	if tile.leaf != nil {
		if vertical {
			return tabStripH + crumbsH + 40
		}
		return minColumnW
	}
	var minimum float32
	total := tile.totalWeight()
	for _, child := range tile.children {
		need := child.minimumExtent(vertical)
		if tile.vertical == vertical {
			need *= total / child.weight
		}
		minimum = max(minimum, need)
	}
	return minimum
}

func (tile *paneTile) layoutBounds() ui.Rect {
	parent := tile.parent
	if parent == nil {
		return ui.Rect{W: 100, H: 100}
	}
	var start, total float32
	for _, sibling := range parent.children {
		if sibling == tile {
			start = total
		}
		total += sibling.weight
	}
	return parent.sliceBounds(parent.layoutBounds(), start/total, (start+tile.weight)/total)
}

func (a *app) horizontalColumn(col *column, step int, unlocked bool) *column {
	origin := col.layoutBounds()
	var best *column
	var distance, overlap float32
	for _, other := range a.cols {
		if other == col || unlocked && other.locked {
			continue
		}
		box := other.layoutBounds()
		shared := min(origin.Y+origin.H, box.Y+box.H) - max(origin.Y, box.Y)
		if shared <= 0 || (box.X > origin.X) != (step > 0) || box.X == origin.X {
			continue
		}
		gap := box.X - (origin.X + origin.W)
		if step < 0 {
			gap = origin.X - (box.X + box.W)
		}
		if best == nil || gap < distance || gap == distance && shared > overlap {
			best, distance, overlap = other, gap, shared
		}
	}
	return best
}

func (a *app) drawPaneGroup(c *ui.Context, group *paneTile, region, viewport ui.Rect) {
	children := slices.Clone(group.children)
	total := group.totalWeight()
	var sum float32
	for index, child := range children {
		start := sum / total
		sum += child.weight
		bounds := group.sliceBounds(region, start, sum/total)
		if col := child.leaf; col != nil {
			if col.parent == nil {
				continue
			}
			box := ui.Column(c.Key(col.id)).Absolute().LeftPercent(bounds.X).TopPercent(bounds.Y).
				WidthPercent(bounds.W).HeightPercent(bounds.H).Clip()
			box.BorderWidth(unit(bounds.Y > 0), 0, 0, unit(bounds.X > 0)).BorderColor(a.pal.border)
			box.Children(func() { a.columnView(c, col) })
		} else {
			a.drawPaneGroup(c, child, bounds, viewport)
		}
		if index == 0 {
			continue
		}
		prev := children[index-1]
		edge, size := edgeLeft, viewport.W*region.W/100
		if group.vertical {
			edge, size = edgeTop, viewport.H*region.H/100
		}
		handle := a.splitter(c.Key(struct{ Divider *paneTile }{child}), edge, func(delta float32) {
			if size <= 0 || child.parent != group || prev.parent != group {
				return
			}
			low := prev.minimumExtent(group.vertical)/size*total - prev.weight
			high := child.weight - child.minimumExtent(group.vertical)/size*total
			if low <= high {
				change := clamp(delta/size*total, low, high)
				prev.weight, child.weight = prev.weight+change, child.weight-change
				a.rememberTabs()
			}
		})
		handle.LeftPercent(bounds.X).TopPercent(bounds.Y)
		if group.vertical {
			handle.RightPercent(100 - bounds.X - bounds.W)
		} else {
			handle.BottomPercent(100 - bounds.Y - bounds.H)
		}
	}
}
