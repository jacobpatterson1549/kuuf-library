// Package mongo provides a database for the library.
package mongo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Database struct {
	client       *mongo.Client
	QueryTimeout time.Duration
}

const (
	libraryDatabase        = "kuuf-library-db"
	booksCollection        = "books"
	usersCollection        = "users"
	adminUsername          = "admin"
	bookIDField            = "_id"
	bookTitleField         = "title"
	bookAuthorField        = "author"
	bookSubjectField       = "subject"
	bookDescriptionField   = "description"
	bookDeweyDecClassField = "dewey-dec-class"
	bookPagesField         = "pages"
	bookPublisherField     = "publisher"
	bookPublishDateField   = "publish-date"
	bookAddedDateField     = "added-date"
	bookEAN_ISBN13Field    = "ean-isbn13"
	bookUPC_ISBN10Field    = "upc-isbn10"
	bookImageBase64Field   = "image-base64"
	usernameField          = "username"
	passwordField          = "password"
	dateLayout             = book.HyphenatedYYYYMMDD
)

func NewDatabase(url string, queryTimeout time.Duration) (*Database, error) {
	d := Database{
		QueryTimeout: queryTimeout,
	}
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		clientOptions := options.Client()
		clientOptions.ApplyURI(url)
		client, err := mongo.Connect(ctx, clientOptions)
		if err != nil {
			return err
		}
		d.client = client
		return nil
	}); err != nil {
		return nil, fmt.Errorf("connecting to mongo: %w", err)
	}
	return &d, nil
}

func (d *Database) withTimeoutContext(f func(ctx context.Context) error) error {
	ctx := context.Background()
	ctx, cancelFunc := context.WithTimeout(ctx, d.QueryTimeout)
	defer cancelFunc()
	return f(ctx)
}

func (d *Database) libraryDatabase() *mongo.Database {
	return d.client.Database(libraryDatabase)
}

func (d *Database) booksCollection() *mongo.Collection {
	return d.libraryDatabase().Collection(booksCollection)
}

func (d *Database) usersCollection() *mongo.Collection {
	return d.libraryDatabase().Collection(usersCollection)
}

func (d *Database) CreateBooks(books ...book.Book) ([]book.Book, error) {
	if len(books) == 0 {
		return nil, nil
	}
	docs := make([]interface{}, len(books))
	for i, b := range books {
		d := map[string]string{
			// book id is created as 24 characters long hex string
			bookTitleField:         b.Title,
			bookAuthorField:        b.Author,
			bookSubjectField:       b.Subject,
			bookDescriptionField:   b.Description,
			bookDeweyDecClassField: b.DeweyDecClass,
			bookPagesField:         strconv.Itoa(b.Pages),
			bookPublisherField:     b.Publisher,
			bookPublishDateField:   b.PublishDate.Format(string(dateLayout)),
			bookAddedDateField:     b.AddedDate.Format(string(dateLayout)),
			bookEAN_ISBN13Field:    b.EAN_ISBN13,
			bookUPC_ISBN10Field:    b.UPC_ISBN10,
			bookImageBase64Field:   b.ImageBase64,
		}
		docs[i] = d
	}
	coll := d.booksCollection()
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		ids, err := coll.InsertMany(ctx, docs)
		if err != nil {
			return err
		}
		for i, id := range ids.InsertedIDs {
			objID := id.(primitive.ObjectID)
			books[i].ID = objID.Hex()
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("creating books: %w", err)
	}
	return books, nil
}

func (d *Database) ReadBookSubjects(limit, offset int) ([]book.Subject, error) {
	var subjects []book.Subject
	const idField = "_id"
	const countField = "count"
	groupStage := d.d(d.e("$group", d.d(
		d.e(idField, "$"+bookSubjectField),
		d.e(countField, d.d(d.e("$sum", 1))),
	)))
	sortStage := d.d(d.e("$sort", d.d(
		d.e(countField, -1),
		d.e(bookIDField, 1),
	)))
	skipStage := d.d(d.e("$skip", offset))
	limitStage := d.d(d.e("$limit", limit))
	pipeline := mongo.Pipeline{groupStage, sortStage, skipStage, limitStage}
	coll := d.booksCollection()
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		cur, err := coll.Aggregate(ctx, pipeline)
		if err != nil {
			return err
		}
		var all []map[string]interface{}
		if err := cur.All(ctx, &all); err != nil {
			return err
		}
		subjects = make([]book.Subject, len(all))
		for i, m := range all {
			name, ok := m[idField].(string)
			if !ok {
				return fmt.Errorf("getting name for subject #%v, type is %T", i, name)
			}
			count, ok := m[countField].(int32)
			if !ok {
				return fmt.Errorf("getting count for subject #%v, type is %T", i, count)
			}
			s := book.Subject{
				Name:  name,
				Count: int(count),
			}
			subjects[i] = s
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reading book subjects: %w", err)
	}
	return subjects, nil
}

func (d *Database) ReadBookHeaders(f book.Filter, limit, offset int) ([]book.Header, error) {
	var headers []book.Header
	filter := d.d(d.filter(f)...)
	sort := d.d(
		d.e(bookSubjectField, 1),
		d.e(bookTitleField, 1),
	)
	projection := d.d(
		d.e(bookIDField, 1),
		d.e(bookTitleField, 1),
		d.e(bookAuthorField, 1),
		d.e(bookSubjectField, 1),
	)
	opts := options.Find()
	opts.SetSort(sort)
	opts.SetLimit(int64(limit))
	opts.SetSkip(int64(offset))
	opts.SetProjection(projection)
	coll := d.booksCollection()
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		cur, err := coll.Find(ctx, filter, opts)
		if err != nil {
			return err
		}
		var all []map[string]string
		if err := cur.All(ctx, &all); err != nil {
			return err
		}
		headers = make([]book.Header, len(all))
		for i, m := range all {
			h := book.Header{
				ID:      m[bookIDField],
				Title:   m[bookTitleField],
				Author:  m[bookAuthorField],
				Subject: m[bookSubjectField],
			}
			headers[i] = h
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reading book headers: %w", err)
	}
	return headers, nil
}

func (d *Database) ReadBook(id string) (*book.Book, error) {
	filter, err := d.objectID(id)
	if err != nil {
		return nil, err
	}
	coll := d.booksCollection()
	var b book.Book
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		result := coll.FindOne(ctx, filter)
		var m map[string]string
		if err := result.Decode(&m); err != nil {
			return err
		}
		sb := book.StringBook{
			ID:            m[bookIDField],
			Title:         m[bookTitleField],
			Author:        m[bookAuthorField],
			Subject:       m[bookSubjectField],
			Description:   m[bookDescriptionField],
			DeweyDecClass: m[bookDeweyDecClassField],
			Pages:         m[bookPagesField],
			Publisher:     m[bookPublisherField],
			PublishDate:   m[bookPublishDateField],
			AddedDate:     m[bookAddedDateField],
			EAN_ISBN13:    m[bookEAN_ISBN13Field],
			UPC_ISBN10:    m[bookUPC_ISBN10Field],
			ImageBase64:   m[bookImageBase64Field],
		}
		b2, err := sb.Book(dateLayout)
		if err != nil {
			return err
		}
		b = *b2
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reading book: %w", err)
	}
	return &b, nil
}

func (d *Database) UpdateBook(b book.Book, updateImage bool) error {
	filter, err := d.objectID(b.ID)
	if err != nil {
		return err
	}
	sets := d.d(
		d.e(bookTitleField, b.Title),
		d.e(bookAuthorField, b.Author),
		d.e(bookSubjectField, b.Subject),
		d.e(bookDescriptionField, b.Description),
		d.e(bookDeweyDecClassField, b.DeweyDecClass),
		d.e(bookPagesField, strconv.Itoa(b.Pages)),
		d.e(bookPublisherField, b.Publisher),
		d.e(bookPublishDateField, b.PublishDate.Format(string(dateLayout))),
		d.e(bookAddedDateField, b.AddedDate.Format(string(dateLayout))),
		d.e(bookEAN_ISBN13Field, b.EAN_ISBN13),
		d.e(bookUPC_ISBN10Field, b.UPC_ISBN10),
	)
	if updateImage {
		sets = append(sets, d.e(bookImageBase64Field, b.ImageBase64))
	}
	update := d.d(d.e("$set", sets))
	coll := d.booksCollection()
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		_, err := coll.UpdateOne(ctx, filter, update)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("updating book: %w", err)
	}
	return nil
}

func (d *Database) DeleteBook(id string) error {
	filter, err := d.objectID(id)
	if err != nil {
		return err
	}
	coll := d.booksCollection()
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		_, err := coll.DeleteOne(ctx, filter)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("deleting book: %w", err)
	}
	return nil
}

func (d *Database) ReadAdminPassword() (hashedPassword []byte, err error) {
	u := d.d(d.e(usernameField, adminUsername))
	filter := u
	coll := d.usersCollection()
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		result := coll.FindOne(ctx, filter)
		if err != nil {
			return err
		}
		var m map[string]string
		if err := result.Decode(&m); err != nil {
			return err
		}
		s, ok := m[passwordField]
		if !ok {
			return fmt.Errorf("user does not exist")
		}
		hashedPassword = []byte(s)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reading admin password: %w", err)
	}
	return hashedPassword, nil
}

func (d *Database) UpdateAdminPassword(hashedPassword string) error {
	u := d.d(d.e(usernameField, adminUsername))
	p := d.d(d.e(passwordField, hashedPassword))
	filter := u
	update := d.d(d.e("$set", p))
	opts := options.Update()
	opts.SetUpsert(true)
	coll := d.usersCollection()
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		if _, err := coll.UpdateOne(ctx, filter, update, opts); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("updating admin password: %w", err)
	}
	return nil
}

func (Database) d(e ...bson.E) bson.D {
	return bson.D(e)
}

func (Database) e(key string, value interface{}) bson.E {
	return bson.E{
		Key:   key,
		Value: value,
	}
}
func (Database) a(d ...interface{}) bson.A {
	return bson.A(d)
}

func (d Database) objectID(id string) (bson.D, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid object id: %w", err)
	}
	return d.d(d.e(bookIDField, objID)), nil
}

func (d Database) filter(filter book.Filter) []bson.E {
	var parts []bson.E
	if len(filter.Subject) != 0 {
		subjectPart := d.e(bookSubjectField, filter.Subject)
		parts = append(parts, subjectPart)
	}
	if len(filter.HeaderParts) != 0 {
		joinedFilter := strings.Join(filter.HeaderParts, "|")
		regex := primitive.Regex{
			Pattern: joinedFilter,
			Options: "i",
		}
		headerParts := (d.e(
			"$or",
			d.a(
				d.d(d.e(bookTitleField, regex)),
				d.d(d.e(bookAuthorField, regex)),
				d.d(d.e(bookSubjectField, regex)),
			)))
		parts = append(parts, headerParts)
	}
	if len(parts) == 0 {
		parts = append(parts, d.e("", nil)) // mongo does not like a nil slice
	}
	return parts
}
