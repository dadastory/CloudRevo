package types

import (
	"sync"
	"testing"
)

func TestShareAccessRuleCasbinPermissionPrefersExplicitUserDeny(t *testing.T) {
	rule := ShareAccessRule{
		Authenticated: SharePermission{Read: true, Create: true},
		Groups: map[int]SharePermission{
			2: {Read: true, Update: true, Delete: true},
		},
		Users: map[int]SharePermission{
			7: {},
		},
	}

	got, err := rule.casbinPermission(7, 2, false)
	if err != nil {
		t.Fatalf("evaluate Casbin policy: %v", err)
	}
	if got.Read || got.Create || got.Update || got.Delete {
		t.Fatalf("explicit user denial must override group and authenticated grants, got %#v", got)
	}
}

func TestShareAccessRuleCasbinPermissionKeepsAnonymousActionsIndependent(t *testing.T) {
	rule := ShareAccessRule{
		Anonymous: SharePermission{Read: true, Update: true},
	}

	got, err := rule.casbinPermission(0, 0, true)
	if err != nil {
		t.Fatalf("evaluate Casbin policy: %v", err)
	}
	if !got.Read || !got.Update || got.Create || got.Delete {
		t.Fatalf("Casbin must preserve partial anonymous grants, got %#v", got)
	}
}

func TestShareAccessRuleReusesBoundedCasbinEvaluator(t *testing.T) {
	rule := ShareAccessRule{Authenticated: SharePermission{Read: true, Update: true}}
	_ = rule.Resolve(42, 0, false)
	afterFirstResolve := shareAccessEnforcerCache.Len()
	_ = rule.Resolve(42, 0, false)
	if got := shareAccessEnforcerCache.Len(); got != afterFirstResolve {
		t.Fatalf("resolving an unchanged rule must reuse its cached evaluator, got cache length %d after %d", got, afterFirstResolve)
	}
	if afterFirstResolve > shareAccessEvaluatorCacheSize {
		t.Fatalf("evaluator cache must remain bounded, got %d", afterFirstResolve)
	}
}

func TestShareAccessRuleCachesSynchronizedEvaluatorForConcurrentReads(t *testing.T) {
	rule := ShareAccessRule{Authenticated: SharePermission{Read: true, Update: true}}
	if _, err := rule.casbinPermission(42, 0, false); err != nil {
		t.Fatalf("prime evaluator: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 64; j++ {
				got, err := rule.casbinPermission(42, 0, false)
				if err != nil || !got.Read || !got.Update || got.Create || got.Delete {
					t.Errorf("concurrent resolve = %#v, %v", got, err)
					return
				}
			}
		}()
	}
	wg.Wait()

	for i := 0; i < 33; i++ {
		if _, err := (ShareAccessRule{Users: map[int]SharePermission{i + 1: {Read: true}}}).casbinPermission(i+1, 0, false); err != nil {
			t.Fatalf("build evaluator %d: %v", i, err)
		}
	}
	if got := shareAccessEnforcerCache.Len(); got > 32 {
		t.Fatalf("cached evaluators must retain at most 32 rules, got %d", got)
	}
}

func TestShareAccessRuleCachesConcurrentMissesAndEvictions(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < shareAccessEvaluatorCacheSize*4; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rule := ShareAccessRule{Users: map[int]SharePermission{id: {Read: true}}}
			got, err := rule.casbinPermission(id, 0, false)
			if err != nil || !got.Read {
				t.Errorf("concurrent cache miss %d = %#v, %v", id, got, err)
			}
		}(i + 1)
	}
	wg.Wait()
	if got := shareAccessEnforcerCache.Len(); got > shareAccessEvaluatorCacheSize {
		t.Fatalf("cached evaluators must retain at most %d rules, got %d", shareAccessEvaluatorCacheSize, got)
	}
}
