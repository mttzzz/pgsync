package screens

import (
	"fmt"
	"strings"

	"github.com/mttzzz/pgsync/internal/models"
)

// MainMenu renders main actions.
func MainMenu() StaticScreen {
	return StaticScreen{ScreenID: MainMenuID, Heading: "Главное меню", Body: "Sync database\nSettings\nQuit", Hint: "enter выбрать · s настройки · q выход"}
}

// DatabaseList renders databases.
func DatabaseList(dbs []models.Database, err error) StaticScreen {
	if err != nil {
		return StaticScreen{ScreenID: DatabaseListID, Heading: "Базы", Body: "Ошибка: " + err.Error(), Hint: "r повторить · esc назад"}
	}
	if len(dbs) == 0 {
		return StaticScreen{ScreenID: DatabaseListID, Heading: "Базы", Body: "Базы не найдены", Hint: "r повторить · esc назад"}
	}
	lines := make([]string, len(dbs))
	for i, db := range dbs {
		lines[i] = db.String()
	}
	return StaticScreen{ScreenID: DatabaseListID, Heading: "Базы", Body: strings.Join(lines, "\n"), Hint: "space выбрать · enter продолжить"}
}

// TablesPick renders selectable tables.
func TablesPick(tables []models.Table) StaticScreen {
	lines := make([]string, len(tables))
	for i, table := range tables {
		lines[i] = fmt.Sprintf("%s rows=%d size=%s", table.QualifiedName(), table.Rows, models.FormatBytes(table.SizeBytes))
	}
	if len(lines) == 0 {
		lines = []string{"Пусто = синхронизация всей базы"}
	}
	return StaticScreen{ScreenID: TablesPickID, Heading: "Таблицы", Body: strings.Join(lines, "\n"), Hint: "space выбрать · enter план"}
}

// ConfirmPlan renders a plan confirmation.
func ConfirmPlan(plan *models.SyncPlan) StaticScreen {
	if plan == nil {
		return StaticScreen{ScreenID: ConfirmPlanID, Heading: "План", Body: "План не построен", Hint: "esc назад"}
	}
	return StaticScreen{ScreenID: ConfirmPlanID, Heading: "Подтверждение", Body: fmt.Sprintf("DB: %s\nTables: %d\nEngine: %s\nTarget will be dropped and recreated", plan.Database, len(plan.Tables), plan.Engine), Hint: "enter старт · esc назад"}
}

// Progress renders sync progress summary.
func Progress(stage string, percent float64) StaticScreen {
	return StaticScreen{ScreenID: ProgressID, Heading: "Синхронизация", Body: fmt.Sprintf("%s %.1f%%", stage, percent), Hint: "space пауза · q отмена"}
}

// Result renders sync result summary.
func Result(result *models.SyncResult) StaticScreen {
	if result == nil {
		return StaticScreen{ScreenID: ResultID, Heading: "Результат", Body: "Нет результата", Hint: "enter меню · q выход"}
	}
	body := fmt.Sprintf("Duration: %s\nRows: %d\nTables: %d\nBytes: %s", result.Duration(), result.RowsCopied, result.TablesCopied, models.FormatBytes(result.BytesCopied))
	if result.Err != nil {
		body += "\nError: " + result.Err.Error()
	}
	return StaticScreen{ScreenID: ResultID, Heading: "Результат", Body: body, Hint: "enter меню · q выход"}
}
