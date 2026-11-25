package main

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fiveleaf/internal/bg"
	"fiveleaf/internal/git"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

type commitGraphQuery struct {
	repo              *git.Repo
	ref, head, branch string
	refsGen, limit    int
}

type commitGraph struct {
	root, ref string
	limit     int
	asked     *commitGraphQuery
	loading   bool
	err       error
	branches  []git.Branch
	commits   []git.Commit
	rows      []graphRow
	lanes     int
	more      bool
	list      ui.ListState
	selected  int
}

func (graph *commitGraph) title() string {
	return locales.CommitGraphTitle() + " · " + filepath.Base(graph.root)
}

func (ctx *gitContext) openCommitGraph() {
	if !ctx.live() {
		ctx.app.showSourceControl()
		return
	}
	graph := &commitGraph{root: ctx.git.repo.Root, limit: historyPage}
	ctx.app.showTool(&toolView{kind: toolCommitGraph, graph: graph})
}

func (graph *commitGraph) load(ctx *gitContext) {
	query := commitGraphQuery{repo: ctx.git.repo, ref: graph.ref, limit: graph.limit, refsGen: ctx.git.refsGen}
	if status := ctx.git.status; status != nil {
		query.head, query.branch = status.Head, status.Branch
	}
	if graph.asked != nil && *graph.asked == query {
		return
	}
	graph.asked, graph.loading, graph.err = &query, true, nil
	bg.Go(func() {
		defer recoverCrash()
		var branches []git.Branch
		var commits []git.Commit
		var err error
		readGit(func() {
			branches, err = query.repo.BranchList()
			if err != nil {
				return
			}
			if query.ref == "" {
				commits, err = query.repo.Log(query.limit+1, true)
			} else {
				commits, err = query.repo.LogRef(query.ref, query.limit+1)
			}
		})
		more := len(commits) > query.limit
		if more {
			commits = commits[:query.limit]
		}
		rows := layoutGraph(commits)
		lanes := graphLanes(rows)
		ctx.app.post(func() {
			if graph.asked != &query || !ctx.live() {
				return
			}
			graph.loading, graph.err, graph.branches = false, err, branches
			graph.commits, graph.rows, graph.lanes, graph.more = commits, rows, lanes, more
		})
	})
}

func (graph *commitGraph) selectRef(ref string) {
	if graph.ref == ref {
		return
	}
	graph.ref, graph.limit, graph.asked = ref, historyPage, nil
	graph.commits, graph.rows, graph.err, graph.more = nil, nil, nil, false
	graph.selected, graph.list = 0, ui.ListState{Columns: graph.list.Columns}
	graph.list.Handle.Focus()
}

func (graph *commitGraph) filterLabel() string {
	if graph.ref == "" {
		return locales.PrefTitleScmAllBranches()
	}
	if name, local := strings.CutPrefix(graph.ref, "refs/heads/"); local {
		return name
	}
	return strings.TrimPrefix(graph.ref, "refs/remotes/")
}

func (ctx *gitContext) pickGraphBranch(graph *commitGraph) {
	branches := graph.branches
	items := make([]pickItem, 1, len(branches)+1)
	items[0] = pickItem{Label: locales.PrefTitleScmAllBranches(), icon: iconGitGraph}
	if graph.ref == "" {
		items[0].icon = iconCheck
	}
	for _, branch := range branches {
		description := locales.BranchLocalGroup()
		if branch.Remote {
			description = locales.BranchRemoteGroup()
		}
		icon := iconBranch
		if panelRef(branch) == graph.ref {
			icon = iconCheck
		}
		items = append(items, pickItem{Label: branch.Name, Description: description, icon: icon})
	}
	ctx.app.openPluginPicker(items, locales.CommitGraphFilter(), func(index int) {
		if index < 0 || !ctx.live() {
			return
		}
		ref := ""
		if index > 0 {
			ref = panelRef(branches[index-1])
		}
		graph.selectRef(ref)
	})
}

func (a *app) commitGraphUI(c *ui.Context, graph *commitGraph) {
	ctx := a.repositoryForRoot(graph.root)
	if !a.online() {
		graph.asked = nil
		if a.link.dialing() {
			a.toolEmpty(c, "graph-connecting", iconCloud, locales.RemoteConnecting(), "", nil)
		} else {
			a.toolEmpty(c, "graph-offline", iconWifiOff, locales.RemoteNotConnected(), locales.RemoteReconnect(), a.reconnect)
		}
		return
	}
	if !ctx.live() {
		graph.asked = nil
		a.toolEmpty(c, "graph-no-repo", iconFolderGit, locales.ScmNoRepo(), "", nil)
		return
	}
	graph.load(ctx)
	p := a.pal
	a.toolBar(c, func() {
		ui.Icon(c, iconFolderGit).FontSize(15).TextColor(p.muted).Shrink(0)
		ui.Text(c, ctx.label()).FontSize(13).FontWeight(600).TextColor(p.text).SingleLine().Shrink(1).MinWidth(0).Tooltip(graph.root)
		ui.Spacer(c)
		filter := ui.ButtonBase(c.Key("branch-filter")).Height(26).MinWidth(0).MaxWidth(250).Shrink(1).Padding(0, 7).Gap(5).
			AlignItems(ui.Center).Radius(5).Background(p.hover).Label(locales.CommitGraphFilter()).Tooltip(locales.CommitGraphFilter()).
			Disabled(graph.loading && len(graph.branches) == 0)
		filter.Children(func() {
			ui.Icon(c, iconBranch).FontSize(13).TextColor(p.muted).Shrink(0)
			ui.Text(c, graph.filterLabel()).FontSize(12).TextColor(p.text).SingleLine().Shrink(1).MinWidth(0)
			ui.Icon(c, iconChevronDown).FontSize(11).TextColor(p.muted).Shrink(0)
		})
		if filter.Clicked() {
			ctx.pickGraphBranch(graph)
		}
		if a.iconButton(c.Key("refresh"), iconRefresh, locales.Refresh(), false).Disabled(graph.loading).Clicked() {
			graph.asked = nil
		}
	})
	switch {
	case graph.err != nil:
		a.toolEmpty(c, "graph-error", iconTriangleAlert, graph.err.Error(), locales.PluginsIndexRetry(), func() { graph.asked = nil })
	case graph.loading && len(graph.commits) == 0:
		a.toolEmpty(c, "graph-loading", iconGitGraph, locales.PanelLoadingCommits(), "", nil)
	case len(graph.commits) == 0:
		a.toolEmpty(c, "graph-empty", iconGitGraph, locales.PanelNoCommits(), "", nil)
	default:
		ctx.graphTable(c, graph)
	}
}

func (ctx *gitContext) graphTable(c *ui.Context, graph *commitGraph) {
	a := ctx.app
	p := a.pal
	pane := ui.Column(c.Key("history")).Grow(1).MinHeight(0).MinWidth(0)
	width := pane.Bounds().W
	columns := []ui.TableColumn{
		{ID: "graph", Title: locales.CommitGraphLanes(), Width: max(68, min(float32(graph.lanes*graphLaneW+24), width/4)), MinWidth: 48},
		{ID: "message", Title: locales.ScmCommitMessage(), MinWidth: 80},
	}
	if width >= 490 {
		columns = append(columns, ui.TableColumn{ID: "author", Title: locales.CommitGraphAuthor(), Width: 110})
	}
	if width >= 620 {
		columns = append(columns, ui.TableColumn{ID: "date", Title: locales.CommitGraphDate(), Width: 108})
	}
	if width >= 400 {
		columns = append(columns, ui.TableColumn{ID: "hash", Title: locales.CommitGraphHash(), Width: 84})
	}
	graph.list.Selected = &graph.selected
	commits := graph.commits
	graph.list.Key = func(index int) any { return commits[index].Hash }
	graph.list.Label = func(index int) string { return commits[index].Subject }
	graph.list.CellMenu = func(menu *ui.Menu, index, _ int) {
		if !ctx.live() || index < 0 || index >= len(commits) {
			return
		}
		commit := &commits[index]
		head := ctx.git.status != nil && ctx.git.status.Head == commit.Hash
		var action func()
		ctx.scmCommitMenu(menu, commit, head, &action)
		if action != nil {
			a.post(func() {
				if ctx.live() {
					action()
				}
			})
		}
	}
	now := time.Now()
	pane.Children(func() {
		theme := c.Theme()
		quiet := *theme
		quiet.Accent, quiet.AccentText, quiet.SurfaceHover = p.active, p.text, p.hover
		c.SetTheme(&quiet)
		table := ui.Table(c.Key("commits"), &graph.list, columns, len(graph.commits), func(index, column int) {
			commit, row := &graph.commits[index], &graph.rows[index]
			switch columns[column].ID {
			case "graph":
				head := ctx.git.status != nil && ctx.git.status.Head == commit.Hash
				ui.Box(c).Absolute().Top(0).Bottom(0).Start(10).End(4).Draw(func(painter *ui.Painter, box ui.Rect) {
					a.paintGraphRow(painter, box, row, commit, head)
				})
			case "message":
				a.graphSubject(c, commit, row.color)
			case "author":
				ui.Text(c, commit.Author).FontSize(12).TextColor(p.muted).SingleLine().Shrink(1)
			case "date":
				ui.Text(c, ago(commit.Time, now)).FontSize(12).TextColor(p.muted).FontFeatures("tnum").SingleLine()
			case "hash":
				ui.Text(c, commit.Short()).Font(a.settings.FontFamily).FontSize(11.5).TextColor(p.muted).SingleLine()
			}
		}).Grow(1).MinHeight(0)
		c.SetTheme(theme)
		if graph.more {
			button := ui.Button(c.Key("more"), locales.ScmShowMoreCommits()).AlignSelf(ui.Center).Shrink(0).Disabled(graph.loading)
			if button.Clicked() {
				graph.limit += historyPage
			}
		}
		if graph.selected < 0 || graph.selected >= len(graph.commits) {
			return
		}
		commit := graph.commits[graph.selected]
		open := func() { ctx.showCommit(commit, true) }
		if table.Submitted() {
			a.post(open)
		}
		ui.Column(c.Key("detail")).Shrink(0).Padding(8, 14).Gap(3).BorderWidth(1, 0, 0, 0).BorderColor(p.border).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, commit.Subject).FontSize(13).FontWeight(600).TextColor(p.text).SingleLine().Grow(1).MinWidth(0)
				if a.toolButton(c, "open", iconDiff, locales.CmdGitOpenChanges()).Clicked() {
					a.post(open)
				}
			})
			ui.Text(c, commit.Author+" · "+dateTime(commit.Time)+" · "+commit.Short()).FontSize(12).TextColor(p.muted).SingleLine().Tooltip(commit.Hash)
			if commit.Body != "" {
				ui.Text(c, commit.Body).FontSize(12).TextColor(p.text).MaxLines(2).Tooltip(commit.Body)
			}
		})
	})
}

func (a *app) graphSubject(c *ui.Context, commit *git.Commit, lane int) {
	row := ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(5).Clip().Tooltip(commit.Subject)
	width := row.Bounds().W
	row.Children(func() {
		badges := refBadges(commit.Refs)
		shown := min(len(badges), max(0, min(2, int(width/145))))
		for _, badge := range badges[:shown] {
			a.refBadge(c, badge, graphColor(a.pal, lane), min(112, width/4))
		}
		if shown > 0 && shown < len(badges) {
			ui.Text(c, "+"+strconv.Itoa(len(badges)-shown)).FontSize(10.5).TextColor(a.pal.muted).Shrink(0)
		}
		ui.Text(c, commit.Subject).FontSize(13).TextColor(a.pal.text).SingleLine().Shrink(1).MinWidth(0)
	})
}
