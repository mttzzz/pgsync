package screens

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/tui/ui"
)

var zoneOnce sync.Once

// HeaderOptions configures the common shell header.
type HeaderOptions struct {
	Title    string
	Config   config.Config
	Database string
	Width    int
	Height   int
	Running  bool
	Started  time.Time
	Now      time.Time
}

// ProgressSnapshot is a presentation-ready live sync state.
type ProgressSnapshot struct {
	Header             HeaderOptions
	Tab                int
	Stage              string
	CurrentTable       string
	CurrentDatabase    string
	DBIndex            int
	DBTotal            int
	StartedAt          time.Time
	CurrentStartedAt   time.Time
	Now                time.Time
	TablesDone         int
	TablesTotal        int
	RowsCopied         int64
	RowsEstimated      int64
	BytesCopied        int64
	BytesEstimated     int64
	BytesPerSec        float64
	TableRows          int64
	TableRowsEstimate  int64
	TableBytes         int64
	TableBytesEstimate int64
	TablePercent       float64
	OverallPercent     float64
	AnimatedPercent    float64
	Errors             int
	Events             []ProgressEventRow
}

// ProgressEventRow describes one row in the live event log.
type ProgressEventRow struct {
	Time    time.Time
	Level   string
	Event   string
	Table   string
	Details string
}

// ResultOptions configures the final report screen.
type ResultOptions struct {
	Header HeaderOptions
	Tab    int
	Tables []TableResultRow
}

// TableResultRow describes a table in the final report.
type TableResultRow struct {
	Table    string
	Rows     int64
	Bytes    int64
	Duration time.Duration
	Speed    float64
}

const defaultViewportWidth = 104

// MainMenu renders main actions.
func MainMenu(selected ...int) StaticScreen {
	items := []string{"Sync database", "Settings", "Quit"}
	index := 0
	if len(selected) > 0 {
		index = selected[0]
	}
	if index < 0 || index >= len(items) {
		index = 0
	}

	styles := ui.NewStyles()
	lines := make([]string, 0, 2+len(items))
	lines = append(lines, styles.PanelTitle.Render("Главное меню"), "")
	for i, item := range items {
		prefix := "  "
		row := item
		if i == index {
			prefix = styles.Primary.Render("▸ ")
			row = styles.SelectedRow.Render(item)
		}
		lines = append(lines, prefix+row)
	}

	viewport, contentWidth := layoutWidths(defaultViewportWidth)
	body := page(viewport, renderHeader(HeaderOptions{Title: "PGSync Control Center", Width: contentWidth}), panel("Main Menu", strings.Join(lines, "\n"), contentWidth), footer("↑/↓ выбрать · enter открыть · s настройки · q выход"))
	return StaticScreen{ScreenID: MainMenuID, Heading: "Главное меню", Body: body, Hint: "↑/↓ выбрать · enter открыть · s настройки · q выход"}
}

// DatabaseListOptions configures the database queue builder screen.
type DatabaseListOptions struct {
	SelectedIndex int
	Checked       map[string]bool
	Width         int
	Height        int
	Status        string
	Config        config.Config
}

// DatabaseList renders databases.
func DatabaseList(dbs []models.Database, err error, options ...DatabaseListOptions) StaticScreen {
	opts := DatabaseListOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	body := renderDatabaseQueueBuilder(dbs, err, opts)
	help := "↑/↓ move   Space select DB   A select all   C clear   R reload   Enter/Y continue   S settings"
	return StaticScreen{ScreenID: DatabaseListID, Heading: "Database Queue Builder", Body: body, Hint: help}
}

//nolint:gocyclo,gocognit // Renderer branches directly by UI state for clear terminal output.
func renderDatabaseQueueBuilder(dbs []models.Database, err error, opts DatabaseListOptions) string {
	styles := ui.NewStyles()
	viewport, bodyWidth := layoutWidths(opts.Width)
	height := viewportHeight(opts.Height)
	header := renderHeader(HeaderOptions{Title: "PGSync Control Center", Config: opts.Config, Database: opts.Config.Runtime.DefaultDatabase, Width: bodyWidth})

	lines := []string{styles.PanelTitle.Render("Database Queue Builder"), ""}
	switch {
	case err != nil:
		lines = append(lines, styles.Danger.Render("Error: "+RedactText(err.Error())))
	case len(dbs) == 0:
		status := opts.Status
		if status == "" {
			status = "Loading remote databases..."
		}
		spin := spinner.New(spinner.WithSpinner(spinner.Dot))
		lines = append(lines, styles.Warning.Render(spin.View()+" "+status))
	default:
		visible := clampInt(height-16, 3, 20)
		lines = append(lines, renderDatabaseTable(dbs, opts, bodyWidth, visible))
	}
	if opts.Status != "" && len(dbs) > 0 {
		lines = append(lines, "", styles.Muted.Render(opts.Status))
	}

	content := panel("Databases", strings.Join(lines, "\n"), bodyWidth)
	return page(viewport, header, content, footer(actionsLine([]actionLabel{{"Space", "select"}, {"A", "all"}, {"C", "clear"}, {"R", "reload"}, {"Enter/Y", "continue"}}, bodyWidth), bodyWidth))
}

func renderDatabaseTable(dbs []models.Database, opts DatabaseListOptions, width int, visible int) string {
	inner := innerBoxWidth(width)
	contentWidth := maxInt(inner-2, 1)
	cursor := clampIndexForTable(opts.SelectedIndex, len(dbs))
	start, end := visibleRange(cursor, len(dbs), visible)
	if contentWidth < 48 {
		nameWidth := maxInt(contentWidth-5, 8)
		lines := []string{renderListHeader([]listColumn{{Width: 1}, {Title: "Database", Width: nameWidth}})}
		for index := start; index < end; index++ {
			db := dbs[index]
			lines = append(lines, renderDatabaseRowNarrow(db, index == cursor, opts.Checked != nil && opts.Checked[db.Name], nameWidth, false))
		}
		return strings.Join(lines, "\n") + "\n" + databaseRangeFooter(dbs, opts.Checked, start, end, width)
	}
	if contentWidth < 68 {
		nameWidth := maxInt(contentWidth-20, 12)
		lines := []string{renderListHeader([]listColumn{{Width: 1}, {Title: "Database", Width: nameWidth}, {Title: "DB size", Width: 12, AlignRight: true}})}
		for index := start; index < end; index++ {
			db := dbs[index]
			lines = append(lines, renderDatabaseRowNarrow(db, index == cursor, opts.Checked != nil && opts.Checked[db.Name], nameWidth, true))
		}
		return strings.Join(lines, "\n") + "\n" + databaseRangeFooter(dbs, opts.Checked, start, end, width)
	}
	nameWidth := clampInt(contentWidth-45, 18, 72)
	ownerWidth := clampInt(contentWidth-nameWidth-39, 8, 28)

	lines := []string{
		renderListHeader([]listColumn{{Width: 1}, {Title: "Database", Width: nameWidth}, {Title: "DB size", Width: 12, AlignRight: true}, {Title: "Tables", Width: 8, AlignRight: true}, {Title: "Owner", Width: ownerWidth}}),
	}
	for index := start; index < end; index++ {
		db := dbs[index]
		lines = append(lines, renderDatabaseRow(db, index == cursor, opts.Checked != nil && opts.Checked[db.Name], nameWidth, ownerWidth))
	}
	return strings.Join(lines, "\n") + "\n" + databaseRangeFooter(dbs, opts.Checked, start, end, width)
}

type listColumn struct {
	Title      string
	Width      int
	AlignRight bool
}

func renderListHeader(columns []listColumn) string {
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		if column.Title == "" {
			continue
		}
		parts = append(parts, renderCell(column.Title, column.Width, ui.NewStyles().Muted.Bold(true), column.AlignRight))
	}
	return strings.Repeat(" ", 5) + strings.Join(parts, "  ")
}

func renderDatabaseRow(db models.Database, active bool, checked bool, nameWidth int, ownerWidth int) string {
	styles := ui.NewStyles()
	nameStyle := styles.Row
	if active {
		nameStyle = styles.Primary
	}
	return renderSelectionPrefix(active, checked) +
		renderCell(db.Name, nameWidth, nameStyle, false) + "  " +
		renderCell(ui.FormatBytes(db.SizeBytes), 12, styles.Accent, true) + "  " +
		renderCell(ui.FormatCount(db.TableCount), 8, styles.Muted, true) + "  " +
		renderCell(dashFallback(db.Owner), ownerWidth, styles.Muted, false)
}

func renderDatabaseRowNarrow(db models.Database, active bool, checked bool, nameWidth int, showSize bool) string {
	styles := ui.NewStyles()
	nameStyle := styles.Row
	if active {
		nameStyle = styles.Primary
	}
	row := renderSelectionPrefix(active, checked) + renderCell(db.Name, nameWidth, nameStyle, false)
	if showSize {
		row += "  " + renderCell(ui.FormatBytes(db.SizeBytes), 12, styles.Accent, true)
	}
	return row
}

func renderSelectionPrefix(active bool, checked bool) string {
	styles := ui.NewStyles()
	cursor := " "
	if active {
		cursor = styles.Primary.Render("▸")
	}
	mark := " "
	if checked {
		mark = styles.Success.Render("✓")
	}
	return cursor + " " + mark + "  "
}

func renderCell(value string, width int, style lipgloss.Style, alignRight bool) string {
	value = truncate(value, width)
	padding := maxInt(width-lipgloss.Width(value), 0)
	if alignRight {
		value = strings.Repeat(" ", padding) + value
	} else {
		value += strings.Repeat(" ", padding)
	}
	return style.Render(value)
}

func rangeFooter(start int, end int, total int) string {
	styles := ui.NewStyles()
	if total <= 0 {
		return styles.Muted.Render("Showing 0 of 0")
	}
	return styles.Muted.Render(fmt.Sprintf("Showing %s-%s of %s", ui.FormatCount(start+1), ui.FormatCount(end), ui.FormatCount(total)))
}

func databaseRangeFooter(dbs []models.Database, checked map[string]bool, start int, end int, widths ...int) string {
	styles := ui.NewStyles()
	selected, bytes, tables := selectedDatabaseStats(dbs, checked)
	if len(dbs) == 0 {
		return styles.Muted.Render("Showing 0 of 0")
	}
	showing := styles.Muted.Render(fmt.Sprintf("Showing %s-%s of %s", ui.FormatCount(start+1), ui.FormatCount(end), ui.FormatCount(len(dbs))))
	width := 0
	if len(widths) > 0 {
		width = widths[0]
	}
	if width > 0 && width < 72 {
		return strings.Join([]string{
			showing,
			ui.Metric("Selected", ui.FormatCount(selected), styles.Success) + "   " + ui.Metric("Tables", ui.FormatCount(tables), styles.Accent),
			ui.Metric("DB size", ui.FormatBytes(bytes), styles.Accent),
		}, "\n")
	}
	return strings.Join([]string{
		showing,
		ui.Metric("Selected", ui.FormatCount(selected), styles.Success),
		ui.Metric("DB size", ui.FormatBytes(bytes), styles.Accent),
		ui.Metric("Tables", ui.FormatCount(tables), styles.Accent),
	}, "   ")
}

func selectedDatabaseStats(dbs []models.Database, checked map[string]bool) (int, int64, int) {
	if len(checked) == 0 {
		return 0, 0, 0
	}
	selected := 0
	var bytes int64
	tables := 0
	for _, db := range dbs {
		if checked[db.Name] {
			selected++
			bytes += db.SizeBytes
			tables += db.TableCount
		}
	}
	return selected, bytes, tables
}

func clampIndexForTable(index int, length int) int {
	if length <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	return index
}

func visibleRange(cursor, total, visible int) (int, int) {
	if total <= visible {
		return 0, total
	}
	half := visible / 2
	start := cursor - half
	if start < 0 {
		start = 0
	}
	if start+visible > total {
		start = total - visible
	}
	return start, start + visible
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// PlanReviewOptions configures the multi-database plan review screen.
type PlanReviewOptions struct {
	Header           HeaderOptions
	Databases        []models.Database
	Engine           string
	Threads          int
	UseSystemPgtools bool
}

func renderConfirmPlan(opts PlanReviewOptions) string {
	styles := ui.NewStyles()
	header := opts.Header
	viewport, bodyWidth := layoutWidths(header.Width)
	header.Title = "Plan Review"
	header.Width = bodyWidth
	if header.Database == "" && len(opts.Databases) > 0 {
		header.Database = opts.Databases[0].Name
	}
	var body string
	if len(opts.Databases) == 0 {
		body = styles.Muted.Render("No sync targets selected.")
	} else {
		var totalBytes int64
		var totalTables int
		for _, db := range opts.Databases {
			totalBytes += db.SizeBytes
			totalTables += db.TableCount
		}
		summary := dotJoin(
			ui.Metric("databases", ui.FormatCount(len(opts.Databases)), styles.Success),
			ui.Metric("tables", ui.FormatCount(totalTables), styles.Accent),
			ui.Metric("disk", ui.FormatBytes(totalBytes), styles.Accent),
		)
		tech := dotJoin(
			ui.Metric("engine", opts.Engine, styles.Primary),
			ui.Metric("copy", ui.CopyTechnology(opts.Engine, opts.UseSystemPgtools), styles.Success),
			ui.Metric("workers", ui.FormatCount(opts.Threads), styles.Primary),
		)
		queueLines := renderPlanQueue(opts.Databases, bodyWidth)
		buttons := lipgloss.JoinHorizontal(lipgloss.Top,
			markZone(ActionZone(ActionCancel), button("Cancel", false)),
			"   ",
			markZone(ActionZone(ActionStart), button("Start Sync", true)),
		)
		body = strings.Join([]string{summary, tech, "", queueLines, "", buttons}, "\n")
	}
	pipeline := strings.Join([]string{
		"1 connect remote",
		"2 snapshot pre-data",
		"3 drop/recreate local",
		"4 COPY table data",
		"5 restore post-data",
		"6 reset sequences",
	}, "\n")
	content := renderPlanContent(body, pipeline, bodyWidth)
	return page(viewport, renderHeader(header), content, footer(actionsLine([]actionLabel{{"Enter/Y", "start"}, {"Esc", "back"}}, bodyWidth), bodyWidth))
}

func renderPlanQueue(dbs []models.Database, width int) string {
	styles := ui.NewStyles()
	if len(dbs) == 0 {
		return ""
	}
	maxRows := clampInt(len(dbs), 1, 8)
	lines := make([]string, 0, maxRows+1)
	for index := 0; index < maxRows; index++ {
		db := dbs[index]
		nameWidth := maxInt(width-32, 12)
		row := styles.Success.Render(" ✓ ") + renderCell(db.Name, nameWidth, styles.Row, false) + "  " +
			renderCell(ui.FormatBytes(db.SizeBytes), 12, styles.Accent, true) + "  " +
			renderCell(ui.FormatCount(db.TableCount)+" tbl", 8, styles.Muted, true)
		lines = append(lines, row)
	}
	if len(dbs) > maxRows {
		lines = append(lines, styles.Muted.Render(fmt.Sprintf("  …и ещё %s", ui.FormatCount(len(dbs)-maxRows))))
	}
	return strings.Join(lines, "\n")
}

func renderPlanContent(summary string, pipeline string, width int) string {
	if width < 92 {
		return strings.Join([]string{panel("Plan", summary, width), panel("Pipeline", pipeline, width)}, "\n")
	}
	leftWidth := width / 2
	rightWidth := maxInt(width-leftWidth-1, 1)
	return lipgloss.JoinHorizontal(lipgloss.Top, panel("Plan", summary, leftWidth), " ", panel("Pipeline", pipeline, rightWidth))
}

func renderProgress(snapshot ProgressSnapshot) string {
	viewport, bodyWidth := layoutWidths(snapshot.Header.Width)
	snapshot.Header.Title = "Live Sync"
	snapshot.Header.Width = bodyWidth
	snapshot.Header.Running = true
	if snapshot.Now.IsZero() {
		snapshot.Now = time.Now()
	}
	if snapshot.Header.Now.IsZero() {
		snapshot.Header.Now = snapshot.Now
	}

	tabs := tabBar([]string{"Overview", "Events"}, snapshot.Tab)
	var content string
	switch normalizedTab(snapshot.Tab, 2) {
	case 1:
		content = panel("", renderEvents(snapshot.Events, bodyWidth), bodyWidth)
	default:
		content = panel("", renderProgressOverview(snapshot, bodyWidth), bodyWidth)
	}
	return page(viewport, renderHeader(snapshot.Header), tabs, content, footer(actionsLine([]actionLabel{{"Tab", "switch"}, {"P", "pause"}, {"Q", "cancel"}}, bodyWidth), bodyWidth))
}

func renderEvents(events []ProgressEventRow, width int) string {
	styles := ui.NewStyles()
	if len(events) == 0 {
		return styles.Muted.Render("Waiting for engine events…")
	}
	limit := minInt(len(events), 6)
	if width < 80 {
		lines := []string{styles.Muted.Render(fmt.Sprintf("%-8s  %-18s  %s", "Time", "Event", "Details")), styles.Muted.Render(strings.Repeat("─", maxInt(width-10, 16)))}
		for i := 0; i < limit; i++ {
			event := events[i]
			stamp := "--:--"
			if !event.Time.IsZero() {
				stamp = event.Time.Format("15:04")
			}
			lines = append(lines, fmt.Sprintf("%-8s  %-18s  %s", stamp, truncate(event.Event, 18), truncate(event.Details, maxInt(width-34, 8))))
		}
		return strings.Join(lines, "\n")
	}
	lines := []string{styles.Muted.Render(fmt.Sprintf("%-9s  %-5s  %-24s  %-28s  %s", "Time", "Level", "Event", "Table", "Details")), styles.Muted.Render(strings.Repeat("─", maxInt(width-10, 40)))}
	for i := 0; i < limit; i++ {
		event := events[i]
		stamp := "--:--:--"
		if !event.Time.IsZero() {
			stamp = event.Time.Format("15:04:05")
		}
		lines = append(lines, fmt.Sprintf("%-9s  %-5s  %-24s  %-28s  %s", stamp, truncate(event.Level, 5), truncate(event.Event, 24), truncate(emptyFallback(event.Table, "-"), 28), truncate(event.Details, maxInt(width-80, 20))))
	}
	return strings.Join(lines, "\n")
}

func renderResult(result *models.SyncResult, opts ResultOptions) string {
	viewport, bodyWidth := layoutWidths(opts.Header.Width)
	opts.Header.Title = "Sync Report"
	opts.Header.Width = bodyWidth
	tabs := tabBar([]string{"Summary", "Stages", "Tables"}, opts.Tab)
	var content string
	switch normalizedTab(opts.Tab, 3) {
	case 1:
		content = panel("", renderStageTimings(result, bodyWidth), bodyWidth)
	case 2:
		content = panel("", renderTableResults(opts.Tables, bodyWidth), bodyWidth)
	default:
		content = panel("", renderResultSummary(result), bodyWidth)
	}
	return page(viewport, renderHeader(opts.Header), tabs, content, footer(actionsLine([]actionLabel{{"Tab", "switch"}, {"Enter/Q", "quit"}, {"B", "back"}, {"R", "again"}}, bodyWidth), bodyWidth))
}

/* renderProgressOverview emits a compact, vertically dense status block:
 *   1. overall progress bar with percent and stage
 *   2. table progress (done/total · errors · elapsed · ETA)
 *   3. byte/row totals across all tables
 *   4. current table name as a divider
 *   5. per-table progress bar with percent
 *   6. per-table byte/row totals
 * Wider terminals (>=78 cols) keep all rows; narrower terminals split the
 * combined rows in two so nothing wraps. Total height: 6 lines wide / 9 lines
 * narrow, vs ~18 lines in the previous layout. */
func renderProgressOverview(snapshot ProgressSnapshot, bodyWidth int) string {
	styles := ui.NewStyles()
	elapsed := time.Duration(0)
	if !snapshot.StartedAt.IsZero() {
		elapsed = snapshot.Now.Sub(snapshot.StartedAt)
	}
	stage := emptyFallback(snapshot.Stage, "waiting")
	current := emptyFallback(snapshot.CurrentTable, "—")
	overall := snapshot.AnimatedPercent
	if overall == 0 {
		overall = snapshot.OverallPercent
	}
	eta := estimateETA(snapshot.BytesCopied, snapshot.BytesEstimated, snapshot.BytesPerSec)
	rowsRate := 0.0
	if elapsed > 0 {
		rowsRate = float64(snapshot.RowsCopied) / elapsed.Seconds()
	}
	tableElapsed := time.Duration(0)
	if !snapshot.CurrentStartedAt.IsZero() {
		tableElapsed = snapshot.Now.Sub(snapshot.CurrentStartedAt)
	}

	barWidth := maxInt(bodyWidth-14, 8)
	tablesText := fmt.Sprintf("%s/%s", ui.FormatCount(snapshot.TablesDone), ui.FormatCount(snapshot.TablesTotal))
	rowsText := fmt.Sprintf("%s/%s", ui.FormatInt(snapshot.RowsCopied), ui.FormatInt(snapshot.RowsEstimated))
	tableRowsText := fmt.Sprintf("%s/%s", ui.FormatInt(snapshot.TableRows), ui.FormatInt(snapshot.TableRowsEstimate))

	statusParts := []string{styles.Primary.Render(stage)}
	if snapshot.DBTotal > 1 {
		dbBadge := ui.Metric("db", fmt.Sprintf("%s/%s", ui.FormatCount(snapshot.DBIndex), ui.FormatCount(snapshot.DBTotal)), styles.Warning)
		statusParts = append(statusParts, dbBadge)
	}
	statusParts = append(statusParts,
		ui.Metric("tables", tablesText, styles.Success),
		ui.Metric("err", ui.FormatCount(snapshot.Errors), styles.Danger),
		ui.Metric("elapsed", ui.FormatDurationTenths(elapsed), styles.Primary),
		ui.Metric("ETA", eta, styles.Warning),
	)
	overallStatus := dotJoin(statusParts...)
	overallBytes := dotJoin(
		ui.Metric("COPY", fmt.Sprintf("%s / %s", ui.FormatBytes(snapshot.BytesCopied), ui.FormatBytes(snapshot.BytesEstimated)), styles.Accent),
		ui.Metric("speed", ui.FormatBytesRate(snapshot.BytesPerSec), styles.Success),
		ui.Metric("rows", rowsText, styles.Accent),
	)

	tableLine := dotJoin(
		styles.Accent.Render(truncate(current, maxInt(bodyWidth/2, 24))),
		ui.Metric("elapsed", ui.FormatDurationTenths(tableElapsed), styles.Primary),
		ui.Metric("rows/s", ui.FormatRowsRate(rowsRate), styles.Success),
	)
	tableBytes := dotJoin(
		ui.Metric("COPY", fmt.Sprintf("%s / %s", ui.FormatBytes(snapshot.TableBytes), ui.FormatBytes(snapshot.TableBytesEstimate)), styles.Accent),
		ui.Metric("rows", tableRowsText, styles.Accent),
	)

	overallBar := ui.ProgressBar(barWidth, overall) + "  " + styles.Accent.Render(ui.FormatPercent(snapshot.OverallPercent))
	tableBar := ui.ProgressBar(barWidth, snapshot.TablePercent) + "  " + styles.Accent.Render(ui.FormatPercent(snapshot.TablePercent))

	if bodyWidth < 78 {
		return strings.Join([]string{
			overallBar,
			styles.Primary.Render(stage) + styles.Muted.Render(" · ") + ui.Metric("tables", tablesText, styles.Success) + styles.Muted.Render(" · ") + ui.Metric("err", ui.FormatCount(snapshot.Errors), styles.Danger),
			ui.Metric("elapsed", ui.FormatDurationTenths(elapsed), styles.Primary) + styles.Muted.Render(" · ") + ui.Metric("ETA", eta, styles.Warning) + styles.Muted.Render(" · ") + ui.Metric("speed", ui.FormatBytesRate(snapshot.BytesPerSec), styles.Success),
			ui.Metric("COPY", fmt.Sprintf("%s / %s", ui.FormatBytes(snapshot.BytesCopied), ui.FormatBytes(snapshot.BytesEstimated)), styles.Accent),
			ui.Metric("rows", rowsText, styles.Accent),
			styles.Muted.Render(strings.Repeat("─", barWidth+8)),
			styles.Accent.Render(truncate(current, maxInt(bodyWidth-2, 16))),
			tableBar,
			tableBytes,
		}, "\n")
	}
	return strings.Join([]string{
		overallBar,
		overallStatus,
		overallBytes,
		styles.Muted.Render(strings.Repeat("─", barWidth+8)),
		tableBar,
		tableLine,
		tableBytes,
	}, "\n")
}

/* dotJoin joins non-empty parts with " · " separators, themed muted. */
func dotJoin(parts ...string) string {
	separator := ui.NewStyles().Muted.Render(" · ")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			clean = append(clean, part)
		}
	}
	return strings.Join(clean, separator)
}

func renderResultSummary(result *models.SyncResult) string {
	styles := ui.NewStyles()
	if result == nil {
		return styles.Muted.Render("No sync result yet.")
	}
	status := styles.Success.Render("SUCCESS")
	if result.Err != nil {
		status = styles.Danger.Render("FAILED")
	}
	duration := result.Duration()
	avgSpeed := 0.0
	if duration > 0 {
		avgSpeed = float64(result.BytesCopied) / duration.Seconds()
	}
	lines := []string{
		dotJoin(
			status,
			styles.Primary.Render(result.Database),
			ui.Metric("duration", ui.FormatDurationTenths(duration), styles.Primary),
		),
		dotJoin(
			ui.Metric("tables", ui.FormatCount(result.TablesCopied), styles.Success),
			ui.Metric("rows", ui.FormatInt(result.RowsCopied), styles.Accent),
			ui.Metric("COPY", ui.FormatBytes(result.BytesCopied), styles.Accent),
			ui.Metric("avg", ui.FormatBytesRate(avgSpeed), styles.Success),
		),
	}
	if result.Err != nil {
		lines = append(lines, styles.Danger.Render("Error: "+RedactText(result.Err.Error())))
	}
	return strings.Join(lines, "\n")
}

func renderStageTimings(result *models.SyncResult, width int) string {
	styles := ui.NewStyles()
	if result == nil || len(result.Stages) == 0 {
		return styles.Muted.Render("No stage timing details recorded yet.")
	}
	lines := []string{styles.Muted.Render(fmt.Sprintf("%-28s  %12s  %s", "Stage", "Duration", "Notes")), styles.Muted.Render(strings.Repeat("─", maxInt(width-10, 40)))}
	for stage, duration := range result.Stages {
		lines = append(lines, fmt.Sprintf("%-28s  %12s  completed", truncate(stage, 28), ui.FormatDurationTenths(duration)))
	}
	return strings.Join(lines, "\n")
}

func renderTableResults(rows []TableResultRow, width int) string {
	styles := ui.NewStyles()
	if len(rows) == 0 {
		return styles.Muted.Render("Per-table report will appear after table metrics are collected.")
	}
	if width < 80 {
		lines := []string{styles.Muted.Render(fmt.Sprintf("%-24s  %10s  %s", "Table", "Rows", "COPY")), styles.Muted.Render(strings.Repeat("─", maxInt(width-10, 24)))}
		for _, row := range rows {
			lines = append(lines, fmt.Sprintf("%-24s  %10s  %s", truncate(row.Table, 24), ui.FormatInt(row.Rows), ui.FormatBytes(row.Bytes)))
		}
		return strings.Join(lines, "\n")
	}
	lines := []string{styles.Muted.Render(fmt.Sprintf("%-32s  %14s  %12s  %10s  %s", "Table", "Rows", "COPY stream", "Duration", "Avg speed")), styles.Muted.Render(strings.Repeat("─", maxInt(width-10, 40)))}
	for _, row := range rows {
		lines = append(lines, fmt.Sprintf("%-32s  %14s  %12s  %10s  %s", truncate(row.Table, 32), ui.FormatInt(row.Rows), ui.FormatBytes(row.Bytes), ui.FormatDurationTenths(row.Duration), ui.FormatBytesRate(row.Speed)))
	}
	return strings.Join(lines, "\n")
}

func renderHeader(opts HeaderOptions) string {
	styles := ui.NewStyles()
	width := fittedWidth(opts.Width)
	styleWidth := innerBoxWidth(width)
	textWidth := panelContentWidth(width)
	title := opts.Title
	if title == "" {
		title = "PGSync"
	}
	modeStyle := styles.Success
	if ui.ConnectionMode(opts.Config.Remote) == "PROXY" {
		modeStyle = styles.Warning
	}
	modeBadge := badge(ui.ConnectionMode(opts.Config.Remote), modeStyle)
	techBadge := badge(headerCopyTechnology(opts.Config.Runtime.Engine, opts.Config.Runtime.UseSystemPgtools), styles.Accent)
	lines := []string{ui.HeaderLine(styles.HeaderTitle.Render(headerTitle(title)), modeBadge+"  "+techBadge, textWidth)}
	if ui.ConnectionMode(opts.Config.Remote) == "PROXY" {
		lines = append(lines, headerProxyBlock(opts.Config.Remote, textWidth))
	}
	lines = append(lines, headerEndpointInline("REMOTE", opts.Config.Remote, nil, textWidth))
	localExtras := []headerField{{Label: "workers", Value: ui.FormatCount(opts.Config.Runtime.Threads), Style: styles.Primary}}
	if elapsed := elapsedHeaderValue(opts); elapsed != "" {
		localExtras = append(localExtras, headerField{Label: "elapsed", Value: elapsed, Style: styles.Primary})
	}
	lines = append(lines, headerEndpointInline("LOCAL", opts.Config.Local, localExtras, textWidth))
	return styles.Header.Width(styleWidth).Render(strings.Join(lines, "\n"))
}

func headerTitle(title string) string {
	if strings.Contains(strings.ToLower(title), "pgsync") {
		return title
	}
	return "PGSync • " + title
}

type headerField struct {
	Label string
	Value string
	Style lipgloss.Style
}

func headerProxyBlock(remote config.Connection, width int) string {
	styles := ui.NewStyles()
	labelWidth := 8
	labelText := styles.Muted.Bold(true).Render(fmt.Sprintf("%-*s", labelWidth, "PROXY"))
	return labelText + styles.Warning.Render(truncate(proxyHeaderLabel(remote), maxInt(width-labelWidth-2, 20)))
}

/* headerEndpointInline lays out one endpoint on a single line:
 *   LABEL host:port · user · ssl · extra1 · extra2
 * The previous block layout used two lines per endpoint and ate vertical space. */
func headerEndpointInline(label string, conn config.Connection, extras []headerField, width int) string {
	styles := ui.NewStyles()
	labelWidth := 7
	labelText := styles.Muted.Bold(true).Render(fmt.Sprintf("%-*s", labelWidth, label))
	available := maxInt(width-labelWidth, 20)
	host := styles.Accent.Render(truncate(headerHostLabel(conn), maxInt(available/2, 16)))
	fields := make([]headerField, 0, 2+len(extras))
	fields = append(fields,
		headerField{Label: "user", Value: dashFallback(conn.User), Style: styles.Primary},
		headerField{Label: "ssl", Value: dashFallback(conn.SSLMode), Style: styles.Primary},
	)
	fields = append(fields, extras...)
	rendered := renderHeaderFields(fields, maxInt(available-lipgloss.Width(host)-3, 12))
	if rendered == "" {
		return labelText + host
	}
	return labelText + host + styles.Muted.Render(" · ") + rendered
}

/* renderHeaderFields lays out a row of "label value" pairs joined by " · ",
 * dropping later fields whenever the running total would exceed the width
 * budget. Earlier fields stay rendered with their full value (no mid-truncation
 * surprise) until a single field has to be shortened to fit. */
func renderHeaderFields(fields []headerField, width int) string {
	styles := ui.NewStyles()
	separator := styles.Muted.Render(" · ")
	separatorWidth := lipgloss.Width(separator)
	parts := make([]string, 0, len(fields))
	used := 0
	for i, field := range fields {
		labelText := styles.Muted.Render(field.Label + " ")
		labelWidth := lipgloss.Width(labelText)
		valueWidth := lipgloss.Width(field.Value)
		extra := separatorWidth
		if i == 0 {
			extra = 0
		}
		needed := extra + labelWidth + valueWidth
		if used+needed > width {
			budget := width - used - extra - labelWidth
			if budget < 4 {
				break
			}
			value := field.Style.Render(truncate(field.Value, budget))
			parts = append(parts, labelText+value)
			break
		}
		value := field.Style.Render(field.Value)
		parts = append(parts, labelText+value)
		used += needed
	}
	return strings.Join(parts, separator)
}

func headerHostLabel(conn config.Connection) string {
	if strings.TrimSpace(conn.Host) == "" {
		return "—"
	}
	if conn.Port <= 0 {
		return conn.Host
	}
	return fmt.Sprintf("%s:%d", conn.Host, conn.Port)
}

func proxyHeaderLabel(remote config.Connection) string {
	proxy := strings.TrimSpace(ui.ProxyLabel(remote))
	if proxy == "" || proxy == "off" {
		return "configured"
	}
	proxy = strings.ReplaceAll(proxy, "://", " ")
	proxy = strings.ReplaceAll(proxy, "/", " ")
	return proxy
}

func dashFallback(value string) string {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) == "-" {
		return "—"
	}
	return value
}

func elapsedHeaderValue(opts HeaderOptions) string {
	if !opts.Running || opts.Started.IsZero() {
		return ""
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	return ui.FormatDurationTenths(now.Sub(opts.Started))
}

func badge(label string, style lipgloss.Style) string {
	return style.Render("● " + label)
}

func headerCopyTechnology(engine string, useSystemPgtools bool) string {
	tech := ui.CopyTechnology(engine, useSystemPgtools)
	switch tech {
	case "Native pgx COPY protocol":
		return "Native"
	case "System pg_dump → pg_restore":
		return "System"
	case "Embedded pg_dump → pg_restore":
		return "Embedded"
	case "Auto · system pgtools fallback":
		return "Auto"
	case "Auto · native/embedded best available":
		return "Auto"
	default:
		return tech
	}
}

func page(viewportWidth int, parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			clean = append(clean, part)
		}
	}
	padding := pageHorizontalPadding(fittedViewportWidth(viewportWidth))
	return ui.NewStyles().Page.Padding(0, padding).Render(strings.Join(clean, "\n"))
}

func panel(title, body string, width int) string {
	styles := ui.NewStyles()
	header := ""
	if strings.TrimSpace(title) != "" {
		header = styles.PanelTitle.Render(title) + "\n"
	}
	return styles.Panel.Width(innerBoxWidth(width)).Render(header + body)
}

func tabBar(labels []string, active int) string {
	styles := ui.NewStyles()
	active = normalizedTab(active, len(labels))
	parts := make([]string, 0, len(labels))
	for i, label := range labels {
		text := " " + label + " "
		if i == active {
			parts = append(parts, styles.HotButton.Render(text))
			continue
		}
		parts = append(parts, styles.Button.Render(text))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func normalizedTab(tab int, count int) int {
	if count <= 0 {
		return 0
	}
	if tab < 0 {
		return (tab%count + count) % count
	}
	return tab % count
}

/* innerBoxWidth returns the value to pass to lipgloss Style.Width() so the
 * rendered border-padded box ends up exactly outerWidth cells wide. lipgloss
 * Width counts padding; the rounded border adds 2 cells on top, so we subtract
 * just the border. */
func innerBoxWidth(outerWidth int) int {
	return maxInt(fittedWidth(outerWidth)-2, 1)
}

/* panelContentWidth returns the cell budget available for text inside a panel
 * (after subtracting both border and horizontal padding). */
func panelContentWidth(outerWidth int) int {
	return maxInt(fittedWidth(outerWidth)-4, 1)
}

func footer(text string, widths ...int) string {
	style := ui.NewStyles().Footer
	if len(widths) > 0 && widths[0] > 0 {
		style = style.Width(innerBoxWidth(widths[0]))
	}
	return style.Render(text)
}

func layoutWidths(viewportWidth int) (int, int) {
	viewport := fittedViewportWidth(viewportWidth)
	content := maxInt(viewport-pageHorizontalPadding(viewport)*2, 1)
	return viewport, content
}

func fittedViewportWidth(width int) int {
	if width <= 0 {
		return defaultViewportWidth
	}
	return maxInt(width, 24)
}

func fittedWidth(width int) int {
	if width <= 0 {
		return defaultViewportWidth
	}
	return maxInt(width, 18)
}

func viewportHeight(height int) int {
	if height <= 0 {
		return 30
	}
	return maxInt(height, 12)
}

func pageHorizontalPadding(viewportWidth int) int {
	if viewportWidth < 80 {
		return 0
	}
	return 1
}

type actionLabel struct{ Key, Text string }

func actionsLine(actions []actionLabel, widths ...int) string {
	styles := ui.NewStyles()
	width := 0
	if len(widths) > 0 {
		width = widths[0]
	}
	showText := width == 0 || width >= 76
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		part := styles.Key.Render("[" + action.Key + "]")
		if showText {
			part += " " + action.Text
		}
		if id := actionZoneFromLabel(action); id != "" {
			part = markZone(ActionZone(id), part)
		}
		parts = append(parts, part)
	}
	return wrapActionParts(parts, maxInt(width-2, 0))
}

func wrapActionParts(parts []string, width int) string {
	if width <= 0 {
		return strings.Join(parts, "   ")
	}
	lines := make([]string, 0, 2)
	current := ""
	for _, part := range parts {
		separator := "   "
		candidate := part
		if current != "" {
			candidate = current + separator + part
		}
		if current != "" && lipgloss.Width(candidate) > width {
			lines = append(lines, current)
			current = part
			continue
		}
		current = candidate
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}

func actionZoneFromLabel(action actionLabel) string {
	switch strings.ToLower(action.Text) {
	case "all":
		return ActionSelectAll
	case "clear":
		return ActionClear
	case "reload":
		return ActionReload
	case "continue", "confirm", "start":
		return ActionConfirm
	case "databases", "back":
		return ActionBack
	case "quit", "cancel safely":
		return ActionQuit
	case "run again":
		return ActionRunAgain
	default:
		return ""
	}
}

func button(label string, hot bool) string {
	if hot {
		return ui.NewStyles().HotButton.Render(label)
	}
	return ui.NewStyles().Button.Render(label)
}

func markZone(id, value string) string {
	zoneOnce.Do(func() { zone.NewGlobal() })
	return zone.Mark(id, value)
}

func truncate(value string, width int) string {
	if width <= 0 || lipgloss.Width(value) <= width {
		return value
	}
	runes := []rune(value)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

func estimateETA(done, total int64, speed float64) string {
	if total <= 0 || done <= 0 || speed <= 0 || done >= total {
		return "-"
	}
	seconds := float64(total-done) / speed
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return "-"
	}
	return ui.FormatDurationTenths(time.Duration(seconds * float64(time.Second)))
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// ConfirmPlan renders a plan confirmation for one or more selected databases.
func ConfirmPlan(opts PlanReviewOptions) StaticScreen {
	if opts.Header.Width == 0 {
		opts.Header.Width = 116
	}
	body := renderConfirmPlan(opts)
	return StaticScreen{ScreenID: ConfirmPlanID, Heading: "Confirm Sync Plan", Body: body, Hint: "Enter start sync · Esc back"}
}

// Progress renders sync progress summary.
func Progress(stage string, percent float64) StaticScreen {
	body := renderProgress(ProgressSnapshot{Stage: stage, OverallPercent: percent, AnimatedPercent: percent, Header: HeaderOptions{Width: 116}, Now: time.Now()})
	return StaticScreen{ScreenID: ProgressID, Heading: "Sync Running", Body: body, Hint: "Sync is running. Press q to cancel."}
}

// ProgressDashboard renders the full live dashboard.
func ProgressDashboard(snapshot ProgressSnapshot) StaticScreen {
	body := renderProgress(snapshot)
	return StaticScreen{ScreenID: ProgressID, Heading: "Sync Running", Body: body, Hint: "Sync is running. Press q to cancel."}
}

// Result renders sync result summary.
func Result(result *models.SyncResult, options ...ResultOptions) StaticScreen {
	opts := ResultOptions{Header: HeaderOptions{Width: 116}}
	if len(options) > 0 {
		opts = options[0]
	}
	body := renderResult(result, opts)
	return StaticScreen{ScreenID: ResultID, Heading: "Sync Report", Body: body, Hint: "Enter/Q/Esc quit · B back to list"}
}

func init() { zoneOnce.Do(func() { zone.NewGlobal() }) }

// keep strconv referenced for older tests that validate numeric rendering paths.
var _ = strconv.IntSize
