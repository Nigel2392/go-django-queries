package queries_test

import (
	"testing"

	queries "github.com/Nigel2392/go-django-queries/src"
)

func TestRelationForeignKey(t *testing.T) {
	var rel = queries.NewForeignKeyRelation(
		&Book{}, "Author", &Author{},
	)

	var author1 = &Author{
		Name: "Author 1",
	}

	var author2 = &Author{
		Name: "Author 2",
	}

	var booksAuthor1 = []*Book{
		{Title: "Book 1", Author: author1},
		{Title: "Book 2", Author: author1},
		{Title: "Book 3", Author: author1},
	}

	var booksAuthor2 = []*Book{
		{Title: "Book 4", Author: author2},
		{Title: "Book 5", Author: author2},
		{Title: "Book 6", Author: author2},
	}

	if err := queries.CreateObject(author1); err != nil {
		t.Errorf("Failed to create author1: %v", err)
	}

	if err := queries.CreateObject(author2); err != nil {
		t.Errorf("Failed to create author2: %v", err)
	}

	for _, book := range booksAuthor1 {
		if err := queries.CreateObject(book); err != nil {
			t.Errorf("Failed to create book %s: %v", book.Title, err)
		}
	}

	for _, book := range booksAuthor2 {
		if err := queries.CreateObject(book); err != nil {
			t.Errorf("Failed to create book %s: %v", book.Title, err)
		}
	}

	t.Run("TestRelationForeignKeyForward", func(t *testing.T) {
		authors, err := rel.Forward(booksAuthor1[0])
		if err != nil {
			t.Errorf("Failed to get author for book %s: %v", booksAuthor1[0].Title, err)
		}

		if len(authors) != 1 {
			t.Errorf("Expected 1 author, got %d", len(authors))
		}

		if authors[0].(*Author).ID != author1.ID {
			t.Errorf("Expected author ID %d, got %d", author1.ID, authors[0].(*Author).ID)
		}

		t.Logf("Book %q belongs to author %q", booksAuthor1[0].Title, authors[0].(*Author).Name)
	})

	t.Run("TestRelationForeignKeyReverse", func(t *testing.T) {
		var books, err = rel.Reverse(author1)
		if err != nil {
			t.Errorf("Failed to get books for author1: %v", err)
		}

		if len(books) != len(booksAuthor1) {
			t.Errorf("Expected %d books for author1, got %d", len(booksAuthor1), len(books))
		}

		for i, book := range books {
			if book.(*Book).Title != booksAuthor1[i].Title {
				t.Errorf("Expected book %s, got %s", booksAuthor1[i].Title, book.(*Book).Title)
			}

			t.Logf("Book %q belongs to author1", book.(*Book).Title)
		}
	})
}
