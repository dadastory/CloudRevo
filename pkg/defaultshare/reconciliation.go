// Package defaultshare contains the small scheduling primitives shared by
// request paths and the background default-share shortcut reconciler.
package defaultshare

import (
	"context"
	"fmt"
	"strconv"

	"github.com/dadastory/CloudRevo/inventory"
	"github.com/dadastory/CloudRevo/pkg/cache"
)

const reconciliationPagePrefix = "default_share_reconcile_page_"

// ReconciliationPageKey returns the KV key used by the bounded shortcut
// worker. It is intentionally shared so request paths can schedule a fresh
// sweep without enumerating users or creating shortcut rows.
func ReconciliationPageKey(shareID int) string {
	return reconciliationPagePrefix + strconv.Itoa(shareID)
}

// Restart resets one default-share worker to its first recipient page.
func Restart(kv cache.Driver, shareID int) error {
	return kv.Set(ReconciliationPageKey(shareID), 0, 0)
}

// RestartForFile resets workers for default shares whose source is fileID.
// It only reads share records; active-user enumeration and shortcut mutation
// remain exclusively in the background worker.
func RestartForFile(ctx context.Context, shares inventory.ShareClient, kv cache.Driver, fileID int) error {
	pageToken := ""
	for {
		res, err := shares.List(ctx, &inventory.ListShareArgs{PaginationArgs: &inventory.PaginationArgs{
			UseCursorPagination: true,
			PageToken:           pageToken,
			PageSize:            100,
		}, FileID: fileID, DefaultOnly: true})
		if err != nil {
			return fmt.Errorf("list source shares for reconciliation restart: %w", err)
		}
		for _, share := range res.Shares {
			if err := Restart(kv, share.ID); err != nil {
				return fmt.Errorf("restart default-share reconciliation %d: %w", share.ID, err)
			}
		}
		if res.NextPageToken == "" {
			return nil
		}
		pageToken = res.NextPageToken
	}
}
