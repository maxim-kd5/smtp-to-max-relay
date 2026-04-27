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

type testAliasAdmin struct{ values map[string]string }
type testStatsReporter struct{ report string }
type testDLQAdmin struct{ summary, list, replay string }

type testAuthorizer struct {
	superAdmin bool
	alias      bool
	dlq        bool
	stats      bool
	whoami     string
	grantErr   error
	revokeErr  error
}

func (s *testStatsReporter) BuildLastDaysReport(days int) string { return s.report }
func (d *testDLQAdmin) Summary() string                          { return d.summary }
func (d *testDLQAdmin) List(limit int) string                    { return fmt.Sprintf("%s:%d", d.list, limit) }
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
func (a *testAliasAdmin) DeleteAlias(alias string) { delete(a.values, alias) }
func (a *testAliasAdmin) SnapshotAliases() map[string]string {
	out := map[string]string{}
	for k, v := range a.values {
		out[k] = v
	}
	return out
}
func (a *testAuthorizer) CanManageAliases(_, _ int64) bool  { return a.alias }
func (a *testAuthorizer) CanReplayDLQ(_, _ int64) bool      { return a.dlq }
func (a *testAuthorizer) CanViewStats(_, _ int64) bool      { return a.stats }
func (a *testAuthorizer) IsSuperAdmin(_, _ int64) bool      { return a.superAdmin }
func (a *testAuthorizer) Grant(_, _ string, _ int64) error  { return a.grantErr }
func (a *testAuthorizer) Revoke(_, _ string, _ int64) error { return a.revokeErr }
func (a *testAuthorizer) WhoAmI(_, _ int64) string          { return a.whoami }

type aliasTargetErr string

func (e aliasTargetErr) Error() string { return string(e) }

type startupNotifySender struct {
	chatID, text string
	silent       bool
}

func (s *startupNotifySender) SendText(_ context.Context, chatID, text string, silent bool) error {
	s.chatID, s.text, s.silent = chatID, text, silent
	return nil
}
func (s *startupNotifySender) SendFile(_ context.Context, _ string, _ email.Attachment, _ bool) error {
	return nil
}

func TestAliasCommandRequiresPermission(t *testing.T) {
	a := &testAliasAdmin{values: map[string]string{}}
	reply, ok := maybeHandleAdminAliasCommand("/alias alerts chatid123", &schemes.User{UserId: 42}, 100, t.TempDir()+"/aliases.json", a, nil, &testAuthorizer{})
	if !ok || reply != "Недостаточно прав" {
		t.Fatalf("unexpected reply: ok=%v %q", ok, reply)
	}
}

func TestAliasCommandSetAndRemove(t *testing.T) {
	a := &testAliasAdmin{values: map[string]string{}}
	auth := &testAuthorizer{alias: true}
	file := t.TempDir() + "/aliases.json"
	reply, ok := maybeHandleAdminAliasCommand("/alias alerts chatid123.silent", &schemes.User{UserId: 42}, 100, file, a, nil, auth)
	if !ok || !strings.Contains(reply, "Алиас сохранён") {
		t.Fatalf("unexpected set reply: %q", reply)
	}
	reply, ok = maybeHandleAdminAliasCommand("/unalias alerts", &schemes.User{UserId: 42}, 100, file, a, nil, auth)
	if !ok || !strings.Contains(reply, "Алиас удалён") {
		t.Fatalf("unexpected remove reply: %q", reply)
	}
}

func TestStatsCommandRequiresPermission(t *testing.T) {
	reply, ok := maybeHandleAdminAliasCommand("/stats7d", &schemes.User{UserId: 42}, 100, "", nil, &testStatsReporter{report: "ok"}, &testAuthorizer{})
	if !ok || reply != "Недостаточно прав" {
		t.Fatalf("unexpected: ok=%v %q", ok, reply)
	}
}

func TestDLQCommandRequiresPermission(t *testing.T) {
	reply, ok := maybeHandleAdminAliasCommandWithDLQ(context.Background(), "/replay id1", &schemes.User{UserId: 42}, 100, "", nil, nil, &testDLQAdmin{replay: "x"}, &testAuthorizer{})
	if !ok || reply != "Недостаточно прав" {
		t.Fatalf("unexpected: ok=%v %q", ok, reply)
	}
}

func TestACLCommandsSuperAdminOnly(t *testing.T) {
	reply, ok := maybeHandleAdminAliasCommand("/grant alias_admin user 10", &schemes.User{UserId: 1}, 100, "", nil, nil, &testAuthorizer{})
	if !ok || reply != "Недостаточно прав" {
		t.Fatalf("unexpected: %q", reply)
	}
}

func TestACLCommands(t *testing.T) {
	auth := &testAuthorizer{superAdmin: true, whoami: "user_id=1 chat_id=100 roles=super_admin"}
	reply, ok := maybeHandleAdminAliasCommand("/grant alias_admin user 10", &schemes.User{UserId: 1}, 100, "", nil, nil, auth)
	if !ok || reply != "Права выданы" {
		t.Fatalf("unexpected grant: %q", reply)
	}
	reply, ok = maybeHandleAdminAliasCommand("/revoke alias_admin user 10", &schemes.User{UserId: 1}, 100, "", nil, nil, auth)
	if !ok || reply != "Права отозваны" {
		t.Fatalf("unexpected revoke: %q", reply)
	}
	reply, ok = maybeHandleAdminAliasCommand("/whoami", &schemes.User{UserId: 1}, 100, "", nil, nil, auth)
	if !ok || !strings.Contains(reply, "roles=super_admin") {
		t.Fatalf("unexpected whoami: %q", reply)
	}
}

func TestMaybeHandleAdminAliasCommandAcceptsNumericTarget(t *testing.T) {
	a := &testAliasAdmin{values: map[string]string{}}
	reply, ok := maybeHandleAdminAliasCommand("/alias admin 260920412", &schemes.User{UserId: 42}, 100, t.TempDir()+"/aliases.json", a, nil, &testAuthorizer{alias: true})
	if !ok || !strings.Contains(reply, "Алиас сохранён") {
		t.Fatalf("unexpected set reply: ok=%v reply=%q", ok, reply)
	}
}

func TestMaybeHandleAdminAliasCommandAliasesList(t *testing.T) {
	a := &testAliasAdmin{values: map[string]string{"alerts": "chatid123.silent", "ops": "chatid-77"}}
	reply, ok := maybeHandleAdminAliasCommand("/aliases", &schemes.User{UserId: 42}, 100, "", a, nil, &testAuthorizer{alias: true})
	if !ok || !strings.Contains(reply, "Алиасы") {
		t.Fatalf("unexpected list reply: %q", reply)
	}
}

func TestMaybeHandleAdminAliasCommandDLQ(t *testing.T) {
	d := &testDLQAdmin{summary: "DLQ summary", list: "DLQ list", replay: "replay"}
	auth := &testAuthorizer{dlq: true}
	reply, ok := maybeHandleAdminAliasCommandWithDLQ(context.Background(), "/dlq", &schemes.User{UserId: 42}, 100, "", nil, nil, d, auth)
	if !ok || reply != "DLQ summary" {
		t.Fatalf("unexpected /dlq reply: %q", reply)
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
	if err != nil || got != "chatid260920412.silent" {
		t.Fatalf("unexpected normalized target: %q err=%v", got, err)
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
	if sender.chatID != "321" || !sender.silent {
		t.Fatalf("unexpected send args")
	}
}

func TestEveryAdminCommandChecksPermission(t *testing.T) {
	commands := []string{"/alias a chatid1", "/unalias a", "/aliases", "/stats7d", "/stats30d", "/dlq", "/dlq_list", "/replay x", "/grant alias_admin user 1", "/revoke alias_admin user 1", "/whoami"}
	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			reply, ok := maybeHandleAdminAliasCommandWithDLQ(context.Background(), cmd, &schemes.User{UserId: 42}, 100, t.TempDir()+"/aliases.json", &testAliasAdmin{values: map[string]string{}}, &testStatsReporter{report: "r"}, &testDLQAdmin{summary: "s", list: "l", replay: "r"}, &testAuthorizer{})
			if !ok || reply != "Недостаточно прав" {
				t.Fatalf("cmd=%s expected denied, got ok=%v reply=%q", cmd, ok, reply)
			}
		})
	}
}
