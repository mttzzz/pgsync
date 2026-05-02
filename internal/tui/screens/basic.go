package screens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mttzzz/pgsync/internal/models"
)

var (
	queuePageStyle    = lipgloss.NewStyle().Padding(1, 2)
	queueHeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F6F2FF")).Background(lipgloss.Color("#6C2BD9")).Padding(0, 2)
	queuePanelStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#353160")).Padding(1, 2)
	queueSelectedRow  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A855F7")).Bold(true)
	queueKeyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED")).Bold(true)
	queueSizeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00D2FF")).Bold(true)
	queueOKStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#00D084")).Bold(true)
	queueDangerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4D6D")).Bold(true)
	queueSubtleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#7E7A9A"))
	queueTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F6F2FF"))
	queueLoadingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Bold(true)
)

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

	lines := make([]string, len(items))
	for i, item := range items {
		prefix := "  "
		if i == index {
			prefix = "› "
		}
		lines[i] = prefix + item
	}

	body := "Главное меню\n\n" + strings.Join(lines, "\n") + "\n\n↑/↓ выбрать · enter открыть · s настройки · q выход"
	return StaticScreen{ScreenID: MainMenuID, Heading: "Главное меню", Body: body, Hint: "↑/↓ выбрать · enter открыть · s настройки · q выход"}
}

// DatabaseListOptions configures the database queue builder screen.
type DatabaseListOptions struct {
	SelectedIndex int
	Checked       map[string]bool
	Width         int
	Height        int
	Status        string
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

func renderDatabaseQueueBuilder(dbs []models.Database, err error, opts DatabaseListOptions) string {
	width := maxInt(opts.Width, 96)
	height := maxInt(opts.Height, 28)
	bodyWidth := maxInt(width-4, 80)
	footer := queueSubtleStyle.Render(fmt.Sprintf("%s move   %s select DB   %s tables   %s select all   %s clear   %s reload   %s confirm   %s settings", queueKeyStyle.Render("↑/↓"), queueKeyStyle.Render("Space"), queueKeyStyle.Render("Enter"), queueKeyStyle.Render("A"), queueKeyStyle.Render("C"), queueKeyStyle.Render("R"), queueKeyStyle.Render("Y"), queueKeyStyle.Render("S")))

	lines := []string{queueTitleStyle.Render("Database Queue Builder"), "", databaseListSummary(dbs), ""}
	if err != nil {
		lines = append(lines, queueDangerStyle.Render("Error: "+err.Error()))
	} else if len(dbs) == 0 {
		status := opts.Status
		if status == "" {
			status = "Loading remote databases..."
		}
		lines = append(lines, queueLoadingStyle.Render(status))
	} else {
		visible := clampInt(height-14, 8, 18)
		start, end := visibleRange(opts.SelectedIndex, len(dbs), visible)
		nameWidth, sizeWidth, tablesWidth := databaseColumnWidths(dbs)
		for index := start; index < end; index++ {
			db := dbs[index]
			mark := "[ ]"
			if opts.Checked != nil && opts.Checked[db.Name] {
				mark = queueOKStyle.Render("[x]")
			}
			prefix := "  "
			row := fmt.Sprintf("%s %s  %s %s", mark, padRight(db.Name, nameWidth), padLeft(queueSizeStyle.Render(models.FormatBytes(db.SizeBytes)), sizeWidth), padLeft(queueSubtleStyle.Render(strconv.Itoa(db.TableCount)+" tables"), tablesWidth+7))
			if index == opts.SelectedIndex {
				prefix = queueKeyStyle.Render("▸ ")
				row = queueSelectedRow.Render(row)
			}
			lines = append(lines, prefix+row)
		}
		if len(dbs) > visible {
			lines = append(lines, "", queueSubtleStyle.Render(fmt.Sprintf("[%d-%d / %d]", start+1, end, len(dbs))))
		}
	}
	if opts.Status != "" && len(dbs) > 0 {
		lines = append(lines, "", queueSubtleStyle.Render(opts.Status))
	}

	panel := queuePanelStyle.Width(bodyWidth).Render(strings.Join(lines, "\n"))
	return queuePageStyle.Render(strings.Join([]string{queueHeaderStyle.Render("PGSync Control Center"), "", panel, "", footer}, "\n"))
}

func renderTablePicker(tables []models.Table, opts TableListOptions) string {
	width := maxInt(opts.Width, 96)
	height := maxInt(opts.Height, 28)
	bodyWidth := maxInt(width-4, 80)
	footer := queueSubtleStyle.Render(fmt.Sprintf("%s move   %s toggle table   %s reload   %s confirm   %s back   %s settings", queueKeyStyle.Render("↑/↓"), queueKeyStyle.Render("Space"), queueKeyStyle.Render("R"), queueKeyStyle.Render("Y/Enter"), queueKeyStyle.Render("Esc"), queueKeyStyle.Render("S")))
	lines := []string{queueTitleStyle.Render("Tables: ") + queueKeyStyle.Render(opts.Database), "", tableListSummary(tables), ""}
	if opts.Err != nil {
		lines = append(lines, queueDangerStyle.Render("Error: "+opts.Err.Error()))
	} else if opts.Loading {
		lines = append(lines, queueLoadingStyle.Render(opts.Status))
	} else if len(tables) == 0 {
		lines = append(lines, queueSubtleStyle.Render("No user tables found. Enter/Y continues with full database selection."))
	} else {
		visible := clampInt(height-14, 8, 18)
		start, end := visibleRange(opts.SelectedIndex, len(tables), visible)
		nameWidth := tableNameWidth(tables)
		for index := start; index < end; index++ {
			table := tables[index]
			prefix := "  "
			mark := "[ ]"
			if opts.Checked == nil || opts.Checked[tableKey(table)] {
				mark = queueOKStyle.Render("[x]")
			}
			row := fmt.Sprintf("%s %s  %s  %s", mark, padRight(table.QualifiedName(), nameWidth), padLeft(queueSizeStyle.Render(models.FormatBytes(table.SizeBytes)), 10), queueSubtleStyle.Render(formatRows(table.Rows)))
			if index == opts.SelectedIndex {
				prefix = queueKeyStyle.Render("▸ ")
				row = queueSelectedRow.Render(row)
			}
			lines = append(lines, prefix+row)
		}
		if len(tables) > visible {
			lines = append(lines, "", queueSubtleStyle.Render(fmt.Sprintf("[%d-%d / %d]", start+1, end, len(tables))))
		}
	}
	if opts.Status != "" && !opts.Loading {
		lines = append(lines, "", queueSubtleStyle.Render(opts.Status))
	}
	panel := queuePanelStyle.Width(bodyWidth).Render(strings.Join(lines, "\n"))
	return queuePageStyle.Render(strings.Join([]string{queueHeaderStyle.Render("PGSync Control Center"), "", panel, "", footer}, "\n"))
}

func databaseListSummary(dbs []models.Database) string {
	return fmt.Sprintf("Visible: %s   Source data est.: %s   Tables: %s", queueSizeStyle.Render(strconv.Itoa(len(dbs))), queueSizeStyle.Render(models.FormatBytes(totalDatabaseBytes(dbs))), strconv.Itoa(totalDatabaseTables(dbs)))
}

func tableListSummary(tables []models.Table) string {
	return fmt.Sprintf("Visible: %s   Source data est.: %s", queueSizeStyle.Render(strconv.Itoa(len(tables))), queueSizeStyle.Render(models.FormatBytes(totalTableBytes(tables))))
}

func databaseColumnWidths(dbs []models.Database) (int, int, int) {
	nameWidth := 24
	sizeWidth := 10
	tablesWidth := 2
	for _, db := range dbs {
		nameWidth = maxInt(nameWidth, len(db.Name))
		sizeWidth = maxInt(sizeWidth, len(models.FormatBytes(db.SizeBytes)))
		tablesWidth = maxInt(tablesWidth, len(strconv.Itoa(db.TableCount)))
	}
	return nameWidth, sizeWidth, tablesWidth
}

func tableNameWidth(tables []models.Table) int {
	width := 24
	for _, table := range tables {
		width = maxInt(width, len(table.QualifiedName()))
	}
	return width
}

func totalDatabaseBytes(dbs []models.Database) int64 {
	var total int64
	for _, db := range dbs {
		total += db.SizeBytes
	}
	return total
}

func totalDatabaseTables(dbs []models.Database) int {
	total := 0
	for _, db := range dbs {
		total += db.TableCount
	}
	return total
}

func totalTableBytes(tables []models.Table) int64 {
	var total int64
	for _, table := range tables {
		total += table.SizeBytes
	}
	return total
}

func formatRows(rows int64) string {
	if rows == 1 {
		return "1 row"
	}
	return fmt.Sprintf("%d rows", rows)
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

func padRight(value string, width int) string {
	padding := width - len(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func padLeft(value string, width int) string {
	plain := lipgloss.Width(value)
	padding := width - plain
	if padding <= 0 {
		return value
	}
	return strings.Repeat(" ", padding) + value
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

func renderConfirmPlan(plan *models.SyncPlan) string {
	bodyWidth := 116
	footer := queueSubtleStyle.Render(fmt.Sprintf("%s switch   %s start sync   %s arm/start   %s back", queueKeyStyle.Render("←/→/Tab"), queueKeyStyle.Render("Enter"), queueKeyStyle.Render("Y"), queueKeyStyle.Render("Esc")))
	lines := []string{queueTitleStyle.Render("Confirm Sync Plan"), ""}
	if plan == nil || plan.Database == "" {
		lines = append(lines, queueSubtleStyle.Render("No sync targets selected."))
	} else {
		estimated := totalTableBytes(plan.Tables)
		lines = append(lines,
			fmt.Sprintf("Databases: %s", queueSizeStyle.Render("1")),
			fmt.Sprintf("Estimated source data: %s", queueSizeStyle.Render(models.FormatBytes(estimated))),
			fmt.Sprintf("Engine: %s   Threads: %d", queueKeyStyle.Render(plan.Engine), plan.Threads),
			"",
		)
		mode := queueOKStyle.Render("FULL DB")
		if len(plan.Tables) > 0 {
			mode = queueLoadingStyle.Render(fmt.Sprintf("%d selected tables", len(plan.Tables)))
		}
		lines = append(lines, fmt.Sprintf("%s  %s", queueSelectedRow.Render(plan.Database), mode))
		lines = append(lines, "", queueDangerStyle.Render("Target local database will be dropped and recreated."))
		lines = append(lines, "", lipgloss.JoinHorizontal(lipgloss.Top, renderButton("Cancel", false, false), "   ", renderButton("Sync", true, true)))
	}
	panel := queuePanelStyle.Width(bodyWidth).Render(strings.Join(lines, "\n"))
	return queuePageStyle.Render(strings.Join([]string{queueHeaderStyle.Render("PGSync Control Center"), "", panel, "", footer}, "\n"))
}

func renderProgress(stage string, percent float64) string {
	bodyWidth := 116
	progress := clampFloat(percent/100, 0, 1)
	lines := []string{queueTitleStyle.Render("Sync Running"), "", fmt.Sprintf("Phase: %s", queueKeyStyle.Render(stage)), renderProgressBar(48, progress), fmt.Sprintf("Progress: %s", queueSizeStyle.Render(fmt.Sprintf("%.1f%%", percent)))}
	panel := queuePanelStyle.Width(bodyWidth).Render(strings.Join(lines, "\n"))
	footer := queueSubtleStyle.Render("Sync is running. Press ? for help.")
	return queuePageStyle.Render(strings.Join([]string{queueHeaderStyle.Render("PGSync Control Center"), "", panel, "", footer}, "\n"))
}

func renderResult(result *models.SyncResult) string {
	bodyWidth := 116
	lines := []string{queueTitleStyle.Render("Sync Report"), ""}
	if result == nil {
		lines = append(lines, queueSubtleStyle.Render("No sync result yet."))
	} else {
		status := queueOKStyle.Render("SUCCESS")
		if result.Err != nil {
			status = queueDangerStyle.Render("FAILED")
		}
		lines = append(lines,
			fmt.Sprintf("Database: %s", queueSelectedRow.Render(result.Database)),
			fmt.Sprintf("Status: %s", status),
			fmt.Sprintf("Duration: %s", result.Duration()),
			fmt.Sprintf("Rows: %s", queueSizeStyle.Render(strconv.FormatInt(result.RowsCopied, 10))),
			fmt.Sprintf("Tables: %s", queueSizeStyle.Render(strconv.Itoa(result.TablesCopied))),
			fmt.Sprintf("Bytes: %s", queueSizeStyle.Render(models.FormatBytes(result.BytesCopied))),
		)
		if result.Err != nil {
			lines = append(lines, "", queueDangerStyle.Render("Error: "+result.Err.Error()))
		}
	}
	panel := queuePanelStyle.Width(bodyWidth).Render(strings.Join(lines, "\n"))
	footer := queueSubtleStyle.Render(fmt.Sprintf("%s quit   %s back to list", queueKeyStyle.Render("Enter/Q/Esc"), queueKeyStyle.Render("B")))
	return queuePageStyle.Render(strings.Join([]string{queueHeaderStyle.Render("PGSync Control Center"), "", panel, "", footer}, "\n"))
}

func renderButton(label string, selected bool, destructive bool) string {
	style := queuePanelStyle.Padding(0, 2)
	if selected {
		style = style.BorderForeground(lipgloss.Color("#7C3AED")).Foreground(lipgloss.Color("#A855F7")).Bold(true)
	}
	if destructive && selected {
		style = style.Foreground(lipgloss.Color("#FF4D6D"))
	}
	return style.Render(label)
}

func renderProgressBar(width int, progress float64) string {
	filled := int(progress * float64(width))
	if filled > width {
		filled = width
	}
	return queueOKStyle.Render(strings.Repeat("█", filled)) + queueSubtleStyle.Render(strings.Repeat("░", width-filled))
}

func clampFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func tableKey(table models.Table) string {
	return table.Schema + "." + table.Name
}

// ConfirmPlan renders a plan confirmation.
func ConfirmPlan(plan *models.SyncPlan) StaticScreen {
	body := renderConfirmPlan(plan)
	return StaticScreen{ScreenID: ConfirmPlanID, Heading: "Confirm Sync Plan", Body: body, Hint: "←/→/Tab switch · Enter start sync · Esc back"}
}

// Progress renders sync progress summary.
func Progress(stage string, percent float64) StaticScreen {
	body := renderProgress(stage, percent)
	return StaticScreen{ScreenID: ProgressID, Heading: "Sync Running", Body: body, Hint: "Sync is running. Press q to cancel."}
}

// Result renders sync result summary.
func Result(result *models.SyncResult) StaticScreen {
	body := renderResult(result)
	return StaticScreen{ScreenID: ResultID, Heading: "Sync Report", Body: body, Hint: "Enter/Q/Esc quit · B back to list"}
}
