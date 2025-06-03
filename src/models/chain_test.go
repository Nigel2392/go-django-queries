package models_test

import (
	"testing"

	"github.com/Nigel2392/go-django-queries/src/models"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

func init() {
	attrs.RegisterModel(&Page{})
	attrs.RegisterModel(&BlogPage{})
	attrs.RegisterModel(&BlogPageCategory{})
}

type BasePage struct {
	models.Model
	ID int64
}

type Page struct {
	BasePage
	Title       string
	Description string
}

func (p *Page) FieldDefs() attrs.Definitions {
	return models.Define(p,
		attrs.Unbound("ID"),
		attrs.Unbound("Title"),
		attrs.Unbound("Description"),
	)
}

type BlogPage struct {
	models.Model
	Proxy  *Page `proxy:"true"`
	Author string
	Tags   []string
}

func (b *BlogPage) FieldDefs() attrs.Definitions {
	return models.Define[attrs.Definer](b,
		models.EmbedProxyModel("Proxy"),
		attrs.Unbound("Author"),
		attrs.Unbound("Tags"),
	)
}

type BlogPageCategory struct {
	models.Model
	*BlogPage
	Category string
}

func (b *BlogPageCategory) FieldDefs() attrs.Definitions {
	return models.Define[attrs.Definer](b,
		models.EmbedProxyModel("BlogPage"),
		attrs.Unbound("Category"),
	)
}

func TestProxyModelChain(t *testing.T) {
	var bCat = &BlogPageCategory{
		BlogPage: &BlogPage{
			//Proxy: &Page{
			//	BasePage: BasePage{
			//		ID: 1,
			//	},
			//	Title:       "Sample Blog Page",
			//	Description: "This is a sample blog page description.",
			//},
			Author: "Jane Doe",
			Tags:   []string{"blog", "sample"},
		},
		Category: "Technology",
	}
	var defs = bCat.FieldDefs()
	_ = defs
	var proxyRoot = bCat.ProxyChain()
	var proxy = proxyRoot
	t.Logf("Proxy Model: %T", proxy)
	for proxy != nil {
		t.Logf("Model: %T, Object: %T %+v", proxy.Model, proxy.Object, proxy.Definitions.Instance())
		if proxy.Next == nil {
			break
		}
		proxy = proxy.Next
	}
	t.Log(bCat.BlogPage.Proxy)

	defs = bCat.FieldDefs()
	defs.Set("Author", "John Smith")

	if bCat.BlogPage.Author != "John Smith" {
		t.Errorf("expected Author to be `John Smith`, got `%s`", bCat.BlogPage.Author)
	}

	var cpyDefs = proxyRoot.Definitions
	cpyDefs.Set("Author", "Alice Johnson")

	if bCat.BlogPage.Author != "John Smith" {
		t.Errorf(
			"expected Author to remain `John Smith`, got `%s` (%p %p %v == %p %p %v | %p %p %v == %p %p %v)",
			bCat.BlogPage.Author,
			bCat, defs.Instance(), bCat == defs.Instance(),
			proxyRoot.Object, proxyRoot.Definitions.Instance(), proxyRoot.Object == proxyRoot.Definitions.Instance(),
			bCat.BlogPage, bCat.BlogPage.FieldDefs().Instance(), bCat.BlogPage == bCat.BlogPage.FieldDefs().Instance(),
			proxyRoot.Next.Object, proxyRoot.Next.Definitions.Instance(), proxyRoot.Next.Object == proxyRoot.Next.Definitions.Instance(),
		)
	}

	cpyDefs = proxyRoot.Next.Definitions
	cpyDefs.Set("Author", "Alice Johnson")

	var cpyCat = proxyRoot.Object.(*BlogPageCategory)
	if cpyCat.BlogPage.Author != "Alice Johnson" {
		t.Errorf("expected cpyCat.BlogPage.Author to be `Alice Johnson`, got `%s`", cpyCat.BlogPage.Author)
	}
}

func TestProxyModelFieldDefs(t *testing.T) {
	var b = &BlogPage{}
	var defs = b.FieldDefs()
	if defs.Len() != 2 {
		t.Errorf("expected 2 fields, got %d", defs.Len())
	}

	b.Proxy = &Page{}
	defs = b.FieldDefs()
	if defs.Len() != 5 {
		t.Errorf("expected 5 fields, got %d", defs.Len())
	}

	defs.Set("ID", 1)
	defs.Set("Title", "New Title")
	defs.Set("Description", "New Description")
	defs.Set("Author", "John Doe")
	defs.Set("Tags", []string{"tag1", "tag2"})

	if b.Proxy.ID != 1 {
		t.Errorf("expected Proxy.ID to be 1, got %d", b.Proxy.ID)
	}

	if b.Proxy.Title != "New Title" {
		t.Errorf("expected Proxy.Title to be 'New Title', got '%s'", b.Proxy.Title)
	}

	if b.Proxy.Description != "New Description" {
		t.Errorf("expected Proxy.Description to be 'New Description', got '%s'", b.Proxy.Description)
	}

	if b.Author != "John Doe" {
		t.Errorf("expected Author to be 'John Doe', got '%s'", b.Author)
	}

	if len(b.Tags) != 2 || b.Tags[0] != "tag1" || b.Tags[1] != "tag2" {
		t.Errorf("expected Tags to be ['tag1', 'tag2'], got %v", b.Tags)
	}

	b.Proxy = nil

	defs = b.FieldDefs()
	if defs.Len() != 2 {
		t.Errorf("expected 2 fields after setting Proxy to nil, got %d", defs.Len())
	}
}
