package screens

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/tui/styles"
)

func TestStaticAndBasicScreens(t *testing.T) {
	t.Parallel()
	s := MainMenu()
	assert.Nil(t, s.Init())
	next, cmd := s.Update(tea.KeyMsg{})
	assert.Nil(t, cmd)
	assert.Equal(t, MainMenuID, next.ID())
	assert.Contains(t, s.View(), "Sync")
	assert.NotEmpty(t, s.Title())
	assert.NotEmpty(t, s.Help())

	assert.Contains(t, DatabaseList(nil, nil).View(), "Loading")
	assert.Contains(t, DatabaseList(nil, errors.New("boom")).View(), "boom")
	assert.Contains(t, DatabaseList([]models.Database{{Name: "db", SizeBytes: 1024, TableCount: 3}}, nil).View(), "1.0 KB")
	assert.Contains(t, DatabaseList([]models.Database{{Name: "db", SizeBytes: 1024, TableCount: 3}}, nil).View(), "▸")
	assert.Contains(t, TablesPick(nil).View(), "No user tables")
	assert.Contains(t, TablesPick([]models.Table{{Schema: "public", Name: "users", Rows: 2, SizeBytes: 2048}}).View(), `"public"."users"`)
	assert.Contains(t, ConfirmPlan(nil).View(), "No sync targets")
	assert.Contains(t, ConfirmPlan(&models.SyncPlan{Database: "db", Tables: []models.Table{{Name: "x"}}, Engine: "native"}).View(), "native")
	assert.Contains(t, Progress("copy", 42).View(), "42.0%")
	assert.Contains(t, Result(nil).View(), "No sync result")
	assert.Contains(t, Result(&models.SyncResult{StartedAt: time.Unix(0, 0), FinishedAt: time.Unix(2, 0), RowsCopied: 3, TablesCopied: 1, BytesCopied: 1024, Err: errors.New("bad")}).View(), "bad")
}

func TestScreensFitNarrowViewport(t *testing.T) {
	t.Parallel()
	const narrowWidth = 60
	cfg := validConfigForScreens()
	assertMaxNarrowLineWidth(t, DatabaseList([]models.Database{{Name: "very_long_database_name", SizeBytes: 1024, TableCount: 3}}, nil, DatabaseListOptions{Width: narrowWidth, Height: 24, Config: cfg}).View())
	assertMaxNarrowLineWidth(t, TablesPick([]models.Table{{Schema: "public", Name: "very_long_table_name", Rows: 2, SizeBytes: 2048}}, TableListOptions{Database: "db", Width: narrowWidth, Height: 24, Config: cfg}).View())
	assertMaxNarrowLineWidth(t, ConfirmPlan(&models.SyncPlan{Database: "db", Tables: []models.Table{{Name: "x"}}, Engine: "native"}, HeaderOptions{Width: narrowWidth, Config: cfg}).View())
	assertMaxNarrowLineWidth(t, ProgressDashboard(ProgressSnapshot{Header: HeaderOptions{Width: narrowWidth, Config: cfg}, Stage: "copy", OverallPercent: 42, AnimatedPercent: 42, Now: time.Now()}).View())
	assertMaxNarrowLineWidth(t, Result(&models.SyncResult{StartedAt: time.Unix(0, 0), FinishedAt: time.Unix(2, 0), RowsCopied: 3, TablesCopied: 1, BytesCopied: 1024}, ResultOptions{Header: HeaderOptions{Width: narrowWidth, Config: cfg}}).View())
}

func assertMaxNarrowLineWidth(t *testing.T, view string) {
	t.Helper()
	const narrowWidth = 60
	for _, line := range strings.Split(view, "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), narrowWidth, line)
	}
}

func TestZoneIdentifiers(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "db-row:3", DatabaseRowZone(3))
	assert.Equal(t, "table-row:4", TableRowZone(4))
	assert.Equal(t, "action:confirm", ActionZone(ActionConfirm))
}

func TestLayoutAndRedaction(t *testing.T) {
	t.Parallel()
	theme := styles.NewTheme(true, 12)
	frame := Frame(theme, "Очень длинный заголовок", "body", "help", "status")
	assert.Contains(t, frame, "body")
	assert.Contains(t, frame, "status")
	assert.LessOrEqual(t, len([]rune(theme.Trim("123456789012345"))), 12)
	assert.Empty(t, ErrorPanel(theme, nil))
	assert.Contains(t, ErrorPanel(theme, errors.New("boom")), "Ошибка")
	assert.Equal(t, "socks5://u:xxxxx@h:1", RedactText("socks5://u:p@h:1"))
}

func TestConfigFieldsAndEditor(t *testing.T) {
	t.Parallel()
	cfg := validConfigForScreens()
	remote := RemoteFields(cfg)
	local := LocalFields(cfg)
	runtime := RuntimeFields(cfg)
	logging := LoggingFields(cfg)
	assert.Len(t, remote, 7)
	assert.Len(t, local, 5)
	assert.Len(t, runtime, 3)
	assert.Len(t, logging, 2)
	assert.True(t, remote[3].Secret)
	assert.NoError(t, remote[0].Validate("prod"))
	assert.NoError(t, remote[1].Validate("5432"))
	assert.Error(t, remote[1].Validate("bad"))
	assert.Error(t, remote[1].Validate("0"))
	assert.NoError(t, runtime[0].Validate("1"))
	assert.Error(t, runtime[0].Validate("bad"))
	assert.Error(t, runtime[0].Validate("0"))
	assert.NoError(t, runtime[1].Validate("native"))
	assert.Error(t, runtime[1].Validate("bad"))
	assert.NoError(t, logging[0].Validate("info"))
	assert.Error(t, logging[0].Validate("bad"))
	assert.NoError(t, logging[1].Validate("json"))
	assert.Error(t, logging[1].Validate("bad"))
	assert.NoError(t, local[2].Validate("postgres"))
	assert.Error(t, local[2].Validate(""))

	store := &fakeEditorStore{}
	editor := NewConfigEditor(cfg, EditMode, store)
	assert.Equal(t, ConfigEditorID, editor.ID())
	assert.Nil(t, editor.Init())
	editor = editor.Set("remote.host", "newprod").Set("local.host", "newlocal").Set("remote.user", "ru").Set("local.user", "lu")
	assert.Equal(t, "newprod", editor.Draft.Remote.Host)
	assert.Contains(t, editor.View(), "newprod")
	assert.Contains(t, editor.Title(), "Настройки")
	assert.Contains(t, editor.Help(), "сохранить")

	saved := editor.Save(t.Context())
	assert.True(t, saved.Saved)
	assert.True(t, store.saved)

	bad := NewConfigEditor(config.Defaults(), WizardMode, nil).Save(t.Context())
	assert.False(t, bad.Saved)
	assert.NotNil(t, bad.Err)

	store.err = errors.New("write")
	failed := editor.Save(t.Context())
	assert.False(t, failed.Saved)
	assert.Contains(t, failed.Status, "Не удалось")

	reset := NewConfigEditor(cfg, ResetMode, nil)
	assert.Empty(t, reset.Draft.Remote.Host)
	assert.NotNil(t, BuildConfigForm(cfg, WizardMode))

	next, _ := editor.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	assert.Equal(t, ConfigEditorID, next.ID())
	next, _ = editor.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Equal(t, ConfigEditorID, next.ID())
	next, _ = editor.Update(struct{}{})
	assert.Equal(t, ConfigEditorID, next.ID())
}

func validConfigForScreens() config.Config {
	cfg := config.Defaults()
	cfg.Remote.Host = "prod"
	cfg.Remote.User = "u"
	cfg.Local.Host = "localhost"
	cfg.Local.User = "postgres"
	return cfg
}

type fakeEditorStore struct {
	saved bool
	err   error
}

func (f *fakeEditorStore) Save(ctx context.Context, cfg config.Config) error {
	f.saved = true
	return f.err
}
