package queries_test

import (
	"context"
	"testing"
	"time"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django-queries/src/models"
	"github.com/Nigel2392/go-django-queries/src/quest"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/contenttypes"
)

type ProxyModel struct {
	models.Model
	ID          int64
	TargetID    int64
	TargetCType string
	Title       string
	Description string
}

func (b *ProxyModel) TargetContentTypeField() attrs.FieldDefinition {
	var defs = b.FieldDefs()
	var f, _ = defs.Field("TargetCType")
	return f
}

func (b *ProxyModel) TargetPrimaryField() attrs.FieldDefinition {
	var defs = b.FieldDefs()
	var f, _ = defs.Field("TargetID")
	return f
}

func (b *ProxyModel) FieldDefs() attrs.Definitions {
	return b.Model.Define(b,
		attrs.Unbound("ID", &attrs.FieldConfig{Primary: true}),
		attrs.Unbound("TargetID"),
		attrs.Unbound("TargetCType"),
		attrs.Unbound("Title"),
		attrs.Unbound("Description"),
	)
}

type ProxiedModel struct {
	models.Model
	*ProxyModel
	ID        int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (p *ProxiedModel) FieldDefs() attrs.Definitions {
	return p.Model.Define(p,
		attrs.Unbound("ID", &attrs.FieldConfig{Primary: true}),
		attrs.Unbound("CreatedAt"),
		attrs.Unbound("UpdatedAt"),
	)
}

func TestProxyModel(t *testing.T) {
	var tables = quest.Table(t,
		&ProxyModel{},
		&ProxiedModel{},
	)

	tables.Create()
	defer tables.Drop()

	var proxyModel = models.Setup(&ProxiedModel{
		ProxyModel: &ProxyModel{
			Title:       "Test Proxy",
			Description: "This is a test proxy model",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	var ctx = context.Background()
	if err := proxyModel.Save(ctx); err != nil {
		t.Fatalf("Failed to save proxy model: %v", err)
	}

	var loadedModel, err = queries.GetQuerySet(&ProxiedModel{}).
		WithContext(ctx).
		Filter("ID", proxyModel.ID).
		First()
	if err != nil {
		t.Fatalf("Failed to load proxy model: %v", err)
	}
	if loadedModel == nil {
		t.Fatal("Expected to load a proxy model, but got nil")
	}
	if loadedModel.Object.ID != proxyModel.ID {
		t.Fatalf("Expected loaded model ID to be %d, but got %d", proxyModel.ID, loadedModel.Object.ID)
	}
	if loadedModel.Object.CreatedAt.IsZero() || loadedModel.Object.UpdatedAt.IsZero() {
		t.Fatal("Expected CreatedAt and UpdatedAt to be set, but they are zero values")
	}
	if loadedModel.Object.ProxyModel == nil {
		t.Fatal("Expected ProxyModel to be initialized, but it is nil")
	}
	if loadedModel.Object.ProxyModel.ID != 1 {
		t.Fatalf("Expected TargetID to be %d, but got %d", 1, loadedModel.Object.ProxyModel.TargetID)
	}
	if loadedModel.Object.ProxyModel.Title != "Test Proxy" {
		t.Fatalf("Expected ProxyModel Title to be 'Test Proxy', but got '%s'", loadedModel.Object.ProxyModel.Title)
	}
	if loadedModel.Object.ProxyModel.Description != "This is a test proxy model" {
		t.Fatalf("Expected ProxyModel Description to be 'This is a test proxy model', but got '%s'", loadedModel.Object.ProxyModel.Description)
	}
	if loadedModel.Object.ProxyModel.TargetCType != contenttypes.NewContentType[attrs.Definer](loadedModel.Object).TypeName() {
		t.Fatalf("Expected TargetCType to be '%s', but got '%s'", contenttypes.NewContentType[attrs.Definer](loadedModel.Object).TypeName(), loadedModel.Object.ProxyModel.TargetCType)
	}
	if loadedModel.Object.ProxyModel.TargetID != loadedModel.Object.ID {
		t.Fatalf("Expected TargetID to be %d, but got %d", loadedModel.Object.ID, loadedModel.Object.ProxyModel.TargetID)
	}
}

func TestProxyFields(t *testing.T) {
	var proxiedModel = models.Setup(&ProxiedModel{
		ProxyModel: &ProxyModel{
			Title:       "Test Proxy",
			Description: "This is a test proxy model",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	var proxyFields = queries.ProxyFields(proxiedModel)
	if proxyFields == nil {
		t.Fatal("Expected proxy fields to be initialized, but got nil")
	}

	if proxyFields.Len() != 1 {
		t.Fatalf("Expected 1 proxy field, but got %d", proxyFields.Len())
	}

	var proxyField, ok = proxyFields.Get("__PROXY")
	if !ok {
		t.Fatal("Expected to find proxy field with name '__PROXY'")
	}

	if proxyField.Name() != "__PROXY" {
		t.Fatalf("Expected proxy field name to be '__PROXY', but got '%s'", proxyField.Name())
	}
}
