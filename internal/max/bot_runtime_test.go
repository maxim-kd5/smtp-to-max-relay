package max

import (
	"strings"
	"testing"

	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

type testAliasAdmin struct {
	values map[string]string
}

type testStatsReporter struct {
	report string
}

func (s *testStatsReporter) BuildLastDaysReport(days int) string {
	return s.report
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

func TestNormalizeAliasName(t *testing.T) {
	if got := normalizeAliasName(" Alerts_1 "); got != "alerts_1" {
		t.Fatalf("unexpected alias normalize: %q", got)
	}
	if got := normalizeAliasName("bad name"); got != "" {
		t.Fatalf("expected invalid alias name")
	}
}
