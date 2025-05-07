package migrator

type Index struct {
	Name    string
	Type    string
	Columns []string
	Unique  bool
	Comment string
}
