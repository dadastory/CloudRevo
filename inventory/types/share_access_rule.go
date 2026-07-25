package types

import (
	"encoding/json"
	"fmt"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	lru "github.com/hashicorp/golang-lru/v2"
)

const (
	shareAccessObject = "share"

	// MaxShareAccessRuleAudiences bounds attacker-controlled policy construction.
	// The limit applies independently to exact user and group audiences.
	MaxShareAccessRuleAudiences = 256

	shareAccessAnonymousSubject     = "audience:anonymous"
	shareAccessAuthenticatedSubject = "audience:authenticated"
	shareAccessUserPriority         = "10"
	shareAccessGroupPriority        = "20"
	shareAccessAudiencePriority     = "30"
	shareAccessEvaluatorCacheSize   = 32
)

var shareAccessActions = []struct {
	name  string
	grant func(SharePermission) bool
}{
	{name: "read", grant: func(p SharePermission) bool { return p.Read }},
	{name: "create", grant: func(p SharePermission) bool { return p.Create }},
	{name: "update", grant: func(p SharePermission) bool { return p.Update }},
	{name: "delete", grant: func(p SharePermission) bool { return p.Delete }},
}

const shareAccessModel = `
[request_definition]
r = sub, grp, obj, act

[policy_definition]
p = priority, sub, obj, act, eft

[policy_effect]
e = priority(p.eft) || deny

[matchers]
m = (r.sub == p.sub || r.grp == p.sub || (r.sub != "audience:anonymous" && p.sub == "audience:authenticated")) && r.obj == p.obj && r.act == p.act
`

// shareAccessEnforcerCache holds immutable Casbin policy evaluators. Rules are
// persisted as JSON on files, so the cache is only an optimization and never
// an authorization source of truth. The LRU bounds memory under repeated rule
// changes or maliciously diverse inputs.
var shareAccessEnforcerCache, _ = lru.New[string, *casbin.SyncedEnforcer](shareAccessEvaluatorCacheSize)

// Resolve returns the most-specific permission granted to a share visitor.
// A direct user grant, including an explicitly empty one, overrides a group
// grant. Anonymous visitors never inherit authenticated permissions.
func (r ShareAccessRule) Resolve(userID, groupID int, anonymous bool) SharePermission {
	if len(r.Users) > MaxShareAccessRuleAudiences || len(r.Groups) > MaxShareAccessRuleAudiences {
		return SharePermission{}
	}
	permission, err := r.casbinPermission(userID, groupID, anonymous)
	if err != nil {
		return SharePermission{}
	}

	return permission
}

// casbinPermission translates the persisted rule into an immutable in-memory
// Casbin policy. The JSON rule remains the only source of truth; no second
// policy store or authorization service is needed.
func (r ShareAccessRule) casbinPermission(userID, groupID int, anonymous bool) (SharePermission, error) {
	key, err := r.casbinCacheKey()
	if err != nil {
		return SharePermission{}, err
	}
	enforcer, ok := shareAccessEnforcerCache.Get(key)
	if !ok {
		enforcer, err = newShareAccessEnforcer(r)
		if err != nil {
			return SharePermission{}, err
		}
		shareAccessEnforcerCache.Add(key, enforcer)
	}

	visitor := shareAccessAnonymousSubject
	visitorGroup := ""
	if !anonymous {
		visitor = shareAccessUserSubject(userID)
		if groupID > 0 {
			visitorGroup = shareAccessGroupSubject(groupID)
		}
	}

	permission := SharePermission{}
	for _, action := range shareAccessActions {
		granted, err := enforcer.Enforce(visitor, visitorGroup, shareAccessObject, action.name)
		if err != nil {
			return SharePermission{}, fmt.Errorf("enforce Casbin %s permission: %w", action.name, err)
		}
		switch action.name {
		case "read":
			permission.Read = granted
		case "create":
			permission.Create = granted
		case "update":
			permission.Update = granted
		case "delete":
			permission.Delete = granted
		}
	}

	return permission, nil
}

func (r ShareAccessRule) casbinCacheKey() (string, error) {
	key, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("marshal Casbin access rule: %w", err)
	}
	return string(key), nil
}

func newShareAccessEnforcer(r ShareAccessRule) (*casbin.SyncedEnforcer, error) {
	accessModel, err := model.NewModelFromString(shareAccessModel)
	if err != nil {
		return nil, fmt.Errorf("create Casbin model: %w", err)
	}

	enforcer, err := casbin.NewSyncedEnforcer(accessModel)
	if err != nil {
		return nil, fmt.Errorf("create Casbin enforcer: %w", err)
	}

	for id, permission := range r.Users {
		if err := addShareAccessPolicies(enforcer, shareAccessUserSubject(id), shareAccessUserPriority, permission); err != nil {
			return nil, err
		}
	}
	for id, permission := range r.Groups {
		if err := addShareAccessPolicies(enforcer, shareAccessGroupSubject(id), shareAccessGroupPriority, permission); err != nil {
			return nil, err
		}
	}
	if err := addShareAccessPolicies(enforcer, shareAccessAuthenticatedSubject, shareAccessAudiencePriority, r.Authenticated); err != nil {
		return nil, err
	}
	if err := addShareAccessPolicies(enforcer, shareAccessAnonymousSubject, shareAccessAudiencePriority, r.Anonymous); err != nil {
		return nil, err
	}

	return enforcer, nil
}

func addShareAccessPolicies(enforcer *casbin.SyncedEnforcer, subject, priority string, permission SharePermission) error {
	for _, action := range shareAccessActions {
		effect := "deny"
		if action.grant(permission) {
			effect = "allow"
		}
		if _, err := enforcer.AddPolicy(priority, subject, shareAccessObject, action.name, effect); err != nil {
			return fmt.Errorf("add Casbin %s policy: %w", action.name, err)
		}
	}
	return nil
}

func shareAccessUserSubject(id int) string {
	return fmt.Sprintf("user:%d", id)
}

func shareAccessGroupSubject(id int) string {
	return fmt.Sprintf("group:%d", id)
}

// Validate rejects impossible audience keys while allowing an empty grant as
// an explicit denial that can override a broader matching audience.
func (r ShareAccessRule) Validate() error {
	if len(r.Users) > MaxShareAccessRuleAudiences {
		return fmt.Errorf("share access-rule has too many users")
	}
	if len(r.Groups) > MaxShareAccessRuleAudiences {
		return fmt.Errorf("share access-rule has too many groups")
	}
	for id := range r.Users {
		if id <= 0 {
			return fmt.Errorf("invalid share access-rule user id %d", id)
		}
	}
	for id := range r.Groups {
		if id <= 0 {
			return fmt.Errorf("invalid share access-rule group id %d", id)
		}
	}
	return nil
}

// IntersectSharePermission returns only actions granted by both rules. It is
// used across independently configured shared rules so a narrower rule can
// never be widened by a link or descendant rule.
func IntersectSharePermission(left, right SharePermission) SharePermission {
	return SharePermission{
		Read:   left.Read && right.Read,
		Create: left.Create && right.Create,
		Update: left.Update && right.Update,
		Delete: left.Delete && right.Delete,
	}
}
