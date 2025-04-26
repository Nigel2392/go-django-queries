package queries_test

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	queries "github.com/Nigel2392/go-django-queries/src"
	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/logger"
	"github.com/Nigel2392/go-django/src/forms/widgets"
)

const (
	createTableImages = `CREATE TABLE IF NOT EXISTS images (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	path TEXT
)`

	createTableProfiles = `CREATE TABLE IF NOT EXISTS profiles (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	image_id INTEGER REFERENCES images(id),
	name TEXT,
	email TEXT
)`

	createTableUsers = `CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	profile_id INTEGER REFERENCES profiles(id),
	name TEXT
)`
	createTableTodos = `CREATE TABLE IF NOT EXISTS todos (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT,
	description TEXT,
	done BOOLEAN,
	user_id INTEGER REFERENCES users(id)
)`
	selectTodo = `SELECT id, title, description, done, user_id FROM todos WHERE id = ?`
)

type Image struct {
	ID   int
	Path string
}

func (m *Image) FieldDefs() attrs.Definitions {
	return attrs.Define(m,
		attrs.NewField(m, "ID", &attrs.FieldConfig{
			Primary:  true,
			ReadOnly: true,
		}),
		attrs.NewField(m, "Path", &attrs.FieldConfig{}),
	).WithTableName("images")
}

type Profile struct {
	ID    int
	Name  string
	Email string
	Image *Image
}

func (m *Profile) FieldDefs() attrs.Definitions {
	return attrs.Define(m,
		attrs.NewField(m, "ID", &attrs.FieldConfig{
			Primary:  true,
			ReadOnly: true,
		}),
		attrs.NewField(m, "Name", &attrs.FieldConfig{}),
		attrs.NewField(m, "Email", &attrs.FieldConfig{}),
		attrs.NewField(m, "Image", &attrs.FieldConfig{
			RelForeignKey: &Image{},
			Column:        "image_id",
		}),
	).WithTableName("profiles")
}

type User struct {
	ID      int
	Name    string
	Profile *Profile
}

func (m *User) FieldDefs() attrs.Definitions {
	return attrs.Define(m,
		attrs.NewField(m, "ID", &attrs.FieldConfig{
			Primary:  true,
			ReadOnly: true,
		}),
		attrs.NewField(m, "Name", &attrs.FieldConfig{}),
		attrs.NewField(m, "Profile", &attrs.FieldConfig{
			RelForeignKey: &Profile{},
			Column:        "profile_id",
		}),
	).WithTableName("users")
}

type Todo struct {
	ID          int
	Title       string
	Description string
	Done        bool
	User        *User
}

func (m *Todo) FieldDefs() attrs.Definitions {
	return attrs.Define(m,
		attrs.NewField(m, "ID", &attrs.FieldConfig{
			Column:   "id", // can be inferred, but explicitly set for clarity
			Primary:  true,
			ReadOnly: true,
		}),
		attrs.NewField(m, "Title", &attrs.FieldConfig{
			Column: "title", // can be inferred, but explicitly set for clarity
		}),
		attrs.NewField(m, "Description", &attrs.FieldConfig{
			Column: "description", // can be inferred, but explicitly set for clarity
			FormWidget: func(cfg attrs.FieldConfig) widgets.Widget {
				return widgets.NewTextarea(nil)
			},
		}),
		attrs.NewField(m, "Done", &attrs.FieldConfig{}),
		attrs.NewField(m, "User", &attrs.FieldConfig{
			Column:        "user_id",
			RelForeignKey: &User{},
		}),
	).WithTableName("todos")
}

func init() {
	// make db globally available
	var db, err = sql.Open("sqlite3", ":memory:")
	if err != nil {
		panic(err)
	}
	var settings = map[string]interface{}{
		django.APPVAR_DATABASE: db,
	}

	// create tables
	if _, err = db.Exec(createTableImages); err != nil {
		panic(fmt.Sprint("failed to create table images ", err))
	}

	if _, err = db.Exec(createTableProfiles); err != nil {
		panic(fmt.Sprint("failed to create table profiles ", err))
	}

	if _, err = db.Exec(createTableUsers); err != nil {
		panic(fmt.Sprint("failed to create table todos ", err))
	}

	if _, err = db.Exec(createTableTodos); err != nil {
		panic(fmt.Sprint("failed to create table todos ", err))
	}

	logger.Setup(&logger.Logger{
		Level:       logger.DBG,
		OutputDebug: os.Stdout,
		OutputInfo:  os.Stdout,
		OutputWarn:  os.Stdout,
		OutputError: os.Stdout,
	})

	django.App(django.Configure(settings))
}

func TestTodoInsert(t *testing.T) {
	var todos = []*Todo{
		{Title: "Test Todo 1", Description: "Description 1", Done: false},
		{Title: "Test Todo 2", Description: "Description 2", Done: true},
		{Title: "Test Todo 3", Description: "Description 3", Done: false},
	}

	var db = django.ConfigGet[*sql.DB](
		django.Global.Settings,
		django.APPVAR_DATABASE,
	)

	for _, todo := range todos {
		if err := queries.CreateObject(todo); err != nil {
			t.Fatalf("Failed to insert todo: %v", err)
		}

		if todo.ID == 0 {
			t.Fatalf("Expected ID to be set after insert, got 0")
		}

		var row = db.QueryRow(selectTodo, todo.ID)
		var test = &Todo{User: &User{}}
		if err := row.Scan(&test.ID, &test.Title, &test.Description, &test.Done, &sql.NullInt64{}); err != nil {
			t.Fatalf("Failed to query todo: %v", err)
		}

		if test.ID != todo.ID || test.Title != todo.Title || test.Description != todo.Description || test.Done != todo.Done {
			t.Fatalf("Inserted todo does not match expected values: got %+v, want %+v", test, todo)
		}

		t.Logf("Inserted todo: %+v", todo)
	}
}

func TestTodoUpdate(t *testing.T) {
	var db = django.ConfigGet[*sql.DB](
		django.Global.Settings,
		django.APPVAR_DATABASE,
	)

	var todo = &Todo{
		ID:          1,
		Title:       "Updated Todo",
		Description: "Updated Description",
		Done:        true,
	}

	if err := queries.UpdateObject(todo); err != nil {
		t.Fatalf("Failed to update todo: %v", err)
	}

	var row = db.QueryRow(selectTodo, todo.ID)
	var test Todo = Todo{User: &User{}}
	if err := row.Scan(&test.ID, &test.Title, &test.Description, &test.Done, &sql.NullInt64{}); err != nil {
		t.Fatalf("Failed to query todo: %v", err)
	}

	if test.ID != todo.ID || test.Title != todo.Title || test.Description != todo.Description || test.Done != todo.Done {
		t.Fatalf("Updated todo does not match expected values: got %+v, want %+v", test, todo)
	}

	t.Logf("Updated todo: %+v", todo)
}

func TestTodoGet(t *testing.T) {
	var db = django.ConfigGet[*sql.DB](
		django.Global.Settings,
		django.APPVAR_DATABASE,
	)

	var todo, err = queries.GetObject[*Todo](1)
	if err != nil {
		t.Fatalf("Failed to get todo: %v", err)
	}

	var row = db.QueryRow(selectTodo, todo.ID)
	var test Todo = Todo{User: &User{}}
	if err := row.Scan(&test.ID, &test.Title, &test.Description, &test.Done, &sql.NullInt64{}); err != nil {
		t.Fatalf("Failed to query todo: %v", err)
	}

	if test.ID != todo.ID || test.Title != todo.Title || test.Description != todo.Description || test.Done != todo.Done {
		t.Fatalf("Got todo does not match expected values: got %+v, want %+v", test, todo)
	}

	t.Logf("Got todo: %+v", todo)
}

func TestTodoList(t *testing.T) {
	var db = django.ConfigGet[*sql.DB](
		django.Global.Settings,
		django.APPVAR_DATABASE,
	)

	var todos, err = queries.ListObjects[*Todo](0, 1000, "-id")
	if err != nil {
		t.Fatalf("Failed to list todos: %v", err)
	}

	var todoCount = len(todos)
	if len(todos) != 3 {
		t.Fatalf("Expected 3 todos, got %d", todoCount)
	}

	for _, todo := range todos {
		var row = db.QueryRow(selectTodo, todo.ID)
		var test Todo = Todo{User: &User{}}
		if err := row.Scan(&test.ID, &test.Title, &test.Description, &test.Done, &sql.NullInt64{}); err != nil {
			t.Fatalf("Failed to query todo: %v", err)
		}

		if test.ID != todo.ID || test.Title != todo.Title || test.Description != todo.Description || test.Done != todo.Done {
			t.Fatalf("Listed todo does not match expected values: got %+v, want %+v", test, todo)
		}

		t.Logf("Listed todo: %+v", todo)
	}
}

func TestListTodoByIDs(t *testing.T) {
	var db = django.ConfigGet[*sql.DB](
		django.Global.Settings,
		django.APPVAR_DATABASE,
	)

	var ids = []int{1, 2}
	var todos, err = queries.ListObjectsByIDs[*Todo](0, 1000, ids)
	if err != nil {
		t.Fatalf("Failed to get todos by IDs: %v", err)
	}

	var todoCount = len(todos)
	if todoCount != len(ids) {
		t.Fatalf("Expected %d todos, got %d", len(ids), todoCount)
	}

	for _, todo := range todos {
		var row = db.QueryRow(selectTodo, todo.ID)
		var test Todo = Todo{User: &User{}}
		if err := row.Scan(&test.ID, &test.Title, &test.Description, &test.Done, &sql.NullInt64{}); err != nil {
			t.Fatalf("Failed to query todo: %v", err)
		}

		if test.ID != todo.ID || test.Title != todo.Title || test.Description != todo.Description || test.Done != todo.Done {
			t.Fatalf("Got todo does not match expected values: got %+v, want %+v", test, todo)
		}

		t.Logf("Got todo by ID: %+v", todo)
	}
}

func TestTodoDelete(t *testing.T) {
	var db = django.ConfigGet[*sql.DB](
		django.Global.Settings,
		django.APPVAR_DATABASE,
	)
	var err error
	var todo = &Todo{ID: 1}
	if err := queries.DeleteObject(todo); err != nil {
		t.Fatalf("Failed to delete todo: %v", err)
	}

	var row = db.QueryRow(selectTodo, todo.ID)
	var test Todo = Todo{User: &User{}}
	if err = row.Scan(&test.ID, &test.Title, &test.Description, &test.Done, &sql.NullInt64{}); err == nil {
		t.Fatalf("Expected error when querying deleted todo, got: %v", test)
	}

	t.Logf("Deleted todo: %+v, (%s)", todo, err)
}

func TestTodoCount(t *testing.T) {
	var count, err = queries.CountObjects(&Todo{})
	if err != nil {
		t.Fatalf("Failed to count todos: %v", err)
	}

	if count != 2 {
		t.Fatalf("Expected 2 todos, got %d", count)
	}

	t.Logf("Counted todos: %d", count)
}

func TestQuerySet_Filter(t *testing.T) {
	todos, err := queries.Objects(&Todo{}).
		Select("ID", "Title", "Description", "Done").
		Filter("Title__icontains", "test").
		Filter("Done", false).
		OrderBy("-ID").
		Limit(5).
		All().Exec()

	if err != nil {
		t.Fatalf("Failed to filter todos: %v", err)
	}

	if len(todos) != 1 {
		t.Fatalf("Expected 2 todos, got %d", len(todos))
	}

	for _, todo := range todos {
		if !strings.Contains(strings.ToLower(todo.Title), "test") {
			t.Fatalf("Expected todo title to contain 'test', got: %s", todo.Title)
		}

		if todo.Done {
			t.Fatalf("Expected todo to be not done, got done: %+v", todo)
		}
	}

	t.Logf("Filtered todos: %+v", todos)
}

func TestQuerySet_First(t *testing.T) {
	todo, err := queries.Objects(&Todo{}).
		Select("ID", "Title", "Description", "Done").
		Filter("Done", true).
		First().Exec()

	if err != nil {
		t.Fatalf("Failed to get first todo: %v", err)
	}

	if todo == nil {
		t.Fatalf("Expected a todo, got nil")
	}

	if !todo.Done {
		t.Fatalf("Expected todo to be done, got not done: %+v", todo)
	}

	t.Logf("First todo: %+v", todo)
}
func TestQuerySet_Where(t *testing.T) {
	todos, err := queries.Objects(&Todo{}).
		Select("ID", "Title", "Description", "Done").
		Filter(
			queries.Expr("Title", "icontains", "test"),
			queries.Q("Done", false),
		).
		All().Exec()

	if err != nil {
		t.Fatalf("Failed to get first todo: %v", err)
	}

	if len(todos) != 1 {
		t.Fatalf("Expected 1 todo, got %d", len(todos))
	}

	for _, todo := range todos {
		if !strings.Contains(strings.ToLower(todo.Title), "test") {
			t.Fatalf("Expected todo title to contain 'test', got: %s", todo.Title)
		}

		if todo.Done {
			t.Fatalf("Expected todo to be not done, got done: %+v", todo)
		}

		t.Logf("Filtered todos: %+v", todo)
	}
}

func TestQuerySet_Count(t *testing.T) {
	count, err := queries.Objects(&Todo{}).
		Filter(queries.And(
			queries.Expr("Title", "icontains", "test"),
			queries.Q("Done", false),
		)).
		Count().Exec()

	if err != nil {
		t.Fatalf("Failed to count todos: %v", err)
	}

	if count != 1 {
		t.Fatalf("Expected 1 todo, got %d", count)
	}

	t.Logf("Counted todos: %d", count)
}

func TestQueryRelated(t *testing.T) {

	var profile = &Profile{
		Name:  "test profile",
		Email: "test@example.com",
	}

	if err := queries.CreateObject(profile); err != nil || profile.ID == 0 {
		t.Fatalf("Failed to insert profile: %v", err)
	}

	var user = &User{
		Name:    "test user",
		Profile: profile,
	}
	if err := queries.CreateObject(user); err != nil || user.ID == 0 {
		t.Fatalf("Failed to insert user: %v", err)
	}

	var todo = &Todo{
		Title:       "New Test Todo",
		Description: "This is a new test todo",
		Done:        false,
		User:        user,
	}

	if err := queries.CreateObject(todo); err != nil {
		t.Fatalf("Failed to insert todo: %v", err)
	}

	todos, err := queries.Objects(&Todo{}).
		Select("ID", "Title", "Description", "Done", "User.*", "User.Profile.*").
		Filter(
			queries.Q("Title__icontains", "new test"),
			queries.Q("Done", false),
			queries.Q("User.Name__icontains", "test"),
		).
		OrderBy("-ID", "-User.Name").
		Limit(5).
		All().Exec()

	if err != nil {
		t.Fatalf("Failed to filter todos: %v", err)
	}

	if len(todos) != 1 {
		t.Fatalf("Expected 1 todo, got %d", len(todos))
	}

	var dbTodo = todos[0]
	t.Logf("Created todo: %+v, %+v", todo, todo.User)
	t.Logf("Filtered todo: %+v, %+v", dbTodo, dbTodo.User)

	if dbTodo.ID != todo.ID || todo.ID == 0 {
		t.Fatalf("Expected todo ID %d, got %d", todo.ID, dbTodo.ID)
	}

	if dbTodo.Title != todo.Title {
		t.Fatalf("Expected todo title %q, got %q", todo.Title, dbTodo.Title)
	}

	if dbTodo.Description != todo.Description {
		t.Fatalf("Expected todo description %q, got %q", todo.Description, dbTodo.Description)
	}

	if dbTodo.Done != todo.Done {
		t.Fatalf("Expected todo done %v, got %v", todo.Done, dbTodo.Done)
	}

	if dbTodo.User == nil {
		t.Fatalf("Expected todo user to be not nil")
	}

	if dbTodo.User.ID != todo.User.ID {
		t.Fatalf("Expected todo user ID %d, got %d", todo.User.ID, dbTodo.User.ID)
	}

	if dbTodo.User.Name != todo.User.Name {
		t.Fatalf("Expected todo user name %q, got %q", todo.User.Name, dbTodo.User.Name)
	}

	if dbTodo.User.Profile == nil {
		t.Fatalf("Expected todo user profile to be not nil")
	}

	if dbTodo.User.Profile.ID != todo.User.Profile.ID {
		t.Fatalf("Expected todo user profile ID %d, got %d", todo.User.Profile.ID, dbTodo.User.Profile.ID)
	}

	if dbTodo.User.Profile.Name != todo.User.Profile.Name {
		t.Fatalf("Expected todo user profile name %q, got %q", todo.User.Profile.Name, dbTodo.User.Profile.Name)
	}

	if dbTodo.User.Profile.Email != todo.User.Profile.Email {
		t.Fatalf("Expected todo user profile email %q, got %q", todo.User.Profile.Email, dbTodo.User.Profile.Email)
	}
}

func TestQueryRelatedIDOnly(t *testing.T) {
	var user = &User{
		Name: "TestQueryRelatedIDOnly",
	}
	if err := queries.CreateObject(user); err != nil || user.ID == 0 {
		t.Fatalf("Failed to insert user: %v", err)
	}

	var todo = &Todo{
		Title:       "TestQueryRelatedIDOnly",
		Description: "This is a new test todo",
		Done:        false,
		User:        user,
	}

	if err := queries.CreateObject(todo); err != nil {
		t.Fatalf("Failed to insert todo: %v", err)
	}

	todos, err := queries.Objects(&Todo{}).
		Select("ID", "Title", "Description", "Done", "User").
		Filter("Title", "TestQueryRelatedIDOnly").
		OrderBy("-ID", "-User").
		Limit(5).
		All().Exec()

	if err != nil {
		t.Fatalf("Failed to filter todos: %v", err)
	}

	if len(todos) != 1 {
		t.Fatalf("Expected 1 todo, got %d", len(todos))
	}

	var dbTodo = todos[0]
	t.Logf("Created todo: %+v, %+v", todo, todo.User)
	t.Logf("Filtered todo: %+v, %+v", dbTodo, dbTodo.User)

	if dbTodo.ID != todo.ID || todo.ID == 0 {
		t.Fatalf("Expected todo ID %d, got %d", todo.ID, dbTodo.ID)
	}

	if dbTodo.Title != todo.Title {
		t.Fatalf("Expected todo title %q, got %q", todo.Title, dbTodo.Title)
	}

	if dbTodo.Description != todo.Description {
		t.Fatalf("Expected todo description %q, got %q", todo.Description, dbTodo.Description)
	}

	if dbTodo.Done != todo.Done {
		t.Fatalf("Expected todo done %v, got %v", todo.Done, dbTodo.Done)
	}

	if dbTodo.User == nil {
		t.Fatalf("Expected todo user to be not nil")
	}

	if dbTodo.User.ID != todo.User.ID {
		t.Fatalf("Expected todo user ID %d, got %d", todo.User.ID, dbTodo.User.ID)
	}

	if dbTodo.User.Name != "" {
		t.Fatalf("Expected todo user name to be empty, got %q", dbTodo.User.Name)
	}
}

func TestQueryValuesList(t *testing.T) {
	var user = &User{
		Name: "TestQueryValuesList",
	}

	if err := queries.CreateObject(user); err != nil || user.ID == 0 {
		t.Fatalf("Failed to insert user: %v", err)
	}

	var todos = []*Todo{
		{Title: "TestQueryValuesList 1", Description: "Description 1", Done: false, User: user},
		{Title: "TestQueryValuesList 2", Description: "Description 2", Done: true, User: user},
		{Title: "TestQueryValuesList 3", Description: "Description 3", Done: false, User: user},
	}

	for _, todo := range todos {
		if err := queries.CreateObject(todo); err != nil {
			t.Fatalf("Failed to insert todo: %v", err)
		}
	}

	var values, err = queries.Objects(&Todo{}).
		Filter("Title__istartswith", "testqueryvalueslist").
		OrderBy("-ID", "-User.Name").
		// ValuesList("ID", "Title", "Description", "Done", "User.ID", "User.Name")
		ValuesList("ID", "Title", "Description", "Done", "User", "User.ID", "User.Name", "User.*").Exec()
	// ValuesList("ID", "Title", "Description", "Done", "User")

	if err != nil {
		t.Fatalf("Failed to get values list: %v", err)
	}

	if len(values) != 3 {
		t.Fatalf("Expected 3 values, got %d", len(values))
	}

	for i, value := range values {
		//if len(value) != 7 {
		//	t.Fatalf("Expected 7 values, got %d", len(value))
		//}

		if value[0] == nil || value[1] == nil || value[2] == nil || value[3] == nil {
			t.Fatalf("Expected all values to be not nil")
		}

		if value[0].(int) != todos[i].ID {
			t.Fatalf("Expected todo ID %d, got %d", todos[i].ID, value[0])
		}

		if value[1].(string) != todos[i].Title {
			t.Fatalf("Expected todo title %q, got %q", todos[i].Title, value[1])
		}

		if value[2].(string) != todos[i].Description {
			t.Fatalf("Expected todo description %q, got %q", todos[i].Description, value[2])
		}

		if value[3].(bool) != todos[i].Done {
			t.Fatalf("Expected todo done %v, got %v", todos[i].Done, value[3])
		}

		//if value[4] == nil {
		//	t.Fatalf("Expected todo user ID to be not nil")
		//}
		//
		//if value[5] == nil {
		//	t.Fatalf("Expected todo user name to be not nil")
		//}
		//
		//if value[4].(int) != todos[i].User.ID {
		//	t.Fatalf("Expected todo user ID %d, got %d", todos[i].User.ID, value[4])
		//}
		//
		//if value[5].(string) != todos[i].User.Name {
		//	t.Fatalf("Expected todo user name %q, got %q", todos[i].User.Name, value[5])
		//}

		t.Logf("Got todo values: %+v", value)
		var b strings.Builder
		b.WriteString("[")
		for i, v := range value {
			b.WriteString(fmt.Sprintf("(%T) %v", v, v))
			if i < len(value)-1 {
				b.WriteString(", ")
			}
		}
		b.WriteString("]")
		t.Logf("Got todo values: %s", b.String())
	}
}

func TestQueryNestedRelated(t *testing.T) {
	var image = &Image{
		Path: "test/path/to/image.jpg",
	}

	if err := queries.CreateObject(image); err != nil || image.ID == 0 {
		t.Fatalf("Failed to insert image: %v", err)
	}

	var profile = &Profile{
		Name:  "test profile",
		Email: "test@example.com",
		Image: image,
	}

	if err := queries.CreateObject(profile); err != nil || profile.ID == 0 {
		t.Fatalf("Failed to insert profile: %v", err)
	}

	var user = &User{
		Name:    "test user",
		Profile: profile,
	}

	if err := queries.CreateObject(user); err != nil || user.ID == 0 {
		t.Fatalf("Failed to insert user: %v", err)
	}

	var todo = &Todo{
		Title:       "New Test Todo",
		Description: "This is a new test todo",
		Done:        false,
		User:        user,
	}

	if err := queries.CreateObject(todo); err != nil {
		t.Fatalf("Failed to insert todo: %v", err)
	}

	todos, err := queries.Objects(&Todo{}).
		Select("ID", "Title", "Description", "Done", "User.*", "User.Profile.*", "User.Profile.Image.*").
		Filter(
			queries.Q("Title__icontains", "new test"),
			queries.Q("Done", false),
			queries.Q("User.Name__icontains", "test"),
			queries.Q("User.ID", user.ID),
			queries.Q("User.Profile.Email__icontains", profile.Email),
			queries.Q("User.Profile.ID", profile.ID),
			//&queries.FuncExpr{
			//	Statement: "LOWER(SUBSTR(%s, 0, 2)) LIKE LOWER(?)",
			//	Fields:    []string{"User.Name"},
			//	Params:    []any{"%te%"},
			//},

			&queries.RawExpr{
				Statement: "%s = ?",
				Fields:    []string{"User.ID"},
				Params:    []any{user.ID},
			},
			// queries.Q("User.Profile.Email__icontains", "example"),
		).
		OrderBy("-ID", "-User.Name", "-User.Profile.Email").
		Limit(5).
		All().Exec()

	if err != nil {
		t.Fatalf("Failed to filter todos: %v", err)
	}

	if len(todos) != 1 {
		t.Fatalf("Expected 1 todo, got %d", len(todos))
	}

	var dbTodo = todos[0]
	t.Logf("Created todo: %+v, %+v, %+v, %+v", todo, todo.User, todo.User.Profile, todo.User.Profile.Image)
	t.Logf("Filtered todo: %+v, %+v, %+v, %+v", dbTodo, dbTodo.User, dbTodo.User.Profile, dbTodo.User.Profile.Image)

	if dbTodo.ID != todo.ID || todo.ID == 0 {
		t.Fatalf("Expected todo ID %d, got %d", todo.ID, dbTodo.ID)
	}

	if dbTodo.Title != todo.Title {
		t.Fatalf("Expected todo title %q, got %q", todo.Title, dbTodo.Title)
	}

	if dbTodo.Description != todo.Description {
		t.Fatalf("Expected todo description %q, got %q", todo.Description, dbTodo.Description)
	}

	if dbTodo.Done != todo.Done {
		t.Fatalf("Expected todo done %v, got %v", todo.Done, dbTodo.Done)
	}

	if dbTodo.User == nil {
		t.Fatalf("Expected todo user to be not nil")
	}

	if dbTodo.User.ID != todo.User.ID {
		t.Fatalf("Expected todo user ID %d, got %d", todo.User.ID, dbTodo.User.ID)
	}

	if dbTodo.User.Profile == nil {
		t.Fatalf("Expected todo user profile to be not nil")
	}

	if dbTodo.User.Profile.ID != todo.User.Profile.ID {
		t.Fatalf("Expected todo user profile ID %d, got %d", todo.User.Profile.ID, dbTodo.User.Profile.ID)
	}

	if dbTodo.User.Profile.Email != todo.User.Profile.Email {
		t.Fatalf("Expected todo user profile email %q, got %q", todo.User.Profile.Email, dbTodo.User.Profile.Email)
	}

	if dbTodo.User.Profile.Name != todo.User.Profile.Name {
		t.Fatalf("Expected todo user profile name %q, got %q", todo.User.Profile.Name, dbTodo.User.Profile.Name)
	}

	if dbTodo.User.Profile.Image == nil {
		t.Fatalf("Expected todo user profile image to be not nil")
	}

	if dbTodo.User.Profile.Image.ID != todo.User.Profile.Image.ID {
		t.Fatalf("Expected todo user profile image ID %d, got %d", todo.User.Profile.Image.ID, dbTodo.User.Profile.Image.ID)
	}
}

func TestQueryUpdate(t *testing.T) {

	var user = &User{
		Name: "TestQueryUpdate",
	}

	if err := queries.CreateObject(user); err != nil || user.ID == 0 {
		t.Fatalf("Failed to insert user: %v", err)
	}

	var todo1 = &Todo{
		Title:       "TestQueryUpdate1",
		Description: "This is a new test todo",
	}

	var todo2 = &Todo{
		Title:       "TestQueryUpdate2",
		Description: "This is a new test todo",
	}

	if err := queries.CreateObject(todo1); err != nil {
		t.Fatalf("Failed to insert todo: %v", err)
	}

	if err := queries.CreateObject(todo2); err != nil {
		t.Fatalf("Failed to insert todo: %v", err)
	}

	var updated, err = queries.Objects(&Todo{}).
		Select("Title", "User").
		Filter("Title__istartswith", "testqueryupdate").
		Filter("Done", false).
		Update(&Todo{
			Title: "Updated Title",
			User:  user,
		}).Exec()

	if err != nil {
		t.Fatalf("Failed to update todo: %v", err)
	}

	if updated == 0 {
		t.Fatalf("Expected 1 todo to be updated, got %d", updated)
	}

	dbTodo1, err := queries.GetObject[*Todo](todo1.ID)
	if err != nil {
		t.Fatalf("Failed to get todo: %v", err)
	}

	if dbTodo1.ID != todo1.ID || todo1.ID == 0 {
		t.Fatalf("Expected todo ID %d, got %d", todo1.ID, dbTodo1.ID)
	}

	if dbTodo1.Title != "Updated Title" {
		t.Fatalf("Expected todo title 'Updated Title', got %q", dbTodo1.Title)
	}

	if dbTodo1.Description != todo1.Description {
		t.Fatalf("Expected todo description %q, got %q", todo1.Description, dbTodo1.Description)
	}

	if dbTodo1.Done != todo1.Done {
		t.Fatalf("Expected todo done %v, got %v", todo1.Done, dbTodo1.Done)
	}

	if dbTodo1.User == nil {
		t.Fatalf("Expected todo user to be not nil")
	}

	if dbTodo1.User.ID != user.ID {
		t.Fatalf("Expected todo user ID %d, got %d", user.ID, dbTodo1.User.ID)
	}

	dbTodo2, err := queries.GetObject[*Todo](todo2.ID)
	if err != nil {
		t.Fatalf("Failed to get todo: %v", err)
	}

	if dbTodo2.ID != todo2.ID || todo2.ID == 0 {
		t.Fatalf("Expected todo ID %d, got %d", todo2.ID, dbTodo2.ID)
	}

	if dbTodo2.Title != "Updated Title" {
		t.Fatalf("Expected todo title %q, got %q", todo2.Title, dbTodo2.Title)
	}

	if dbTodo2.Description != todo2.Description {
		t.Fatalf("Expected todo description %q, got %q", todo2.Description, dbTodo2.Description)
	}

	if dbTodo2.Done != todo2.Done {
		t.Fatalf("Expected todo done %v, got %v", todo2.Done, dbTodo2.Done)
	}

	if dbTodo2.User == nil {
		t.Fatalf("Expected todo user to be not nil")
	}

	if dbTodo2.User.ID != user.ID {
		t.Fatalf("Expected todo user ID %d, got %d", todo2.User.ID, dbTodo2.User.ID)
	}

	t.Logf("Updated todo: %+v", dbTodo1)
	t.Logf("Updated todo user: %+v", dbTodo1.User)

	t.Logf("Updated todo: %+v", dbTodo2)
	t.Logf("Updated todo user: %+v", dbTodo2.User)

}
