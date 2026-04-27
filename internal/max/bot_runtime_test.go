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

type testStatsReporter struct{ report string }

type testDLQAdmin struct {
	summary string
	list    string
	show    string
	replay  string
	dry     string
	batch   string
}

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
func (d *testDLQAdmin) Show(id string) string                    { return fmt.Sprintf("%s:%s", d.show, id) }
func (d *testDLQAdmin) Replay(ctx context.Context, id string) string {
	_ = ctx
	return fmt.Sprintf("%s:%s", d.replay, id)
}
func (d *testDLQAdmin) ReplayDry(ctx context.Context, id string) string {
	_ = ctx
	return fmt.Sprintf("%s:%s", d.dry, id)
}
func (d *testDLQAdmin) ReplayBatch(ctx context.Context, limit int, mode string) string {
	_ = ctx
	return fmt.Sprintf("%s:%d:%s", d.batch, limit, mode)
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
		existing = append(existing, item)
		seen[item] = struct{}{}
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
func (a *testAliasAdmin) DeleteAlias(alias string) { delete(a.values, alias) }
func (a *testAliasAdmin) SnapshotAliases() map[string][]string {
	out := map[string][]string{}
	for k, v := range a.values {
		out[k] = append([]string(nil), v...)
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

func TestAliasAndGroupCommands(t *testing.T) {
	a := &testAliasAdmin{values: map[string][]string{}}
	auth := &testAuthorizer{alias: true}
	file := t.TempDir() + "/aliases.json"
	admin := &schemes.User{UserId: 42}

	reply, ok := maybeHandleAdminAliasCommand("/alias alerts chatid123.silent", admin, 100, file, a, nil, auth)
	if !ok || !strings.Contains(reply, "Алиас сохранён") {
		t.Fatalf("unexpected alias reply: %q", reply)
	}

	reply, ok = maybeHandleAdminAliasCommand("/alias_group ops chatid1,chatid2.silent", admin, 100, file, a, nil, auth)
	if !ok || !strings.Contains(reply, "Группа алиаса сохранена") {
		t.Fatalf("unexpected alias_group reply: %q", reply)
	}
	reply, ok = maybeHandleAdminAliasCommand("/alias_add ops chatid3", admin, 100, file, a, nil, auth)
	if !ok || !strings.Contains(reply, "Target'ы добавлены") {
		t.Fatalf("unexpected alias_add reply: %q", reply)
	}
	reply, ok = maybeHandleAdminAliasCommand("/alias_remove ops chatid2.silent", admin, 100, file, a, nil, auth)
	if !ok || !strings.Contains(reply, "Target'ы удалены") {
		t.Fatalf("unexpected alias_remove reply: %q", reply)
	}
}

func TestACLCommands(t *testing.T) {
	denied := &testAuthorizer{}
	if reply, ok := maybeHandleAdminAliasCommand("/grant alias_admin user 10", &schemes.User{UserId: 1}, 100, "", nil, nil, denied); !ok || reply != "Недостаточно прав" {
		t.Fatalf("expected denied for grant, got ok=%v reply=%q", ok, reply)
	}

	auth := &testAuthorizer{superAdmin: true, whoami: "user_id=1 chat_id=100 roles=super_admin"}
	if reply, ok := maybeHandleAdminAliasCommand("/grant alias_admin user 10", &schemes.User{UserId: 1}, 100, "", nil, nil, auth); !ok || reply != "Права выданы" {
		t.Fatalf("unexpected grant: %q", reply)
	}
	if reply, ok := maybeHandleAdminAliasCommand("/revoke alias_admin user 10", &schemes.User{UserId: 1}, 100, "", nil, nil, auth); !ok || reply != "Права отозваны" {
		t.Fatalf("unexpected revoke: %q", reply)
	}
	if reply, ok := maybeHandleAdminAliasCommand("/whoami", &schemes.User{UserId: 1}, 100, "", nil, nil, auth); !ok || !strings.Contains(reply, "roles=super_admin") {
		t.Fatalf("unexpected whoami: %q", reply)
	}
}

func TestDLQCommandsAndConfirm(t *testing.T) {
	auth := &testAuthorizer{dlq: true}
	d := &testDLQAdmin{summary: "DLQ summary", list: "DLQ list", show: "show", replay: "replay", dry: "dry", batch: "batch"}
	admin := &schemes.User{UserId: 42}

	cases := []string{"/dlq", "/dlq_list 5", "/dlq_show id1", "/replay_dry id1"}
	for _, cmd := range cases {
		if _, ok := maybeHandleAdminAliasCommandWithDLQ(context.Background(), cmd, admin, 100, "", nil, nil, d, auth); !ok {
			t.Fatalf("expected command handled: %s", cmd)
		}
	}

	reply, ok := maybeHandleAdminAliasCommandWithDLQ(context.Background(), "/replay abc", admin, 100, "", nil, nil, d, auth)
	if !ok || !strings.Contains(reply, "Требуется подтверждение") {
		t.Fatalf("unexpected replay confirmation: %q", reply)
	}
	token := strings.TrimSpace(strings.TrimPrefix(strings.Split(reply, "(")[0], "Требуется подтверждение: /confirm"))
	reply, ok = maybeHandleAdminAliasCommandWithDLQ(context.Background(), "/confirm "+token, admin, 100, "", nil, nil, d, auth)
	if !ok || reply != "replay:abc" {
		t.Fatalf("unexpected confirm replay reply: %q", reply)
	}
}

func TestEveryAdminCommandChecksPermission(t *testing.T) {
	commands := []string{"/alias a chatid1", "/alias_group a chatid1,chatid2", "/alias_add a chatid1", "/alias_remove a chatid1", "/unalias a", "/aliases", "/stats7d", "/stats30d", "/dlq", "/dlq_list", "/dlq_show id1", "/replay x", "/replay_dry x", "/replay_batch 1", "/confirm x", "/grant alias_admin user 1", "/revoke alias_admin user 1", "/whoami"}
	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			reply, ok := maybeHandleAdminAliasCommandWithDLQ(context.Background(), cmd, &schemes.User{UserId: 42}, 100, t.TempDir()+"/aliases.json", &testAliasAdmin{values: map[string][]string{}}, &testStatsReporter{report: "r"}, &testDLQAdmin{summary: "s", list: "l", show: "sh", replay: "r", dry: "d", batch: "b"}, &testAuthorizer{})
			if !ok || reply != "Недостаточно прав" {
				t.Fatalf("cmd=%s expected denied, got ok=%v reply=%q", cmd, ok, reply)
			}
		})
	}
}

func TestBuildAliasesListReplyEmpty(t *testing.T) {
	if got := buildAliasesListReply(nil); got != "Список алиасов пуст" {
		t.Fatalf("unexpected empty reply: %q", got)
	}
}

func TestNormalizeAliasHelpers(t *testing.T) {
	if got := normalizeAliasName(" Alerts_1 "); got != "alerts_1" {
		t.Fatalf("unexpected alias normalize: %q", got)
	}
	if got := normalizeAliasName("bad name"); got != "" {
		t.Fatalf("expected invalid alias name")
	}
	got, err := normalizeAliasTarget("260920412.silent")
	if err != nil || got != "chatid260920412.silent" {
		t.Fatalf("unexpected normalized target: %q err=%v", got, err)
	}
	targets, err := normalizeAliasTargetsArg("260920412,chatid-77.silent,260920412")
	if err != nil || len(targets) != 2 {
		t.Fatalf("unexpected normalized targets: %#v err=%v", targets, err)
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
