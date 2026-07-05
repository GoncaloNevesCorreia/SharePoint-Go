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

type SearchResponse[T any] struct {
	HasMore bool `json:"hasMore"`
	Items   []*T `json:"items"`
}

type Result[T any] struct {
	Success bool
	Data    T
	Error   *ApiError
}

type ApiError struct {
	Code    string
	Message string
}

func (list *SharePointList[T]) Columns(columns []string) *SharePointList[T] {
	list.options.Select = columns

	return list
}

func (list *SharePointList[T]) Limit(value int) *SharePointList[T] {
	list.options.Limit = value

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

func (list *SharePointList[T]) GetAll() Result[[]*T] {
	items := list.getItems()

	result, err := items.GetAll()

	if err != nil {
		return Result[[]*T]{
			Error: &ApiError{
				Code:    "fetch_error",
				Message: fmt.Sprintf("Não foi possivel aceder à lista '%s': %v\n", list.listURI, err),
			},
		}
	}

	var response []*T

	for _, data := range result {

		if err := json.Unmarshal(data.Normalized(), &response); err != nil {
			return Result[[]*T]{
				Error: &ApiError{
					Code:    "parse_error",
					Message: err.Error(),
				},
			}
		}
	}

	return Result[[]*T]{
		Success: true,
		Data:    response,
	}
}

func (list *SharePointList[T]) Get() Result[*SearchResponse[T]] {
	list.validateOptions()

	items := list.getItems()

	list.applySelect(items)

	list.applyFilters(items)

	list.applyOrderBy(items)

	list.applyLimit(items)

	page, err := items.GetPaged()

	list.clearFilters()

	if err != nil {
		return Result[*SearchResponse[T]]{
			Error: &ApiError{
				Code:    "fetch_error",
				Message: fmt.Sprintf("Não foi possivel aceder à lista '%s': %v\n", list.listURI, err),
			},
		}
	}

	return list.parseResponse(page)
}

func (list *SharePointList[T]) Next() Result[*SearchResponse[T]] {
	if list.page == nil {
		return Result[*SearchResponse[T]]{
			Error: &ApiError{
				Code:    "api_no_page",
				Message: fmt.Sprintf("Não foi possivel obter a proxima pagina da lista '%s'.\n", list.listURI),
			},
		}
	}

	nextPage, err := list.page.GetNextPage()

	if err != nil {
		return Result[*SearchResponse[T]]{
			Error: &ApiError{
				Code:    "fetch_error",
				Message: fmt.Sprintf("Não foi possivel aceder à lista '%s': %v\n", list.listURI, err),
			},
		}
	}

	return list.parseResponse(nextPage)
}

func (list *SharePointList[T]) GetByID(itemId int) Result[*T] {

	items := list.getItems()

	data, err := items.GetByID(itemId).Get()

	if err != nil {
		return Result[*T]{
			Error: &ApiError{
				Code:    "fetch_error",
				Message: fmt.Sprintf("Não foi possivel aceder à lista '%s': %v\n", list.listURI, err),
			},
		}
	}

	var response *T

	if err := json.Unmarshal(data.Normalized(), &response); err != nil {
		return Result[*T]{
			Error: &ApiError{
				Code:    "parse_error",
				Message: err.Error(),
			},
		}
	}

	return Result[*T]{
		Success: true,
		Data:    response,
	}
}

func (list *SharePointList[T]) Add() Result[*T] {
	list.validatePayload()

	payload, err := json.Marshal(list.payload)

	if err != nil {
		return Result[*T]{
			Error: &ApiError{
				Code:    "parse_error",
				Message: err.Error(),
			},
		}
	}

	items := list.getItems()

	data, err := items.Add(payload)

	if err != nil {
		return Result[*T]{
			Error: &ApiError{
				Code:    "add_error",
				Message: fmt.Sprintf("Não foi possivel adicionar à lista '%s': %v\n", list.listURI, err),
			},
		}
	}

	var response *T

	if err := json.Unmarshal(data.Normalized(), &response); err != nil {
		return Result[*T]{
			Error: &ApiError{
				Code:    "parse_error",
				Message: err.Error(),
			},
		}
	}

	return Result[*T]{
		Success: true,
		Data:    response,
	}
}

func (list *SharePointList[T]) Update(itemId int) Result[bool] {
	list.validatePayload()

	payload, err := json.Marshal(list.payload)

	if err != nil {
		return Result[bool]{
			Error: &ApiError{
				Code:    "parse_error",
				Message: err.Error(),
			},
		}
	}

	items := list.getItems()

	_, err = items.GetByID(itemId).Update(payload)

	if err != nil {
		return Result[bool]{
			Error: &ApiError{
				Code:    "update_error",
				Message: fmt.Sprintf("Não foi possivel atualizar a lista '%s': %v\n", list.listURI, err),
			},
		}
	}

	return Result[bool]{
		Success: true,
		Data:    true,
	}
}

func (list *SharePointList[T]) Delete(itemId int) Result[bool] {
	if itemId <= 0 {
		return Result[bool]{
			Error: &ApiError{
				Code:    "delete_error",
				Message: fmt.Sprintf("Precisa de especificar um itemID para apagar da Lista: %s\n", list.listURI),
			},
		}
	}

	items := list.getItems()

	err := items.GetByID(itemId).Delete()

	if err != nil {
		return Result[bool]{
			Error: &ApiError{
				Code:    "delete_error",
				Message: fmt.Sprintf("Não foi possivel apagar da lista '%s': %v\n", list.listURI, err),
			},
		}
	}

	return Result[bool]{
		Success: true,
		Data:    true,
	}

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

func (list *SharePointList[T]) Payload(item *T, columns ...string) {
	t := reflect.TypeFor[T]()

	list.payload = ItemPayload{}

	for _, column := range columns {
		key, ok := list.columns[column]

		if !ok {
			panic(fmt.Errorf("Column '%s' not found in struct '%s'", column, t.Name()))
		}

		reflectValue := reflect.ValueOf(item)

		value := reflectValue.Elem().FieldByName(key)

		// TODO: Just Set the Value without formatting.
		list.payload[key] = fmt.Sprintf("%v", value)
	}
}

func (list *SharePointList[T]) parseResponse(page *api.ItemsPage) Result[*SearchResponse[T]] {
	if page.HasNextPage() {
		list.page = page
	} else {
		list.page = nil
	}

	var items []*T

	if err := json.Unmarshal(page.Items.Normalized(), &items); err != nil {
		return Result[*SearchResponse[T]]{
			Error: &ApiError{
				Code:    "parse_error",
				Message: err.Error(),
			},
		}
	}

	return Result[*SearchResponse[T]]{
		Success: true,
		Data: &SearchResponse[T]{
			HasMore: list.page != nil,
			Items:   items,
		},
	}
}

func (list *SharePointList[T]) clearFilters() {
	if len(list.options.Filters) == 0 {
		return
	}

	list.options.Filters = nil
}
