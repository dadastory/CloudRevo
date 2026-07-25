package dbfs

import (
	"strings"
	"testing"

	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/inventory"
)

func TestPaginateVisibleChildrenRejectsExcessiveOffsetBeforeFetching(t *testing.T) {
	called := false
	_, err := paginateVisibleChildren(&ListArgs{Page: &inventory.PaginationArgs{Page: 100001, PageSize: 1}}, func(*ListArgs) (*ListResult, error) {
		called = true
		return nil, nil
	}, func(*File) bool { return true })
	if err == nil || !strings.Contains(err.Error(), "offset") {
		t.Fatalf("excessive visible offset must be rejected, got %v", err)
	}
	if called {
		t.Fatal("excessive visible offset must be rejected before physical page fetches")
	}
}

func TestPaginateVisibleChildrenCursorContinuesPastHiddenPhysicalPage(t *testing.T) {
	hidden := &File{Model: &ent.File{ID: 1, Name: "hidden"}}
	visible := &File{Model: &ent.File{ID: 2, Name: "visible"}}
	pages := map[string]*ListResult{
		"":     {Files: []*File{hidden}, Pagination: &inventory.PaginationResults{PageSize: 1, NextPageToken: "next"}},
		"next": {Files: []*File{visible}, Pagination: &inventory.PaginationResults{PageSize: 1, IsCursor: true}},
	}

	result, err := paginateVisibleChildren(&ListArgs{Page: &inventory.PaginationArgs{PageSize: 1, UseCursorPagination: true}}, func(args *ListArgs) (*ListResult, error) {
		return pages[args.Page.PageToken], nil
	}, func(file *File) bool { return file.Model.ID == visible.Model.ID })
	if err != nil {
		t.Fatalf("paginate visible cursor page: %v", err)
	}
	if len(result.Files) != 1 || result.Files[0].Model.ID != visible.Model.ID {
		t.Fatalf("later visible child must be reachable, got %#v", result.Files)
	}
	if result.Pagination.NextPageToken != "" || result.Pagination.TotalItems != 0 {
		t.Fatalf("exhausted visible cursor page must not leak hidden pagination state: %#v", result.Pagination)
	}
}

func TestPaginateVisibleChildrenOffsetCountsOnlyVisibleChildren(t *testing.T) {
	hidden := &File{Model: &ent.File{ID: 1, Name: "hidden"}}
	visible := &File{Model: &ent.File{ID: 2, Name: "visible"}}
	pages := map[int]*ListResult{
		0: {Files: []*File{hidden}, Pagination: &inventory.PaginationResults{Page: 0, PageSize: 1, TotalItems: 2}},
		1: {Files: []*File{visible}, Pagination: &inventory.PaginationResults{Page: 1, PageSize: 1, TotalItems: 2}},
	}

	result, err := paginateVisibleChildren(&ListArgs{Page: &inventory.PaginationArgs{Page: 0, PageSize: 1}}, func(args *ListArgs) (*ListResult, error) {
		return pages[args.Page.Page], nil
	}, func(file *File) bool { return file.Model.ID == visible.Model.ID })
	if err != nil {
		t.Fatalf("paginate visible offset page: %v", err)
	}
	if len(result.Files) != 1 || result.Files[0].Model.ID != visible.Model.ID {
		t.Fatalf("later visible child must be returned on the first visible page, got %#v", result.Files)
	}
	if result.Pagination.TotalItems != 1 {
		t.Fatalf("only visible children may contribute to total, got %#v", result.Pagination)
	}
}

func TestPaginateVisibleChildrenCursorOnlyFetchesRemainingVisibleCapacity(t *testing.T) {
	hidden := &File{Model: &ent.File{ID: 1, Name: "hidden"}}
	firstVisible := &File{Model: &ent.File{ID: 2, Name: "first"}}
	secondVisible := &File{Model: &ent.File{ID: 3, Name: "second"}}
	pageSizes := make([]int, 0, 2)

	result, err := paginateVisibleChildren(&ListArgs{Page: &inventory.PaginationArgs{PageSize: 2, UseCursorPagination: true}}, func(args *ListArgs) (*ListResult, error) {
		pageSizes = append(pageSizes, args.Page.PageSize)
		switch args.Page.PageToken {
		case "":
			return &ListResult{Files: []*File{hidden, firstVisible}, Pagination: &inventory.PaginationResults{PageSize: 2, NextPageToken: "after-first"}}, nil
		case "after-first":
			return &ListResult{Files: []*File{secondVisible}, Pagination: &inventory.PaginationResults{PageSize: 1, NextPageToken: "after-second"}}, nil
		case "after-second":
			return &ListResult{Pagination: &inventory.PaginationResults{PageSize: 1, IsCursor: true}}, nil
		default:
			t.Fatalf("unexpected cursor %q", args.Page.PageToken)
			return nil, nil
		}
	}, func(file *File) bool { return file.Model.ID != hidden.Model.ID })
	if err != nil {
		t.Fatalf("paginate visible cursor page: %v", err)
	}
	if len(result.Files) != 2 || result.Files[0] != firstVisible || result.Files[1] != secondVisible {
		t.Fatalf("expected two visible files, got %#v", result.Files)
	}
	if result.Pagination.NextPageToken != "" {
		t.Fatalf("cursor must not continue after an exhausted visible result set, got %#v", result.Pagination)
	}
	if len(pageSizes) != 3 || pageSizes[0] != 2 || pageSizes[1] != 1 || pageSizes[2] != 1 {
		t.Fatalf("cursor must use a one-item look-ahead after filling visible capacity, got %#v", pageSizes)
	}
}

func TestPaginateVisibleChildrenCursorDoesNotReturnTokenForHiddenTail(t *testing.T) {
	visible := &File{Model: &ent.File{ID: 1, Name: "visible"}}
	hidden := &File{Model: &ent.File{ID: 2, Name: "hidden"}}
	pages := map[string]*ListResult{
		"":            {Files: []*File{visible}, Pagination: &inventory.PaginationResults{PageSize: 1, NextPageToken: "hidden-tail"}},
		"hidden-tail": {Files: []*File{hidden}, Pagination: &inventory.PaginationResults{PageSize: 1}},
	}
	result, err := paginateVisibleChildren(&ListArgs{Page: &inventory.PaginationArgs{PageSize: 1, UseCursorPagination: true}}, func(args *ListArgs) (*ListResult, error) {
		return pages[args.Page.PageToken], nil
	}, func(file *File) bool { return file == visible })
	if err != nil {
		t.Fatalf("paginate visible cursor page: %v", err)
	}
	if result.Pagination.NextPageToken != "" {
		t.Fatalf("hidden-only tail must not create a continuation token: %#v", result.Pagination)
	}
}

func TestPaginateVisibleChildrenOffsetStopsAfterRequestedVisibleWindow(t *testing.T) {
	visible := &File{Model: &ent.File{ID: 1, Name: "visible"}}
	called := 0
	result, err := paginateVisibleChildren(&ListArgs{Page: &inventory.PaginationArgs{PageSize: 1}}, func(args *ListArgs) (*ListResult, error) {
		called++
		if args.Page.Page != 0 {
			t.Fatalf("offset pagination must not scan later pages after filling visible window")
		}
		return &ListResult{Files: []*File{visible}, Pagination: &inventory.PaginationResults{Page: 0, PageSize: 1, TotalItems: 1000}}, nil
	}, func(*File) bool { return true })
	if err != nil {
		t.Fatalf("paginate visible offset page: %v", err)
	}
	if len(result.Files) != 1 || called != 1 {
		t.Fatalf("expected one bounded fetch, got files=%#v calls=%d", result.Files, called)
	}
	if result.Pagination.TotalItems != 0 {
		t.Fatalf("visible offset page must not expose an unscanned physical total: %#v", result.Pagination)
	}
}

func TestPaginateVisibleChildrenSearchKeepsCursorSemantics(t *testing.T) {
	visible := &File{Model: &ent.File{ID: 1, Name: "visible"}}
	result, err := paginateVisibleChildren(&ListArgs{
		Page:   &inventory.PaginationArgs{PageSize: 1},
		Search: &inventory.SearchFileParameters{},
	}, func(args *ListArgs) (*ListResult, error) {
		if !args.Page.UseCursorPagination {
			t.Fatal("search pagination must fetch physical cursor pages")
		}
		return &ListResult{Files: []*File{visible}, Pagination: &inventory.PaginationResults{IsCursor: true, PageSize: 1}}, nil
	}, func(*File) bool { return true })
	if err != nil {
		t.Fatalf("paginate visible search page: %v", err)
	}
	if !result.Pagination.IsCursor || len(result.Files) != 1 {
		t.Fatalf("search result must remain a cursor page, got %#v", result)
	}
}

func TestPaginateVisibleChildrenDoesNotStreamHiddenPhysicalEntries(t *testing.T) {
	hidden := &File{Model: &ent.File{ID: 1, Name: "hidden"}}
	visible := &File{Model: &ent.File{ID: 2, Name: "visible"}}
	streamed := make([]*File, 0, 1)
	result, err := paginateVisibleChildren(&ListArgs{
		Page: &inventory.PaginationArgs{PageSize: 2, UseCursorPagination: true},
		StreamCallback: func(files []*File) {
			streamed = append(streamed, files...)
		},
	}, func(args *ListArgs) (*ListResult, error) {
		if args.StreamCallback != nil {
			args.StreamCallback([]*File{hidden, visible})
		}
		return &ListResult{Files: []*File{hidden, visible}, Pagination: &inventory.PaginationResults{PageSize: 2, IsCursor: true}}, nil
	}, func(file *File) bool { return file.Model.ID == visible.Model.ID })
	if err != nil {
		t.Fatalf("paginate visible stream page: %v", err)
	}
	if len(streamed) != 1 || streamed[0] != visible || len(result.Files) != 0 {
		t.Fatalf("streaming must expose only visible entries, got streamed=%#v result=%#v", streamed, result.Files)
	}
}
