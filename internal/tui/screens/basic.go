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
	lines := []string{styles.PanelTitle.Render("Главное меню"), ""}
	for i, item := range items {
		prefix := "  "
		row := item
		if i == index {
			prefix = styles.Primary.Render("▸ ")
			row = styles.SelectedRow.Render(item)
		}
		lines = append(lines, prefix+row)
	}

	body := page(100, renderHeader(HeaderOptions{Title: "PGSync Control Center", Width: 100}), panel("Main Menu", strings.Join(lines, "\n"), 96), footer("↑/↓ выбрать · enter открыть · s настройки · q выход"))
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
	help := "↑/↓ move   Space select DB   Enter tables   A select all   C clear   R reload   Y confirm   S settings"
	return StaticScreen{ScreenID: DatabaseListID, Heading: "Database Queue Builder", Body: body, Hint: help}
}

// TableListOptions configures the table picker screen.
type TableListOptions struct {
	Database      string
	SelectedIndex int
	Checked       map[string]bool
	Loading       bool
	Err           error
	Width         int
	Height        int
	Status        string
	Config        config.Config
}

// TablesPick renders selectable tables.
func TablesPick(tables []models.Table, options ...TableListOptions) StaticScreen {
	opts := TableListOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	body := renderTablePicker(tables, opts)
	return StaticScreen{ScreenID: TablesPickID, Heading: "Tables", Body: body, Hint: "↑/↓ move · Space toggle table · Y/Enter confirm · Esc back"}
}

//nolint:gocyclo,gocognit // Renderer branches directly by UI state for clear terminal output.
func renderDatabaseQueueBuilder(dbs []models.Database, err error, opts DatabaseListOptions) string {
	styles := ui.NewStyles()
	width := maxInt(opts.Width, 104)
	height := maxInt(opts.Height, 30)
	bodyWidth := maxInt(width-4, 88)
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
		visible := clampInt(height-16, 8, 20)
		lines = append(lines, renderDatabaseTable(dbs, opts, bodyWidth, visible))
	}
	if opts.Status != "" && len(dbs) > 0 {
		lines = append(lines, "", styles.Muted.Render(opts.Status))
	}

	content := panel("Databases", strings.Join(lines, "\n"), bodyWidth)
	return page(bodyWidth, header, content, footer(actionsLine([]actionLabel{{"Space", "select"}, {"Enter", "tables"}, {"A", "all"}, {"C", "clear"}, {"R", "reload"}, {"Y", "continue"}})))
}

//nolint:gocyclo // Renderer branches directly by UI state for clear terminal output.
func renderTablePicker(tables []models.Table, opts TableListOptions) string {
	styles := ui.NewStyles()
	width := maxInt(opts.Width, 104)
	height := maxInt(opts.Height, 30)
	bodyWidth := maxInt(width-4, 88)
	header := renderHeader(HeaderOptions{Title: "PGSync Control Center", Config: opts.Config, Database: opts.Database, Width: bodyWidth})
	lines := []string{styles.PanelTitle.Render("Tables: ") + styles.Primary.Render(opts.Database), "", tableListSummary(tables, opts.Checked), ""}
	switch {
	case opts.Err != nil:
		lines = append(lines, styles.Danger.Render("Error: "+RedactText(opts.Err.Error())))
	case opts.Loading:
		spin := spinner.New(spinner.WithSpinner(spinner.Dot))
		lines = append(lines, styles.Warning.Render(spin.View()+" "+opts.Status))
	case len(tables) == 0:
		lines = append(lines, styles.Muted.Render("No user tables found. Enter/Y continues with full database selection."))
	default:
		visible := clampInt(height-18, 8, 18)
		lines = append(lines, renderTablesTable(tables, opts, bodyWidth, visible))
	}
	if opts.Status != "" && !opts.Loading {
		lines = append(lines, "", styles.Muted.Render(opts.Status))
	}
	return page(bodyWidth, header, panel("Tables", strings.Join(lines, "\n"), bodyWidth), footer(actionsLine([]actionLabel{{"Space", "toggle"}, {"A", "all"}, {"C", "clear"}, {"R", "reload"}, {"Enter/Y", "confirm"}, {"Esc", "databases"}})))
}

func renderDatabaseTable(dbs []models.Database, opts DatabaseListOptions, width int, visible int) string {
	inner := innerBoxWidth(width)
	contentWidth := maxInt(inner-2, 42)
	cursor := clampIndexForTable(opts.SelectedIndex, len(dbs))
	start, end := visibleRange(cursor, len(dbs), visible)
	nameWidth := clampInt(contentWidth-45, 18, 72)
	ownerWidth := clampInt(contentWidth-nameWidth-39, 8, 28)

	lines := []string{
		renderListHeader([]listColumn{{Width: 1}, {Title: "Database", Width: nameWidth}, {Title: "Size", Width: 12, AlignRight: true}, {Title: "Tables", Width: 8, AlignRight: true}, {Title: "Owner", Width: ownerWidth}}),
	}
	for index := start; index < end; index++ {
		db := dbs[index]
		lines = append(lines, renderDatabaseRow(db, index == cursor, opts.Checked != nil && opts.Checked[db.Name], nameWidth, ownerWidth))
	}
	return strings.Join(lines, "\n") + "\n" + databaseRangeFooter(dbs, opts.Checked, start, end)
}

func renderTablesTable(tables []models.Table, opts TableListOptions, width int, visible int) string {
	inner := innerBoxWidth(width)
	contentWidth := maxInt(inner-2, 42)
	cursor := clampIndexForTable(opts.SelectedIndex, len(tables))
	start, end := visibleRange(cursor, len(tables), visible)
	nameWidth := clampInt(contentWidth-33, 28, 78)

	lines := []string{
		renderListHeader([]listColumn{{Width: 1}, {Title: "Table", Width: nameWidth}, {Title: "Rows", Width: 14, AlignRight: true}, {Title: "Size", Width: 12, AlignRight: true}}),
	}
	for index := start; index < end; index++ {
		table := tables[index]
		checked := opts.Checked == nil || opts.Checked[tableKey(table)]
		lines = append(lines, renderTableRow(table, index == cursor, checked, nameWidth))
	}
	return strings.Join(lines, "\n") + "\n" + rangeFooter(start, end, len(tables))
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

func renderTableRow(table models.Table, active bool, checked bool, nameWidth int) string {
	styles := ui.NewStyles()
	nameStyle := styles.Row
	if active {
		nameStyle = styles.Primary
	}
	return renderSelectionPrefix(active, checked) +
		renderCell(table.QualifiedName(), nameWidth, nameStyle, false) + "  " +
		renderCell(ui.FormatInt(table.Rows), 14, styles.Muted, true) + "  " +
		renderCell(ui.FormatBytes(table.SizeBytes), 12, styles.Accent, true)
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

func databaseRangeFooter(dbs []models.Database, checked map[string]bool, start int, end int) string {
	styles := ui.NewStyles()
	selected, bytes, tables := selectedDatabaseStats(dbs, checked)
	if len(dbs) == 0 {
		return styles.Muted.Render("Showing 0 of 0")
	}
	return strings.Join([]string{
		styles.Muted.Render(fmt.Sprintf("Showing %s-%s of %s", ui.FormatCount(start+1), ui.FormatCount(end), ui.FormatCount(len(dbs)))),
		ui.Metric("Selected", ui.FormatCount(selected), styles.Success),
		ui.Metric("Size", ui.FormatBytes(bytes), styles.Accent),
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

func tableListSummary(tables []models.Table, checked map[string]bool) string {
	selectedTables := selectedTableCount(tables, checked)
	return fmt.Sprintf("Visible: %s   Selected: %s   Selected size est.: %s   Selected rows est.: %s", ui.NewStyles().Accent.Render(ui.FormatCount(len(tables))), ui.NewStyles().Success.Render(ui.FormatCount(selectedTables)), ui.NewStyles().Accent.Render(ui.FormatBytes(selectedTableBytes(tables, checked))), ui.NewStyles().Accent.Render(ui.FormatInt(selectedTableRows(tables, checked))))
}

func totalTableBytes(tables []models.Table) int64 {
	var total int64
	for _, table := range tables {
		total += table.SizeBytes
	}
	return total
}

func selectedTableCount(tables []models.Table, selected map[string]bool) int {
	if len(tables) == 0 {
		return 0
	}
	if selected == nil {
		return len(tables)
	}
	count := 0
	for _, table := range tables {
		if selected[tableKey(table)] {
			count++
		}
	}
	return count
}

func selectedTableBytes(tables []models.Table, selected map[string]bool) int64 {
	if len(tables) == 0 {
		return 0
	}
	if selected == nil {
		return totalTableBytes(tables)
	}
	var total int64
	for _, table := range tables {
		if selected[tableKey(table)] {
			total += table.SizeBytes
		}
	}
	return total
}

func selectedTableRows(tables []models.Table, selected map[string]bool) int64 {
	var total int64
	for _, table := range tables {
		if selected == nil || selected[tableKey(table)] {
			total += table.Rows
		}
	}
	return total
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

func renderConfirmPlan(plan *models.SyncPlan, header HeaderOptions) string {
	styles := ui.NewStyles()
	bodyWidth := maxInt(header.Width, 104)
	header.Title = "PGSync Plan Review"
	header.Width = bodyWidth
	if plan != nil && header.Database == "" {
		header.Database = plan.Database
	}
	lines := []string{styles.PanelTitle.Render("Confirm Sync Plan"), ""}
	if plan == nil || plan.Database == "" {
		lines = append(lines, styles.Muted.Render("No sync targets selected."))
	} else {
		estimated := totalTableBytes(plan.Tables)
		selectedRows := selectedTableRows(plan.Tables, nil)
		mode := styles.Success.Render("FULL DB")
		if len(plan.Tables) > 0 {
			mode = styles.Warning.Render(fmt.Sprintf("%s selected tables", ui.FormatCount(len(plan.Tables))))
		}
		lines = append(lines,
			ui.Metric("Database", plan.Database, styles.Primary),
			ui.Metric("Tables", ui.FormatCount(len(plan.Tables)), styles.Success),
			ui.Metric("Estimated rows", ui.FormatInt(selectedRows), styles.Accent),
			ui.Metric("Estimated size", ui.FormatBytes(estimated), styles.Accent),
			ui.Metric("Engine", plan.Engine, styles.Primary),
			ui.Metric("Copy technology", ui.CopyTechnology(plan.Engine, plan.UseSystemPgtools), styles.Success),
			ui.Metric("Workers", ui.FormatCount(plan.Threads), styles.Primary),
			"",
			styles.SelectedRow.Render(plan.Database+"  ")+" "+mode,
			"",
			lipgloss.JoinHorizontal(lipgloss.Top, markZone(ActionZone(ActionCancel), button("Cancel", false)), "   ", markZone(ActionZone(ActionStart), button("Start Sync", true))),
		)
	}
	pipeline := strings.Join([]string{"1  Connect remote", "2  Snapshot schema pre-data", "3  Drop/recreate local DB", "4  COPY table data", "5  Restore post-data schema/indexes", "6  Reset sequences", "7  Verify result"}, "\n")
	content := lipgloss.JoinHorizontal(lipgloss.Top, panel("Plan Summary", strings.Join(lines, "\n"), bodyWidth/2), " ", panel("Pipeline", pipeline, bodyWidth/2-3))
	return page(bodyWidth, renderHeader(header), content, footer(actionsLine([]actionLabel{{"Enter/Y", "start"}, {"Esc", "back"}})))
}

func renderProgress(snapshot ProgressSnapshot) string {
	bodyWidth := maxInt(snapshot.Header.Width, 104)
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
	content := ""
	switch normalizedTab(snapshot.Tab, 2) {
	case 1:
		content = panelFixed("Events", renderEvents(snapshot.Events, bodyWidth), bodyWidth, 18)
	default:
		content = panelFixed("Overview", renderProgressOverview(snapshot, bodyWidth), bodyWidth, 18)
	}
	return page(bodyWidth, renderHeader(snapshot.Header), tabs, content, footer(actionsLine([]actionLabel{{"Tab", "switch tab"}, {"P", "pause/resume"}, {"Q", "cancel safely"}})))
}

func renderEvents(events []ProgressEventRow, width int) string {
	styles := ui.NewStyles()
	if len(events) == 0 {
		return styles.Muted.Render("Waiting for engine events…")
	}
	lines := []string{styles.Muted.Render(fmt.Sprintf("%-9s  %-5s  %-24s  %-28s  %s", "Time", "Level", "Event", "Table", "Details")), styles.Muted.Render(strings.Repeat("─", maxInt(width-10, 40)))}
	limit := minInt(len(events), 6)
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
	bodyWidth := maxInt(opts.Header.Width, 104)
	opts.Header.Title = "Sync Report"
	opts.Header.Width = bodyWidth
	tabs := tabBar([]string{"Summary", "Stages", "Tables"}, opts.Tab)
	content := ""
	switch normalizedTab(opts.Tab, 3) {
	case 1:
		content = panelFixed("Stage timings", renderStageTimings(result, bodyWidth), bodyWidth, 18)
	case 2:
		content = panelFixed("Slowest tables", renderTableResults(opts.Tables, bodyWidth), bodyWidth, 18)
	default:
		content = panelFixed("Result", renderResultSummary(result), bodyWidth, 18)
	}
	return page(bodyWidth, renderHeader(opts.Header), tabs, content, footer(actionsLine([]actionLabel{{"Tab/←/→", "switch tab"}, {"Enter/Q/Esc", "quit"}, {"B", "back to databases"}, {"R", "run again"}})))
}

func renderProgressOverview(snapshot ProgressSnapshot, bodyWidth int) string {
	styles := ui.NewStyles()
	elapsed := snapshot.Now.Sub(snapshot.StartedAt)
	if snapshot.StartedAt.IsZero() {
		elapsed = 0
	}
	stage := emptyFallback(snapshot.Stage, "waiting")
	current := emptyFallback(snapshot.CurrentTable, "-")
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
	return strings.Join([]string{
		ui.ProgressBar(bodyWidth-12, overall) + "  " + styles.Accent.Render(ui.FormatPercent(snapshot.OverallPercent)),
		"",
		ui.Metric("Stage", stage, styles.Primary),
		ui.Metric("Current", current, styles.Accent),
		fmt.Sprintf("%s     %s", ui.Metric("Tables", fmt.Sprintf("%s / %s", ui.FormatCount(snapshot.TablesDone), ui.FormatCount(snapshot.TablesTotal)), styles.Success), ui.Metric("Rows done", fmt.Sprintf("%s / %s", ui.FormatInt(snapshot.RowsCopied), ui.FormatInt(snapshot.RowsEstimated)), styles.Accent)),
		fmt.Sprintf("%s     %s", ui.Metric("Data", fmt.Sprintf("%s / %s", ui.FormatBytes(snapshot.BytesCopied), ui.FormatBytes(snapshot.BytesEstimated)), styles.Accent), ui.Metric("Speed", ui.FormatBytesRate(snapshot.BytesPerSec), styles.Success)),
		fmt.Sprintf("%s     %s     %s", ui.Metric("Elapsed", ui.FormatDurationTenths(elapsed), styles.Primary), ui.Metric("ETA", eta, styles.Warning), ui.Metric("Errors", ui.FormatCount(snapshot.Errors), styles.Danger)),
		"",
		styles.PanelTitle.Render("Current table"),
		ui.ProgressBar(bodyWidth-12, snapshot.TablePercent) + "  " + styles.Accent.Render(ui.FormatPercent(snapshot.TablePercent)),
		ui.Metric("Data copied", fmt.Sprintf("%s / %s", ui.FormatBytes(snapshot.TableBytes), ui.FormatBytes(snapshot.TableBytesEstimate)), styles.Accent),
		ui.Metric("Rows done", fmt.Sprintf("%s / %s", ui.FormatInt(snapshot.TableRows), ui.FormatInt(snapshot.TableRowsEstimate)), styles.Accent),
		ui.Metric("Rows/sec", ui.FormatRowsRate(rowsRate), styles.Success),
		ui.Metric("Table elapsed", ui.FormatDurationTenths(tableElapsed), styles.Primary),
	}, "\n")
}

func renderResultSummary(result *models.SyncResult) string {
	styles := ui.NewStyles()
	lines := []string{}
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
	lines = append(lines,
		ui.Metric("Status", status, styles.Success),
		ui.Metric("Database", result.Database, styles.Primary),
		ui.Metric("Duration", ui.FormatDurationTenths(duration), styles.Primary),
		ui.Metric("Tables", ui.FormatCount(result.TablesCopied), styles.Success),
		ui.Metric("Rows", ui.FormatInt(result.RowsCopied), styles.Accent),
		ui.Metric("Data", ui.FormatBytes(result.BytesCopied), styles.Accent),
		ui.Metric("Avg speed", ui.FormatBytesRate(avgSpeed), styles.Success),
	)
	if result.Err != nil {
		lines = append(lines, "", styles.Danger.Render("Error: "+RedactText(result.Err.Error())))
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
	lines := []string{styles.Muted.Render(fmt.Sprintf("%-32s  %14s  %12s  %10s  %s", "Table", "Rows", "Size", "Duration", "Avg speed")), styles.Muted.Render(strings.Repeat("─", maxInt(width-10, 40)))}
	for _, row := range rows {
		lines = append(lines, fmt.Sprintf("%-32s  %14s  %12s  %10s  %s", truncate(row.Table, 32), ui.FormatInt(row.Rows), ui.FormatBytes(row.Bytes), ui.FormatDurationTenths(row.Duration), ui.FormatBytesRate(row.Speed)))
	}
	return strings.Join(lines, "\n")
}

func renderHeader(opts HeaderOptions) string {
	styles := ui.NewStyles()
	width := maxInt(opts.Width, 96)
	innerWidth := innerBoxWidth(width)
	title := opts.Title
	if title == "" {
		title = "PGSync Control Center"
	}
	modeStyle := styles.Success
	if ui.ConnectionMode(opts.Config.Remote) == "PROXY" {
		modeStyle = styles.Warning
	}
	modeBadge := badge(ui.ConnectionMode(opts.Config.Remote), modeStyle)
	techBadge := badge(headerCopyTechnology(opts.Config.Runtime.Engine, opts.Config.Runtime.UseSystemPgtools), styles.Accent)
	lines := []string{ui.HeaderLine(styles.HeaderTitle.Render(headerTitle(title)), modeBadge+"  "+techBadge, innerWidth)}
	if ui.ConnectionMode(opts.Config.Remote) == "PROXY" {
		lines = append(lines, headerProxyBlock(opts.Config.Remote, innerWidth))
	}
	lines = append(lines, headerEndpointBlock("REMOTE", opts.Config.Remote, nil, innerWidth)...)
	localExtras := []headerField{{Label: "workers", Value: ui.FormatCount(opts.Config.Runtime.Threads), Style: styles.Primary}}
	if elapsed := elapsedHeaderValue(opts); elapsed != "" {
		localExtras = append(localExtras, headerField{Label: "elapsed", Value: elapsed, Style: styles.Primary})
	}
	lines = append(lines, headerEndpointBlock("LOCAL", opts.Config.Local, localExtras, innerWidth)...)
	return styles.Header.Width(innerWidth).Render(strings.Join(lines, "\n"))
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

func headerEndpointBlock(label string, conn config.Connection, extras []headerField, width int) []string {
	styles := ui.NewStyles()
	labelWidth := 8
	labelText := styles.Muted.Bold(true).Render(fmt.Sprintf("%-*s", labelWidth, label))
	hostWidth := maxInt(width-labelWidth-2, 20)
	lines := []string{labelText + styles.Accent.Render(truncate(headerHostLabel(conn), hostWidth))}
	fields := []headerField{
		{Label: "user", Value: dashFallback(conn.User), Style: styles.Primary},
		{Label: "ssl", Value: dashFallback(conn.SSLMode), Style: styles.Primary},
	}
	fields = append(fields, extras...)
	lines = append(lines, strings.Repeat(" ", labelWidth)+renderHeaderFields(fields, maxInt(width-labelWidth, 20)))
	return lines
}

func renderHeaderFields(fields []headerField, width int) string {
	styles := ui.NewStyles()
	parts := make([]string, 0, len(fields))
	available := width
	remaining := len(fields)
	for _, field := range fields {
		remaining--
		label := styles.Muted.Render(field.Label + " ")
		separatorWidth := 4
		if remaining == 0 {
			separatorWidth = 0
		}
		maxValueWidth := maxInt(available-lipgloss.Width(field.Label)-1-separatorWidth-(remaining*8), 4)
		value := field.Style.Render(truncate(field.Value, maxValueWidth))
		part := label + value
		parts = append(parts, part)
		available -= lipgloss.Width(part) + separatorWidth
		if available < 12 && remaining > 0 {
			break
		}
	}
	return strings.Join(parts, "    ")
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

func elapsedHeader(opts HeaderOptions) string {
	if !opts.Running || opts.Started.IsZero() {
		return ""
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	return "   " + ui.Metric("Elapsed", ui.FormatDurationTenths(now.Sub(opts.Started)), ui.NewStyles().Primary)
}

func elapsedHeaderPlain(opts HeaderOptions) string {
	if value := elapsedHeaderValue(opts); value != "" {
		return "   Elapsed " + value
	}
	return ""
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

func headerKV(label, value string, width int, valueStyle lipgloss.Style) string {
	labelText := ui.NewStyles().Muted.Render(fmt.Sprintf("%-7s", label))
	maxValueWidth := maxInt(width-lipgloss.Width(labelText)-1, 12)
	return labelText + " " + valueStyle.Render(truncate(value, maxValueWidth))
}

func badge(label string, style lipgloss.Style) string {
	return style.Render("● " + label)
}

func headerCopyTechnology(engine string, useSystemPgtools bool) string {
	tech := ui.CopyTechnology(engine, useSystemPgtools)
	switch tech {
	case "Native pgx COPY protocol":
		return "Native COPY"
	case "System pg_dump → pg_restore":
		return "System pgtools"
	case "Embedded pg_dump → pg_restore":
		return "Embedded pgtools"
	case "Auto · system pgtools fallback":
		return "Auto System"
	case "Auto · native/embedded best available":
		return "Auto"
	default:
		return tech
	}
}

func page(_ int, parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			clean = append(clean, part)
		}
	}
	return ui.NewStyles().Page.Render(strings.Join(clean, "\n\n"))
}

func panel(title, body string, width int) string {
	styles := ui.NewStyles()
	return styles.Panel.Width(innerBoxWidth(width)).Render(styles.PanelTitle.Render(title) + "\n" + body)
}

func panelFixed(title, body string, width int, height int) string {
	styles := ui.NewStyles()
	content := styles.PanelTitle.Render(title) + "\n" + fixedHeight(body, maxInt(height-1, 1))
	return styles.Panel.Width(innerBoxWidth(width)).Height(height).Render(content)
}

func fixedHeight(body string, height int) string {
	lines := strings.Split(body, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
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

func warningPanel(title, body string, width int) string {
	styles := ui.NewStyles()
	return styles.Panel.BorderForeground(ui.ColorWarning).Width(innerBoxWidth(width)).Render(styles.Warning.Render(title) + "\n" + body)
}

func innerBoxWidth(outerWidth int) int {
	// Rounded border adds 2 cells and horizontal padding adds 4 cells.
	return maxInt(outerWidth-6, 20)
}

func footer(text string) string { return ui.NewStyles().Footer.Render(text) }

type actionLabel struct{ Key, Text string }

func actionsLine(actions []actionLabel) string {
	styles := ui.NewStyles()
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		part := styles.Key.Render("[ "+action.Key+" ]") + " " + action.Text
		if id := actionZoneFromLabel(action); id != "" {
			part = markZone(ActionZone(id), part)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "   ") + "\n" + styles.Muted.Render("Mouse: click rows and buttons")
}

func actionZoneFromLabel(action actionLabel) string {
	switch strings.ToLower(action.Text) {
	case "tables":
		return ActionTables
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

func tableKey(table models.Table) string { return table.Schema + "." + table.Name }

// ConfirmPlan renders a plan confirmation.
func ConfirmPlan(plan *models.SyncPlan, options ...HeaderOptions) StaticScreen {
	header := HeaderOptions{Width: 116}
	if len(options) > 0 {
		header = options[0]
	}
	body := renderConfirmPlan(plan, header)
	return StaticScreen{ScreenID: ConfirmPlanID, Heading: "Confirm Sync Plan", Body: body, Hint: "←/→/Tab switch · Enter start sync · Esc back"}
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
