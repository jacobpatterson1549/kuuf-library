// Package mongo provides a database for the library.
package mongo

import (
	"context"
	"fmt"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/mongo/bson"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/mongo/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type (
	Database struct {
		booksCollection mCollection
		usersCollection mCollection
		QueryTimeout    time.Duration
	}
	mCollection interface {
		InsertMany(ctx context.Context, documents []interface{}, opts ...*options.InsertManyOptions) (*mongo.InsertManyResult, error)
		Aggregate(ctx context.Context, pipeline interface{}, opts ...*options.AggregateOptions) (*mongo.Cursor, error)
		Find(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (cur *mongo.Cursor, err error)
		FindOne(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult
		UpdateOne(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error)
		DeleteOne(ctx context.Context, filter interface{}, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error)
	}
	mBook struct {
		Header        mHeader   `bson:",inline"`
		Description   string    `bson:"description"`
		DeweyDecClass string    `bson:"dewey_dec_class"`
		Pages         int       `bson:"pages"`
		Publisher     string    `bson:"publisher"`
		PublishDate   time.Time `bson:"publish_date"`
		AddedDate     time.Time `bson:"added_date"`
		EanIsbn13     string    `bson:"ean_isbn13"`
		UpcIsbn10     string    `bson:"upc_isbn10"`
		ImageBase64   string    `bson:"image_base64"`
	}
	mHeader struct {
		ID      string `bson:"_id,omitempty"`
		Title   string `bson:"title"`
		Author  string `bson:"author"`
		Subject string `bson:"subject"`
	}
	mSubject struct {
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
	bookEanIsbn13Field     = "ean_isbn13"
	bookUpcIsbn0Field      = "upc_isbn10"
	bookImageBase64Field   = "image_base64"
	subjectNameField       = "_id"
	subjectCountField      = "count"
	usernameField          = "username"
	passwordField          = "password"
	dateLayout             = book.HyphenatedYYYYMMDD
)

func NewDatabase(url string, queryTimeout time.Duration) (*Database, error) {
	d := Database{
		QueryTimeout: queryTimeout,
	}
	opts := options.Client().
		ApplyURI(url)
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		client, err := mongo.Connect(ctx, opts)
		if err != nil {
			return err
		}
		database := client.Database(libraryDatabase)
		d.booksCollection = database.Collection(booksCollection)
		d.usersCollection = database.Collection(usersCollection)
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

func (d *Database) CreateBooks(books ...book.Book) ([]book.Book, error) {
	if len(books) == 0 {
		return nil, nil
	}
	docs := make([]interface{}, len(books))
	for i, b := range books {
		b.ID = "" // request a new id
		docs[i] = mongoBook(b)
	}
	opts := options.InsertMany()
	coll := d.booksCollection
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		ids, err := coll.InsertMany(ctx, docs, opts)
		if err != nil {
			return err
		}
		for i, id := range ids.InsertedIDs {
			objID, err := primitive.ToObjectID(id)
			if err != nil {
				return err
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
	pipeline := mongo.Pipeline{
		bson.D(bson.E("$group", bson.D(
			bson.E(subjectNameField, "$"+bookSubjectField),
			bson.E(subjectCountField, bson.D(bson.E("$sum", 1))),
		))),
		bson.D(bson.E("$sort", bson.D(
			bson.E(subjectNameField, 1),
		))),
		bson.D(bson.E("$skip", offset)),
		bson.D(bson.E("$limit", limit)),
	}
	opts := options.Aggregate()
	coll := d.booksCollection
	var all []mSubject
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		cur, err := coll.Aggregate(ctx, pipeline, opts)
		if err != nil {
			return err
		}
		if err := cur.All(ctx, &all); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reading book subjects: %w", err)
	}
	subjects := make([]book.Subject, len(all))
	for i, m := range all {
		subjects[i] = m.Subject()
	}
	return subjects, nil
}

func (d *Database) ReadBookHeaders(filter book.Filter, limit, offset int) ([]book.Header, error) {
	bsonFilter := bson.Filter{
		SubjectKey: bookSubjectField,
		HeaderKeys: []string{
			bookTitleField,
			bookAuthorField,
			bookSubjectField,
		},
	}
	mongoFilter := bson.D(bsonFilter.From(filter)...)
	opts := options.Find().
		SetSort(bson.D(
			bson.E(bookSubjectField, 1),
			bson.E(bookTitleField, 1),
		)).
		SetLimit(int64(limit)).
		SetSkip(int64(offset)).
		SetProjection(bson.D(
			bson.E(bookIDField, 1),
			bson.E(bookTitleField, 1),
			bson.E(bookAuthorField, 1),
			bson.E(bookSubjectField, 1),
		))
	coll := d.booksCollection
	var all []mHeader
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		cur, err := coll.Find(ctx, mongoFilter, opts)
		if err != nil {
			return err
		}
		if err := cur.All(ctx, &all); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reading book headers: %w", err)
	}
	headers := make([]book.Header, len(all))
	for i, m := range all {
		headers[i] = m.Header()
	}
	return headers, nil
}

func (d *Database) ReadBook(id string) (*book.Book, error) {
	filter, err := d.idFilter(id)
	if err != nil {
		return nil, err
	}
	coll := d.booksCollection
	opts := options.FindOne()
	var m mBook
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		result := coll.FindOne(ctx, filter, opts)
		if err := result.Decode(&m); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reading book: %w", err)
	}
	b := m.Book()
	return &b, nil
}

func (d *Database) UpdateBook(b book.Book, updateImage bool) error {
	filter, err := d.idFilter(b.ID)
	if err != nil {
		return err
	}
	sets := bson.D(
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
	)
	if updateImage {
		sets = append(sets, bson.E(bookImageBase64Field, b.ImageBase64))
	}
	update := bson.D(bson.E("$set", sets))
	opts := options.Update()
	coll := d.booksCollection
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		result, err := coll.UpdateOne(ctx, filter, update, opts)
		if err != nil {
			return err
		}
		return d.expectSingleModify(result.ModifiedCount)
	}); err != nil {
		return fmt.Errorf("updating book: %w", err)
	}
	return nil
}

func (d *Database) DeleteBook(id string) error {
	filter, err := d.idFilter(id)
	if err != nil {
		return err
	}
	opts := options.Delete()
	coll := d.booksCollection
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		result, err := coll.DeleteOne(ctx, filter, opts)
		if err != nil {
			return err
		}
		return d.expectSingleModify(result.DeletedCount)
	}); err != nil {
		return fmt.Errorf("deleting book: %w", err)
	}
	return nil
}

func (d *Database) ReadAdminPassword() (hashedPassword []byte, err error) {
	filter := bson.D(bson.E(usernameField, adminUsername))
	coll := d.usersCollection
	opts := options.FindOne()
	var u mUser
	if err = d.withTimeoutContext(func(ctx context.Context) error {
		result := coll.FindOne(ctx, filter, opts)
		if err = result.Decode(&u); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reading admin password: %w", err)
	}
	hashedPassword = []byte(u.Password)
	return hashedPassword, nil
}

func (d *Database) UpdateAdminPassword(hashedPassword string) error {
	filter := bson.D(bson.E(usernameField, adminUsername))
	update := bson.D(bson.E("$set", bson.D(bson.E(passwordField, hashedPassword))))
	opts := options.Update().
		SetUpsert(true)
	coll := d.usersCollection
	if err := d.withTimeoutContext(func(ctx context.Context) error {
		result, err := coll.UpdateOne(ctx, filter, update, opts)
		if err != nil {
			return err
		}
		return d.expectSingleModify(result.ModifiedCount)
	}); err != nil {
		return fmt.Errorf("updating admin password: %w", err)
	}
	return nil
}

func (d Database) idFilter(id string) (interface{}, error) {
	objID, err := primitive.ObjectIDFromString(id)
	if err != nil {
		return nil, err
	}
	return bson.D(bson.E(bookIDField, objID)), nil
}

func (d Database) expectSingleModify(got int64) error {
	if got != 1 {
		return fmt.Errorf("wanted to modify 1 document, got %v", got)
	}
	return nil
}
