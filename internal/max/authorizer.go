package max

type ACLAuthorizer interface {
	CanManageAliases(userID, chatID int64) bool
	CanReplayDLQ(userID, chatID int64) bool
	CanViewStats(userID, chatID int64) bool
	IsSuperAdmin(userID, chatID int64) bool
	Grant(roleName, scope string, id int64) error
	Revoke(roleName, scope string, id int64) error
	WhoAmI(userID, chatID int64) string
}
