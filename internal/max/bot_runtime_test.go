package max

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/max-messenger/max-bot-api-client-go/schemes"

	"smtp-to-max-relay/internal/email"
	"smtp-to-max-relay/internal/version"
)

type testAliasAdmin struct {
	values map[string]string
}

type testStatsReporter struct {
	report string
}

type testDLQAdmin struct {
	summary string
	list    string
	replay  string
}

func (s *testStatsReporter) BuildLastDaysReport(days int) string {
	return s.report
}

func (d *testDLQAdmin) Summary() string { return d.summary }
func (d *testDLQAdmin) List(limit int) string {
	return fmt.Sprintf("%s:%d", d.list, limit)
}
func (d *testDLQAdmin) Replay(ctx context.Context, id string) string {
	_ = ctx
	return fmt.Sprintf("%s:%s", d.replay, id)
}

func (a *testAliasAdmin) ValidateAliasTarget(local string) error {
	if !strings.HasPrefix(local, "chatid") {
		return aliasTargetErr("invalid")
	}
	return nil
}

func (a *testAliasAdmin) SetAlias(alias, target string) {
	if a.values == nil {
		a.values = map[string]string{}
	}
	a.values[alias] = target
}

func (a *testAliasAdmin) DeleteAlias(alias string) {
	delete(a.values, alias)
}

func (a *testAliasAdmin) SnapshotAliases() map[string]string {
	out := map[string]string{}
	for k, v := range a.values {
		out[k] = v
	}
	return out
}

type aliasTargetErr string

func (e aliasTargetErr) Error() string { return string(e) }

type startupNotifySender struct {
	chatID string
	text   string
	silent bool
}

func (s *startupNotifySender) SendText(_ context.Context, chatID, text string, silent bool) error {
	s.chatID = chatID
	s.text = text
	s.silent = silent
	return nil
}

func (s *startupNotifySender) SendFile(_ context.Context, _ string, _ email.Attachment, _ bool) error {
	return nil
}

func TestMaybeHandleAdminAliasCommandSetAndRemove(t *testing.T) {
	a := &testAliasAdmin{values: map[string]string{}}
	dir := t.TempDir()
	file := dir + "/aliases.json"
	admin := &schemes.User{UserId: 42}

	reply, ok := maybeHandleAdminAliasCommand("/alias alerts chatid123.silent", admin, 100, file, a, nil, 100)
	if !ok || !strings.Contains(reply, "Алиас сохранён") {
		t.Fatalf("unexpected set reply: ok=%v reply=%q", ok, reply)
	}
	if got := a.values["alerts"]; got != "chatid123.silent" {
		t.Fatalf("unexpected alias value: %q", got)
	}

	reply, ok = maybeHandleAdminAliasCommand("/unalias alerts", admin, 100, file, a, nil, 100)
	if !ok || !strings.Contains(reply, "Алиас удалён") {
		t.Fatalf("unexpected remove reply: ok=%v reply=%q", ok, reply)
	}
	if _, exists := a.values["alerts"]; exists {
		t.Fatalf("alias should be removed")
	}
}

func TestMaybeHandleAdminAliasCommandAcceptsNumericTarget(t *testing.T) {
	a := &testAliasAdmin{values: map[string]string{}}
	admin := &schemes.User{UserId: 42}

	reply, ok := maybeHandleAdminAliasCommand("/alias admin 260920412", admin, 100, t.TempDir()+"/aliases.json", a, nil, 100)
	if !ok || !strings.Contains(reply, "Алиас сохранён") {
		t.Fatalf("unexpected set reply: ok=%v reply=%q", ok, reply)
	}
	if got := a.values["admin"]; got != "chatid260920412" {
		t.Fatalf("expected numeric target to be normalized to chatid prefix, got %q", got)
	}
}

func TestMaybeHandleAdminAliasCommandRejectsNonAdmin(t *testing.T) {
	a := &testAliasAdmin{values: map[string]string{}}
	if _, ok := maybeHandleAdminAliasCommand("/alias alerts chatid123", &schemes.User{UserId: 5}, 101, t.TempDir()+"/aliases.json", a, nil, 100); ok {
		t.Fatalf("expected non-admin chat id command to be ignored")
	}
}

func TestMaybeHandleAdminAliasCommandStats7d(t *testing.T) {
	reporter := &testStatsReporter{report: "Статистика за 7 дней"}
	reply, ok := maybeHandleAdminAliasCommand("/stats7d", &schemes.User{UserId: 42}, 100, "", nil, reporter, 100)
	if !ok || reply != "Статистика за 7 дней" {
		t.Fatalf("unexpected stats reply: ok=%v reply=%q", ok, reply)
	}
}

func TestMaybeHandleAdminAliasCommandStats30d(t *testing.T) {
	reporter := &testStatsReporter{report: "Статистика за 30 дней"}
	reply, ok := maybeHandleAdminAliasCommand("/stats30d", &schemes.User{UserId: 42}, 100, "", nil, reporter, 100)
	if !ok || reply != "Статистика за 30 дней" {
		t.Fatalf("unexpected stats reply: ok=%v reply=%q", ok, reply)
	}
}

func TestMaybeHandleAdminAliasCommandAliasesList(t *testing.T) {
	a := &testAliasAdmin{values: map[string]string{"alerts": "chatid123.silent", "ops": "chatid-77"}}
	reply, ok := maybeHandleAdminAliasCommand("/aliases", &schemes.User{UserId: 42}, 100, "", a, nil, 100)
	if !ok {
		t.Fatalf("expected command to be handled")
	}
	if !strings.Contains(reply, "Алиасы (имя -> chatid -> чат):") {
		t.Fatalf("unexpected list header: %q", reply)
	}
	if !strings.Contains(reply, "- alerts -> 123 -> (название чата недоступно через Bot API)") {
		t.Fatalf("expected alerts entry, got %q", reply)
	}
	if !strings.Contains(reply, "- ops -> -77 -> (название чата недоступно через Bot API)") {
		t.Fatalf("expected ops entry, got %q", reply)
	}
}

func TestMaybeHandleAdminAliasCommandDLQ(t *testing.T) {
	d := &testDLQAdmin{summary: "DLQ summary", list: "DLQ list", replay: "replay"}
	admin := &schemes.User{UserId: 42}
	reply, ok := maybeHandleAdminAliasCommandWithDLQ(context.Background(), "/dlq", admin, 100, "", nil, nil, d, 100)
	if !ok || reply != "DLQ summary" {
		t.Fatalf("unexpected /dlq reply: ok=%v reply=%q", ok, reply)
	}

	reply, ok = maybeHandleAdminAliasCommandWithDLQ(context.Background(), "/dlq_list 5", admin, 100, "", nil, nil, d, 100)
	if !ok || reply != "DLQ list:5" {
		t.Fatalf("unexpected /dlq_list reply: ok=%v reply=%q", ok, reply)
	}

	reply, ok = maybeHandleAdminAliasCommandWithDLQ(context.Background(), "/replay abc123", admin, 100, "", nil, nil, d, 100)
	if !ok || reply != "replay:abc123" {
		t.Fatalf("unexpected /replay reply: ok=%v reply=%q", ok, reply)
	}
}

func TestBuildAliasesListReplyEmpty(t *testing.T) {
	if got := buildAliasesListReply(nil); got != "Список алиасов пуст" {
		t.Fatalf("unexpected empty reply: %q", got)
	}
}

func TestNormalizeAliasName(t *testing.T) {
	if got := normalizeAliasName(" Alerts_1 "); got != "alerts_1" {
		t.Fatalf("unexpected alias normalize: %q", got)
	}
	if got := normalizeAliasName("bad name"); got != "" {
		t.Fatalf("expected invalid alias name")
	}
}

func TestNormalizeAliasTarget(t *testing.T) {
	got, err := normalizeAliasTarget("260920412.silent")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "chatid260920412.silent" {
		t.Fatalf("unexpected normalized target: %q", got)
	}
	if _, err := normalizeAliasTarget("bad-target"); err == nil {
		t.Fatalf("expected validation error for bad target")
	}
}

func TestSendStartupNotification(t *testing.T) {
	originalBuild := version.BuildNumber
	version.BuildNumber = "99"
	t.Cleanup(func() { version.BuildNumber = originalBuild })

	sender := &startupNotifySender{}
	if err := SendStartupNotification(context.Background(), sender, 321); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sender.chatID != "321" {
		t.Fatalf("unexpected chat id: %q", sender.chatID)
	}
	expected := fmt.Sprintf("✅ smtp-to-max-relay запущен. Версия бота: %s", version.BotVersion())
	if sender.text != expected {
		t.Fatalf("unexpected text: %q", sender.text)
	}
	if !sender.silent {
		t.Fatalf("expected silent notification")
	}
}

func TestSendStartupNotificationNoop(t *testing.T) {
	sender := &startupNotifySender{}
	if err := SendStartupNotification(context.Background(), sender, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sender.chatID != "" || sender.text != "" {
		t.Fatalf("expected no message to be sent")
	}
}
