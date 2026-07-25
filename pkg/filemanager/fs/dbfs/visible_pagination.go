package dbfs

import (
	"fmt"

	"github.com/dadastory/CloudRevo/inventory"
)

const maxVisibleOffsetItems = 100000

// paginateVisibleChildren applies a disclosure filter before pagination is
// exposed. It advances through physical pages so a hidden row never makes a
// later readable row unreachable.
func paginateVisibleChildren(args *ListArgs, fetch func(*ListArgs) (*ListResult, error), visible func(*File) bool) (*ListResult, error) {
	if args == nil || args.Page == nil {
		return nil, fmt.Errorf("pagination arguments are required")
	}
	var (
		result *ListResult
		err    error
	)
	if args.Page.UseCursorPagination || args.Search != nil {
		result, err = paginateVisibleCursorChildren(args, fetch, visible)
	} else {
		result, err = paginateVisibleOffsetChildren(args, fetch, visible)
	}
	if err != nil {
		return nil, err
	}
	if args.StreamCallback != nil {
		args.StreamCallback(result.Files)
		result.Files = result.Files[:0]
	}
	return result, nil
}

func paginateVisibleCursorChildren(args *ListArgs, fetch func(*ListArgs) (*ListResult, error), visible func(*File) bool) (*ListResult, error) {
	pageSize := args.Page.PageSize
	if pageSize < 1 {
		pageSize = 1
	}
	page := *args.Page
	page.UseCursorPagination = true
	page.PageSize = pageSize
	result := &ListResult{Pagination: &inventory.PaginationResults{PageSize: pageSize, IsCursor: true}}

	for {
		page.PageSize = pageSize - len(result.Files)
		physical, err := fetch(listArgsWithPage(args, &page))
		if err != nil {
			return nil, err
		}
		if physical == nil {
			return nil, fmt.Errorf("physical page is nil")
		}
		copyListMetadata(result, physical)
		for _, file := range physical.Files {
			if visible(file) {
				result.Files = append(result.Files, file)
			}
		}
		if len(result.Files) >= pageSize {
			result.Files = result.Files[:pageSize]
			if physical.Pagination != nil && physical.Pagination.NextPageToken != "" {
				hasVisible, err := hasVisibleCursorChild(args, fetch, visible, physical.Pagination.NextPageToken)
				if err != nil {
					return nil, err
				}
				if hasVisible {
					result.Pagination.NextPageToken = physical.Pagination.NextPageToken
				}
			}
			return result, nil
		}
		if physical.Pagination == nil || physical.Pagination.NextPageToken == "" {
			return result, nil
		}
		page.PageToken = physical.Pagination.NextPageToken
	}
}

// hasVisibleCursorChild checks whether a physical continuation contains at
// least one readable result. The client receives the original continuation so
// its next request re-evaluates every intervening row under the current rule.
func hasVisibleCursorChild(args *ListArgs, fetch func(*ListArgs) (*ListResult, error), visible func(*File) bool, token string) (bool, error) {
	for token != "" {
		page := *args.Page
		page.UseCursorPagination = true
		page.PageSize = 1
		page.PageToken = token
		physical, err := fetch(listArgsWithPage(args, &page))
		if err != nil {
			return false, err
		}
		if physical == nil {
			return false, fmt.Errorf("physical page is nil")
		}
		for _, file := range physical.Files {
			if visible(file) {
				return true, nil
			}
		}
		if physical.Pagination == nil {
			return false, nil
		}
		token = physical.Pagination.NextPageToken
	}
	return false, nil
}

func paginateVisibleOffsetChildren(args *ListArgs, fetch func(*ListArgs) (*ListResult, error), visible func(*File) bool) (*ListResult, error) {
	pageSize := args.Page.PageSize
	if pageSize < 1 {
		pageSize = 1
	}
	requestedPage := max(args.Page.Page, 0)
	if pageSize > maxVisibleOffsetItems || requestedPage > (maxVisibleOffsetItems-pageSize)/pageSize {
		return nil, fmt.Errorf("requested visible offset exceeds maximum %d items", maxVisibleOffsetItems)
	}
	start := requestedPage * pageSize
	end := start + pageSize
	physicalPage := 0
	visibleCount := 0
	result := &ListResult{Pagination: &inventory.PaginationResults{Page: requestedPage, PageSize: pageSize}}

	for {
		page := *args.Page
		page.Page = physicalPage
		page.PageSize = pageSize
		physical, err := fetch(listArgsWithPage(args, &page))
		if err != nil {
			return nil, err
		}
		if physical == nil {
			return nil, fmt.Errorf("physical page is nil")
		}
		copyListMetadata(result, physical)
		for _, file := range physical.Files {
			if !visible(file) {
				continue
			}
			if visibleCount >= start && visibleCount < end {
				result.Files = append(result.Files, file)
			}
			visibleCount++
			if visibleCount >= end {
				if isFinalOffsetPhysicalPage(physicalPage, pageSize, physical) {
					result.Pagination.TotalItems = visibleCount
				}
				return result, nil
			}
		}
		if physical.Pagination == nil || (physicalPage+1)*pageSize >= physical.Pagination.TotalItems {
			result.Pagination.TotalItems = visibleCount
			return result, nil
		}
		physicalPage++
	}
}

func isFinalOffsetPhysicalPage(page, pageSize int, physical *ListResult) bool {
	return physical.Pagination == nil || (page+1)*pageSize >= physical.Pagination.TotalItems
}

func listArgsWithPage(args *ListArgs, page *inventory.PaginationArgs) *ListArgs {
	copy := *args
	copy.Page = page
	copy.StreamCallback = nil
	return &copy
}

func copyListMetadata(target, source *ListResult) {
	target.MixedType = source.MixedType
	target.RecursionLimitReached = source.RecursionLimitReached
	target.SingleFileView = source.SingleFileView
}
