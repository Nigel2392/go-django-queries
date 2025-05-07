package migrator

import (
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type Column struct {
	Table        Table              `json:"-"`
	Field        attrs.Field        `json:"-"`
	Name         string             `json:"name"`
	Column       string             `json:"column"`
	UseInDB      bool               `json:"use_in_db,omitempty"`
	MinLength    int64              `json:"min_length,omitempty"`
	MaxLength    int64              `json:"max_length,omitempty"`
	MinValue     float64            `json:"min_value,omitempty"`
	MaxValue     float64            `json:"max_value,omitempty"`
	Unique       bool               `json:"unique,omitempty"`
	Nullable     bool               `json:"nullable,omitempty"`
	Primary      bool               `json:"primary,omitempty"`
	Auto         bool               `json:"auto,omitempty"`
	Default      interface{}        `json:"default,omitempty"`
	ReverseAlias string             `json:"reverse_alias,omitempty"`
	Rel          *MigrationRelation `json:"relation,omitempty"`
}

func (c *Column) Equals(other *Column) bool {
	if c == nil && other == nil {
		return true
	}
	if (c == nil) != (other == nil) {
		return false
	}
	if c.Name != other.Name {
		return false
	}
	if c.Column != other.Column {
		return false
	}
	if c.MinLength != other.MinLength {
		return false
	}
	if c.MaxLength != other.MaxLength {
		return false
	}
	if c.MinValue != other.MinValue {
		return false
	}
	if c.MaxValue != other.MaxValue {
		return false
	}
	if c.Unique != other.Unique {
		return false
	}
	if c.Nullable != other.Nullable {
		return false
	}
	if c.Primary != other.Primary {
		return false
	}
	if c.Auto != other.Auto {
		return false
	}

	if !EqualDefaultValue(c.Default, other.Default) {
		return false
	}

	if c.ReverseAlias != other.ReverseAlias {
		return false
	}
	if (c.Rel == nil) != (other.Rel == nil) {
		return false
	}
	if c.Rel != nil {

		var other = other.Rel
		if c.Rel.Type != other.Type {
			return false
		}
		if (c.Rel.TargetModel == nil) != (other.TargetModel == nil) {
			return false
		}

		if c.Rel.TargetModel != nil {
			if c.Rel.TargetModel.TypeName() != other.TargetModel.TypeName() {
				return false
			}
		}

		if (c.Rel.TargetField == nil) != (other.TargetField == nil) {
			return false
		}

		if c.Rel.TargetField != nil {
			if c.Rel.TargetField.Name() != other.TargetField.Name() {
				return false
			}

			if c.Rel.TargetField.ColumnName() != other.TargetField.ColumnName() {
				return false
			}

			if c.Rel.TargetField.AllowNull() != other.TargetField.AllowNull() {
				return false
			}

			if c.Rel.TargetField.IsPrimary() != other.TargetField.IsPrimary() {
				return false
			}

			if c.Rel.TargetField.GetDefault() != other.TargetField.GetDefault() {
				return false
			}

			if c.Rel.TargetField.Type() != other.TargetField.Type() {
				return false
			}
		}
	}
	return true
}
