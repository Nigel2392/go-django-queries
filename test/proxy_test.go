package queries_test

import (
	"context"
	"testing"
	"time"

	"github.com/Nigel2392/go-django-queries/src/models"
	"github.com/Nigel2392/go-django-queries/src/quest"
	"github.com/Nigel2392/go-django/src/core/attrs"
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
	var defs = b.Defs()
	var f, _ = defs.Field("CategoryContentType")
	return f
}

func (b *ProxyModel) TargetPrimaryField() attrs.FieldDefinition {
	var defs = b.Defs()
	var f, _ = defs.Field("Category")
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
}
