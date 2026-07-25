package user

import (
	"context"
	"strings"

	"github.com/dadastory/CloudRevo/application/dependency"
	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/inventory"
	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/filemanager/fs"
	"github.com/dadastory/CloudRevo/pkg/filemanager/manager"
	"github.com/dadastory/CloudRevo/pkg/hashid"
	"github.com/dadastory/CloudRevo/pkg/serializer"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

func GetUser(c *gin.Context) (*ent.User, error) {
	uid := hashid.FromContext(c)
	dep := dependency.FromContext(c)
	userClient := dep.UserClient()
	ctx := context.WithValue(c, inventory.LoadUserGroup{}, true)
	return userClient.GetByID(ctx, uid)
}

func GetUserCapacity(c *gin.Context) (*fs.Capacity, error) {
	user := inventory.UserFromContext(c)
	dep := dependency.FromContext(c)
	m := manager.NewFileManager(dep, user)
	defer m.Recycle()

	return m.Capacity(c)
}

type (
	SearchUserService struct {
		Keyword string `form:"keyword" binding:"required,min=2"`
	}
	SearchUserParamCtx      struct{}
	SearchShareGroupService struct {
		Keyword string `form:"keyword" binding:"required,min=2"`
	}
	SearchShareGroupParamCtx  struct{}
	ResolveShareGroupsService struct {
		IDs []string `json:"ids" binding:"max=256"`
	}
	ResolveShareGroupsParamCtx struct{}
)

const (
	resultLimit            = 10
	groupSearchResultLimit = 20
)

func (s *SearchUserService) Search(c *gin.Context) ([]*ent.User, error) {
	dep := dependency.FromContext(c)
	userClient := dep.UserClient()
	res, err := userClient.SearchActive(c, resultLimit, s.Keyword)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to search user", err)
	}

	return res, nil
}

// Search returns a bounded, case-insensitive group result for the permission editor.
func (s *SearchShareGroupService) Search(c *gin.Context) ([]*ent.Group, error) {
	keyword := strings.TrimSpace(s.Keyword)
	if len([]rune(keyword)) < 2 {
		return nil, serializer.NewError(serializer.CodeParamErr, "group search keyword is too short", nil)
	}

	dep := dependency.FromContext(c)
	res, err := dep.GroupClient().Search(c, keyword, groupSearchResultLimit)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "failed to search groups", err)
	}

	return lo.Filter(res, func(g *ent.Group, _ int) bool {
		return g.ID != inventory.AnonymousGroupID
	}), nil
}

// Resolve loads the current identity of exact group audiences stored as public hash IDs.
func (s *ResolveShareGroupsService) Resolve(c *gin.Context) ([]*ent.Group, error) {
	if len(s.IDs) > types.MaxShareAccessRuleAudiences {
		return nil, serializer.NewError(serializer.CodeParamErr, "too many group identities", nil)
	}
	dep := dependency.FromContext(c)
	ids := make([]int, 0, len(s.IDs))
	seen := make(map[int]struct{}, len(s.IDs))
	for _, encoded := range s.IDs {
		id, err := dep.HashIDEncoder().Decode(encoded, hashid.GroupID)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "invalid group identity", err)
		}
		if id == inventory.AnonymousGroupID {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	res, err := dep.GroupClient().GetByIDs(c, ids)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "failed to resolve groups", err)
	}

	return res, nil
}
