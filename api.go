package sharepoint

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/koltyakov/gosip/api"
)

type ItemPayload map[string]any

func (list *SharePointList[T]) Columns(columns []string) *SharePointList[T] {
	list.options.Select = columns

	return list
}

func (list *SharePointList[T]) Equal(column string, value string) *SharePointList[T] {
	list.setFilter(column, "eq", value)

	return list
}

func (list *SharePointList[T]) NotEqual(column string, value string) *SharePointList[T] {
	list.setFilter(column, "ne", value)

	return list
}

func (list *SharePointList[T]) GreaterThan(column string, value string) *SharePointList[T] {
	list.setFilter(column, "gt", value)

	return list
}

func (list *SharePointList[T]) GreaterThanOrEqual(column string, value string) *SharePointList[T] {
	list.setFilter(column, "ge", value)

	return list
}

func (list *SharePointList[T]) LessThan(column string, value string) *SharePointList[T] {
	list.setFilter(column, "lt", value)

	return list
}

func (list *SharePointList[T]) LessThanOrEqual(column string, value string) *SharePointList[T] {
	list.setFilter(column, "le", value)

	return list
}

func (list *SharePointList[T]) Contains(column string, value string) *SharePointList[T] {
	list.setFilter(column, "substringof", value)

	return list
}

func (list *SharePointList[T]) StartsWith(column string, value string) *SharePointList[T] {
	list.setFilter(column, "startswith", value)

	return list
}

func (list *SharePointList[T]) And() *SharePointList[T] {
	list.setLogicalOperator("and")

	return list
}

func (list *SharePointList[T]) Or() *SharePointList[T] {
	list.setLogicalOperator("or")

	return list
}

func (list *SharePointList[T]) Not() *SharePointList[T] {
	list.setLogicalOperator("not")

	return list
}

func (list *SharePointList[T]) OrderByAsc(column string) *SharePointList[T] {
	list.options.OrderBy = &orderBy{column: column, ascending: true}

	return list
}

func (list *SharePointList[T]) OrderByDesc(column string) *SharePointList[T] {
	list.options.OrderBy = &orderBy{column: column, ascending: false}

	return list
}

func (list *SharePointList[T]) Get() ([]*T, error) {
	list.validateOptions()

	items := list.getItems()

	list.applySelect(items)

	list.applyFilters(items)

	list.applyOrderBy(items)

	list.applyLimit(items)

	page, err := items.GetPaged()

	list.clearFilters()

	if err != nil {
		return nil, fmt.Errorf("Unable to fetch Lists/%s: %v\n", list.listURI, err)
	}

	return list.parseResponse(page)
}

func (list *SharePointList[T]) HasMore() bool {
	return list.page != nil
}

func (list *SharePointList[T]) Next() ([]*T, error) {
	if !list.HasMore() {
		return nil, fmt.Errorf("Unable to fetch next page of Lists/%s. Run the 'Get' method before 'Next'\n", list.listURI)
	}

	nextPage, err := list.page.GetNextPage()

	if err != nil {
		return nil, fmt.Errorf("Unable to fetch Lists/%s: %v\n", list.listURI, err)
	}

	return list.parseResponse(nextPage)
}

func (list *SharePointList[T]) GetByID(itemId int) (*T, error) {

	items := list.getItems()

	data, err := items.GetByID(itemId).Get()

	if err != nil {
		return nil, err
	}

	var response *T

	if err := json.Unmarshal(data.Normalized(), &response); err != nil {
		panic(err)
	}

	return response, nil
}

func (list *SharePointList[T]) Add() (*T, error) {
	list.validatePayload()

	payload, err := json.Marshal(list.payload)

	if err != nil {
		return nil, err
	}

	items := list.getItems()

	data, err := items.Add(payload)

	if err != nil {
		return nil, err
	}

	var response *T

	if err := json.Unmarshal(data.Normalized(), &response); err != nil {
		panic(err)
	}

	return response, nil
}

func (list *SharePointList[T]) Update(itemId int) error {
	list.validatePayload()

	payload, err := json.Marshal(list.payload)

	if err != nil {
		return err
	}

	items := list.getItems()

	_, err = items.GetByID(itemId).Update(payload)

	return err
}

func (list *SharePointList[T]) Delete(itemId int) error {

	items := list.getItems()

	return items.GetByID(itemId).Delete()
}

func (list *SharePointList[T]) getItems() *api.Items {
	return list.api.Web().GetList(fmt.Sprintf("Lists/%s", list.listURI)).Items()
}

func (list *SharePointList[T]) applySelect(items *api.Items) {
	if len(list.options.Select) != 0 {
		columns := list.options.Select

		items.Select(strings.Join(columns, ","))
		return
	}

	columns := slices.Collect(maps.Keys(list.columns))

	items.Select(strings.Join(columns, ","))
}

func (list *SharePointList[T]) applyFilters(items *api.Items) {
	filters := list.options.Filters

	if len(filters) == 0 {
		return
	}

	builder := strings.Builder{}

	for _, filter := range list.options.Filters {
		switch filter.op {
		case "substringof":
			fmt.Fprintf(&builder, "%s('%s',%s)", filter.op, filter.value, filter.column)

		case "startswith":
			fmt.Fprintf(&builder, "%s(%s,'%s')", filter.op, filter.column, filter.value)

		case "eq", "ne":
			fmt.Fprintf(&builder, "%s %s '%s'", filter.column, filter.op, filter.value)

		default:
			fmt.Fprintf(&builder, "%s %s %s", filter.column, filter.op, filter.value)
		}

		if filter.logicalOp != "" {
			fmt.Fprintf(&builder, " %s ", filter.logicalOp)
		}
	}

	items.Filter(builder.String())

}

func (list *SharePointList[T]) applyOrderBy(items *api.Items) {
	if list.options.OrderBy == nil {
		return
	}

	items.OrderBy(list.options.OrderBy.column, list.options.OrderBy.ascending)
}

func (list *SharePointList[T]) applyLimit(items *api.Items) {
	if list.options.Limit <= 0 {
		items.Top(10)
		return
	}

	items.Top(list.options.Limit)
}

func (list *SharePointList[T]) setFilter(column string, op string, value string) {
	list.options.Filters = append(list.options.Filters, &filter{column: column, op: op, value: value})
}

func (list *SharePointList[T]) setLogicalOperator(logicalOp string) {
	filters := list.options.Filters

	if len(filters) == 0 {
		return
	}

	lastFilter := filters[len(filters)-1]

	lastFilter.logicalOp = logicalOp
}

func (list *SharePointList[T]) Payload(item *T, columns ...string) error {
	t := reflect.TypeFor[T]()

	list.payload = ItemPayload{}

	for _, column := range columns {
		key, ok := list.columns[column]

		if !ok {
			return fmt.Errorf("Column '%s' not found in struct '%s'", column, t.Name())
		}

		reflectValue := reflect.ValueOf(item)

		value := reflectValue.Elem().FieldByName(key)

		list.payload[key] = fmt.Sprintf("%v", value)
	}

	return nil
}

func (list *SharePointList[T]) parseResponse(page *api.ItemsPage) ([]*T, error) {
	if page.HasNextPage() {
		list.page = page
	} else {
		list.page = nil
	}

	var items []*T

	if err := json.Unmarshal(page.Items.Normalized(), &items); err != nil {
		panic(err)
	}

	return items, nil
}

func (list *SharePointList[T]) clearFilters() {
	if len(list.options.Filters) == 0 {
		return
	}

	list.options.Filters = nil
}
