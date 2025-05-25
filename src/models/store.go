package models

import (
	"fmt"
	"strings"
)

type mapDataStore map[string]interface{}

func (m mapDataStore) String() string {
	var sb strings.Builder
	sb.WriteString("[")
	var i = 0
	for k, v := range m {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "%q: %v", k, v)
		i++
	}
	sb.WriteString("]")
	return sb.String()
}

func (m mapDataStore) HasValue(key string) bool {
	_, ok := m[key]
	return ok
}

func (m mapDataStore) SetValue(key string, value any) error {
	m[key] = value
	return nil
}

func (m mapDataStore) GetValue(key string) (any, bool) {
	if v, ok := m[key]; ok {
		return v, true
	}
	return nil, false
}

func (m mapDataStore) DeleteValue(key string) error {
	delete(m, key)
	return nil
}
