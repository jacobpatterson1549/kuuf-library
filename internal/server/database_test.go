package server

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
)

func TestBookIteratorHasNext(t *testing.T) {
	tests := []struct {
		name             string
		iter             bookIterator
		wantOk           bool
		want             bool
		wantBatchIndex   int
		wantHeaderIndex  int
		wantBatchHeaders []book.Header
		wantClosed       bool
	}{
		{
			name: "closed",
			iter: bookIterator{
				closed: true,
			},
			wantOk: true,
		},
		{
			name: "no next",
			iter: bookIterator{
				batchSize:    3,
				batchIndex:   1,
				batchHeaders: []book.Header{{}, {}},
				headerIndex:  2,
			},
			wantOk:           true,
			wantClosed:       true,
			wantBatchIndex:   1,
			wantHeaderIndex:  2,
			wantBatchHeaders: []book.Header{{}, {}},
		},
		{
			name: "empty iterator: read headers error",
			iter: bookIterator{
				batchSize: 3,
				database: mockDatabase{
					readBookHeadersFunc: func(f book.Filter, limit, offset int) ([]book.Header, error) {
						return nil, fmt.Errorf("read headers error")
					},
				},
			},
		},
		{
			name: "empty iterator: no books",
			iter: bookIterator{
				batchSize: 3,
				database: mockDatabase{
					readBookHeadersFunc: func(f book.Filter, limit, offset int) ([]book.Header, error) {
						return nil, nil
					},
				},
			},
			wantOk:          true,
			want:            false,
			wantBatchIndex:  1,
			wantHeaderIndex: 0,
		},
		{
			name: "empty iterator: initial read",
			iter: bookIterator{
				batchSize: 3,
				database: mockDatabase{
					readBookHeadersFunc: func(f book.Filter, limit, offset int) ([]book.Header, error) {
						return []book.Header{{}}, nil
					},
				},
			},
			wantOk:           true,
			want:             true,
			wantBatchIndex:   1,
			wantHeaderIndex:  0,
			wantBatchHeaders: []book.Header{{}},
		},
		{
			name: "happy path: middle of batch",
			iter: bookIterator{
				batchSize:    3,
				batchIndex:   6,
				batchHeaders: []book.Header{{}, {}, {}, {}},
				headerIndex:  1,
			},
			wantOk:           true,
			want:             true,
			wantBatchIndex:   6,
			wantHeaderIndex:  1,
			wantBatchHeaders: []book.Header{{}, {}, {}, {}},
		},
		{
			name: "happy path: last of batch: request next",
			iter: bookIterator{
				database: mockDatabase{
					readBookHeadersFunc: func(f book.Filter, limit, offset int) ([]book.Header, error) {
						wantArgs := []interface{}{book.Filter{}, 4, 6}
						gotArgs := []interface{}{f, limit, offset}
						if !reflect.DeepEqual(wantArgs, gotArgs) {
							t.Errorf("arguments not equal: \n wanted: %#v \n got:    %#v", wantArgs, gotArgs)
						}
						return []book.Header{{}, {}, {}}, nil
					},
				},
				batchSize:    3,
				batchIndex:   2,
				batchHeaders: []book.Header{{}, {}, {}, {}},
				headerIndex:  3,
			},
			wantOk:           true,
			want:             true,
			wantBatchIndex:   3,
			wantHeaderIndex:  0,
			wantBatchHeaders: []book.Header{{}, {}, {}},
		},
		{
			name: "happy path: last of batch",
			iter: bookIterator{
				batchSize:    100,
				batchIndex:   8,
				batchHeaders: []book.Header{{}, {}, {}, {}, {}},
				headerIndex:  4,
			},
			wantOk:           true,
			want:             true,
			wantBatchIndex:   8,
			wantHeaderIndex:  4,
			wantBatchHeaders: []book.Header{{}, {}, {}, {}, {}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			got := test.iter.HasNext(ctx)
			switch {
			case !test.wantOk:
				if test.iter.Err() == nil {
					t.Errorf("wanted error")
				}
			case test.iter.Err() != nil:
				t.Errorf("unwanted error: %v", test.iter.Err())
			case test.want != got:
				t.Errorf("hasNext ok values not equal: wanted %v, got %v", test.want, got)
			case test.wantBatchIndex != test.iter.batchIndex:
				t.Errorf("batch indexes not equal: wanted %v, got %v", test.wantBatchIndex, test.iter.batchIndex)
			case test.wantHeaderIndex != test.iter.headerIndex:
				t.Errorf("header indexes not equal: wanted %v, got %v", test.wantHeaderIndex, test.iter.headerIndex)
			case !reflect.DeepEqual(test.wantBatchHeaders, test.iter.batchHeaders):
				t.Errorf("header indexes not equal: \n wanted: %v \n got:    %v", test.wantBatchHeaders, test.iter.batchHeaders)
			}
		})
	}
}

func TestBookIteratorNext(t *testing.T) {
	tests := []struct {
		name            string
		iter            bookIterator
		wantOk          bool
		want            *book.Book
		wantHeaderIndex int
	}{
		{
			name: "no next",
			iter: bookIterator{
				closed: true,
			},
		},
		{
			name: "hasNext error ",
			iter: bookIterator{
				nextErr: fmt.Errorf("hasNext error"),
			},
		},
		{
			name: "read book error",
			iter: bookIterator{
				batchIndex:   1,
				batchHeaders: []book.Header{{ID: "xyz"}, {}},
				database: mockDatabase{
					readBookFunc: func(id string) (*book.Book, error) {
						return nil, fmt.Errorf("read book error")
					},
				},
			},
		},
		{
			name: "happy path",
			iter: bookIterator{
				batchIndex:   1,
				batchHeaders: []book.Header{{ID: "xyz"}, {}},
				database: mockDatabase{
					readBookFunc: func(id string) (*book.Book, error) {
						if want, got := "xyz", id; want != got {
							return nil, fmt.Errorf("ids not equal: wanted %v, got %v", want, got)
						}
						b := book.Book{Publisher: "the end"}
						return &b, nil
					},
				},
			},
			wantOk:          true,
			want:            &book.Book{Publisher: "the end"},
			wantHeaderIndex: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			got, err := test.iter.Next(ctx)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("books not equal: wanted %v, got %v", test.want, got)
			case test.wantHeaderIndex != test.iter.headerIndex:
				t.Errorf("header indexes not equal: wanted %v, got %v", test.wantHeaderIndex, test.iter.headerIndex)
			}
		})
	}
}

func TestBookIteratorAllBooks(t *testing.T) {
	tests := []struct {
		name   string
		iter   bookIterator
		wantOk bool
		want   []book.Book
	}{
		{
			name: "hasNext error",
			iter: bookIterator{
				database: mockDatabase{
					readBookHeadersFunc: func(f book.Filter, limit, offset int) ([]book.Header, error) {
						return nil, fmt.Errorf("db error")
					},
				},
			},
		},
		{
			name: "no books",
			iter: bookIterator{
				database: mockDatabase{
					readBookHeadersFunc: func(f book.Filter, limit, offset int) ([]book.Header, error) {
						return nil, nil
					},
				},
			},
			wantOk: true,
		},
		{
			name: "read book error",
			iter: bookIterator{
				database: mockDatabase{
					readBookHeadersFunc: func(f book.Filter, limit, offset int) ([]book.Header, error) {
						return []book.Header{{}}, nil
					},
					readBookFunc: func(id string) (*book.Book, error) {
						return nil, fmt.Errorf("db error")
					},
				},
			},
		},
		{
			name: "happy path: three books",
			iter: bookIterator{
				batchSize: 2,
				database: mockDatabase{
					readBookHeadersFunc: func(f book.Filter, limit, offset int) ([]book.Header, error) {
						var wantFilter book.Filter
						if !reflect.DeepEqual(wantFilter, f) {
							return nil, fmt.Errorf("wanted empty filter, got %#v", f)
						}
						switch {
						case limit == 3 && offset == 0:
							return []book.Header{{ID: "id-b"}, {ID: "id-a"}, {ID: "id-c"}}, nil
						case limit == 3 && offset == 2:
							return []book.Header{{ID: "id-c"}}, nil
						}
						return nil, fmt.Errorf("unwanted limit and offset: %v and %v", limit, offset)
					},
					readBookFunc: func(id string) (*book.Book, error) {
						return &book.Book{Header: book.Header{ID: id}}, nil
					},
				},
			},
			wantOk: true,
			want: []book.Book{
				{Header: book.Header{ID: "id-b"}},
				{Header: book.Header{ID: "id-a"}},
				{Header: book.Header{ID: "id-c"}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			got, err := test.iter.AllBooks(ctx)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("books not equal: wanted %v, got %v", test.want, got)
			}
		})
	}
}

func TestReadBookSubjects(t *testing.T) {
	wantCtx := context.Background()
	wantLimit := 1
	wantOffset := 2
	wantSubjects := []book.Subject{{}}
	f := func(ctx context.Context, limit, offset int) ([]book.Subject, error) {
		wantArgs := []interface{}{wantCtx, wantLimit, wantOffset}
		gotArgs := []interface{}{ctx, limit, offset}
		if !reflect.DeepEqual(wantArgs, gotArgs) {
			t.Errorf("arguments not equal: \n wanted: %#v \n got:    %#v", wantArgs, gotArgs)
		}
		return wantSubjects, nil
	}
	d := readOnlyDatabase{
		ReadBookSubjectsFunc: f,
	}
	got, err := d.ReadBookSubjects(wantCtx, wantLimit, wantOffset)
	wantResult := []interface{}{wantSubjects, nil}
	gotResult := []interface{}{got, err}
	if !reflect.DeepEqual(wantResult, gotResult) {
		t.Errorf("results not equal: \n wanted: %#v \n got:    %#v", wantResult, gotResult)
	}
}

func TestDatabaseReadBookHeaders(t *testing.T) {
	wantCtx := context.Background()
	wantFilter := book.Filter{Subject: "everything"}
	wantLimit := 11
	wantOffset := 22
	wantHeaders := []book.Header{{}, {}}
	f := func(ctx context.Context, filter book.Filter, limit, offset int) ([]book.Header, error) {
		wantArgs := []interface{}{wantCtx, wantFilter, wantLimit, wantOffset}
		gotArgs := []interface{}{ctx, filter, limit, offset}
		if !reflect.DeepEqual(wantArgs, gotArgs) {
			t.Errorf("arguments not equal: \n wanted: %#v \n got:    %#v", wantArgs, gotArgs)
		}
		return wantHeaders, nil
	}
	d := readOnlyDatabase{
		ReadBookHeadersFunc: f,
	}
	got, err := d.ReadBookHeaders(wantCtx, wantFilter, wantLimit, wantOffset)
	wantResult := []interface{}{wantHeaders, nil}
	gotResult := []interface{}{got, err}
	if !reflect.DeepEqual(wantResult, gotResult) {
		t.Errorf("results not equal: \n wanted: %#v \n got:    %#v", wantResult, gotResult)
	}
}

func TestDatabaseReadBook(t *testing.T) {
	wantCtx := context.Background()
	wantID := "3"
	wantBook := new(book.Book)
	f := func(ctx context.Context, id string) (*book.Book, error) {
		wantArgs := []interface{}{wantCtx, wantID}
		gotArgs := []interface{}{ctx, id}
		if !reflect.DeepEqual(wantArgs, gotArgs) {
			t.Errorf("arguments not equal: \n wanted: %#v \n got:    %#v", wantArgs, gotArgs)
		}
		return wantBook, nil
	}
	d := readOnlyDatabase{
		ReadBookFunc: f,
	}
	got, err := d.ReadBook(wantCtx, wantID)
	wantResult := []interface{}{wantBook, nil}
	gotResult := []interface{}{got, err}
	if !reflect.DeepEqual(wantResult, gotResult) {
		t.Errorf("results not equal: \n wanted: %#v \n got:    %#v", wantResult, gotResult)
	}
}

func TestDatabaseNotAllowed(t *testing.T) {
	tests := []struct {
		name string
		f    func(ctx context.Context, d readOnlyDatabase) error
	}{
		{"CreateBooks", func(ctx context.Context, d readOnlyDatabase) error { _, err := d.CreateBooks(ctx); return err }},
		{"UpdateBook", func(ctx context.Context, d readOnlyDatabase) error { return d.UpdateBook(ctx, book.Book{}, false) }},
		{"DeleteBook", func(ctx context.Context, d readOnlyDatabase) error { return d.DeleteBook(ctx, "id") }},
		{"ReadAdminPassword", func(ctx context.Context, d readOnlyDatabase) error { _, err := d.ReadAdminPassword(ctx); return err }},
		{"UpdateAdminPassword", func(ctx context.Context, d readOnlyDatabase) error { return d.UpdateAdminPassword(ctx, "Bilbo123") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var d readOnlyDatabase
			ctx := context.Background()
			if err := test.f(ctx, d); err == nil {
				t.Errorf("wanted error")
			}
		})
	}
}
