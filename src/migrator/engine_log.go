package migrator

import (
	"strings"

	"github.com/Nigel2392/go-django/src/core/logger"
)

var _ MigrationLog = &MigrationEngineConsoleLog{}

type MigrationEngineConsoleLog struct {
}

func (e *MigrationEngineConsoleLog) Log(action ActionType, file *MigrationFile, table *Changed[*ModelTable], column *Changed[*Column], index *Changed[*Index]) {
	var actionStr strings.Builder
	actionStr.WriteString(file.AppName)
	actionStr.WriteString(" / ")
	actionStr.WriteString(file.ModelName)
	actionStr.WriteString(" / ")
	actionStr.WriteString(file.FileName())
	actionStr.WriteString(": ")

	switch action {
	case ActionCreateTable:
		actionStr.WriteString("Creating table for model ")
		actionStr.WriteString(table.New.ModelName())
		actionStr.WriteString(" ")
		actionStr.WriteString(table.New.TableName())
	case ActionDropTable:
		actionStr.WriteString("Dropping table for model ")
		actionStr.WriteString(table.New.ModelName())
		actionStr.WriteString(" ")
		actionStr.WriteString(table.New.TableName())
	case ActionRenameTable:
		actionStr.WriteString("Renaming table for model ")
		actionStr.WriteString(table.New.ModelName())
		actionStr.WriteString(" from ")
		actionStr.WriteString(table.Old.TableName())
		actionStr.WriteString(" to ")
		actionStr.WriteString(table.New.TableName())
	case ActionAddIndex:
		actionStr.WriteString("Adding index for model ")
		actionStr.WriteString(table.New.ModelName())
		actionStr.WriteString(" on table ")
		actionStr.WriteString(table.New.TableName())
		actionStr.WriteString(" with index ")
		actionStr.WriteString(index.New.Name)
	case ActionDropIndex:
		actionStr.WriteString("Dropping index for model ")
		actionStr.WriteString(table.New.ModelName())
		actionStr.WriteString(" on table ")
		actionStr.WriteString(table.New.TableName())
		actionStr.WriteString(" with index ")
		actionStr.WriteString(index.New.Name)
	case ActionRenameIndex:
		actionStr.WriteString("Renaming index for model ")
		actionStr.WriteString(table.New.ModelName())
		actionStr.WriteString(" on table ")
		actionStr.WriteString(table.New.TableName())
		actionStr.WriteString(" from ")
		actionStr.WriteString(index.Old.Name)
		actionStr.WriteString(" to ")
		actionStr.WriteString(index.New.Name)
	//case ActionAlterUniqueTogether:
	//	actionStr.WriteString("Altering unique together for model ")
	//	actionStr.WriteString(table.New.ModelName())
	//	actionStr.WriteString(" on table ")
	//	actionStr.WriteString(table.New.TableName())
	//case ActionAlterIndexTogether:
	//	actionStr.WriteString("Altering index together for model ")
	//	actionStr.WriteString(table.New.ModelName())
	//	actionStr.WriteString(" on table ")
	//	actionStr.WriteString(table.New.TableName())
	case ActionAddField:
		actionStr.WriteString("Adding field for model ")
		actionStr.WriteString(table.New.ModelName())
		actionStr.WriteString(" on table ")
		actionStr.WriteString(table.New.TableName())
		actionStr.WriteString(" with field ")
		actionStr.WriteString(table.New.ModelName())
		actionStr.WriteString(".")
		actionStr.WriteString(column.New.Name)
	case ActionAlterField:
		actionStr.WriteString("Altering field \"")
		actionStr.WriteString(column.Old.Name)
		actionStr.WriteString("\" for model ")
		actionStr.WriteString(table.New.ModelName())
		actionStr.WriteString(" on table ")
		actionStr.WriteString(table.New.TableName())
	case ActionRemoveField:
		actionStr.WriteString("Removing field \"")
		actionStr.WriteString(column.New.Name)
		actionStr.WriteString("\" for model ")
		actionStr.WriteString(table.New.ModelName())
		actionStr.WriteString(" on table ")
		actionStr.WriteString(table.New.TableName())
	}

	logger.Info(actionStr.String())
}
