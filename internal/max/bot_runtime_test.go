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
	values map[string][]string
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

func (a *testAliasAdmin) SetAliasGroup(alias string, targets []string) {
	if a.values == nil {
		a.values = map[string][]string{}
	}
	a.values[alias] = append([]string(nil), targets...)
}

func (a *testAliasAdmin) AddAliasTargets(alias string, targets []string) {
	existing := a.values[alias]
	seen := map[string]struct{}{}
	for _, item := range existing {
		seen[item] = struct{}{}
	}
	for _, item := range targets {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		existing = append(existing, item)
	}
	a.values[alias] = existing
}

func (a *testAliasAdmin) RemoveAliasTargets(alias string, targets []string) {
	existing := a.values[alias]
	remove := map[string]struct{}{}
	for _, item := range targets {
		remove[item] = struct{}{}
	}
	updated := make([]string, 0, len(existing))
	for _, item := range existing {
		if _, ok := remove[item]; ok {
			continue
		}
		updated = append(updated, item)
	}
	if len(updated) == 0 {
		delete(a.values, alias)
		return
	}
	a.values[alias] = updated
}

func (a *testAliasAdmin) DeleteAlias(alias string) {
	delete(a.values, alias)
}

func (a *testAliasAdmin) SnapshotAliases() map[string][]string {
	out := map[string][]string{}
	for k, v := range a.values {
		out[k] = append([]string(nil), v...)
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
	a := &testAliasAdmin{values: map[string][]string{}}
	dir := t.TempDir()
	file := dir + "/aliases.json"
	admin := &schemes.User{UserId: 42}

	reply, ok := maybeHandleAdminAliasCommand("/alias alerts chatid123.silent", admin, 100, file, a, nil, 100)
	if !ok || !strings.Contains(reply, "Алиас сохранён") {
		t.Fatalf("unexpected set reply: ok=%v reply=%q", ok, reply)
	}
	if got := a.values["alerts"]; len(got) != 1 || got[0] != "chatid123.silent" {
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
	a := &testAliasAdmin{values: map[string][]string{}}
	admin := &schemes.User{UserId: 42}

	reply, ok := maybeHandleAdminAliasCommand("/alias admin 260920412", admin, 100, t.TempDir()+"/aliases.json", a, nil, 100)
	if !ok || !strings.Contains(reply, "Алиас сохранён") {
		t.Fatalf("unexpected set reply: ok=%v reply=%q", ok, reply)
	}
	if got := a.values["admin"]; len(got) != 1 || got[0] != "chatid260920412" {
		t.Fatalf("expected numeric target to be normalized to chatid prefix, got %q", got)
	}
}

func TestMaybeHandleAdminAliasCommandRejectsNonAdmin(t *testing.T) {
	a := &testAliasAdmin{values: map[string][]string{}}
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
	a := &testAliasAdmin{values: map[string][]string{"alerts": []string{"chatid123.silent", "chatid999"}, "ops": []string{"chatid-77"}}}
	reply, ok := maybeHandleAdminAliasCommand("/aliases", &schemes.User{UserId: 42}, 100, "", a, nil, 100)
	if !ok {
		t.Fatalf("expected command to be handled")
	}
	if !strings.Contains(reply, "Алиасы (имя -> chatid -> чат):") {
		t.Fatalf("unexpected list header: %q", reply)
	}
	if !strings.Contains(reply, "- alerts -> 123,999 -> (название чата недоступно через Bot API)") {
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

func TestNormalizeAliasTargetsArg(t *testing.T) {
	got, err := normalizeAliasTargetsArg("260920412,chatid-77.silent,260920412")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 2 || got[0] != "chatid260920412" || got[1] != "chatid-77.silent" {
		t.Fatalf("unexpected normalized targets: %#v", got)
	}
}

func TestMaybeHandleAdminAliasGroupCommands(t *testing.T) {
	a := &testAliasAdmin{values: map[string][]string{}}
	admin := &schemes.User{UserId: 42}
	file := t.TempDir() + "/aliases.json"

	reply, ok := maybeHandleAdminAliasCommand("/alias_group alerts chatid1,chatid2.silent", admin, 100, file, a, nil, 100)
	if !ok || !strings.Contains(reply, "Группа алиаса сохранена") {
		t.Fatalf("unexpected /alias_group reply: ok=%v reply=%q", ok, reply)
	}
	if got := a.values["alerts"]; len(got) != 2 {
		t.Fatalf("unexpected group targets after /alias_group: %#v", got)
	}

	reply, ok = maybeHandleAdminAliasCommand("/alias_add alerts chatid3", admin, 100, file, a, nil, 100)
	if !ok || !strings.Contains(reply, "Target'ы добавлены") {
		t.Fatalf("unexpected /alias_add reply: ok=%v reply=%q", ok, reply)
	}
	if got := a.values["alerts"]; len(got) != 3 {
		t.Fatalf("unexpected targets after /alias_add: %#v", got)
	}

	reply, ok = maybeHandleAdminAliasCommand("/alias_remove alerts chatid2.silent", admin, 100, file, a, nil, 100)
	if !ok || !strings.Contains(reply, "Target'ы удалены") {
		t.Fatalf("unexpected /alias_remove reply: ok=%v reply=%q", ok, reply)
	}
	if got := a.values["alerts"]; len(got) != 2 {
		t.Fatalf("unexpected targets after /alias_remove: %#v", got)
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
