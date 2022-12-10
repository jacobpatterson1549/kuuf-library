package mongo

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/mongo/bson"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/mongo/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestNewDatabase(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		wantOk       bool
	}{
		{"bad url", "bad url", false},
		{"happy path", "mongodb://localhost:27017/", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			d, err := NewDatabase(ctx, test.url)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case d.booksCollection == nil:
				t.Errorf("books collection not set")
			case d.usersCollection == nil:
				t.Errorf("users collection not set")
			}
		})
	}
}

const (
	okID1 = "63913898359dc4441bd976e8"
	okID2 = "63913898359dc4441bd976e9"
)

func objectIDHelper(t *testing.T, id string) interface{} {
	objID, err := primitive.ObjectIDFromString(id)
	if err != nil {
		t.Errorf("creating object id for test: %v", err)
	}
	return objID
}

func TestCreateBooks(t *testing.T) {
	b1 := book.Book{
		Header:      book.Header{Title: "2", Author: "3", Subject: "4"},
		Description: "5", DeweyDecClass: "6", Pages: 7, Publisher: "8",
		PublishDate: time.Date(2000, 2, 29, 0, 0, 0, 0, time.UTC),
		AddedDate:   time.Date(2022, 11, 16, 0, 0, 0, 0, time.UTC),
		EanIsbn13:   "11", UpcIsbn10: "12", ImageBase64: "13",
	}
	b2 := func() book.Book { b2 := b1; b2.ID = "wipeME"; b2.Title += "_EDITED"; return b2 }()
	tests := []struct {
		name           string
		InsertManyFunc func(ctx context.Context, documents []interface{}, opts ...*options.InsertManyOptions) (*mongo.InsertManyResult, error)
		insertBooks    []book.Book
		wantOk         bool
		want           []book.Book
	}{
		{
			name:   "no books (calling coll.InsertMany(nil) is illegal)",
			wantOk: true,
		},
		{
			name:        "insert error",
			insertBooks: []book.Book{b1, b2},
			InsertManyFunc: func(ctx context.Context, documents []interface{}, opts ...*options.InsertManyOptions) (*mongo.InsertManyResult, error) {
				return nil, fmt.Errorf("insert error")
			},
		},
		{
			name:        "bad insert id",
			insertBooks: []book.Book{b1},
			InsertManyFunc: func(ctx context.Context, documents []interface{}, opts ...*options.InsertManyOptions) (*mongo.InsertManyResult, error) {
				result := mongo.InsertManyResult{
					InsertedIDs: []interface{}{"bad insert id"},
				}
				return &result, nil
			},
		},
		{
			name:        "wrong number of insert ids",
			insertBooks: []book.Book{b1},
			InsertManyFunc: func(ctx context.Context, documents []interface{}, opts ...*options.InsertManyOptions) (*mongo.InsertManyResult, error) {
				result := mongo.InsertManyResult{
					InsertedIDs: []interface{}{
						objectIDHelper(t, okID1),
						objectIDHelper(t, okID2),
					},
				}
				return &result, nil
			},
		},
		{
			name:        "happy path",
			insertBooks: []book.Book{b1, b2},
			InsertManyFunc: func(ctx context.Context, documents []interface{}, opts ...*options.InsertManyOptions) (*mongo.InsertManyResult, error) {
				wantDocuments := make([]interface{}, 2)
				for i, b := range []book.Book{b1, b2} {
					b.ID = ""                       // want to insert not upsert
					wantDocuments[i] = mongoBook(b) // struct with bson tags
				}
				gotDocuments := documents
				wantOpts := options.InsertMany()
				gotOpts := options.MergeInsertManyOptions(opts...)
				switch {
				case !reflect.DeepEqual(wantDocuments, gotDocuments):
					t.Errorf("documents not equal: \n wanted: %v \n got:    %v", wantDocuments, gotDocuments)
				case !reflect.DeepEqual(wantOpts, gotOpts):
					t.Errorf("opts not equal: \n wanted: %#v \n got:    %#v", wantOpts, gotOpts)
				}
				result := mongo.InsertManyResult{
					InsertedIDs: []interface{}{
						objectIDHelper(t, okID1),
						objectIDHelper(t, okID2),
					},
				}
				return &result, nil
			},
			wantOk: true,
			want: []book.Book{
				func() book.Book { b := b1; b.ID = okID1; return b }(),
				func() book.Book { b := b2; b.ID = okID2; return b }(),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := Database{
				booksCollection: mockCollection{
					InsertManyFunc: test.InsertManyFunc,
				},
			}
			ctx := context.Background()
			got, err := d.CreateBooks(ctx, test.insertBooks...)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("books not equal: \n wanted: %q \n got:    %q", test.want, got)
			}
		})
	}
}

func TestReadBookSubjects(t *testing.T) {
	tests := []struct {
		name          string
		limit         int
		offset        int
		AggregateFunc func(ctx context.Context, pipeline interface{}, opts ...*options.AggregateOptions) (*mongo.Cursor, error)
		wantOk        bool
		want          []book.Subject
	}{
		{
			name: "aggregate error",
			AggregateFunc: func(ctx context.Context, pipeline interface{}, opts ...*options.AggregateOptions) (*mongo.Cursor, error) {
				return nil, fmt.Errorf("aggregate error")
			},
		},
		{
			name: "decode error",
			AggregateFunc: func(ctx context.Context, pipeline interface{}, opts ...*options.AggregateOptions) (*mongo.Cursor, error) {
				documents := []interface{}{
					map[string]interface{}{
						subjectCountField: "cannot decode string into an integer type",
					},
				}
				return mongo.NewCursorFromDocuments(documents, nil, nil)
			},
		},
		{
			name:   "happy path ",
			limit:  2,
			offset: 8,
			AggregateFunc: func(ctx context.Context, pipeline interface{}, opts ...*options.AggregateOptions) (*mongo.Cursor, error) {
				wantPipeline := mongo.Pipeline{
					bson.D(bson.E("$group", bson.D(
						bson.E(subjectNameField, "$"+bookSubjectField),
						bson.E(subjectCountField, bson.D(bson.E("$sum", 1))),
					))),
					bson.D(bson.E("$sort", bson.D(
						bson.E(subjectNameField, 1),
					))),
					bson.D(bson.E("$skip", 8)),
					bson.D(bson.E("$limit", 2)),
				}
				gotPipeline := pipeline
				wantOpts := options.Aggregate()
				gotOpts := options.MergeAggregateOptions(opts...)
				switch {
				case !reflect.DeepEqual(wantPipeline, gotPipeline):
					t.Errorf("pipelines not equal: \n wanted: %q \n got:    %q", wantPipeline, gotPipeline)
				case !reflect.DeepEqual(wantOpts, gotOpts):
					t.Errorf("opts not equal: \n wanted: %#v \n got:    %#v", wantOpts, gotOpts)
				}
				documents := []interface{}{
					mSubject{Name: "sub-I", Count: 3},
					mSubject{Name: "sub-J", Count: 4},
				}
				return mongo.NewCursorFromDocuments(documents, nil, nil)
			},
			wantOk: true,
			want: []book.Subject{
				{Name: "sub-I", Count: 3},
				{Name: "sub-J", Count: 4},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := Database{
				booksCollection: mockCollection{
					AggregateFunc: test.AggregateFunc,
				},
			}
			ctx := context.Background()
			got, err := d.ReadBookSubjects(ctx, test.limit, test.offset)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("subjects not equal: \n wanted: %q \n got:    %q", test.want, got)
			}
		})
	}
}

func TestReadBookHeaders(t *testing.T) {
	tests := []struct {
		name     string
		filter   book.Filter
		limit    int
		offset   int
		FindFunc func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (cur *mongo.Cursor, err error)
		wantOk   bool
		want     []book.Header
	}{
		{
			name: "find error",
			FindFunc: func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (cur *mongo.Cursor, err error) {
				return nil, fmt.Errorf("find error")
			},
		},
		{
			name: "decode error",
			FindFunc: func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (cur *mongo.Cursor, err error) {
				documents := []interface{}{
					map[string]interface{}{
						bookTitleField: -1,
					},
				}
				return mongo.NewCursorFromDocuments(documents, nil, nil)
			},
		},
		{
			name:   "happy path ",
			filter: book.Filter{HeaderPart: "T"},
			limit:  3,
			offset: 9,
			FindFunc: func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (cur *mongo.Cursor, err error) {
				bsonFilter := bson.Filter{
					SubjectKey: bookSubjectField,
					HeaderKeys: []string{
						bookTitleField,
						bookAuthorField,
						bookSubjectField,
					},
				}
				bookFilter := book.Filter{HeaderPart: "T"}
				wantFilter := bson.D(bsonFilter.From(bookFilter)...)
				gotFilter := filter
				wantOpts := options.Find().
					SetSort(bson.D(
						bson.E(bookSubjectField, 1),
						bson.E(bookTitleField, 1),
					)).
					SetLimit(int64(3)).
					SetSkip(int64(9)).
					SetProjection(bson.D(
						bson.E(bookIDField, 1),
						bson.E(bookTitleField, 1),
						bson.E(bookAuthorField, 1),
						bson.E(bookSubjectField, 1),
					))
				gotOpts := options.MergeFindOptions(opts...)
				switch {
				case !reflect.DeepEqual(wantFilter, gotFilter):
					t.Errorf("filters not equal: \n wanted: %#v \n got:    %#v", wantFilter, gotFilter)
				case !reflect.DeepEqual(wantOpts, gotOpts):
					t.Errorf("opts not equal: \n wanted: %#v \n got:    %#v", wantOpts, gotOpts)
				}
				documents := []interface{}{
					mHeader{ID: "2b8", Title: "T3", Author: "a8", Subject: "a"},
					mHeader{ID: "3b7", Title: "T2", Author: "a6", Subject: "b"},
					mHeader{ID: "1c7", Title: "T4", Author: "a7", Subject: "b"},
				}
				return mongo.NewCursorFromDocuments(documents, nil, nil)
			},
			wantOk: true,
			want: []book.Header{
				{ID: "2b8", Title: "T3", Author: "a8", Subject: "a"},
				{ID: "3b7", Title: "T2", Author: "a6", Subject: "b"},
				{ID: "1c7", Title: "T4", Author: "a7", Subject: "b"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := Database{
				booksCollection: mockCollection{
					FindFunc: test.FindFunc,
				},
			}
			ctx := context.Background()
			got, err := d.ReadBookHeaders(ctx, test.filter, test.limit, test.offset)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("subjects not equal: \n wanted: %q \n got:    %q", test.want, got)
			}
		})
	}
}

func TestReadBook(t *testing.T) {
	b := book.Book{
		Header:      book.Header{ID: "1", Title: "2", Author: "3", Subject: "4"},
		Description: "5", DeweyDecClass: "6", Pages: 7, Publisher: "8",
		PublishDate: time.Date(2000, 2, 29, 0, 0, 0, 0, time.UTC),
		AddedDate:   time.Date(2022, 11, 16, 0, 0, 0, 0, time.UTC),
		EanIsbn13:   "11", UpcIsbn10: "12", ImageBase64: "13",
	}
	tests := []struct {
		name        string
		bookID      string
		FindOneFunc func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult
		wantOk      bool
		want        *book.Book
	}{
		{
			name:   "bad id",
			bookID: "bad id",
		},
		{
			name:   "bad book",
			bookID: okID1,
			FindOneFunc: func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
				err := fmt.Errorf("bad book")
				return mongo.NewSingleResultFromDocument(nil, err, nil)
			},
		},
		{
			name:   "happy path",
			bookID: okID1,
			FindOneFunc: func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
				wantFilter := bson.D(bson.E(bookIDField, objectIDHelper(t, okID1)))
				gotFilter := filter
				wantOpts := options.FindOne()
				gotOpts := options.MergeFindOneOptions(opts...)
				switch {
				case !reflect.DeepEqual(wantFilter, gotFilter):
					t.Errorf("filters not equal: \n wanted: %#v \n got:    %#v", wantFilter, gotFilter)
				case !reflect.DeepEqual(wantOpts, gotOpts):
					t.Errorf("options not equal: \n wanted: %#v \n got:    %#v", wantOpts, gotOpts)
				}
				document := mongoBook(b)
				return mongo.NewSingleResultFromDocument(document, nil, nil)
			},
			wantOk: true,
			want:   &b,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := Database{
				booksCollection: mockCollection{
					FindOneFunc: test.FindOneFunc,
				},
			}
			ctx := context.Background()
			got, err := d.ReadBook(ctx, test.bookID)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("books not equal: \n wanted: %q \n got:    %q", test.want, got)
			}
		})
	}
}

func TestUpdateBook(t *testing.T) {
	happyPathUpdateOneFunc := func(t *testing.T, wantUpdate interface{}) func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
		t.Helper()
		return func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
			wantFilter := bson.D(bson.E(bookIDField, objectIDHelper(t, okID1)))
			gotFilter := filter
			gotUpdate := update
			wantOpts := options.Update()
			gotOps := options.MergeUpdateOptions(opts...)
			switch {
			case !reflect.DeepEqual(wantFilter, gotFilter):
				t.Errorf("filters not equal: \n wanted: %#v \n got:    %#v", wantFilter, gotFilter)
			case !reflect.DeepEqual(wantUpdate, gotUpdate):
				t.Errorf("updates not equal: \n wanted: %#v \n got:    %#v", wantUpdate, gotUpdate)
			case !reflect.DeepEqual(wantOpts, gotOps):
				t.Errorf("options not equal: \n wanted: %#v \n got:    %#v", wantOpts, gotOps)
			}
			return &mongo.UpdateResult{ModifiedCount: 1}, nil
		}
	}
	b := book.Book{
		Header:      book.Header{ID: okID1, Title: "2", Author: "3", Subject: "4"},
		Description: "5", DeweyDecClass: "6", Pages: 7, Publisher: "8",
		PublishDate: time.Date(2000, 2, 29, 0, 0, 0, 0, time.UTC),
		AddedDate:   time.Date(2022, 11, 16, 0, 0, 0, 0, time.UTC),
		EanIsbn13:   "11", UpcIsbn10: "12", ImageBase64: "13",
	}
	wantUpdate1 := bson.D(bson.E("$set", bson.D(
		bson.E(bookTitleField, b.Title),
		bson.E(bookAuthorField, b.Author),
		bson.E(bookSubjectField, b.Subject),
		bson.E(bookDescriptionField, b.Description),
		bson.E(bookDeweyDecClassField, b.DeweyDecClass),
		bson.E(bookPagesField, b.Pages),
		bson.E(bookPublisherField, b.Publisher),
		bson.E(bookPublishDateField, b.PublishDate),
		bson.E(bookAddedDateField, b.AddedDate),
		bson.E(bookEanIsbn13Field, b.EanIsbn13),
		bson.E(bookUpcIsbn0Field, b.UpcIsbn10),
	)))
	wantUpdate2 := bson.D(bson.E("$set", bson.D(
		bson.E(bookTitleField, b.Title),
		bson.E(bookAuthorField, b.Author),
		bson.E(bookSubjectField, b.Subject),
		bson.E(bookDescriptionField, b.Description),
		bson.E(bookDeweyDecClassField, b.DeweyDecClass),
		bson.E(bookPagesField, b.Pages),
		bson.E(bookPublisherField, b.Publisher),
		bson.E(bookPublishDateField, b.PublishDate),
		bson.E(bookAddedDateField, b.AddedDate),
		bson.E(bookEanIsbn13Field, b.EanIsbn13),
		bson.E(bookUpcIsbn0Field, b.UpcIsbn10),
		bson.E(bookImageBase64Field, b.ImageBase64),
	)))
	tests := []struct {
		name          string
		book          book.Book
		updateImage   bool
		UpdateOneFunc func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error)
		wantOk        bool
	}{
		{
			name: "bad id",
			book: func() book.Book { b2 := b; b2.ID = "bad id"; return b2 }(),
		},
		{
			name: "update error",
			book: b,
			UpdateOneFunc: func(ctx context.Context, filter, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
				return nil, fmt.Errorf("update error")
			},
		},
		{
			name: "bad ModifiedCount: 0",
			book: b,
			UpdateOneFunc: func(ctx context.Context, filter, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
				return &mongo.UpdateResult{ModifiedCount: 0}, nil
			},
		},
		{
			name: "bad ModifiedCount: 2",
			book: b,
			UpdateOneFunc: func(ctx context.Context, filter, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
				return &mongo.UpdateResult{ModifiedCount: 2}, nil
			},
		},
		{
			name:          "happy path",
			book:          b,
			UpdateOneFunc: happyPathUpdateOneFunc(t, wantUpdate1),
			wantOk:        true,
		},
		{
			name:          "happy path updateImage",
			book:          b,
			updateImage:   true,
			UpdateOneFunc: happyPathUpdateOneFunc(t, wantUpdate2),
			wantOk:        true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := Database{
				booksCollection: mockCollection{
					UpdateOneFunc: test.UpdateOneFunc,
				},
			}
			ctx := context.Background()
			err := d.UpdateBook(ctx, test.book, test.updateImage)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			}
		})
	}
}

func TestDeleteBook(t *testing.T) {
	const okID = okID1
	tests := []struct {
		name          string
		bookID        string
		DeleteOneFunc func(ctx context.Context, filter interface{}, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error)
		wantOk        bool
	}{
		{
			name:   "bad id",
			bookID: "bad id",
		},
		{
			name:   "delete error",
			bookID: okID,
			DeleteOneFunc: func(ctx context.Context, filter interface{}, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
				return nil, fmt.Errorf("update error")
			},
		},
		{
			name:   "bad ModifiedCount: 0",
			bookID: okID,
			DeleteOneFunc: func(ctx context.Context, filter interface{}, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
				return &mongo.DeleteResult{DeletedCount: 0}, nil
			},
		},
		{
			name:   "bad ModifiedCount: 2",
			bookID: okID,
			DeleteOneFunc: func(ctx context.Context, filter interface{}, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
				return &mongo.DeleteResult{DeletedCount: 0}, nil
			},
		},
		{
			name:   "happy path",
			bookID: okID,
			DeleteOneFunc: func(ctx context.Context, filter interface{}, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error) {
				wantFilter := bson.D(bson.E(bookIDField, objectIDHelper(t, okID)))
				gotFilter := filter
				wantOpts := options.Delete()
				gotOpts := options.MergeDeleteOptions(opts...)
				switch {
				case !reflect.DeepEqual(wantFilter, gotFilter):
					t.Errorf("filters not equal: \n wanted: %#v \n got:    %#v", wantFilter, gotFilter)
				case !reflect.DeepEqual(wantOpts, gotOpts):
					t.Errorf("options not equal: \n wanted: %#v \n got:    %#v", wantOpts, gotOpts)
				}
				return &mongo.DeleteResult{DeletedCount: 1}, nil
			},
			wantOk: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := Database{
				booksCollection: mockCollection{
					DeleteOneFunc: test.DeleteOneFunc,
				},
			}
			ctx := context.Background()
			err := d.DeleteBook(ctx, test.bookID)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			}
		})
	}
}

func TestReadAdminPassword(t *testing.T) {
	tests := []struct {
		name        string
		FindOneFunc func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult
		wantOk      bool
		want        []byte
	}{
		{
			name: "bad hashed password",
			FindOneFunc: func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
				err := fmt.Errorf("bad hashed password")
				return mongo.NewSingleResultFromDocument(nil, err, nil)
			},
		},
		{
			name: "happy path",
			FindOneFunc: func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
				wantFilter := bson.D(bson.E(usernameField, adminUsername))
				gotFilter := filter
				wantOpts := options.FindOne()
				gotOpts := options.MergeFindOneOptions(opts...)
				switch {
				case !reflect.DeepEqual(wantFilter, gotFilter):
					t.Errorf("filters not equal: \n wanted: %#v \n got:    %#v", wantFilter, gotFilter)
				case !reflect.DeepEqual(wantOpts, gotOpts):
					t.Errorf("options not equal: \n wanted: %#v \n got:    %#v", wantOpts, gotOpts)
				}
				document := mUser{
					Password: "$n0wY",
				}
				return mongo.NewSingleResultFromDocument(document, nil, nil)
			},
			wantOk: true,
			want:   []byte("$n0wY"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := Database{
				usersCollection: mockCollection{
					FindOneFunc: test.FindOneFunc,
				},
			}
			ctx := context.Background()
			got, err := d.ReadAdminPassword(ctx)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("hashed passwords not equal: \n wanted: %q \n got:    %q", test.want, got)
			}
		})
	}
}

func TestUpdateAdminPassword(t *testing.T) {
	tests := []struct {
		name           string
		hashedPassword string
		UpdateOneFunc  func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error)
		wantOk         bool
	}{
		{
			name: "delete error",
			UpdateOneFunc: func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
				return nil, fmt.Errorf("delete error")
			},
		},
		{
			name: "bad ModifiedCount: 0",
			UpdateOneFunc: func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
				return &mongo.UpdateResult{ModifiedCount: 0}, nil
			},
		},
		{
			name: "bad ModifiedCount: 66",
			UpdateOneFunc: func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
				return &mongo.UpdateResult{ModifiedCount: 2}, nil
			},
		},
		{
			name:           "happy path",
			hashedPassword: "t0p_S3cr3t!",
			UpdateOneFunc: func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
				wantFilter := bson.D(bson.E(usernameField, adminUsername))
				gotFilter := filter
				wantUpdate := bson.D(bson.E("$set", bson.D(bson.E(passwordField, "t0p_S3cr3t!"))))
				gotUpdate := update
				wantOpts := options.Update().
					SetUpsert(true)
				gotOps := options.MergeUpdateOptions(opts...)
				switch {
				case !reflect.DeepEqual(wantFilter, gotFilter):
					t.Errorf("filters not equal: \n wanted: %#v \n got:    %#v", wantFilter, gotFilter)
				case !reflect.DeepEqual(wantUpdate, gotUpdate):
					t.Errorf("updates not equal: \n wanted: %#v \n got:    %#v", wantUpdate, gotUpdate)
				case !reflect.DeepEqual(wantOpts, gotOps):
					t.Errorf("options not equal: \n wanted: %#v \n got:    %#v", wantOpts, gotOps)
				}
				return &mongo.UpdateResult{ModifiedCount: 1}, nil
			},
			wantOk: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := Database{
				usersCollection: mockCollection{
					UpdateOneFunc: test.UpdateOneFunc,
				},
			}
			ctx := context.Background()
			err := d.UpdateAdminPassword(ctx, test.hashedPassword)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			}
		})
	}
}
