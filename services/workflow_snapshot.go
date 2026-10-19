package services

import "reflect"

// cloneWorkflowSnapshot detaches mutable JSON/BSON values and workflow
// definitions while preserving their Go types (including BSON scalar types).
func cloneWorkflowSnapshot[T any](value T) T {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return value
	}
	return cloneWorkflowSnapshotValue(reflected).Interface().(T)
}

func cloneWorkflowSnapshotValue(value reflect.Value) reflect.Value {
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return value
		}
		copy := reflect.New(value.Type().Elem())
		copy.Elem().Set(cloneWorkflowSnapshotValue(value.Elem()))
		return copy
	case reflect.Interface:
		if value.IsNil() {
			return value
		}
		copy := reflect.New(value.Type()).Elem()
		copy.Set(cloneWorkflowSnapshotValue(value.Elem()))
		return copy
	case reflect.Map:
		if value.IsNil() {
			return value
		}
		copy := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			copy.SetMapIndex(iterator.Key(), cloneWorkflowSnapshotValue(iterator.Value()))
		}
		return copy
	case reflect.Slice, reflect.Array:
		var copy reflect.Value
		if value.Kind() == reflect.Slice {
			if value.IsNil() {
				return value
			}
			copy = reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		} else {
			copy = reflect.New(value.Type()).Elem()
		}
		for index := 0; index < value.Len(); index++ {
			copy.Index(index).Set(cloneWorkflowSnapshotValue(value.Index(index)))
		}
		return copy
	case reflect.Struct:
		copy := reflect.New(value.Type()).Elem()
		copy.Set(value)
		for index := 0; index < value.NumField(); index++ {
			if copy.Field(index).CanSet() && value.Field(index).CanInterface() {
				copy.Field(index).Set(cloneWorkflowSnapshotValue(value.Field(index)))
			}
		}
		return copy
	default:
		return value
	}
}
