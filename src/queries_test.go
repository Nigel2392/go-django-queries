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
	createTableProfiles = `CREATE TABLE IF NOT EXISTS profiles (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT,
	email TEXT
)`

	createTableUsers = `CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	profile_id INTEGER,
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

type Profile struct {
	ID    int
	Name  string
	Email string
}

func (m *Profile) FieldDefs() attrs.Definitions {
	return attrs.Define(m,
		attrs.NewField(m, "ID", &attrs.FieldConfig{
			Primary:  true,
			ReadOnly: true,
		}),
		attrs.NewField(m, "Name", &attrs.FieldConfig{}),
		attrs.NewField(m, "Email", &attrs.FieldConfig{}),
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
		Fields("ID", "Title", "Description", "Done").
		Filter("Title__icontains", "test").
		Filter("Done", false).
		OrderBy("-ID").
		Limit(5).
		All()

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
		Fields("ID", "Title", "Description", "Done").
		Filter("Done", true).
		First()

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
		Fields("ID", "Title", "Description", "Done").
		Filter(
			queries.Expr("Title", "icontains", "test"),
			queries.Q("Done", false),
		).
		All()

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
		Count()

	if err != nil {
		t.Fatalf("Failed to count todos: %v", err)
	}

	if count != 1 {
		t.Fatalf("Expected 1 todo, got %d", count)
	}

	t.Logf("Counted todos: %d", count)
}

func TestQueryRelated(t *testing.T) {

	var user = &User{
		Name: "test user",
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
		Fields("ID", "Title", "Description", "Done", "User").
		Filter(
			queries.Q("Title__icontains", "new test"),
			queries.Q("Done", false),
			queries.Q("User.Name__icontains", "test"),
		).
		OrderBy("-ID", "-User.Name").
		Limit(5).
		All()

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
}

func TestQueryNestedRelated(t *testing.T) {
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
		Fields("ID", "Title", "Description", "Done", "User", "User.Profile").
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

			&queries.FuncExpr{
				Statement: "%s = ?",
				Fields:    []string{"User.ID"},
				Params:    []any{user.ID},
			},
			// queries.Q("User.Profile.Email__icontains", "example"),
		).
		OrderBy("-ID", "-User.Name", "-User.Profile.Email").
		Limit(5).
		All()

	if err != nil {
		t.Fatalf("Failed to filter todos: %v", err)
	}

	if len(todos) != 1 {
		t.Fatalf("Expected 1 todo, got %d", len(todos))
	}

	var dbTodo = todos[0]
	t.Logf("Created todo: %+v, %+v, %+v", todo, todo.User, todo.User.Profile)
	t.Logf("Filtered todo: %+v, %+v, %+v", dbTodo, dbTodo.User, dbTodo.User.Profile)

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
}
