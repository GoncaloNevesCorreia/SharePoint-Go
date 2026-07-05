package sharepoint

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
)

func (list *SharePointList[T]) validateOptions() {

	list.validateColums(list.options.Select)

	list.validateFilters()
}

func (list *SharePointList[T]) validatePayload() {
	payloadKeys := slices.Collect(maps.Keys(list.payload))

	if len(payloadKeys) == 0 {
		panic(fmt.Errorf("No Columns found in item used to update List '%v'.", list.listURI))
	}

	list.validateColums(payloadKeys)
}

func (list *SharePointList[T]) validateColums(columns []string) {

	if len(columns) == 0 {
		return
	}

	jsonColumns := slices.Collect(maps.Keys(list.columns))

	notFoundColumns := make([]string, 0)

	for _, column := range columns {
		if ok := slices.Contains(jsonColumns, column); !ok {

			notFoundColumns = append(notFoundColumns, column)
		}
	}

	if len(notFoundColumns) == 0 {
		return
	}

	t := reflect.TypeFor[T]()

	panic(fmt.Errorf("Columns %v not found in struct '%s' used to represent List '%v'", notFoundColumns, t.Name(), list.listURI))
}

func (list *SharePointList[T]) validateFilters() {
	columns := make([]string, 0)

	size := len(list.options.Filters)

	missingLogicalOpColumns := make([]string, 0)

	t := reflect.TypeFor[T]()

	for i, filter := range list.options.Filters {
		columns = append(columns, filter.column)

		if i != size-1 && filter.logicalOp == "" {
			missingLogicalOpColumns = append(missingLogicalOpColumns, filter.column)
		}

		if i == size-1 && filter.logicalOp != "" {
			panic(fmt.Errorf("Trailing Logical Operator '%s' Filter for Column %v in struct '%s' used to represent List '%v'", filter.logicalOp, missingLogicalOpColumns, t.Name(), list.listURI))
		}
	}

	if len(missingLogicalOpColumns) != 0 {
		panic(fmt.Errorf("Missing Logical Operator Filters for Columns %v in struct '%s' used to represent List '%v'", missingLogicalOpColumns, t.Name(), list.listURI))
	}

	list.validateColums(columns)
}
