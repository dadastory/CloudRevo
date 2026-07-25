package explorer

import (
	"fmt"

	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/hashid"
)

// ShareAccessRule is the public representation of a share access rule. User
// and group map keys use opaque hash IDs rather than database primary keys.
type ShareAccessRule struct {
	Anonymous     types.SharePermission            `json:"anonymous"`
	Authenticated types.SharePermission            `json:"authenticated"`
	Users         map[string]types.SharePermission `json:"users"`
	Groups        map[string]types.SharePermission `json:"groups"`
}

func (r *ShareAccessRule) ToInternal(encoder hashid.Encoder) (*types.ShareAccessRule, error) {
	if r == nil {
		return nil, nil
	}
	// Check cardinality before allocating maps or decoding externally supplied
	// hash IDs. This keeps malformed public payloads bounded.
	if len(r.Users) > types.MaxShareAccessRuleAudiences {
		return nil, fmt.Errorf("share access-rule has too many users")
	}
	if len(r.Groups) > types.MaxShareAccessRuleAudiences {
		return nil, fmt.Errorf("share access-rule has too many groups")
	}

	internal := &types.ShareAccessRule{
		Anonymous:     r.Anonymous,
		Authenticated: r.Authenticated,
		Users:         make(map[int]types.SharePermission, len(r.Users)),
		Groups:        make(map[int]types.SharePermission, len(r.Groups)),
	}
	for id, permission := range r.Users {
		decoded, err := encoder.Decode(id, hashid.UserID)
		if err != nil {
			return nil, fmt.Errorf("invalid share access-rule user %q: %w", id, err)
		}
		internal.Users[decoded] = permission
	}
	for id, permission := range r.Groups {
		decoded, err := encoder.Decode(id, hashid.GroupID)
		if err != nil {
			return nil, fmt.Errorf("invalid share access-rule group %q: %w", id, err)
		}
		internal.Groups[decoded] = permission
	}
	if err := internal.Validate(); err != nil {
		return nil, err
	}

	return internal, nil
}

func BuildShareAccessRule(rule *types.ShareAccessRule, encoder hashid.Encoder) *ShareAccessRule {
	if rule == nil {
		return nil
	}

	res := &ShareAccessRule{
		Anonymous:     rule.Anonymous,
		Authenticated: rule.Authenticated,
		Users:         make(map[string]types.SharePermission, len(rule.Users)),
		Groups:        make(map[string]types.SharePermission, len(rule.Groups)),
	}
	for id, permission := range rule.Users {
		res.Users[hashid.EncodeUserID(encoder, id)] = permission
	}
	for id, permission := range rule.Groups {
		res.Groups[hashid.EncodeGroupID(encoder, id)] = permission
	}

	return res
}
