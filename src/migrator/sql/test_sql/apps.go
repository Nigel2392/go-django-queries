package testsql

import (
	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/apps"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

func NewAuthAppConfig() django.AppConfig {
	var app = apps.NewAppConfig("auth")
	app.ModelObjects = []attrs.Definer{
		&User{},
		&Profile{},
	}
	return app
}

func NewTodoAppConfig() django.AppConfig {
	var app = apps.NewAppConfig("todo")
	app.ModelObjects = []attrs.Definer{
		&Todo{},
	}
	return app
}

func NewBlogAppConfig() django.AppConfig {
	var app = apps.NewAppConfig("blog")
	app.ModelObjects = []attrs.Definer{
		&BlogPost{},
		&BlogComment{},
	}
	return app
}
