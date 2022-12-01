package mongo

import "github.com/jacobpatterson1549/kuuf-library/internal/book"

func mongoBook(b book.Book) mBook {
	return mBook{
		Header:        mongoHeader(b.Header),
		Description:   b.Description,
		DeweyDecClass: b.DeweyDecClass,
		Pages:         b.Pages,
		Publisher:     b.Publisher,
		PublishDate:   b.PublishDate,
		AddedDate:     b.AddedDate,
		EanIsbn13:     b.EanIsbn13,
		UpcIsbn10:     b.UpcIsbn10,
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
		EanIsbn13:     m.EanIsbn13,
		UpcIsbn10:     m.UpcIsbn10,
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

func (m mSubject) Subject() book.Subject {
	return book.Subject{
		Name:  m.Name,
		Count: m.Count,
	}
}
