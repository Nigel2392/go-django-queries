package migrator

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/contenttypes"
	"github.com/elliotchance/orderedmap/v2"
	"github.com/pkg/errors"
)

func getFromAttrs[T any](attrMap map[string]any, key string) (T, bool) {
	var n T
	if v, ok := attrMap[key]; ok {
		if t, ok := v.(T); ok {
			return t, true
		}
		var (
			rT = reflect.TypeOf((*T)(nil)).Elem()
			vT = reflect.TypeOf(v)
			vV = reflect.ValueOf(v)
		)

		if vT.AssignableTo(rT) {
			return vV.Interface().(T), true
		}

		if vT.ConvertibleTo(rT) {
			return vV.Convert(rT).Interface().(T), true
		}

		return n, false
	}
	return n, false
}

var suffixWithoutDot = strings.TrimPrefix(MIGRATION_FILE_SUFFIX, ".")

func parseMigrationFileName(n string) (orderNum int, name string, err error) {
	// The migration file name is expected to be in the format <order>_<name>.sql
	// where <order> is an integer and <name> is the name of the migration.
	// For example: 0001_create_users_table.sql

	var parts = strings.SplitN(n, ".", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("invalid migration file name %q", n)
	}

	if !strings.HasSuffix(parts[1], suffixWithoutDot) {
		return 0, "", fmt.Errorf("invalid migration file name %q, expected suffix %q", n, MIGRATION_FILE_SUFFIX)
	}

	parts = strings.SplitN(parts[0], "_", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("invalid migration file name %q, expected format <order>_<file_name>", n)
	}

	orderNum, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", errors.Wrapf(err, "invalid order number %q", parts[0])
	}

	name = parts[1]
	return orderNum, name, nil
}

func generateMigrationFileName(mig *MigrationFile) string {
	// The migration file name is expected to be in the format <order>_<name>.migration
	// where <order> is an integer and <name> is the name of the migration.
	// For example: 0001_create_users_table.migration
	//var orderStr = fmt.Sprintf("%04d", orderNum)
	//return fmt.Sprintf("%s_%s%s", orderStr, name, MIGRATION_FILE_SUFFIX)

	var orderStr = fmt.Sprintf("%04d_", mig.Order)
	var sb = strings.Builder{}
	if len(mig.Actions) == 0 {
		return fmt.Sprintf(
			"%s%s%s",
			orderStr,
			"auto_generated",
			MIGRATION_FILE_SUFFIX,
		)
	}

	sb.WriteString(orderStr)
	var action = mig.Actions[0]
	switch action.ActionType {
	case ActionCreateTable:
		sb.WriteString("create_table")
	case ActionDropTable:
		sb.WriteString("drop_table")
	case ActionRenameTable:
		sb.WriteString("rename_table_")
		sb.WriteString(action.Table.Old.Table)
		sb.WriteString("_to_")
		sb.WriteString(action.Table.New.Table)
	case ActionAddIndex:
		sb.WriteString("add_idx_")
		sb.WriteString(action.Index.New.Name)
	case ActionDropIndex:
		sb.WriteString("drop_idx_")
		sb.WriteString(action.Table.New.Table)
		sb.WriteString("_on_")
		sb.WriteString(action.Index.Old.Name)
	case ActionRenameIndex:
		sb.WriteString("rename_idx_")
		sb.WriteString(action.Index.Old.Name)
		sb.WriteString("_to_")
		sb.WriteString(action.Index.New.Name)
	// case ActionAlterUniqueTogether:

	// case ActionAlterIndexTogether:

	case ActionAddField:
		sb.WriteString("add_field_")
		sb.WriteString(action.Field.New.Column)
	case ActionAlterField:
		sb.WriteString("alter_field_")
		sb.WriteString(action.Field.New.Column)
	case ActionRemoveField:
		sb.WriteString("remove_field_")
		sb.WriteString(action.Field.Old.Column)
	}

	if len(mig.Actions) > 1 {
		sb.WriteString("_and_")
		sb.WriteString(fmt.Sprintf("%d_more", len(mig.Actions)-1))
	}

	sb.WriteString(MIGRATION_FILE_SUFFIX)

	return sb.String()
}

func EqualDefaultValue(a, b any) bool {
	var cDefault = reflect.ValueOf(a)
	var otherDefault = reflect.ValueOf(b)
	if cDefault.IsValid() != otherDefault.IsValid() {
		var (
			aIsZero bool
			bIsZero bool
		)

		if cDefault.IsValid() && cDefault.Kind() == reflect.Ptr && cDefault.IsNil() ||
			cDefault.IsValid() && cDefault.Kind() != reflect.Ptr && cDefault.IsZero() ||
			!cDefault.IsValid() {
			aIsZero = true
		}

		if otherDefault.IsValid() && otherDefault.Kind() == reflect.Ptr && otherDefault.IsNil() ||
			otherDefault.IsValid() && otherDefault.Kind() != reflect.Ptr && otherDefault.IsZero() ||
			!otherDefault.IsValid() {
			bIsZero = true
		}

		if aIsZero != bIsZero {
			return false
		}

		return true
	}

	if cDefault.Kind() != reflect.Ptr && cDefault.IsZero() != otherDefault.IsZero() ||
		cDefault.Kind() == reflect.Ptr && cDefault.IsNil() != otherDefault.IsNil() {
		return false
	} else if cDefault.Kind() != reflect.Ptr && !cDefault.IsZero() ||
		cDefault.Kind() == reflect.Ptr && !cDefault.IsNil() {
		if !reflect.DeepEqual(cDefault.Interface(), otherDefault.Interface()) {
			return false
		}
	}

	return true
}

// var registeredModels = make(map[string]*contenttypes.BaseContentType[attrs.Definer])
var registeredModels = orderedmap.NewOrderedMap[string, *contenttypes.BaseContentType[attrs.Definer]]()

func Register(obj attrs.Definer) {
	var cType = contenttypes.NewContentType(obj)

	if contenttypes.DefinitionForType(cType.TypeName()) == nil {
		contenttypes.Register(&contenttypes.ContentTypeDefinition{
			ContentObject: obj,
		})
	}

	registeredModels.Set(cType.TypeName(), cType)
}
