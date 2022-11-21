// Package mongo provides a database for the library.
package mongo

import (
	"context"
	"fmt"
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

type (
	mBook struct {
		Header        mHeader   `bson:",inline"`
		Description   string    `bson:"description"`
		DeweyDecClass string    `bson:"dewey_dec_class"`
		Pages         int       `bson:"pages"`
		Publisher     string    `bson:"publisher"`
		PublishDate   time.Time `bson:"publish_date"`
		AddedDate     time.Time `bson:"added_date"`
		EAN_ISBN13    string    `bson:"ean_isbn13"`
		UPC_ISBN10    string    `bson:"upc_isbn10"`
		ImageBase64   string    `bson:"image_base64"`
	}
	mHeader struct {
		ID      string `bson:"_id,omitempty"`
		Title   string `bson:"title"`
		Author  string `bson:"author"`
		Subject string `bson:"subject"`
	}
	MSubject struct {
		Name  string `bson:"_id"`
		Count int    `bson:"count"`
	}
	mUser struct {
		Username string `bson:"username"`
		Password string `bson:"password"`
	}
)

const (
	libraryDatabase        = "kuuf_library_db"
	booksCollection        = "books"
	usersCollection        = "users"
	adminUsername          = "admin"
	bookIDField            = "_id"
	bookTitleField         = "title"
	bookAuthorField        = "author"
	bookSubjectField       = "subject"
	bookDescriptionField   = "description"
	bookDeweyDecClassField = "dewey_dec_class"
	bookPagesField         = "pages"
	bookPublisherField     = "publisher"
	bookPublishDateField   = "publish_date"
	bookAddedDateField     = "added_date"
	bookEAN_ISBN13Field    = "ean_isbn13"
	bookUPC_ISBN10Field    = "upc_isbn10"
	bookImageBase64Field   = "image_base64"
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
		b.ID = "" // request a new id
		docs[i] = mongoBook(b)
	}
	coll := d.booksCollection()
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		ids, err := coll.InsertMany(ctx, docs)
		if err != nil {
			return err
		}
		for i, id := range ids.InsertedIDs {
			objID, ok := id.(primitive.ObjectID)
			if !ok {
				return fmt.Errorf("ID of inserted book #%v is not a string: %T (%v)", i, id, id)
			}
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
		var all []MSubject
		if err := cur.All(ctx, &all); err != nil {
			return err
		}
		subjects = make([]book.Subject, len(all))
		for i, m := range all {
			subjects[i] = m.Subject()
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
		var all []mHeader
		if err := cur.All(ctx, &all); err != nil {
			return err
		}
		headers = make([]book.Header, len(all))
		for i, m := range all {
			headers[i] = m.Header()
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
		var m mBook
		if err := result.Decode(&m); err != nil {
			return err
		}
		b = m.Book()
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
		d.e(bookPagesField, b.Pages),
		d.e(bookPublisherField, b.Publisher),
		d.e(bookPublishDateField, b.PublishDate),
		d.e(bookAddedDateField, b.AddedDate),
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
		var u mUser
		if err := result.Decode(&u); err != nil {
			return err
		}
		hashedPassword = []byte(u.Password)
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

func mongoBook(b book.Book) mBook {
	return mBook{
		Header:        mongoHeader(b.Header),
		Description:   b.Description,
		DeweyDecClass: b.DeweyDecClass,
		Pages:         b.Pages,
		Publisher:     b.Publisher,
		PublishDate:   b.PublishDate,
		AddedDate:     b.AddedDate,
		EAN_ISBN13:    b.EAN_ISBN13,
		UPC_ISBN10:    b.UPC_ISBN10,
		ImageBase64:   b.ImageBase64,
	}
}

func mongoHeader(h book.Header) mHeader {
	return mHeader{
		ID:      h.ID,
		Title:   h.Title,
		Author:  h.Author,
		Subject: h.Subject,
	}
}

func (m mBook) Book() book.Book {
	return book.Book{
		Header:        m.Header.Header(),
		Description:   m.Description,
		DeweyDecClass: m.DeweyDecClass,
		Pages:         m.Pages,
		Publisher:     m.Publisher,
		PublishDate:   m.PublishDate,
		AddedDate:     m.AddedDate,
		EAN_ISBN13:    m.EAN_ISBN13,
		UPC_ISBN10:    m.UPC_ISBN10,
		ImageBase64:   m.ImageBase64,
	}
}

func (m mHeader) Header() book.Header {
	return book.Header{
		ID:      m.ID,
		Title:   m.Title,
		Author:  m.Author,
		Subject: m.Subject,
	}
}

func (m MSubject) Subject() book.Subject {
	return book.Subject{
		Name:  m.Name,
		Count: m.Count,
	}
}
